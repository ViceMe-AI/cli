package skillcontent

import (
	"os"
	"path/filepath"
	"runtime"
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
