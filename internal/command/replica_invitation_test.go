package command

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
)

func TestReplicaInvitationSurvivesTimeoutAndRecoveryWithoutAnotherStart(t *testing.T) {
	for _, account := range []bool{false, true} {
		t.Run(map[bool]string{false: "anonymous", true: "account"}[account], func(t *testing.T) {
			const id = "66666666-6666-4666-8666-666666666666"
			f := newReplicaRecoveryDiagnosticsFixture(t, false)
			f.account = account
			f.paymentStatus.Store("PENDING")
			f.validLicense.Store(true)
			now := time.Now().UTC()
			f.deps.Now = func() time.Time { return now }
			f.deps.Sleep = func(_ context.Context, d time.Duration) error { now = now.Add(d); return nil }
			wait := []string{"--payment-presented", "--payment-result-first", "--timeout", "6s", "--interval", "3s"}
			if account {
				t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
				f.run(output.ExitConfirmation, "REPLICA_PURCHASE_CONFIRMATION_REQUIRED", "--invitation-flow-id", id)
				f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED", "--confirm")
				wait[0] = "--confirm"
			} else {
				f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED", "--invitation-flow-id", id)
			}
			timeout := f.run(output.ExitNetwork, "REPLICA_PAYMENT_TIMEOUT", wait...)
			recovery := timeout.Error.Details["recovery"].(map[string]any)
			args := recovery["args"].([]any)
			found := false
			for _, arg := range args {
				if arg == id {
					found = true
				}
			}
			if !found {
				t.Fatal("recovery lost its invitation identity")
			}
			f.paymentStatus.Store("PAID")
			f.run(0, "", "--recovery-only", "--expected-order-no", f.orderNo)
			if f.checkoutCalls.Load() != 1 {
				t.Fatal("analytics changed checkout behavior")
			}
			f.invitationMu.Lock()
			defer f.invitationMu.Unlock()
			installed := 0
			for _, event := range f.invitationEvents {
				if event["flowId"] != id {
					t.Fatal("flow identity changed")
				}
				if event["event"] == "RESTORED" {
					t.Fatal("normal paid continuation excluded the original flow")
				}
				if event["event"] == "INSTALL_COMPLETED" {
					installed++
				}
				if event["path"] == "/v1/website-replica-invitation-flows" {
					t.Fatal("install or retry created another start")
				}
			}
			if installed != 1 {
				t.Fatalf("expected installation report, got %d", installed)
			}
		})
	}
}

func TestReplicaInvitationUnavailableDoesNotChangeSuccessfulInstall(t *testing.T) {
	f := newReplicaRecoveryDiagnosticsFixture(t, true)
	f.validLicense.Store(true)
	f.invitationStatus = 503
	result := f.run(0, "", "--invitation-flow-id", "66666666-6666-4666-8666-666666666666")
	if result.Data.NextAction != "DEPLOY" || f.checkoutCalls.Load() != 1 {
		t.Fatal("analytics failure changed install")
	}
}

func TestReplicaInstallWithoutInvitationDoesNotSendEvents(t *testing.T) {
	f := newReplicaRecoveryDiagnosticsFixture(t, false)
	f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED")
	if len(f.invitationEvents) != 0 {
		t.Fatal("ordinary command generated analytics")
	}
}

func TestReplicaNewInvitationContinuingOldOrderCompletesOriginalFlow(t *testing.T) {
	const original = "66666666-6666-4666-8666-666666666666"
	const resumed = "77777777-7777-4777-8777-777777777777"
	f := newReplicaRecoveryDiagnosticsFixture(t, false)
	f.validLicense.Store(true)
	f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED", "--invitation-flow-id", original)
	f.paymentStatus.Store("PAID")
	f.run(0, "", "--recovery-only", "--expected-order-no", f.orderNo, "--invitation-flow-id", resumed)
	restored, installed := 0, 0
	for _, event := range f.invitationEvents {
		if event["event"] == "RESTORED" {
			restored++
			if event["flowId"] != resumed {
				t.Fatal("excluded original invitation")
			}
		}
		if event["event"] == "INSTALL_COMPLETED" {
			installed++
			if event["flowId"] != original {
				t.Fatal("installation lost original attribution")
			}
		}
	}
	if restored != 1 || installed != 1 || f.checkoutCalls.Load() != 1 {
		t.Fatal("recovery did not retain original order and flow")
	}
}

func TestReplicaInvitationLeavesLegacyPurchaseFilesUnchanged(t *testing.T) {
	const id = "66666666-6666-4666-8666-666666666666"
	f := newReplicaRecoveryDiagnosticsFixture(t, false)
	f.validLicense.Store(true)
	f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED", "--invitation-flow-id", id)
	runtime := &Runtime{configBase: f.deps.Environment.ConfigDir, apiBaseURL: f.deps.APIBaseURL, deps: f.deps}
	target, err := validateReplicaTarget(f.target)
	if err != nil {
		t.Fatal(err)
	}
	store, err := newReplicaPurchaseStore(runtime, "VMR-ABCDEFGHIJKLMNOPQRST", target)
	if err != nil {
		t.Fatal(err)
	}
	assertCompatible := func(filename string) {
		t.Helper()
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("invitationFlowId")) {
			t.Fatal("analytics changed a strictly decoded legacy file")
		}
		if readReplicaInvitation(filename, data) != id {
			t.Fatal("sidecar lost attribution")
		}
		if readReplicaInvitation(filename, append(data, ' ')) != "" {
			t.Fatal("old-client rewrite reused stale attribution")
		}
	}
	assertCompatible(store.filename)
	f.paymentStatus.Store("PAID")
	f.run(0, "", "--recovery-only", "--expected-order-no", f.orderNo)
	assertCompatible(store.paidFilename)
	assertCompatible(store.completionFilename)
}

func TestReplicaInvitationUnreadableOrUnwritableSidecarDoesNotBlockRecovery(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "corrupt", true: "unwritable"}[directory], func(t *testing.T) {
			const id = "66666666-6666-4666-8666-666666666666"
			f := newReplicaRecoveryDiagnosticsFixture(t, false)
			f.validLicense.Store(true)
			f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED", "--invitation-flow-id", id)
			sidecars, err := filepath.Glob(filepath.Join(f.deps.Environment.ConfigDir, "replica-purchases", "*.invitation.json"))
			if err != nil || len(sidecars) != 1 {
				t.Fatal("missing sidecar")
			}
			if directory {
				if err := os.Remove(sidecars[0]); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(sidecars[0], 0700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(sidecars[0], []byte("corrupt"), 0600); err != nil {
				t.Fatal(err)
			}
			if directory {
				saveReplicaInvitation(sidecars[0][:len(sidecars[0])-len(".invitation.json")], []byte("state"), id)
			}
			f.paymentStatus.Store("PAID")
			result := f.run(0, "", "--recovery-only", "--expected-order-no", f.orderNo)
			if result.Data.NextAction != "DEPLOY" || f.checkoutCalls.Load() != 1 {
				t.Fatal("analytics affected purchase recovery")
			}
		})
	}
}
