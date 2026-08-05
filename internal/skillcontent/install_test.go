package skillcontent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCleanupPreservesTheOnlyBackupAfterRollbackFailure(t *testing.T) {
	stageRoot := t.TempDir()
	backup := filepath.Join(stageRoot, "previous")
	if err := os.Mkdir(backup, 0o755); err != nil {
		t.Fatal(err)
	}
	staged := []*stagedInstall{{
		target:    targetPath{name: "codex", path: filepath.Join(t.TempDir(), "viceme")},
		stageRoot: stageRoot,
		backup:    backup,
		backedUp:  true,
	}}

	cleanupSkillInstallStaging(staged, false)
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("failed rollback cleanup deleted the previous Skill backup: %v", err)
	}

	cleanupSkillInstallStaging(staged, true)
	if _, err := os.Stat(stageRoot); !os.IsNotExist(err) {
		t.Fatalf("committed install left staging root behind: %v", err)
	}
}

func TestRollbackReportsAndPreservesBackupWhenRestoreFails(t *testing.T) {
	stageRoot := t.TempDir()
	backup := filepath.Join(stageRoot, "previous")
	if err := os.Mkdir(backup, 0o755); err != nil {
		t.Fatal(err)
	}
	item := &stagedInstall{
		target:    targetPath{name: "codex", path: filepath.Join(t.TempDir(), "missing-parent", "viceme")},
		stageRoot: stageRoot,
		backup:    backup,
		backedUp:  true,
	}

	err := rollbackSkillInstalls([]*stagedInstall{item})
	if err == nil || !strings.Contains(err.Error(), backup) {
		t.Fatalf("rollback error did not report the preserved backup: %v", err)
	}
	cleanupSkillInstallStaging([]*stagedInstall{item}, false)
	if _, statErr := os.Stat(backup); statErr != nil {
		t.Fatalf("rollback failure deleted the previous Skill backup: %v", statErr)
	}
}

func TestDefaultConfigDirUsesVicemeHomeAndExplicitOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("VICEME_CLI_CONFIG_DIR", "")
	localAppData := filepath.Join(t.TempDir(), "谢忻彤", "AppData", "Local")
	t.Setenv("LOCALAPPDATA", localAppData)
	expected := filepath.Join(home, ".viceme-cli")
	if runtime.GOOS == "windows" {
		expected = filepath.Join(localAppData, "ViceMe", "Config")
	}
	if actual := defaultConfigDir(home); actual != expected {
		t.Fatalf("default config dir=%q", actual)
	}
	override := filepath.Join(t.TempDir(), "profiles")
	t.Setenv("VICEME_CLI_CONFIG_DIR", override)
	if actual := defaultConfigDir(home); actual != override {
		t.Fatalf("overridden config dir=%q", actual)
	}
}
