//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigRejectsBroadUnixPermissions(t *testing.T) {
	base := t.TempDir()
	configured := Default(RegionCN)
	if _, err := Save(base, configured); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(base, "config.json")
	if err := os.Chmod(filename, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrDefault(base); err == nil || !strings.Contains(err.Error(), "permissions 0600") {
		t.Fatalf("broad config permissions were accepted: %v", err)
	}
}
