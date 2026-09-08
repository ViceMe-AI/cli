package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestSkillReadyUsesOnlyInstalledResources(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "install", downloadableProductID, "--agent", "workbuddy")
	if code != 0 {
		t.Fatalf("install failed: %#v", result)
	}
	installed := result["data"].(map[string]any)
	state.server.Close() // Any API access in ready now fails.
	code, result, _ = executeSkillTrialCommand(t, state.server, home, store, "skill", "ready", downloadableProductID, "--agent", "workbuddy")
	data := result["data"].(map[string]any)
	if code != 0 || data["ready"] != true || data["runner"] != "cli" {
		t.Fatalf("not locally ready: %#v", result)
	}
	for _, field := range []string{"skillPath", "runtimePath", "onboardingGuidePath", "onboardingTemplatePath", "paymentTemplatePath"} {
		path, _ := data[field].(string)
		if path == "" || path != installed[field] {
			t.Fatalf("missing or inconsistent %s: %#v", field, data)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	if len(state.useRequests) != 0 || len(state.trialPurchaseRequests) != 0 {
		t.Fatal("onboarding consumed quota or ordered")
	}
	// A suspended or user-edited authored entry is not a missing runtime.
	if err := os.WriteFile(data["skillPath"].(string), []byte("---\nname: free-test\n---\ntrial suspended\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, result, _ = executeSkillTrialCommand(t, state.server, home, store, "skill", "ready", downloadableProductID, "--agent", "workbuddy")
	if code != 0 || result["data"].(map[string]any)["ready"] != true {
		t.Fatalf("authored entry was revalidated as runtime: %#v", result)
	}
	if err := os.WriteFile(data["onboardingTemplatePath"].(string), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, result, _ = executeSkillTrialCommand(t, state.server, home, store, "skill", "ready", downloadableProductID, "--agent", "workbuddy")
	if got := result["data"].(map[string]any); got["ready"] != false || got["nextAction"] != "REPAIR_INSTALLATION" {
		t.Fatalf("corrupt host masked by shared install: %#v", result)
	}
	code, _, _ = executeSkillTrialCommand(t, state.server, home, store, "skill", "ready", "https://example.test/work.md?product="+downloadableProductID+"&install=owned", "--agent", "workbuddy")
	if code == 0 {
		t.Fatal("owned URL bypassed entitlement route")
	}
}

func TestTrialIdentityConflictDoesNotCreateLockOrMutateState(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	credential := `{"installId":"11111111-1111-4111-8111-111111111111","secret":"cli-secret"}`
	if err := store.Set(skillTrialStoreKey(downloadableProductID), credential); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".viceme", "trial", downloadableProductID+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(map[string]string{"installId": "22222222-2222-4222-8222-222222222222", "secret": "script-secret", "market": "cn", "productId": downloadableProductID})
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"use", "trial-purchase", "trial-status"} {
		code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", command, downloadableProductID)
		if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_TRIAL_IDENTITY_MISMATCH" {
			t.Fatalf("%s conflict not stopped: %#v", command, result)
		}
		if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
			t.Fatalf("conflict left a lock: %v", err)
		}
	}
	if current, _ := os.ReadFile(path); string(current) != string(original) {
		t.Fatal("script identity changed")
	}
	if current, _ := store.Get(skillTrialStoreKey(downloadableProductID)); current != credential {
		t.Fatal("CLI identity changed")
	}
	if len(state.useRequests)+len(state.grantRequests)+len(state.trialPurchaseRequests) != 0 {
		t.Fatal("conflict reached API")
	}
}

func TestTrialMarketConflictStopsBeforeCredentialAdoptionOrAPI(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	for _, withCLI := range []bool{false, true} {
		for _, arguments := range [][]string{
			{"skill", "use", downloadableProductID},
			{"skill", "trial-status", downloadableProductID},
			{"skill", "trial-purchase", downloadableProductID},
			{"skill", "install", downloadableProductID, "--agent", "workbuddy"},
		} {
			name := strings.Join(arguments[1:], "-")
			if withCLI {
				name += "-with-cli-credential"
			}
			t.Run(name, func(t *testing.T) {
				state := newSkillTrialTestServer(t)
				defer state.server.Close()
				home, store := t.TempDir(), securestore.NewMemory()
				credential := `{"installId":"11111111-1111-4111-8111-111111111111","secret":"` + skillTrialSecret + `"}`
				if withCLI {
					if err := store.Set(skillTrialStoreKey(downloadableProductID), credential); err != nil {
						t.Fatal(err)
					}
				}
				path := filepath.Join(home, ".viceme", "trial", downloadableProductID+".json")
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				original, _ := json.Marshal(scriptTrialState{
					InstallID: "11111111-1111-4111-8111-111111111111", Secret: skillTrialSecret,
					ProductID: downloadableProductID, Market: "global",
					Purchase: &trialPurchaseState{ClientRequestID: "22222222-2222-4222-8222-222222222222", OrderNo: skillPurchaseOrderNo},
				})
				if err := os.WriteFile(path, original, 0o600); err != nil {
					t.Fatal(err)
				}
				code, result, _ := executeSkillTrialCommand(t, state.server, home, store, arguments...)
				if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_TRIAL_IDENTITY_MISMATCH" {
					t.Fatalf("market conflict not stopped: %#v", result)
				}
				if current, _ := os.ReadFile(path); string(current) != string(original) {
					t.Fatal("foreign market state changed")
				}
				stored, _ := store.Get(skillTrialStoreKey(downloadableProductID))
				if (!withCLI && stored != "") || (withCLI && stored != credential) {
					t.Fatalf("CLI credential changed: %q", stored)
				}
				if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
					t.Fatalf("market conflict left a lock: %v", err)
				}
				state.mu.Lock()
				defer state.mu.Unlock()
				if len(state.useRequests)+len(state.grantRequests)+len(state.trialPurchaseRequests) != 0 {
					t.Fatal("foreign market identity reached API")
				}
			})
		}
	}
}

func TestTrialReadyDoesNotQueryQuotaWithForeignMarketState(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	credential := `{"installId":"11111111-1111-4111-8111-111111111111","secret":"` + skillTrialSecret + `"}`
	if err := store.Set(skillTrialStoreKey(downloadableProductID), credential); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".viceme", "trial", downloadableProductID+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(scriptTrialState{
		InstallID: "11111111-1111-4111-8111-111111111111", Secret: skillTrialSecret,
		ProductID: downloadableProductID, Market: "global",
	})
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "ready", downloadableProductID, "--agent", "workbuddy")
	if code != 0 || result["data"].(map[string]any)["ready"] != false {
		t.Fatalf("ready failed: %#v", result)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.grantRequests) != 0 {
		t.Fatal("ready sent a foreign market identity to the API")
	}
}

func TestTrialLockReleaseFailureIsReported(t *testing.T) {
	previous := removeScriptTrialLock
	t.Cleanup(func() { removeScriptTrialLock = previous })
	removeScriptTrialLock = func(string) error { return os.ErrPermission }
	home := t.TempDir()
	err := withScriptTrialLockAt(home, downloadableProductID, func() error { return nil })
	var failure *output.Error
	if !errors.As(err, &failure) || failure.Subtype != "SKILL_TRIAL_LOCK_RELEASE_FAILED" {
		t.Fatalf("cleanup silently ignored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".viceme/trial", downloadableProductID+".json.lock")); err != nil {
		t.Fatal("lock evidence was destroyed")
	}
}

func TestTrialUseKeepsRetryKeyWhenSharedLockReleaseFails(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "install", downloadableProductID, "--agent", "workbuddy")
	if code != 0 {
		t.Fatalf("install: %#v", result)
	}
	grant := result["data"].(map[string]any)["trial"].(map[string]any)
	path := filepath.Join(home, ".viceme/trial", downloadableProductID+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"installId": grant["installId"], "secret": skillTrialSecret, "productId": downloadableProductID, "market": "cn", "pendingRequestId": "22222222-2222-4222-8222-222222222222"})
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	previous, calls := removeScriptTrialLock, 0
	t.Cleanup(func() { removeScriptTrialLock = previous })
	removeScriptTrialLock = func(path string) error {
		calls++
		if calls == 2 {
			return os.ErrPermission
		}
		return previous(path)
	}
	code, result, _ = executeSkillTrialCommand(t, state.server, home, store, "skill", "use", downloadableProductID)
	if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_TRIAL_SCRIPT_PENDING_CLEAR_FAILED" {
		t.Fatalf("cleanup failure: %#v", result)
	}
	// The isolated fixture's filesystem permission is restored, then the same
	// operation retries. Never change a real user's lock timestamp or identity.
	removeScriptTrialLock = previous
	if err := os.Remove(path + ".lock"); err != nil {
		t.Fatal(err)
	}
	code, result, _ = executeSkillTrialCommand(t, state.server, home, store, "skill", "use", downloadableProductID)
	if code != 0 {
		t.Fatalf("retry failed: %#v", result)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.useRequests) != 2 || state.useRequests[0]["requestId"] != state.useRequests[1]["requestId"] {
		t.Fatalf("retry used another key: %#v", state.useRequests)
	}
}

func TestClosedTrialOrderDoesNotConsumeOrClaimExhaustion(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "install", downloadableProductID, "--agent", "workbuddy")
	if code != 0 {
		t.Fatalf("install: %#v", result)
	}
	state.paymentStatus = "CLOSED"
	code, result, _ = executeSkillTrialCommand(t, state.server, home, store, "skill", "trial-purchase", downloadableProductID, "--wait", "0")
	if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_PURCHASE_ORDER_CLOSED" {
		t.Fatalf("closure: %#v", result)
	}
	if text, _ := json.Marshal(result); strings.Contains(string(text), `"EXHAUSTED"`) {
		t.Fatal("closure reported exhaustion")
	}
	count := len(state.trialPurchaseRequests)
	code, result, _ = executeSkillTrialCommand(t, state.server, home, store, "skill", "use", downloadableProductID)
	if code != 0 || result["data"].(map[string]any)["allowed"] != true {
		t.Fatalf("closed order blocked available use: %#v", result)
	}
	if len(state.trialPurchaseRequests) != count {
		t.Fatal("use implicitly created another purchase")
	}
}

func TestSkillReadyReportsRemainingWithoutUse(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "install", downloadableProductID, "--agent", "workbuddy")
	if code != 0 {
		t.Fatalf("install: %#v", result)
	}
	installed := result["data"].(map[string]any)
	if installed["remainingUses"] != float64(2) || installed["trialExhausted"] == true {
		t.Fatalf("install snapshot: %#v", installed)
	}
	code, result, _ = executeSkillTrialCommand(t, state.server, home, store, "skill", "ready", downloadableProductID, "--agent", "workbuddy")
	data := result["data"].(map[string]any)
	if code != 0 || data["ready"] != true || data["remainingUses"] != float64(2) || data["limitUses"] != float64(2) || data["trialExhausted"] == true {
		t.Fatalf("ready snapshot: %#v", result)
	}
	if data["nextAction"] != "CONTINUE_ORIGINAL_TASK_WITH_INSTALLED_SKILL" {
		t.Fatalf("available trial should continue: %#v", data)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.useRequests) != 0 || len(state.trialPurchaseRequests) != 0 || state.grantUses != 0 {
		t.Fatal("ready consumed a use or created an order")
	}
}

func TestSkillReadyExhaustedRequiresPurchase(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	if code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "install", downloadableProductID, "--agent", "workbuddy"); code != 0 {
		t.Fatalf("install: %#v", result)
	}
	for i := 0; i < 2; i++ {
		if code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "use", downloadableProductID); code != 0 {
			t.Fatalf("use %d: %#v", i, result)
		}
	}
	code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "ready", downloadableProductID, "--agent", "workbuddy")
	data := result["data"].(map[string]any)
	if code != 0 || data["ready"] != true || data["remainingUses"] != float64(0) || data["trialExhausted"] != true || data["nextAction"] != "PURCHASE_REQUIRED" {
		t.Fatalf("exhausted ready: %#v", result)
	}
	message, _ := data["message"].(string)
	if !strings.Contains(message, "不要读商品 SKILL.md") {
		t.Fatalf("exhausted ready missing stop message: %#v", data)
	}
	entry, err := os.ReadFile(data["skillPath"].(string))
	if err != nil || !bytes.Contains(entry, []byte(skillcontent.TrialDisabledMarker)) {
		t.Fatalf("exhausted ready should suspend SKILL.md: %v %s", err, entry)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.useRequests) != 2 || len(state.trialPurchaseRequests) != 0 {
		t.Fatal("ready consumed another use or ordered")
	}
}

func TestClosedTrialOrderDoesNotHijackInstall(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	code, result, _ := executeSkillTrialCommand(t, state.server, home, store, "skill", "install", downloadableProductID, "--agent", "workbuddy")
	if code != 0 {
		t.Fatalf("install: %#v", result)
	}
	grant := result["data"].(map[string]any)["trial"].(map[string]any)
	path := filepath.Join(home, ".viceme", "trial", downloadableProductID+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(scriptTrialState{
		InstallID: grant["installId"].(string), Secret: skillTrialSecret, ProductID: downloadableProductID, Market: "cn",
		Purchase: &trialPurchaseState{ClientRequestID: "22222222-2222-4222-8222-222222222222", OrderNo: skillPurchaseOrderNo, Presented: true},
	})
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	state.paymentStatus = "CLOSED"
	code, result, _ = executeSkillTrialCommand(t, state.server, home, store, "skill", "install", downloadableProductID, "--agent", "workbuddy", "--wait", "0")
	data, _ := result["data"].(map[string]any)
	if code != 0 || data["trial"] == nil || data["owned"] == true {
		t.Fatalf("closed order hijacked trial install: %#v", result)
	}
	if text, _ := json.Marshal(result); strings.Contains(string(text), "PAYMENT_CLOSED") {
		t.Fatal("install returned PAYMENT_CLOSED")
	}
	entry, err := os.ReadFile(filepath.Join(home, ".workbuddy", "skills", "free-test", "SKILL.md"))
	if err != nil || !strings.Contains(string(entry), skillTrialGateMarker) {
		t.Fatalf("trial entry missing after closed install: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved scriptTrialState
	if err := json.Unmarshal(raw, &saved); err != nil || saved.Purchase == nil || !saved.Purchase.Closed {
		t.Fatalf("closed order was not marked: %v %#v", err, saved)
	}
	if code, result, _ = executeSkillTrialCommand(t, state.server, home, store, "skill", "use", downloadableProductID); code != 0 || result["data"].(map[string]any)["allowed"] != true {
		t.Fatalf("closed install blocked available use: %#v", result)
	}
}
