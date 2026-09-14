package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	"github.com/gofrs/flock"
)

func TestTrialFinalUseSuspensionFailureReplaysSameTask(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	state.trialLimit = 1
	var mu sync.Mutex
	responses := map[string][]byte{}
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/trial-use") {
			state.serveHTTP(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		id := body["requestId"].(string)
		requests = append(requests, id)
		if _, exists := responses[id]; !exists {
			raw, _ := json.Marshal(body)
			r.Body = http.NoBody
			clone := httptest.NewRequest(r.Method, r.URL.String(), bytes.NewReader(raw))
			recorded := httptest.NewRecorder()
			state.serveHTTP(recorded, clone)
			responses[id] = recorded.Body.Bytes()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(responses[id])
	}))
	defer server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	invoke := func(args ...string) (int, map[string]any) {
		code, result, _ := executeSkillTrialCommand(t, server, home, store, args...)
		return code, result
	}
	if code, result := invoke("skill", "install", downloadableProductID, "--agent", "agents"); code != 0 {
		t.Fatalf("install: %+v", result)
	}
	directory := filepath.Join(home, ".agents", "skills", "free-test")
	normalized, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		normalized = strings.ToLower(normalized)
	}
	digest := sha256.Sum256([]byte(normalized))
	lock := flock.New(filepath.Join(filepath.Dir(normalized), fmt.Sprintf(".viceme-install-%x.lock", digest)))
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Unlock() })
	code, failure := invoke("skill", "use", downloadableProductID, "--wait", "0")
	if code == 0 || failure["error"].(map[string]any)["code"] != "SKILL_TRIAL_SUSPEND_FAILED" || failure["error"].(map[string]any)["retryable"] != true {
		t.Fatalf("write refusal: %+v", failure)
	}
	if code, ready := invoke("skill", "ready", downloadableProductID); code != 0 || ready["data"].(map[string]any)["nextAction"] != "RESUME_TRIAL_USE" {
		t.Fatalf("ready: %+v", ready)
	}
	if code, purchase := invoke("skill", "trial-purchase", downloadableProductID, "--wait", "0"); code == 0 || purchase["error"].(map[string]any)["code"] != "SKILL_TRIAL_USE_PENDING" {
		t.Fatalf("purchase: %+v", purchase)
	}
	if err := lock.Unlock(); err != nil {
		t.Fatal(err)
	}
	code, result := invoke("skill", "use", downloadableProductID, "--wait", "0")
	if code != 0 {
		t.Fatalf("replay: %+v", result)
	}
	data := result["data"].(map[string]any)
	if data["entrySuspended"] != true || !strings.Contains(data["skillMarkdown"].(string), "# Free Test Skill") {
		t.Fatalf("task lost: %+v", data)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 || requests[0] != requests[1] || len(responses) != 1 {
		t.Fatalf("not an idempotent replay: %+v", requests)
	}
}

func TestTrialWorkspaceAliasFinalUseAndPaidRestore(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	invoke := func(args ...string) (int, map[string]any) {
		code, result, _ := executeSkillTrialCommand(t, state.server, home, store, args...)
		return code, result
	}
	if code, result := invoke("skill", "install", downloadableProductID, "--agent", "agents"); code != 0 {
		t.Fatalf("install: %+v", result)
	}
	source := filepath.Join(home, ".agents", "skills", "free-test")
	directory := filepath.Join(home, "project with spaces", "skills", "adnaks")
	if err := os.MkdirAll(filepath.Dir(directory), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(source, directory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "user-output.txt"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	for use := 1; use <= 2; use++ {
		code, result := invoke("skill", "use", downloadableProductID, "--skill-dir", directory, "--wait", "0")
		if code != 0 {
			t.Fatalf("use %d: %+v", use, result)
		}
		data := result["data"].(map[string]any)
		if data["allowed"] != true || !strings.Contains(data["skillMarkdown"].(string), "# Free Test Skill") {
			t.Fatalf("missing task: %+v", data)
		}
		entry, err := os.ReadFile(filepath.Join(directory, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(entry, []byte("# Free Test Skill")) || bytes.Contains(entry, []byte(skillcontent.TrialDisabledMarker)) != (use == 2) {
			t.Fatalf("incorrect entry after use %d", use)
		}
		if use == 2 && (data["entrySuspended"] != true || data["disabledSkillCount"] != float64(1)) {
			t.Fatalf("not suspended: %+v", data)
		}
	}
	code, payment := invoke("skill", "trial-purchase", downloadableProductID, "--skill-dir", directory, "--wait", "0")
	if code == 0 || payment["error"].(map[string]any)["code"] != "SKILL_PURCHASE_REQUIRED" {
		t.Fatalf("payment: %+v", payment)
	}
	state.mu.Lock()
	state.paymentStatus = "PAID"
	state.mu.Unlock()
	if code, result := invoke("skill", "trial-purchase", downloadableProductID, "--skill-dir", directory, "--wait", "0"); code != 0 {
		t.Fatalf("restore: %+v", result)
	}
	entry, err := os.ReadFile(filepath.Join(directory, "SKILL.md"))
	if err != nil || !bytes.Contains(entry, []byte("Owned Current Skill")) || bytes.Contains(entry, []byte(skillTrialGateMarker)) {
		t.Fatalf("formal entry: %v", err)
	}
	userFile, err := os.ReadFile(filepath.Join(directory, "user-output.txt"))
	if err != nil || string(userFile) != "keep me" {
		t.Fatalf("user output lost: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("restored a different directory: %v", err)
	}
	if code, result := invoke("skill", "ready", downloadableProductID, "--skill-dir", directory); code != 0 || result["data"].(map[string]any)["kind"] != "owned" {
		t.Fatalf("owned ready: %+v", result)
	}
}

func TestTrialUseMissingBodyConsumesNothing(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "install", downloadableProductID, "--agent", "agents")
	if code != 0 {
		t.Fatalf("install: %+v", result)
	}
	if err := os.Remove(filepath.Join(home, ".agents", "skills", "free-test", skillcontent.TrialBodyPath)); err != nil {
		t.Fatal(err)
	}
	code, result, _ = executeSkillTrialCommand(t, state.server, home, store, "skill", "use", downloadableProductID)
	if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_TRIAL_INSTALLATION_REQUIRED" {
		t.Fatalf("missing body: %+v", result)
	}
	if len(recordedUseRequestIDs(state)) != 0 {
		t.Fatal("consumed without a body")
	}
}

func TestTrialExplicitDirectoryRejectsForeignIdentity(t *testing.T) {
	for _, field := range []string{"productId", "market", "apiBaseUrl"} {
		t.Run(field, func(t *testing.T) {
			t.Setenv(processAccessTokenEnvironment, "")
			state := newSkillTrialTestServer(t)
			defer state.server.Close()
			home, store := t.TempDir(), securestore.NewMemory()
			code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "install", downloadableProductID, "--agent", "agents")
			if code != 0 {
				t.Fatalf("install: %+v", result)
			}
			directory := filepath.Join(home, ".agents", "skills", "free-test")
			manifestPath := filepath.Join(directory, ".viceme/runtime.json")
			raw, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			var manifest map[string]any
			if err := json.Unmarshal(raw, &manifest); err != nil {
				t.Fatal(err)
			}
			manifest[field] = "foreign"
			raw, _ = json.Marshal(manifest)
			if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(filepath.Join(directory, "SKILL.md"))
			for _, command := range []string{"use", "trial-purchase"} {
				code, result, _ = executeSkillTrialCommand(t, state.server, home, store, "skill", command, downloadableProductID, "--skill-dir", directory, "--wait", "0")
				if code == 0 {
					t.Fatalf("foreign directory accepted: %+v", result)
				}
			}
			after, _ := os.ReadFile(filepath.Join(directory, "SKILL.md"))
			if !bytes.Equal(before, after) {
				t.Fatal("foreign entry changed")
			}
			state.mu.Lock()
			defer state.mu.Unlock()
			if len(state.useRequests) != 0 || len(state.trialPurchaseRequests) != 0 {
				t.Fatal("foreign directory reached consumption or purchase")
			}
		})
	}
}

func TestTrialPendingPurchaseRepairsExhaustedEntry(t *testing.T) {
	for _, exhausted := range []bool{false, true} {
		t.Run(map[bool]string{false: "early", true: "exhausted"}[exhausted], func(t *testing.T) {
			t.Setenv(processAccessTokenEnvironment, "")
			state := newSkillTrialTestServer(t)
			defer state.server.Close()
			home, store := t.TempDir(), securestore.NewMemory()
			invoke := func(args ...string) (int, map[string]any) {
				code, result, _ := executeSkillTrialCommand(t, state.server, home, store, args...)
				return code, result
			}
			if code, result := invoke("skill", "install", downloadableProductID, "--agent", "agents"); code != 0 {
				t.Fatalf("install: %+v", result)
			}
			invoke("skill", "trial-purchase", downloadableProductID, "--wait", "0")
			state.mu.Lock()
			if exhausted {
				state.grantUses = state.trialLimit
			}
			state.mu.Unlock()
			code, result := invoke("skill", "use", downloadableProductID, "--wait", "0")
			if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_PURCHASE_REQUIRED" {
				t.Fatalf("resume purchase: %+v", result)
			}
			entry, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "free-test", "SKILL.md"))
			if err != nil || bytes.Contains(entry, []byte(skillcontent.TrialDisabledMarker)) != exhausted {
				t.Fatalf("incorrect suspension: %v", err)
			}
			if len(recordedUseRequestIDs(state)) != 0 {
				t.Fatal("purchase resume consumed a use")
			}
		})
	}
}

func TestTrialPendingPurchaseFinishesPartialSuspensionAcrossHosts(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	invoke := func(args ...string) (int, map[string]any) {
		code, result, _ := executeSkillTrialCommand(t, state.server, home, store, args...)
		return code, result
	}
	if code, result := invoke("skill", "install", downloadableProductID, "--agent", "workbuddy"); code != 0 {
		t.Fatalf("install: %+v", result)
	}
	invoke("skill", "trial-purchase", downloadableProductID, "--wait", "0")
	first := filepath.Join(home, ".agents", "skills", "free-test")
	// Reproduce an older interrupted suspension: the first host is already
	// disabled, but another matching installation still has an active entry.
	if count, err := skillcontent.SuspendTrialSkills(skillcontent.Environment{Home: t.TempDir()}, downloadableProductID, state.server.URL, "", "https://s3.viceme.cn/start/agent-install.md", first); err != nil || count != 1 {
		t.Fatalf("partial fixture: %d %v", count, err)
	}
	state.mu.Lock()
	state.grantUses = state.trialLimit
	state.mu.Unlock()
	code, result := invoke("skill", "use", downloadableProductID, "--wait", "0")
	if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_PURCHASE_REQUIRED" {
		t.Fatalf("purchase: %+v", result)
	}
	other, err := os.ReadFile(filepath.Join(home, ".workbuddy", "skills", "free-test", "SKILL.md"))
	if err != nil || !bytes.Contains(other, []byte(skillcontent.TrialDisabledMarker)) {
		t.Fatalf("second host remained active: %v", err)
	}
}
