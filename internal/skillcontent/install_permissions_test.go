package skillcontent

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/ViceMe-AI/cli/internal/privatefile"
)

func TestProbeInstallPermissionsChecksAutoTargetsWithoutDebris(t *testing.T) {
	home := t.TempDir()
	environment := Environment{
		Home:               home,
		ConfigDir:          filepath.Join(home, ".viceme-cli"),
		WorkBuddyConfigDir: filepath.Join(home, ".workbuddy"),
		AgentsSkillsDir:    filepath.Join(home, ".agents", "skills"),
	}
	if err := os.MkdirAll(environment.WorkBuddyConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ProbeInstallPermissions("auto", environment); err != nil {
		t.Fatalf("ProbeInstallPermissions() error = %v", err)
	}
	for _, directory := range []string{
		environment.ConfigDir,
		environment.AgentsSkillsDir,
		filepath.Join(environment.WorkBuddyConfigDir, "skills"),
	} {
		if _, err := os.Stat(filepath.Join(directory, installPermissionProbeDirectory)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("permission probe left debris in %s: %v", directory, err)
		}
	}
}

func TestProbeInstallPermissionsIdentifiesDeniedAgentTarget(t *testing.T) {
	home := t.TempDir()
	environment := Environment{
		Home:            home,
		ConfigDir:       filepath.Join(home, ".viceme-cli"),
		AgentsSkillsDir: filepath.Join(home, ".agents", "skills"),
	}
	originalRename := renamePath
	renameCalls := 0
	renamePath = func(oldName, newName string) error {
		renameCalls++
		if renameCalls > 2 {
			return &os.PathError{Op: "rename", Path: oldName, Err: syscall.EPERM}
		}
		return os.Rename(oldName, newName)
	}
	t.Cleanup(func() { renamePath = originalRename })

	err := ProbeInstallPermissions("agents", environment)
	var permissionErr *InstallPermissionError
	if !errors.As(err, &permissionErr) || permissionErr.Target != "agents" || !privatefile.IsPermissionDenial(err) {
		t.Fatalf("permission error = %#v, want denied agents target", err)
	}
}

func TestReadOnlyFilesystemIsPermissionDenial(t *testing.T) {
	err := &os.PathError{Op: "write", Path: "/readonly", Err: syscall.EROFS}
	if !privatefile.IsPermissionDenial(err) {
		t.Fatal("read-only filesystem was not classified as a permission denial")
	}
}

func TestProbeInstallPermissionsAuthorizedRetryCleansDeniedProbe(t *testing.T) {
	home := t.TempDir()
	environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	originalRename, originalRemoveAll := renamePath, removeAllPath
	t.Cleanup(func() {
		renamePath, removeAllPath = originalRename, originalRemoveAll
	})
	renamePath = func(oldName, _ string) error {
		return &os.PathError{Op: "rename", Path: oldName, Err: syscall.EPERM}
	}
	removeAllPath = func(target string) error {
		if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return &os.PathError{Op: "remove", Path: target, Err: syscall.EPERM}
	}
	if err := ProbeInstallPermissions("agents", environment); !privatefile.IsPermissionDenial(err) {
		t.Fatalf("denied preflight error = %v", err)
	}
	probe := filepath.Join(environment.ConfigDir, installPermissionProbeDirectory)
	if _, err := os.Stat(probe); err != nil {
		t.Fatalf("denied preflight did not retain its fixed recovery probe: %v", err)
	}

	renamePath, removeAllPath = originalRename, originalRemoveAll
	if err := ProbeInstallPermissions("agents", environment); err != nil {
		t.Fatalf("authorized retry error = %v", err)
	}
	if _, err := os.Stat(probe); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("authorized retry left the denied probe behind: %v", err)
	}
}

func TestCodexPermissionProbeDoesNotCreateLegacySkillsDirectory(t *testing.T) {
	home := t.TempDir()
	environment := Environment{Home: home, CodexHome: filepath.Join(home, "custom-codex"), ConfigDir: filepath.Join(home, ".viceme-cli")}
	if err := os.MkdirAll(environment.CodexHome, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"codex", "auto"} {
		if err := ProbeInstallPermissions(target, environment); err != nil {
			t.Fatalf("%s permission preflight failed: %v", target, err)
		}
	}
	if _, err := os.Stat(filepath.Join(environment.CodexHome, "skills")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("permission preflight created a legacy Skill directory: %v", err)
	}
}
