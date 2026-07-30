package skillcontent

import (
	"path/filepath"
	"runtime"
	"testing"
)

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
