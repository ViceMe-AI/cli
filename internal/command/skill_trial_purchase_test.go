package command

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/securestore"
)

func TestAnonymousTrialPurchasePresentsBeforeWaitAndRestoresThroughInstall(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	invoke := func(args ...string) (int, map[string]any) {
		exit, envelope, _ := executeSkillTrialCommand(t, state.server, home, store, args...)
		return exit, envelope
	}
	if code, result := invoke("skill", "install", downloadableProductID, "--agent", "codex"); code != 0 {
		t.Fatalf("trial: %#v", result)
	}
	// Merely querying the balance cannot call trial-use.
	code, result := invoke("skill", "trial-status", downloadableProductID)
	if code != 0 {
		t.Fatalf("status: %#v", result)
	}
	if result["data"].(map[string]any)["nextAction"] != "SHOW_REMAINING_USES" {
		t.Fatalf("status nextAction: %#v", result)
	}
	state.mu.Lock()
	uses := state.grantUses
	state.mu.Unlock()
	if uses != 0 {
		t.Fatal("status consumed a use")
	}
	code, result = invoke("skill", "trial-purchase", downloadableProductID, "--wait", "60s")
	if code == 0 {
		t.Fatal("an unpaid order was treated as installed")
	}
	failure := result["error"].(map[string]any)
	if failure["code"] != "SKILL_PURCHASE_REQUIRED" {
		t.Fatalf("payment: %#v", result)
	}
	presentation := failure["details"].(map[string]any)["paymentPresentation"].(map[string]any)
	widget, err := os.ReadFile(presentation["widgetPath"].(string))
	if err != nil || !bytes.Contains(widget, []byte("<!DOCTYPE html")) || !bytes.Contains(widget, []byte(`aria-label="微信支付二维码"`)) || bytes.Contains(widget, []byte("weixin://")) {
		t.Fatalf("invalid Widget: %v", err)
	}
	image, err := os.ReadFile(presentation["imagePath"].(string))
	if err != nil || len(image) < 8 || !bytes.Equal(image[:8], []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("invalid QR image: %v", err)
	}
	if presentation["imageChatSrc"] != localFileChatSrc(presentation["imagePath"].(string)) {
		t.Fatalf("imageChatSrc: %#v", presentation["imageChatSrc"])
	}
	statePath := filepath.Join(home, ".viceme", "trial", downloadableProductID+".json")
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var saved scriptTrialState
	if err := json.Unmarshal(raw, &saved); err != nil || saved.Purchase == nil || !saved.Purchase.Presented {
		t.Fatalf("missing shared purchase state: %v", err)
	}
	state.mu.Lock()
	if len(state.trialPurchaseRequests) != 1 {
		t.Fatal("first command waited before presentation")
	}
	state.paymentStatus = "PAID"
	state.mu.Unlock()
	// The ordinary open install route resumes the saved anonymous purchase,
	// whereas --owned remains the separate current-account route.
	if code, result = invoke("skill", "install", downloadableProductID, "--agent", "codex", "--wait", "0"); code != 0 {
		t.Fatalf("formal restore: %#v", result)
	}
	data := result["data"].(map[string]any)
	if data["owned"] != true || data["allowed"] != true {
		t.Fatalf("not restored: %#v", data)
	}
	for _, root := range []string{".codex", ".agents"} {
		body, err := os.ReadFile(filepath.Join(home, root, "skills", "free-test", "SKILL.md"))
		if err != nil || !strings.Contains(string(body), "Owned Current Skill") || strings.Contains(string(body), skillTrialGateMarker) {
			t.Fatalf("formal package not installed: %v", err)
		}
	}
	if _, err := os.Stat(presentation["widgetPath"].(string)); !os.IsNotExist(err) {
		t.Fatalf("stale Widget was not cleaned: %v", err)
	}
	state.mu.Lock()
	downloads := state.ownedDownloadCalls
	grants := len(state.grantRequests)
	useCalls := len(state.useRequests)
	state.mu.Unlock()
	code, result = invoke("skill", "ready", downloadableProductID, "--agent", "codex")
	data = result["data"].(map[string]any)
	if code != 0 || data["kind"] != "owned" || data["nextAction"] != "CONTINUE_ORIGINAL_TASK_WITH_INSTALLED_SKILL" || data["trialExhausted"] == true {
		t.Fatalf("owned ready: %#v", result)
	}
	if _, ok := data["remainingUses"]; ok {
		t.Fatalf("owned ready leaked remainingUses: %#v", data)
	}
	code, result = invoke("skill", "use", downloadableProductID)
	data = result["data"].(map[string]any)
	if code != 0 || data["allowed"] != true || data["owned"] != true || data["nextAction"] != "CONTINUE_ORIGINAL_TASK_WITH_INSTALLED_SKILL" {
		t.Fatalf("owned use: %#v", result)
	}
	if _, ok := data["remainingUses"]; ok {
		t.Fatalf("owned use leaked remainingUses: %#v", data)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.grantUses != 0 {
		t.Fatal("conversion charged quota")
	}
	if state.ownedDownloadCalls != downloads || len(state.useRequests) != useCalls || len(state.grantRequests) != grants {
		t.Fatal("owned ready/use re-fetched the package or checked trial quota")
	}
	var creations int
	for _, request := range state.trialPurchaseRequests {
		if request["clientRequestId"] != "" {
			creations++
		}
		if request["orderNo"] != "" && request["orderNo"] != saved.Purchase.OrderNo {
			t.Fatal("did not resume the original order")
		}
	}
	if creations != 1 || len(state.trialPurchaseRequests) < 2 {
		t.Fatalf("opened %d orders across %d purchase calls", creations, len(state.trialPurchaseRequests))
	}
}

func TestTrialPurchaseCrossProcessPythonAndGoShareOneOrder(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python is required for cross-client contract validation")
	}
	script, err := filepath.Abs("../../skills/use-a-skill/scripts/trial_runtime.py")
	if err != nil {
		t.Fatal(err)
	}
	for _, first := range []string{"python", "go"} {
		t.Run(first, func(t *testing.T) {
			t.Setenv(processAccessTokenEnvironment, "")
			state := newSkillTrialTestServer(t)
			defer state.server.Close()
			home, store := t.TempDir(), securestore.NewMemory()
			invoke := func(args ...string) (int, map[string]any) {
				code, result, _ := executeSkillTrialCommand(t, state.server, home, store, args...)
				return code, result
			}
			runPython := func(command string) map[string]any {
				t.Helper()
				// Test-only origin and home injection: the shipped script has no
				// production endpoint override, and no process uses the real home.
				code := `import importlib.util,sys
spec=importlib.util.spec_from_file_location("trial",sys.argv[1]);trial=importlib.util.module_from_spec(spec);spec.loader.exec_module(trial)
trial.API_ORIGIN["cn"]=sys.argv[2];trial.SCRIPT_ORIGIN["cn"]=sys.argv[2];trial.home_directory=lambda:sys.argv[3]
sys.exit(trial.run([sys.argv[5],"--product",sys.argv[4],"--market","cn","--agent","codex"]))`
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, python, "-B", "-c", code, script, state.server.URL, home, downloadableProductID, command)
				raw, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("Python %s: %v %s", command, err, raw)
				}
				var result map[string]any
				if err := json.Unmarshal(raw, &result); err != nil {
					t.Fatalf("Python JSON: %v %s", err, raw)
				}
				return result
			}
			if first == "python" {
				if result := runPython("install"); result["kind"] != "trial" {
					t.Fatalf("Python trial: %#v", result)
				}
				if result := runPython("purchase"); result["allowed"] != false || result["paymentPresentation"] == nil {
					t.Fatalf("Python did not present payment: %#v", result)
				}
			} else {
				if code, result := invoke("skill", "install", downloadableProductID, "--agent", "codex"); code != 0 {
					t.Fatalf("Go trial: %#v", result)
				}
				if code, result := invoke("skill", "trial-purchase", downloadableProductID, "--wait", "0"); code == 0 {
					t.Fatalf("unpaid Go purchase succeeded: %#v", result)
				}
			}
			state.mu.Lock()
			state.paymentStatus = "PAID"
			state.mu.Unlock()
			if first == "python" {
				if code, result := invoke("skill", "install", downloadableProductID, "--agent", "codex", "--wait", "0"); code != 0 {
					t.Fatalf("Go restore: %#v", result)
				}
			} else {
				if result := runPython("install"); result["owned"] != true || result["allowed"] != true {
					t.Fatalf("Python restore: %#v", result)
				}
			}
			body, err := os.ReadFile(filepath.Join(home, ".codex", "skills", "free-test", "SKILL.md"))
			if err != nil || !strings.Contains(string(body), "Owned Current Skill") || strings.Contains(string(body), skillTrialGateMarker) {
				t.Fatalf("formal entry: %v", err)
			}
			state.mu.Lock()
			defer state.mu.Unlock()
			var creations int
			for _, request := range state.trialPurchaseRequests {
				if request["clientRequestId"] != "" {
					creations++
				}
			}
			if creations != 1 || state.grantUses != 0 {
				t.Fatalf("cross-client retry created %d orders or consumed %d uses", creations, state.grantUses)
			}
		})
	}
}
