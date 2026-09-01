package command

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
)

func TestAutomaticUpdateSkipsWhenActivationIsSandboxDenied(t *testing.T) {
	clearAutomaticUpdateReexecutionEnvironment(t)
	root := t.TempDir()
	updater := &automaticUpdater{
		check: updatepkg.CheckResult{CurrentVersion: "0.15.2", AvailableVersion: "0.16.0", UpdateAvailable: true},
	}

	original := privatefile.RenameFile
	privatefile.RenameFile = func(oldName, _ string) error {
		return fmt.Errorf("rename %s: %w", oldName, syscall.EPERM)
	}
	t.Cleanup(func() { privatefile.RenameFile = original })

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var reexecuted atomic.Bool
	exit := Execute([]string{"version"}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), Updater: updater,
		Environment:                skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:                     config.RegionCN,
		allowDevelopmentAutoUpdate: true,
		Reexecute: func(context.Context, []string, []string) (int, error) {
			reexecuted.Store(true)
			return 0, nil
		},
	})
	if exit != 0 || reexecuted.Load() || updater.checkCalls.Load() != 1 || updater.applyCalls.Load() != 0 {
		t.Fatalf("sandbox denial changed the business command: exit=%d reexecuted=%t checks=%d applies=%d", exit, reexecuted.Load(), updater.checkCalls.Load(), updater.applyCalls.Load())
	}
	if !strings.Contains(stderr.String(), "Automatic CLI update skipped") {
		t.Fatalf("stderr did not explain the skipped update: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"ok": true`) {
		t.Fatalf("business command did not succeed: %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, "config", ".viceme-activation-probe")); err != nil {
		t.Fatalf("sandboxed probe left no reusable staged file: %v", err)
	}
}
