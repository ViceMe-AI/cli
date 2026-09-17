package command

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ViceMe-AI/cli/internal/publication"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

// The author channel delivery must reuse the official gate generator. The Go
// injector and trial_runtime.py are the two maintained implementations of the
// same protocol; this test keeps them byte-identical on the three files that
// define the channel contract: the gated SKILL.md entry, the preserved trial
// body, and the generated runtime reference.
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
	for _, name := range []string{"SKILL.md", skillcontent.TrialBodyPath, skillcontent.TrialRuntimePath} {
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
