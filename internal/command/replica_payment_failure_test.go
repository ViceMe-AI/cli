package command

import (
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatepath"
	"testing"
)

func TestReplicaPaidFailureRetainsPaymentContext(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		t.Run(map[bool]string{false: "payment polling", true: "paid checkout"}[immediate], func(t *testing.T) {
			f := newReplicaRecoveryDiagnosticsFixture(t, immediate)
			if !immediate {
				f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED")
			}
			result := f.run(output.ExitPolicy, "REPLICA_LICENSE_SIGNATURE_INVALID", "--payment-presented")
			assertReplicaPaidFailure(t, result, f.orderNo)
			if f.checkoutCalls.Load() != 1 {
				t.Fatal("failure created another order")
			}
		})
	}
}

func assertReplicaPaidFailure(t *testing.T, result replicaDiagnosticsEnvelope, orderNo string) {
	t.Helper()
	details := result.Error.Details
	payment, _ := details["payment"].(map[string]any)
	if details["orderNo"] != orderNo || payment["status"] != "PAID" || details["nextAction"] != "STOP_AND_REPORT" {
		t.Fatalf("paid delivery failure lost confirmed payment context: %#v", details)
	}
	recovery, _ := details["recovery"].(map[string]any)
	if recovery["mode"] != "RECOVERY_ONLY" {
		t.Fatalf("paid failure did not restrict retry to delivery: %#v", recovery)
	}
}

func TestReplicaDownloadFailurePreservesPaidTimestampAndRecovery(t *testing.T) {
	f := newReplicaRecoveryDiagnosticsFixture(t, false)
	f.downloadFailure.Store(true)
	f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED")
	result := f.run(output.ExitNetwork, "DEPENDENCY_UNAVAILABLE", "--payment-presented")
	assertReplicaPaidFailure(t, result, f.orderNo)
	payment := result.Error.Details["payment"].(map[string]any)
	if payment["paidAt"] != "2026-09-08T14:38:37.000Z" {
		t.Fatalf("lost authoritative paidAt: %#v", payment)
	}
	// A new user-requested recovery uses the existing order without accepting a
	// price or creating another checkout, even when delivery still fails.
	recovered := f.run(output.ExitNetwork, "DEPENDENCY_UNAVAILABLE", "--recovery-only")
	assertReplicaPaidFailure(t, recovered, f.orderNo)
	if f.checkoutCalls.Load() != 1 {
		t.Fatal("paid recovery created a new order")
	}
	if err := privatepath.RequirePrivateFile(replicaTargetReservationPath(f.target)); err != nil {
		t.Fatal(err)
	}
}

func TestReplicaUnknownPaymentFailureDoesNotClaimPaid(t *testing.T) {
	f := newReplicaRecoveryDiagnosticsFixture(t, false)
	pending := f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED")
	f.statusFailure.Store(true)
	failed := f.run(output.ExitNetwork, "DEPENDENCY_UNAVAILABLE", "--payment-presented")
	for _, result := range []replicaDiagnosticsEnvelope{pending, failed} {
		payment, _ := result.Error.Details["payment"].(map[string]any)
		if payment["status"] == "PAID" {
			t.Fatal("unknown payment was reported as paid")
		}
	}
	if f.checkoutCalls.Load() != 1 {
		t.Fatal("status error replaced the order")
	}
}

func TestReplicaAccountPaidFailureRetainsPaymentContext(t *testing.T) {
	f := newReplicaRecoveryDiagnosticsFixture(t, false)
	f.account = true
	t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
	f.run(output.ExitConfirmation, "REPLICA_PURCHASE_CONFIRMATION_REQUIRED")
	f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED", "--confirm")
	result := f.run(output.ExitPolicy, "REPLICA_LICENSE_SIGNATURE_INVALID", "--confirm")
	assertReplicaPaidFailure(t, result, f.orderNo)
	recovered := f.run(output.ExitPolicy, "REPLICA_LICENSE_SIGNATURE_INVALID", "--recovery-only")
	assertReplicaPaidFailure(t, recovered, f.orderNo)
	if f.checkoutCalls.Load() != 1 {
		t.Fatal("account recovery created another order")
	}
}
