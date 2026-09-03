package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
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

func TestAutomaticNPMPermissionRefusalOnlyContinuesUntouchedGeneration(t *testing.T) {
	for _, state := range []string{"blocked", "recovery_pending"} {
		t.Run(state, func(t *testing.T) {
			clearAutomaticUpdateReexecutionEnvironment(t)
			root := t.TempDir()
			updater := &automaticUpdater{
				check:    updatepkg.CheckResult{CurrentVersion: "0.15.2", AvailableVersion: "0.16.0", UpdateAvailable: true},
				apply:    updatepkg.ApplyResult{Targets: []updatepkg.TargetResult{{Target: "npm_global", Status: state}, {Target: "agent_skill:auto", Status: state}}},
				applyErr: &updatepkg.OperationError{Kind: updatepkg.ErrorNPMPermission, Cause: errors.New("private npm output must not leak")},
			}
			var stdout, stderr bytes.Buffer
			exit := Execute([]string{"version"}, Dependencies{
				Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), Updater: updater,
				Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
				Region:      config.RegionCN, allowDevelopmentAutoUpdate: true,
				Reexecute: func(context.Context, []string, []string) (int, error) {
					t.Fatal("permission denial re-executed the process")
					return 1, nil
				},
			})
			var envelope struct {
				OK    bool          `json:"ok"`
				Error *output.Error `json:"error"`
				Meta  output.Meta   `json:"meta"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if state == "blocked" {
				if exit != 0 || !envelope.OK || envelope.Meta.AutoUpdate == nil || envelope.Meta.AutoUpdate.Status != "permission_required" || envelope.Meta.AutoUpdate.Code != "UPDATE_PERMISSION_REQUIRED" {
					t.Fatalf("unchanged generation lost business output or permission notice: %s", stdout.String())
				}
			} else if exit != output.ExitPolicy || envelope.OK || envelope.Error == nil || envelope.Error.Subtype != "UPDATE_PERMISSION_REQUIRED" || envelope.Error.Retryable {
				t.Fatalf("partial installation was allowed to continue: exit=%d %s", exit, stdout.String())
			}
			if updater.applyCalls.Load() != 1 || strings.Contains(stdout.String()+stderr.String(), "private npm output") {
				t.Fatal("permission refusal retried or leaked private diagnostics")
			}
		})
	}
}

func TestExplicitUpdatePermissionRefusalRequestsHostApproval(t *testing.T) {
	root := t.TempDir()
	updater := &automaticUpdater{
		check:    updatepkg.CheckResult{CurrentVersion: "0.15.2", AvailableVersion: "0.16.0", UpdateAvailable: true},
		applyErr: &updatepkg.OperationError{Kind: updatepkg.ErrorNPMPermission},
	}
	var stdout bytes.Buffer
	exit := Execute([]string{"update"}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), Updater: updater,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")}, Region: config.RegionCN,
	})
	if exit != output.ExitPolicy || !strings.Contains(stdout.String(), "UPDATE_PERMISSION_REQUIRED") || !strings.Contains(stdout.String(), "official approval mechanism") || strings.Contains(stdout.String(), "registry, proxy") || updater.applyCalls.Load() != 1 {
		t.Fatalf("missing bounded host authorization guidance: exit=%d %s", exit, stdout.String())
	}
}

func TestStartupRecoveryPermissionRefusalStopsBeforeBusiness(t *testing.T) {
	root := t.TempDir()
	updater := &startupRecoveryUpdater{err: &updatepkg.OperationError{Kind: updatepkg.ErrorNPMPermission}}
	var stdout bytes.Buffer
	exit := Execute([]string{"version"}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), Updater: updater,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")}, Region: config.RegionCN,
	})
	if exit != output.ExitPolicy || !strings.Contains(stdout.String(), "UPDATE_PERMISSION_REQUIRED") || !strings.Contains(stdout.String(), `"recovery_required": true`) || strings.Contains(stdout.String(), `"ok": true`) {
		t.Fatalf("recovery denial hidden or business continued: exit=%d %s", exit, stdout.String())
	}
}
