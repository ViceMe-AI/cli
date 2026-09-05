package command

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
)

func TestAutomaticUpdateLaunchPermissionDenialDoesNotChangeForegroundCommand(t *testing.T) {
	enableAutomaticUpdateTest(t)
	clearAutomaticUpdateReexecutionEnvironment(t)
	root := t.TempDir()
	updater := &automaticUpdater{}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"version"}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), Updater: updater,
		Environment:                skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:                     config.RegionCN,
		allowDevelopmentAutoUpdate: true,
		StartBackgroundUpdate: func() error {
			return &os.PathError{Op: "start", Path: "viceme", Err: os.ErrPermission}
		},
	})
	if exit != 0 || updater.checkCalls.Load() != 0 || updater.applyCalls.Load() != 0 {
		t.Fatalf("sandbox denial affected foreground work: exit=%d checks=%d applies=%d", exit, updater.checkCalls.Load(), updater.applyCalls.Load())
	}
	if !strings.Contains(stdout.String(), `"ok": true`) || stderr.Len() != 0 {
		t.Fatalf("sandbox denial polluted the command response: stdout=%q stderr=%q", stdout.String(), stderr.String())
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

func TestBootstrapRecoveryPermissionRefusalRetriesInstaller(t *testing.T) {
	root := t.TempDir()
	updater := &startupRecoveryUpdater{err: &updatepkg.OperationError{Kind: updatepkg.ErrorNPMPermission}}
	var stdout bytes.Buffer
	exit := Execute([]string{"bootstrap", "activate", "--destination", filepath.Join(root, "bin", "viceme")}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), Updater: updater,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")}, Region: config.RegionCN,
	})
	if exit != output.ExitPolicy || !strings.Contains(stdout.String(), "UPDATE_PERMISSION_REQUIRED") ||
		!strings.Contains(stdout.String(), `"recovery_required": true`) ||
		!strings.Contains(stdout.String(), "same versioned installer") || strings.Contains(stdout.String(), "viceme update") ||
		strings.Contains(stdout.String(), `"ok": true`) {
		t.Fatalf("first-install recovery lost its error or retry entrypoint: exit=%d %s", exit, stdout.String())
	}
}

func TestBootstrapPermissionHintPreservesErrorDetails(t *testing.T) {
	original := updatePermissionRequired(os.ErrPermission).WithDetails(map[string]any{"recovery_required": true})
	adjusted := bootstrapCommandError(original, true).(*output.Error)
	if adjusted.Code != original.Code || adjusted.Type != original.Type || adjusted.Details == nil ||
		!strings.Contains(adjusted.Hint, "same versioned installer") || original.Hint != updatePermissionHint {
		t.Fatalf("bootstrap guidance mutated the shared error or lost the protocol: %#v", adjusted)
	}
	if bootstrapCommandError(original, false) != original {
		t.Fatal("ordinary update guidance changed")
	}
}

func TestBootstrapDestinationPermissionRefusalRetriesInstaller(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires POSIX directory permissions enforced for a non-root user")
	}
	root := t.TempDir()
	directory := filepath.Join(root, "bin")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(directory, "viceme")
	if err := os.WriteFile(destination, []byte("previous generation"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0o755) })
	var stdout bytes.Buffer
	exit := Execute([]string{"bootstrap", "activate", "--destination", destination}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")}, Region: config.RegionCN,
	})
	if exit != output.ExitPolicy || !strings.Contains(stdout.String(), "same versioned installer") || strings.Contains(stdout.String(), "viceme update") {
		t.Fatalf("bootstrap preflight did not guide the correct retry: exit=%d %s", exit, stdout.String())
	}
	if data, err := os.ReadFile(destination); err != nil || string(data) != "previous generation" {
		t.Fatalf("preflight changed the previous executable: %v", err)
	}
}
