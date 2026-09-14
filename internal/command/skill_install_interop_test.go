package command

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/securestore"
)

// Each installer must recognize the other installer's published file ownership.
// A receipt replaces retired publisher resources but keeps unowned user output.
func TestMarketplaceFileOwnershipCrossClientRestore(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python is required for cross-client installation validation")
	}
	source, err := filepath.Abs("../../skills/use-a-skill/scripts/trial_runtime.py")
	if err != nil {
		t.Fatal(err)
	}
	archive := func(resource string) []byte {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		for name, body := range map[string]string{
			"SKILL.md":                        "---\nname: free-test\ndescription: Test published rules.\n---\nApply the rules in references.\n",
			"references/" + resource + ".txt": resource,
		} {
			entry, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}
	for _, producer := range []string{"go", "python"} {
		t.Run(producer, func(t *testing.T) {
			t.Setenv(processAccessTokenEnvironment, "")
			state := newSkillTrialTestServer(t, func(state *skillTrialTestServer) {
				state.archive, state.ownedArchive = archive("retired"), archive("current")
				state.archiveDigest = fmt.Sprintf("%x", sha256.Sum256(state.archive))
				state.ownedArchiveDigest = fmt.Sprintf("%x", sha256.Sum256(state.ownedArchive))
			})
			defer state.server.Close()
			home, store := t.TempDir(), securestore.NewMemory()
			invokeGo := func(args ...string) map[string]any {
				t.Helper()
				code, result, _ := executeSkillTrialCommand(t, state.server, home, store, args...)
				if code != 0 {
					t.Fatalf("Go command failed: %#v", result)
				}
				return result["data"].(map[string]any)
			}
			invokePython := func(script, command string) {
				t.Helper()
				code := `import importlib.util,sys
spec=importlib.util.spec_from_file_location("trial",sys.argv[1]);trial=importlib.util.module_from_spec(spec);spec.loader.exec_module(trial)
trial.API_ORIGIN["cn"]=sys.argv[2];trial.SCRIPT_ORIGIN["cn"]=sys.argv[2];trial.home_directory=lambda:sys.argv[3]
sys.exit(trial.run([sys.argv[5],"--product",sys.argv[4],"--market","cn","--agent","codex","--wait","0"]))`
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, python, "-B", "-c", code, script, state.server.URL, home, downloadableProductID, command)
				cmd.Env = isolatedMarketplaceInteropEnvironment(home)
				raw, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("Python %s failed: %v %s", command, err, raw)
				}
				var result map[string]any
				if err := json.Unmarshal(raw, &result); err != nil || result["ok"] != true {
					t.Fatalf("Python %s response: %s (%v)", command, raw, err)
				}
			}
			if producer == "go" {
				invokeGo("skill", "install", downloadableProductID, "--agent", "codex")
			} else {
				invokePython(source, "install")
			}
			original := filepath.Join(home, ".agents", "skills", "free-test")
			directory := filepath.Join(home, "workspace with spaces", "renamed-skill")
			if err := os.MkdirAll(filepath.Dir(directory), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(original, directory); err != nil {
				t.Fatal(err)
			}
			userFile := filepath.Join(directory, "references", "user-notes.txt")
			if err := os.WriteFile(userFile, []byte("keep my notes"), 0o644); err != nil {
				t.Fatal(err)
			}
			state.mu.Lock()
			state.paymentStatus = "PAID"
			state.mu.Unlock()
			if producer == "go" {
				invokePython(filepath.Join(directory, ".viceme", "scripts", "trial.py"), "purchase")
			} else {
				invokeGo("skill", "trial-purchase", downloadableProductID, "--skill-dir", directory, "--wait", "0")
			}
			if _, err := os.Lstat(filepath.Join(directory, "references", "retired.txt")); !os.IsNotExist(err) {
				t.Fatalf("retired publisher resource remains in the active package: %v", err)
			}
			if data, err := os.ReadFile(filepath.Join(directory, "references", "current.txt")); err != nil || string(data) != "current" {
				t.Fatalf("current publisher resource missing: %q %v", data, err)
			}
			if data, err := os.ReadFile(userFile); err != nil || string(data) != "keep my notes" {
				t.Fatalf("cross-client restore lost user output: %q %v", data, err)
			}
			if _, err := os.Lstat(original); !os.IsNotExist(err) {
				t.Fatalf("restore wrote to the old installation location: %v", err)
			}
			if ready := invokeGo("skill", "ready", downloadableProductID, "--skill-dir", directory); ready["ready"] != true || ready["kind"] != "owned" {
				t.Fatalf("complete cross-client package was not ready: %#v", ready)
			}
		})
	}
}
