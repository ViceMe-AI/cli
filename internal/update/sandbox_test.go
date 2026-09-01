package update

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/ViceMe-AI/cli/internal/privatefile"
)

func TestProbeRenameCapabilityPassesWithoutDebris(t *testing.T) {
	directory := t.TempDir()
	if err := ProbeRenameCapability(directory); err != nil {
		t.Fatalf("ProbeRenameCapability() error = %v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("probe left debris: %v", entries)
	}
}

func TestProbeRenameCapabilityReportsSandboxDenial(t *testing.T) {
	original := privatefile.RenameFile
	privatefile.RenameFile = func(oldName, _ string) error {
		return fmt.Errorf("rename %s: %w", oldName, syscall.EPERM)
	}
	t.Cleanup(func() { privatefile.RenameFile = original })

	directory := t.TempDir()
	if err := ProbeRenameCapability(directory); !errors.Is(err, ErrRenameDenied) {
		t.Fatalf("ProbeRenameCapability() error = %v, want ErrRenameDenied", err)
	}
	// A second probe reuses the same staged file instead of accumulating debris.
	if err := ProbeRenameCapability(directory); !errors.Is(err, ErrRenameDenied) {
		t.Fatalf("second ProbeRenameCapability() error = %v, want ErrRenameDenied", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != activationProbeStaged {
		t.Fatalf("probe debris = %v, want exactly the staged probe file", entries)
	}
}
