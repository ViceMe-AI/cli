package command

import (
	"bytes"
	"github.com/ViceMe-AI/cli/internal/output"
	"os"
	"path/filepath"
	"testing"
)

func TestReplicaMissingReservationResumesOriginalPurchase(t *testing.T) {
	f := newReplicaRecoveryDiagnosticsFixture(t, false)
	f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED")
	marker := replicaTargetReservationPath(f.target)
	original, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	f.run(output.ExitPolicy, "REPLICA_LICENSE_SIGNATURE_INVALID", "--payment-presented")
	restored, err := os.ReadFile(marker)
	if err != nil || !bytes.Equal(original, restored) {
		t.Fatalf("original reservation was not restored: %v", err)
	}
	if f.checkoutCalls.Load() != 1 {
		t.Fatal("reservation repair created another order")
	}
}

func TestReplicaReservationRecoveryPreservesConflicts(t *testing.T) {
	for _, kind := range []string{"mismatch", "symlink", "occupied target", "replaced parent"} {
		t.Run(kind, func(t *testing.T) {
			f := newReplicaRecoveryDiagnosticsFixture(t, false)
			f.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED")
			marker := replicaTargetReservationPath(f.target)
			code := "REPLICA_TARGET_RESERVATION_INVALID"
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "mismatch":
				if err := os.WriteFile(marker, []byte("another reservation\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				alternate := filepath.Join(filepath.Dir(f.target), "another-marker")
				if err := os.WriteFile(alternate, []byte("another reservation\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(alternate, marker); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			case "occupied target":
				if err := os.Mkdir(f.target, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(f.target, "keep.txt"), []byte("user content"), 0600); err != nil {
					t.Fatal(err)
				}
			case "replaced parent":
				// Keep the purchase configuration outside the exchanged target parent.
				// The dedicated parent-identity test exercises a copied marker as well.
				parent := filepath.Dir(f.target)
				replacement := parent + "-moved"
				if err := os.Rename(parent, replacement); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.RemoveAll(parent); os.Rename(replacement, parent) })
				if err := os.Mkdir(parent, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(filepath.Join(replacement, "config"), filepath.Join(parent, "config")); err != nil {
					t.Fatal(err)
				}
				code = "REPLICA_TARGET_PARENT_CHANGED"
			}
			result := f.run(output.ExitPolicy, code, "--payment-presented")
			if result.Error.Details["source"] != "LOCAL_FILESYSTEM" || result.Error.Details["nextAction"] != "STOP_AND_REPORT" {
				t.Fatalf("local failure was not identified: %#v", result.Error.Details)
			}
			if f.checkoutCalls.Load() != 1 || f.paidObserved.Load() {
				t.Fatal("conflicting target reached payment or created another order")
			}
			if kind == "mismatch" {
				if data, err := os.ReadFile(marker); err != nil || string(data) != "another reservation\n" {
					t.Fatal("foreign reservation changed")
				}
			}
			if kind == "symlink" {
				if info, err := os.Lstat(marker); err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatal("symlink was replaced")
				}
			}
			if kind == "occupied target" {
				if data, err := os.ReadFile(filepath.Join(f.target, "keep.txt")); err != nil || string(data) != "user content" {
					t.Fatal("existing target changed")
				}
			}
		})
	}
}
