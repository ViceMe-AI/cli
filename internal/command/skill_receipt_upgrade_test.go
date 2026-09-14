package command

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func receiptUpgradeArchive(t *testing.T, asset string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	files := map[string]string{
		"SKILL.md":                "---\nname: free-test\ndescription: rules demo\n---\nRun scripts/run.py to apply all rules.\n",
		"scripts/run.py":          "from pathlib import Path\nfor p in sorted((Path(__file__).parent.parent / 'rules').glob('*.txt')): print(p.read_text().strip())\n",
		"rules/" + asset + ".txt": asset + "\n",
	}
	for name, body := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestPaidReceiptRemovesRetiredAuthoredResources(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("Python is required for the authored script fixture")
	}
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	state.archive = receiptUpgradeArchive(t, "legacy")
	state.archiveDigest = fmt.Sprintf("%x", sha256Sum256ForTest(state.archive))
	state.ownedArchive = receiptUpgradeArchive(t, "current")
	state.ownedArchiveDigest = fmt.Sprintf("%x", sha256Sum256ForTest(state.ownedArchive))
	home, store := t.TempDir(), securestore.NewMemory()
	if code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "install", downloadableProductID, "--agent", "agents"); code != 0 {
		t.Fatalf("install: %+v", result)
	}
	directory := filepath.Join(home, ".agents", "skills", "free-test")
	if err := os.WriteFile(filepath.Join(directory, "user-output.txt"), []byte("user output"), 0644); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	state.paymentStatus = "PAID"
	state.mu.Unlock()
	if code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "trial-purchase", downloadableProductID, "--agent", "agents", "--wait", "0"); code != 0 {
		t.Fatalf("receipt: %+v", result)
	}
	got, err := exec.Command("python3", filepath.Join(directory, "scripts", "run.py")).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %s %v", got, err)
	}
	if strings.TrimSpace(string(got)) != "current" {
		t.Fatalf("retired publisher resource is still executed after verified paid receipt: %q", got)
	}
	if data, err := os.ReadFile(filepath.Join(directory, "user-output.txt")); err != nil || string(data) != "user output" {
		t.Fatalf("user output lost: %q %v", data, err)
	}
	// Receipt replay is an owned-to-owned update and must keep the same distinction.
	state.ownedArchive = receiptUpgradeArchive(t, "latest")
	state.ownedArchiveDigest = fmt.Sprintf("%x", sha256Sum256ForTest(state.ownedArchive))
	if code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "trial-purchase", downloadableProductID, "--agent", "agents", "--wait", "0"); code != 0 {
		t.Fatalf("owned update: %+v", result)
	}
	got, err = exec.Command("python3", filepath.Join(directory, "scripts", "run.py")).CombinedOutput()
	if err != nil || strings.TrimSpace(string(got)) != "latest" {
		t.Fatalf("owned update retained old resources: %q %v", got, err)
	}
}

func TestPaidReceiptPreservesPythonVirtualEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows venv uses copied executables; link preservation has platform-neutral unit coverage")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("Python is required to create the virtual environment")
	}
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	if code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "install", downloadableProductID, "--agent", "agents"); code != 0 {
		t.Fatalf("install: %+v", result)
	}
	directory := filepath.Join(home, ".agents", "skills", "free-test")
	if got, err := exec.Command("python3", "-m", "venv", "--without-pip", filepath.Join(directory, ".venv")).CombinedOutput(); err != nil {
		t.Fatalf("venv: %s %v", got, err)
	}
	state.mu.Lock()
	state.paymentStatus = "PAID"
	state.mu.Unlock()
	if code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "trial-purchase", downloadableProductID, "--agent", "agents", "--wait", "0"); code != 0 {
		t.Fatalf("paid receipt rejected ordinary Python virtual environment: %+v", result)
	}
	if info, err := os.Lstat(filepath.Join(directory, ".venv", "bin", "python3")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("venv link lost: %v", err)
	}
	if got, err := exec.Command(filepath.Join(directory, ".venv", "bin", "python3"), "-c", "print('venv intact')").CombinedOutput(); err != nil || strings.TrimSpace(string(got)) != "venv intact" {
		t.Fatalf("venv unusable: %q %v", got, err)
	}
}

func TestSkillInstallRepairsExplicitWorkspaceDirectory(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	if code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "install", downloadableProductID, "--agent", "agents"); code != 0 {
		t.Fatalf("install: %+v", result)
	}
	source := filepath.Join(home, ".agents", "skills", "free-test")
	directory := filepath.Join(home, "workspace with spaces", "skills", "alias")
	if err := os.MkdirAll(filepath.Dir(directory), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(source, directory); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(directory, skillcontent.TrialBodyPath)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "user-output.txt"), []byte("keep local output"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "install", downloadableProductID, "--skill-dir", directory); code != 0 {
		t.Fatalf("repair: %+v", result)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("repair created a different installation: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(directory, "user-output.txt")); err != nil || string(data) != "keep local output" {
		t.Fatalf("user output lost: %q %v", data, err)
	}
	if code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "ready", downloadableProductID, "--skill-dir", directory); code != 0 || result["data"].(map[string]any)["ready"] != true {
		t.Fatalf("not repaired: %+v", result)
	}
}

func TestSkillInstallRejectsExplicitForeignDirectoryBeforeAPI(t *testing.T) {
	for _, field := range []string{"productId", "market", "apiBaseUrl"} {
		t.Run(field, func(t *testing.T) {
			t.Setenv(processAccessTokenEnvironment, "")
			state := newSkillTrialTestServer(t)
			defer state.server.Close()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); state.serveHTTP(w, r) }))
			defer server.Close()
			home, store := t.TempDir(), securestore.NewMemory()
			if code, result, _ := executeSkillTrialCommand(t, server, home, store, "skill", "install", downloadableProductID, "--agent", "agents"); code != 0 {
				t.Fatalf("install: %+v", result)
			}
			directory := filepath.Join(home, ".agents", "skills", "free-test")
			filename := filepath.Join(directory, ".viceme", "runtime.json")
			data, err := os.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			var manifest map[string]any
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatal(err)
			}
			manifest[field] = "foreign"
			data, _ = json.Marshal(manifest)
			if err := os.WriteFile(filename, data, 0o644); err != nil {
				t.Fatal(err)
			}
			calls.Store(0)
			code, result, _ := executeSkillTrialCommand(t, server, home, store, "skill", "install", downloadableProductID, "--skill-dir", directory)
			if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_INSTALLATION_IDENTITY_INVALID" || calls.Load() != 0 {
				t.Fatalf("foreign repair reached API or accepted identity: %+v calls=%d", result, calls.Load())
			}
		})
	}
}

func TestRegularTrialRepairPreservesUnownedLocalOutput(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	for attempt := 0; attempt < 2; attempt++ {
		if code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "install", downloadableProductID, "--agent", "agents"); code != 0 {
			t.Fatalf("install %d: %+v", attempt, result)
		}
		directory := filepath.Join(home, ".agents", "skills", "free-test")
		if attempt == 0 {
			if err := os.WriteFile(filepath.Join(directory, "user-output.txt"), []byte("normal reinstall keeps output"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(directory, skillcontent.TrialBodyPath)); err != nil {
				t.Fatal(err)
			}
		} else {
			if data, err := os.ReadFile(filepath.Join(directory, "user-output.txt")); err != nil || string(data) != "normal reinstall keeps output" {
				t.Fatalf("regular repair lost user output: %q %v", data, err)
			}
		}
	}
}
