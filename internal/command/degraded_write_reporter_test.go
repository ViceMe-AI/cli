package command

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	"github.com/spf13/cobra"
)

// A degraded write warning must reach the stderr of the command instance that
// performed the write, never another instance sharing the process.
func TestDegradedWriteWarningBelongsToWritingCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this reproduction injects the Unix ACL-mismatch fixture")
	}
	t.Setenv("VICEME_ACCESS_TOKEN", "")
	t.Setenv("VICEME_INSTALL_METHOD", "")
	t.Setenv("VICEME_API_BASE_URL", "")

	var errA, errB bytes.Buffer
	newRoot := func(stderr io.Writer) (*cobra.Command, *Runtime) {
		dir := t.TempDir()
		root, rt, err := NewRoot(Dependencies{
			Out: io.Discard, ErrOut: stderr, Store: securestore.NewMemory(),
			Environment: skillcontent.Environment{Home: dir, ConfigDir: filepath.Join(dir, "config")},
			Region:      config.RegionCN,
		})
		if err != nil {
			t.Fatal(err)
		}
		return root, rt
	}
	rootA, runtimeA := newRoot(&errA)
	_, _ = newRoot(&errB)

	filename := filepath.Join(t.TempDir(), "command-a-state.json")
	if err := os.WriteFile(filename, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filename, 0o644); err != nil {
		t.Fatal(err)
	}
	originalRename := privatefile.RenameFile
	privatefile.RenameFile = func(string, string) error { return fmt.Errorf("injected rename denial: %w", syscall.EPERM) }
	t.Cleanup(func() { privatefile.RenameFile = originalRename })

	rootA.AddCommand(&cobra.Command{
		Use: "review-degraded-write",
		RunE: func(*cobra.Command, []string) error {
			return privatefile.WriteTolerant(filename, []byte("new"), ".review-*.tmp", runtimeA.deps.ReportDegradedWrite)
		},
	})
	rootA.SetArgs([]string{"review-degraded-write"})
	if err := rootA.Execute(); err != nil {
		t.Fatal(err)
	}
	if errA.Len() == 0 || errB.Len() != 0 {
		t.Fatalf("warning routed to wrong command: A_has_warning=%t B_has_warning=%t", errA.Len() > 0, errB.Len() > 0)
	}
}
