package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
)

func TestReplicaPaymentTimeoutRetainsOriginalAttempt(t *testing.T) {
	for _, account := range []bool{false, true} {
		for _, terminal := range []bool{false, true} {
			name := map[bool]string{false: "anonymous", true: "account"}[account] + map[bool]string{false: "/late-paid", true: "/closed"}[terminal]
			t.Run(name, func(t *testing.T) {
				f := newReplicaRecoveryDiagnosticsFixture(t, false)
				f.account = account
				f.paymentStatus.Store("PENDING")
				f.validLicense.Store(true)
				now := time.Now().UTC()
				f.deps.Now = func() time.Time { return now }
				f.deps.Sleep = func(_ context.Context, delay time.Duration) error { now = now.Add(delay); return nil }
				waitArgs := []string{"--payment-presented", "--payment-result-first", "--timeout", "6s", "--interval", "3s"}
				if account {
					t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
					f.run(output.ExitConfirmation, "REPLICA_PURCHASE_CONFIRMATION_REQUIRED")
					f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED", "--confirm")
					waitArgs[0] = "--confirm"
				} else {
					f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED")
				}
				timedOut := f.run(output.ExitNetwork, "REPLICA_PAYMENT_TIMEOUT", waitArgs...)
				if timedOut.Error.Retryable || timedOut.Error.Details["nextAction"] != "STOP_AND_REPORT" || timedOut.Error.Details["orderNo"] != f.orderNo {
					t.Fatal("timeout did not preserve the original payment and stop")
				}
				if _, ok := timedOut.Error.Details["recovery"]; !ok {
					t.Fatal("timeout omitted its original-order recovery action")
				}
				f.run(output.ExitPolicy, "REPLICA_PAYMENT_RESTART_REQUIRED")
				f.run(output.ExitNetwork, "REPLICA_PAYMENT_TIMEOUT", waitArgs...)
				if terminal {
					f.paymentStatus.Store("CLOSED")
					if account {
						f.run(output.ExitPolicy, "REPLICA_PAYMENT_RESTART_REQUIRED", waitArgs...)
					} else {
						f.run(output.ExitPolicy, "REPLICA_PAYMENT_TERMINAL", waitArgs...)
					}
					f.run(output.ExitPolicy, "REPLICA_PAYMENT_RESTART_REQUIRED")
					f.run(output.ExitPolicy, "REPLICA_PAYMENT_RESTART_REQUIRED")
				} else {
					f.paymentStatus.Store("PAID")
					result := f.run(0, "", "--recovery-only", "--expected-order-no", f.orderNo)
					if result.Data.NextAction != "DEPLOY" || result.Data.OrderNo != f.orderNo {
						t.Fatal("late payment did not recover the original order")
					}
				}
				if f.checkoutCalls.Load() != 1 {
					t.Fatal("timeout or repeated invocation wrote another checkout")
				}
			})
		}
	}
}

func TestReplicaPaymentCannotReplaceStandaloneAttemptByChangingEngine(t *testing.T) {
	for _, status := range []string{"PENDING", "CLOSED", "PAID"} {
		t.Run(status, func(t *testing.T) {
			f := newReplicaRecoveryDiagnosticsFixture(t, false)
			f.paymentStatus.Store(status)
			runtime := &Runtime{configBase: f.deps.Environment.ConfigDir, apiBaseURL: f.deps.APIBaseURL, deps: f.deps}
			const shortCode = "VMR-ABCDEFGHIJKLMNOPQRST"
			resolved := api.WebsiteReplicaResolution{ReplicaID: "11111111-1111-4111-8111-111111111111", ShortCode: shortCode, ViceMeWorkURL: "https://viceme.example/alice/site"}
			fingerprint := sha256.Sum256([]byte("https://viceme.example/api/v1\n" + shortCode))
			directory := filepath.Join(runtime.configBase, "replica-purchases")
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			filename := filepath.Join(directory, "standalone-"+hex.EncodeToString(fingerprint[:])+".json")
			receipt, err := json.Marshal(standaloneReplicaAttempt{SchemaVersion: 1, ReplicaID: resolved.ReplicaID, OrderNo: f.orderNo, RecoverySecret: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"})
			if err != nil {
				t.Fatal(err)
			}
			if err := privatefile.Write(filename, receipt, ".standalone-test-*.tmp"); err != nil {
				t.Fatal(err)
			}
			if err := requireStandaloneAttemptResolved(context.Background(), runtime, resolved); err == nil || output.AsError(err).Subtype != "REPLICA_STANDALONE_RECOVERY_REQUIRED" {
				t.Fatal("changing engine allowed another purchase")
			}
			after, err := os.ReadFile(filename)
			if err != nil || !bytes.Equal(after, receipt) || f.checkoutCalls.Load() != 0 {
				t.Fatal("standalone attempt was changed")
			}
		})
	}
}
