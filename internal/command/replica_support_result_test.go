package command

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatepath"
)

func TestReplicaSupportResultPrecedesDeliveryAndResumesOriginalPurchase(t *testing.T) {
	for _, account := range []bool{false, true} {
		t.Run(map[bool]string{false: "anonymous", true: "account"}[account], func(t *testing.T) {
			f := newReplicaRecoveryDiagnosticsFixture(t, false)
			f.account = account
			f.validLicense.Store(true)
			waitArgs := []string{"--payment-presented", "--payment-result-first"}
			if account {
				t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
				f.run(output.ExitConfirmation, "REPLICA_PURCHASE_CONFIRMATION_REQUIRED")
				f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED", "--confirm")
				waitArgs = []string{"--confirm", "--payment-result-first"}
			} else {
				f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED")
			}
			before := f.downloadCalls.Load()
			result := f.run(0, "", waitArgs...).Data
			if result.NextAction != "PRESENT_SUPPORT_RESULT" || result.Payment.Status != "PAID" || result.OrderNo != f.orderNo || f.downloadCalls.Load() != before {
				t.Fatalf("support result did not precede delivery: %#v", result)
			}
			if _, err := os.Stat(f.target); !os.IsNotExist(err) {
				t.Fatal("target was installed before the presentation boundary")
			}
			if result.Presentation.WidgetPath == result.Presentation.ReplacesWidgetPath || !filepath.IsAbs(result.Presentation.WidgetPath) {
				t.Fatal("support result reused the cached pending path")
			}
			if err := privatepath.RequirePrivateFile(result.Presentation.WidgetPath); err != nil {
				t.Fatal(err)
			}
			html, err := os.ReadFile(result.Presentation.WidgetPath)
			if err != nil || !bytes.Contains(html, []byte(`"status":"PAID"`)) || bytes.Contains(html, []byte("weixin://")) || !bytes.Contains(html, []byte("创作者已收到你的支持")) {
				t.Fatal("invalid support presentation")
			}
			if result.Continuation.Mode != "RECOVERY_ONLY" || strings.Contains(strings.Join(result.Continuation.Args, " "), "--accept-price-cents") {
				t.Fatal("continuation could purchase again")
			}
			var stdout bytes.Buffer
			f.deps.Out = &stdout
			wrong := append([]string{}, result.Continuation.Args...)
			for i, arg := range wrong {
				if arg == "--expected-order-no" {
					wrong[i+1] = "VMO-OTHER"
				}
			}
			if exit := Execute(wrong, f.deps); exit != output.ExitPolicy || !strings.Contains(stdout.String(), "REPLICA_PURCHASE_RECOVERY_CONFLICT") || f.downloadCalls.Load() != before {
				t.Fatal("continuation accepted a different purchase")
			}
			stdout.Reset()
			if exit := Execute(result.Continuation.Args, f.deps); exit != 0 {
				t.Fatalf("continuation failed: %d %s", exit, stdout.String())
			}
			var completed struct {
				Data struct {
					NextAction string `json:"nextAction"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &completed); err != nil || completed.Data.NextAction != "DEPLOY" {
				t.Fatal("continuation did not complete delivery")
			}
			if content, err := os.ReadFile(filepath.Join(f.target, "index.html")); err != nil || string(content) != "<h1>Recovered</h1>" || f.checkoutCalls.Load() != 1 {
				t.Fatal("continuation changed the purchase or failed to install")
			}
			if _, err := os.Stat(result.Presentation.WidgetPath); err != nil {
				t.Fatal("delivery removed the displayed support result")
			}
		})
	}
}

func TestReplicaSupportResultKeepsPaidFactWhenDeliveryFails(t *testing.T) {
	f := newReplicaRecoveryDiagnosticsFixture(t, false)
	f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED")
	result := f.run(0, "", "--payment-presented", "--payment-result-first").Data
	f.downloadFailure.Store(true)
	failed := f.run(output.ExitNetwork, "DEPENDENCY_UNAVAILABLE", "--recovery-only")
	assertReplicaPaidFailure(t, failed, result.OrderNo)
	if f.checkoutCalls.Load() != 1 {
		t.Fatal("delivery failure caused another checkout")
	}
	if _, err := os.Stat(result.Presentation.WidgetPath); err != nil {
		t.Fatal("delivery failure removed the support result")
	}
}

func TestReplicaSupportResultRejectsUnknownPayment(t *testing.T) {
	f := newReplicaRecoveryDiagnosticsFixture(t, false)
	f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED")
	f.statusFailure.Store(true)
	result := f.run(output.ExitNetwork, "DEPENDENCY_UNAVAILABLE", "--payment-presented", "--payment-result-first")
	if result.Data.NextAction != "" || result.Data.Payment.Status == "PAID" {
		t.Fatal("unknown payment produced a support result")
	}
}
