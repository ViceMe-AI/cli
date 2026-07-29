package skillcontent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsConfigDirKeepsExistingLegacyConfig(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, ".viceme-cli")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VICEME_CLI_CONFIG_DIR", "")
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "AppData", "Local"))
	if actual := defaultConfigDir(home); actual != legacy {
		t.Fatalf("default config dir=%q, want legacy %q", actual, legacy)
	}
}
