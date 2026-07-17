package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCreatesAndPreservesConfig(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	result, err := Ensure(base, Config{DefaultProfile: "default", APIBaseURL: "https://api.viceme.test"})
	if err != nil || result.Status != "created" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	info, err := os.Stat(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config permissions are too broad: %o", info.Mode().Perm())
	}
	result, err = Ensure(base, Config{DefaultProfile: "other", APIBaseURL: "https://changed.test"})
	if err != nil || result.Status != "unchanged" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	data, err := os.ReadFile(filepath.Join(base, "viceme", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" {
		t.Fatal("config is empty")
	}
}

func TestEnsureRefusesInvalidExistingConfig(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	directory := filepath.Join(base, "viceme")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(directory, "config.json")
	if err := os.WriteFile(filename, []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(base, Config{}); err == nil {
		t.Fatal("expected invalid config to be preserved and reported")
	}
}
