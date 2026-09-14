package command

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/securestore"
)

func TestMarketplacePythonIdentityTransitionCanBeRepairedByGo(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python is required for cross-client recovery validation")
	}
	for _, resumeCommand := range []string{"trial-purchase", "install", "legacy-trial-purchase", "legacy-install"} {
		t.Run(resumeCommand, func(t *testing.T) {
			legacy := strings.HasPrefix(resumeCommand, "legacy-")
			command := strings.TrimPrefix(resumeCommand, "legacy-")
			t.Setenv(processAccessTokenEnvironment, "")
			fixture := newSkillTrialTestServer(t)
			defer fixture.server.Close()
			const nextRelease = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recorded := httptest.NewRecorder()
				fixture.serveHTTP(recorded, r)
				raw := recorded.Body.Bytes()
				if strings.HasSuffix(r.URL.Path, "/trial-purchase/download") {
					var receipt map[string]any
					if err := json.Unmarshal(raw, &receipt); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					receipt["access"].(map[string]any)["release"].(map[string]any)["id"] = nextRelease
					receipt["download"].(map[string]any)["releaseId"] = nextRelease
					raw, _ = json.Marshal(receipt)
				}
				w.Header().Set("Content-Type", recorded.Header().Get("Content-Type"))
				w.WriteHeader(recorded.Code)
				_, _ = w.Write(raw)
			}))
			defer server.Close()
			home, store := t.TempDir(), securestore.NewMemory()
			if code, result, _ := executeSkillTrialCommand(t, server, home, store, "skill", "install", downloadableProductID, "--agent", "agents"); code != 0 {
				t.Fatalf("trial install failed: %#v", result)
			}
			directory := filepath.Join(home, ".agents", "skills", "free-test")
			if legacy {
				if err := os.Remove(filepath.Join(directory, ".viceme", "package-files.json")); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, "user-notes.txt"), []byte("keep legacy notes"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			fixture.mu.Lock()
			fixture.paymentStatus = "PAID"
			fixture.mu.Unlock()
			source, err := filepath.Abs("../../skills/use-a-skill/scripts/trial_runtime.py")
			if err != nil {
				t.Fatal(err)
			}
			// Execute the real Python writer and interrupt precisely between
			// the owner and runtime identity files. The two release IDs differ.
			code := `import importlib.util,sys
spec=importlib.util.spec_from_file_location("trial",sys.argv[1]);trial=importlib.util.module_from_spec(spec);spec.loader.exec_module(trial)
trial.API_ORIGIN["cn"]=sys.argv[2];trial.SCRIPT_ORIGIN["cn"]=sys.argv[2];trial.home_directory=lambda:sys.argv[3];trial.invoking_skill_directory=lambda:sys.argv[5]
write=trial.write_install_file
def interrupt_after_owner(destination,name,file):
    write(destination,name,file)
    if name == ".viceme/install-manifest.json": raise OSError("simulated interruption after owner identity")
trial.write_install_file=interrupt_after_owner
sys.exit(trial.run(["purchase","--product",sys.argv[4],"--market","cn","--agent","agents","--wait","0"]))`
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, python, "-B", "-c", code, source, server.URL, home, downloadableProductID, directory)
			cmd.Env = isolatedMarketplaceInteropEnvironment(home)
			raw, err := cmd.CombinedOutput()
			var failed map[string]any
			if err == nil || json.Unmarshal(raw, &failed) != nil || failed["ok"] != false {
				t.Fatalf("Python writer was not interrupted with a controlled response: %s (%v)", raw, err)
			}
			if code, result, _ := executeSkillTrialCommand(t, server, home, store, "skill", "ready", downloadableProductID, "--skill-dir", directory); code != 0 || result["data"].(map[string]any)["ready"] != false {
				t.Fatalf("unfinished transition was reported ready: %#v", result)
			}
			resultCode, restored, _ := executeSkillTrialCommand(t, server, home, store, "skill", command, downloadableProductID, "--skill-dir", directory, "--wait", "0")
			if resultCode != 0 {
				t.Fatalf("Go could not resume paid transition: %#v", restored)
			}
			if legacy {
				installed := restored["data"].(map[string]any)["install"].(map[string]any)
				report := installed["install"].(map[string]any)
				recoveries, _ := report["localRecoveries"].([]any)
				if len(recoveries) != 1 {
					t.Fatalf("Go did not return Python's preserved legacy generation: %#v", report)
				}
				recovery := recoveries[0].(map[string]any)
				recoveredDirectory, _ := filepath.EvalSymlinks(recovery["skillDirectory"].(string))
				expectedDirectory, _ := filepath.EvalSymlinks(directory)
				if recoveredDirectory != expectedDirectory {
					t.Fatalf("legacy recovery lost the original directory: %#v", recovery)
				}
				notes := filepath.Join(recovery["directory"].(string), "files", "user-notes.txt")
				if data, err := os.ReadFile(notes); err != nil || string(data) != "keep legacy notes" {
					t.Fatalf("cross-client recovery lost legacy output: %q %v", data, err)
				}
			}
			if code, result, _ := executeSkillTrialCommand(t, server, home, store, "skill", "ready", downloadableProductID, "--skill-dir", directory); code != 0 || result["data"].(map[string]any)["ready"] != true || result["data"].(map[string]any)["kind"] != "owned" {
				t.Fatalf("completed transition was not ready: %#v", result)
			}
		})
	}
}

func isolatedMarketplaceInteropEnvironment(home string) []string {
	var environment []string
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		switch key {
		case "HOME", "USERPROFILE", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "WORKBUDDY_CONFIG_DIR", "VICEME_AGENTS_SKILLS_DIR", "VICEME_CLI_CONFIG_DIR", processAccessTokenEnvironment:
			continue
		}
		environment = append(environment, value)
	}
	return append(environment, "HOME="+home, "USERPROFILE="+home, "VICEME_CLI_CONFIG_DIR="+filepath.Join(home, ".viceme-cli"))
}
