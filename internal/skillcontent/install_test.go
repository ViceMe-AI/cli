package skillcontent

import (
	"path/filepath"
	"testing"
)

func TestDefaultConfigDirUsesVicemeHomeAndExplicitOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("VICEME_CLI_CONFIG_DIR", "")
	if actual := defaultConfigDir(home); actual != filepath.Join(home, ".viceme-cli") {
		t.Fatalf("default config dir=%q", actual)
	}
	override := filepath.Join(t.TempDir(), "profiles")
	t.Setenv("VICEME_CLI_CONFIG_DIR", override)
	if actual := defaultConfigDir(home); actual != override {
		t.Fatalf("overridden config dir=%q", actual)
	}
}
