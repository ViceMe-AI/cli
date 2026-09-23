package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/agentenv"
	"github.com/ViceMe-AI/cli/internal/securestore"
)

func TestTrialHostedCheckoutSurvivesLocalPresentationFailure(t *testing.T) {
	for _, marker := range agentenv.AgentEnvMarkers {
		for _, key := range marker.Markers {
			t.Setenv(key, "")
		}
	}
	t.Setenv("AI_AGENT", "codex")
	t.Setenv(processAccessTokenEnvironment, "")
	for _, hosted := range []bool{false, true} {
		for _, blocked := range []bool{false, true} {
			name := "legacy"
			if hosted {
				name = "hosted"
			}
			if blocked {
				name += "-local-io-failure"
			}
			t.Run(name, func(t *testing.T) {
				state := newSkillTrialTestServer(t)
				state.trialHostedCheckout = hosted
				defer state.server.Close()
				home, store := t.TempDir(), securestore.NewMemory()
				invoke := func(args ...string) (int, map[string]any) {
					code, result, _ := executeSkillTrialCommand(t, state.server, home, store, args...)
					return code, result
				}
				if code, result := invoke("skill", "install", downloadableProductID, "--agent", "codex"); code != 0 {
					t.Fatalf("install: %#v", result)
				}
				if blocked {
					// Block only the optional artifact directory; credentials remain writable.
					if err := os.WriteFile(filepath.Join(home, ".viceme-cli", "payment-presentations"), []byte("fixture"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				_, result := invoke("skill", "trial-purchase", downloadableProductID, "--wait", "0")
				failure := result["error"].(map[string]any)
				if blocked && !hosted {
					if failure["code"] == "SKILL_PURCHASE_REQUIRED" {
						t.Fatal("legacy local IO failure was hidden")
					}
					return
				}
				if failure["code"] != "SKILL_PURCHASE_REQUIRED" {
					t.Fatalf("payment: %#v", result)
				}
				details := failure["details"].(map[string]any)
				hint := failure["hint"].(string)
				if hosted {
					if details["checkoutUrl"] == nil || details["checkoutImageUrl"] == nil {
						t.Fatal("lost hosted entry")
					}
					if strings.Contains(hint, "paymentPresentation.checkoutImageUrl") || !strings.Contains(hint, "[打开支付页面](完整 checkoutUrl)") || !strings.Contains(hint, "只嵌入一张") {
						t.Fatalf("wrong hosted output contract: %s", hint)
					}
				} else if details["checkoutUrl"] != nil || details["checkoutImageUrl"] != nil {
					t.Fatal("legacy server behavior changed")
				}
				if blocked && details["paymentPresentation"] != nil {
					t.Fatal("claimed nonexistent local presentation")
				}
				_, retry := invoke("skill", "trial-purchase", downloadableProductID, "--wait", "0")
				retried := retry["error"].(map[string]any)["details"].(map[string]any)
				if retried["orderNo"] != details["orderNo"] || retried["checkoutUrl"] != details["checkoutUrl"] {
					t.Fatal("recovery changed the order or hosted entry")
				}
				state.mu.Lock()
				state.paymentStatus = "PAID"
				state.mu.Unlock()
				if code, paid := invoke("skill", "trial-purchase", downloadableProductID, "--wait", "0", "--agent", "codex"); code != 0 || paid["data"].(map[string]any)["owned"] != true {
					t.Fatalf("paid restore: %#v", paid)
				}
			})
		}
	}
}

func TestHostedCheckoutPreferencesRespectHostCapabilities(t *testing.T) {
	for _, host := range []string{"workbuddy", "doubao", "codex", "claude", "unknown"} {
		hint := paymentPresentationHint(func(key string) string {
			for _, entry := range agentenv.AgentEnvMarkers {
				if string(entry.Platform) == host && key == entry.Markers[0] {
					return "fixture"
				}
			}
			return ""
		}, true)
		for _, text := range []string{"当前宿主明确支持", "Codex Desktop", "present_files", "ToolSearch", "DeferExecuteTool", "右侧内置浏览器", "不输出裸 URL", "queued", "Codex 终端版", "WorkBuddy", "豆包工作", "Claude", "同一订单只嵌入一张二维码图片", "不要提前在后台启动等待"} {
			if !strings.Contains(hint, text) {
				t.Fatalf("%s omitted %q", host, text)
			}
		}
	}
}

func TestTrialExpiredPendingRecoveryNeverPresentsEmptyPayment(t *testing.T) {
	state := newSkillTrialTestServer(t, func(s *skillTrialTestServer) { s.trialPurchaseExpired = true })
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	invoke := func(args ...string) (int, map[string]any) {
		code, result, _ := executeSkillTrialCommand(t, state.server, home, store, args...)
		return code, result
	}
	if code, result := invoke("skill", "install", downloadableProductID, "--agent", "codex"); code != 0 {
		t.Fatalf("install: %#v", result)
	}
	for i := 0; i < 2; i++ {
		_, result := invoke("skill", "trial-purchase", downloadableProductID, "--wait", "0")
		failure := result["error"].(map[string]any)
		if failure["code"] != "PAYMENT_CONFIRMATION_PENDING" {
			t.Fatalf("must wait for authoritative recovery: %#v", result)
		}
		details := failure["details"].(map[string]any)
		if details["nextAction"] != "WAIT_PAYMENT_CONFIRMATION" || details["paymentPresentation"] != nil {
			t.Fatalf("invalid presentation: %#v", details)
		}
	}
	state.mu.Lock()
	requests := append([]map[string]string(nil), state.trialPurchaseRequests...)
	state.paymentStatus = "PAID"
	state.mu.Unlock()
	if len(requests) != 3 || requests[0]["clientRequestId"] == "" || requests[0]["clientRequestId"] != requests[2]["clientRequestId"] {
		t.Fatalf("must resume same request after local status: %#v", requests)
	}
	if code, result := invoke("skill", "trial-purchase", downloadableProductID, "--wait", "0", "--agent", "codex"); code != 0 {
		t.Fatalf("paid recovery: %#v", result)
	}
}
