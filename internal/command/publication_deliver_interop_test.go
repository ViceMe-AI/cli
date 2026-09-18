package command

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/publication"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

// The author channel delivery must reuse the official gate generator. The Go
// injector and trial_runtime.py are the two maintained implementations of the
// same protocol; this test keeps them byte-identical on the four files that
// define the channel contract: the gated SKILL.md entry, the preserved trial
// body, the generated runtime reference, and the credential-free install
// identity.
func TestChannelGateMatchesPythonOfficialGenerator(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python is required for official generator parity validation")
	}
	script, err := filepath.Abs("../../skills/use-a-skill/scripts/trial_runtime.py")
	if err != nil {
		t.Fatal(err)
	}
	const productID = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	const releaseID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"

	root := t.TempDir()
	source := filepath.Join(root, "skill")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(`---
name: parity-demo
description: Official generator parity fixture.
---

# Parity Demo

Body used by both generators.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg, err := publication.Build(source)
	if err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(root, "original.zip")
	if err := os.WriteFile(original, pkg.Bytes, 0o644); err != nil {
		t.Fatal(err)
	}

	pythonZip := filepath.Join(root, "python-channel.zip")
	command := exec.Command(python, script, "export-package",
		"--product", productID, "--market", "cn", "--kind", "trial",
		"--input", original, "--output", pythonZip, "--release-id", releaseID)
	command.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("python export-package failed: %v\n%s", err, output)
	}

	goFiles, err := extractDownloadableSkill(pkg.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := injectSkillTrialGate(goFiles, productID, "cn"); err != nil {
		t.Fatal(err)
	}
	channelIdentity := channelInstallManifest(productID, releaseID)
	goFiles[skillcontent.InstallManifestPath] = downloadableSkillFile{Data: channelIdentity, Mode: 0o644}

	readZipFiles := func(filename string) map[string][]byte {
		reader, err := zip.OpenReader(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		files := map[string][]byte{}
		for _, entry := range reader.File {
			if entry.FileInfo().IsDir() {
				continue
			}
			handle, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(handle)
			_ = handle.Close()
			if err != nil {
				t.Fatal(err)
			}
			files[entry.Name] = data
		}
		return files
	}
	pyFiles := readZipFiles(pythonZip)
	for _, name := range []string{"SKILL.md", skillcontent.TrialBodyPath, skillcontent.TrialRuntimePath, skillcontent.InstallManifestPath} {
		goFile, ok := goFiles[name]
		if !ok {
			t.Fatalf("Go generator did not produce %s", name)
		}
		pyFile, ok := pyFiles[name]
		if !ok {
			t.Fatalf("Python generator did not produce %s", name)
		}
		if !bytes.Equal(goFile.Data, pyFile) {
			t.Fatalf("generator output drifted for %s:\nGo:     %q\nPython: %q", name, goFile.Data, pyFile)
		}
	}
}

// TestChannelZipSurvivesConsumerFirstUse closes the loop the review demanded:
// unpack the ZIP the delivery actually produced into a clean directory, run
// the official Python runtime against it, and prove the first use initializes
// the per-machine install identity, obtains a grant, consumes one trial use,
// and returns the authored body. The bootstrap's pinned runtime download is
// replaced by copying the repository runtime (the same file the CDN serves);
// everything else — install manifest, runtime.json digests, gate entry, API
// protocol — is the delivered package as-is.
func TestChannelZipSurvivesConsumerFirstUse(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python is required for consumer first-use validation")
	}
	repoRuntime, err := filepath.Abs("../../skills/use-a-skill/scripts/trial_runtime.py")
	if err != nil {
		t.Fatal(err)
	}
	harness := newDeliverHarness(t, deliverAuthorBody)
	defer harness.server.Close()
	exit, envelope := harness.execute("publication", "deliver", harness.firstID, "--skill-dir", harness.source)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("delivery failed: exit=%d envelope=%v", exit, envelope)
	}
	data, _ := envelope["data"].(map[string]any)
	zipInfo, _ := data["zip"].(map[string]any)
	zipPath, _ := zipInfo["path"].(string)
	if zipPath == "" {
		t.Fatal("delivery did not report the channel ZIP")
	}

	home := t.TempDir()
	channelDir := filepath.Join(home, "parity-skill")
	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, entry := range archive.File {
		target := filepath.Join(channelDir, filepath.FromSlash(entry.Name))
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		handle, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(handle)
		_ = handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, content, entry.Mode()); err != nil {
			t.Fatal(err)
		}
	}
	// The package ships the pinned bootstrap; stand in the repository copy of
	// the full runtime (byte-identical to the CDN artifact) so the test stays
	// offline.
	if err := copyFile(repoRuntime, filepath.Join(channelDir, ".viceme", "scripts", "trial_runtime.py")); err != nil {
		t.Fatal(err)
	}

	harnessPath := filepath.Join(t.TempDir(), "consumer.py")
	harnessScript := `import importlib.util
import sys

channel_dir, api_base, product_id, market = sys.argv[1:5]
spec = importlib.util.spec_from_file_location(
    "viceme_channel_runtime", channel_dir + "/.viceme/scripts/trial_runtime.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
module.API_ORIGIN[market] = api_base
sys.exit(module.run(["use", "--product", product_id, "--market", market]))
`
	if err := os.WriteFile(harnessPath, []byte(harnessScript), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(python, "-B", harnessPath, channelDir, harness.server.URL, harness.state.productID, "global")
	command.Env = append(os.Environ(), "HOME="+home, "PYTHONIOENCODING=utf-8")
	command.Dir = channelDir
	raw, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("consumer first use failed: %v\n%s", err, raw)
	}
	var result map[string]any
	line := strings.TrimSpace(string(raw))
	if index := strings.LastIndex(line, "\n"); index >= 0 {
		line = line[index+1:]
	}
	if err := json.Unmarshal([]byte(line), &result); err != nil {
		t.Fatalf("consumer output was not one JSON line: %q err=%v", raw, err)
	}
	if result["ok"] != true || result["allowed"] != true {
		t.Fatalf("first use was not allowed: %s", raw)
	}
	markdown, _ := result["skillMarkdown"].(string)
	if !strings.Contains(markdown, "Author business body") {
		t.Fatalf("first use did not return the authored body: %q", markdown)
	}
	if result["remainingUses"] != float64(2) {
		t.Fatalf("first use did not consume exactly one trial use: %#v", result)
	}
	paths := map[string]bool{}
	for _, request := range harness.state.trialRequests {
		paths[request] = true
	}
	if !paths["/v1/skills/"+harness.state.productID+"/trial-grants"] || !paths["/v1/skills/"+harness.state.productID+"/trial-use"] {
		t.Fatalf("first use did not run the grant+use protocol: %v", harness.state.trialRequests)
	}
}

func envelope2Data(t *testing.T, harness *deliverHarness) (map[string]any, map[string]any) {
	t.Helper()
	_ = harness
	return nil, nil
}

func copyFile(source, target string) error {
	raw, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, raw, 0o644)
}
