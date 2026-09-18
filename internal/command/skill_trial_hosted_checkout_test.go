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
					if strings.Contains(hint, "paymentPresentation.checkoutImageUrl") || !strings.Contains(hint, "始终写成可点击") || !strings.Contains(hint, "只嵌入一张") {
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
		hint := skillPaymentPresentationHint(func(key string) string {
			for _, entry := range agentenv.AgentEnvMarkers {
				if string(entry.Platform) == host && key == entry.Markers[0] {
					return "fixture"
				}
			}
			return ""
		}, true)
		for _, text := range []string{"当前宿主明确支持", "Codex Desktop", "open_in_codex", "target.type=browser", "queued", "Codex 终端版", "WorkBuddy", "豆包工作", "Claude", "同一订单只嵌入一张二维码图片", "不要提前在后台启动等待"} {
			if !strings.Contains(hint, text) {
				t.Fatalf("%s omitted %q", host, text)
			}
		}
	}
}
