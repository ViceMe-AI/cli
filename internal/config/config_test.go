package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCreatesPreservesAndUpdatesRegion(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	result, err := Ensure(base, Config{Region: RegionCN})
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
	result, err = Ensure(base, Config{Region: RegionCN})
	if err != nil || result.Status != "unchanged" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	result, err = Ensure(base, Config{Region: RegionGlobal})
	if err != nil || result.Status != "updated" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	loaded, err := LoadOrDefault(base)
	if err != nil || loaded.Region != RegionGlobal {
		t.Fatalf("config=%#v err=%v", loaded, err)
	}
	data, err := os.ReadFile(filepath.Join(base, "viceme", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\n  \"region\": \"global\"\n}\n" {
		t.Fatalf("unexpected config: %s", data)
	}
}

func TestLoadDefaultsToCN(t *testing.T) {
	t.Parallel()
	loaded, err := LoadOrDefault(t.TempDir())
	if err != nil || loaded.Region != RegionCN {
		t.Fatalf("config=%#v err=%v", loaded, err)
	}
	if APIBaseURL(loaded.Region) != "https://api.viceme.cn" || APIBaseURL(RegionGlobal) != "https://api.viceme.ai" {
		t.Fatal("region endpoint mapping is incorrect")
	}
}

func TestEnsureCanonicalizesPreRegionConfig(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	directory := filepath.Join(base, "viceme")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(directory, "config.json")
	if err := os.WriteFile(filename, []byte("{\"api_base_url\":\"https://api.viceme.ai\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOrDefault(base)
	if err != nil || loaded.Region != RegionCN {
		t.Fatalf("config=%#v err=%v", loaded, err)
	}
	result, err := Ensure(base, Config{Region: RegionCN})
	if err != nil || result.Status != "updated" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\n  \"region\": \"cn\"\n}\n" {
		t.Fatalf("unexpected config: %s", data)
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
	if _, err := Ensure(base, Config{Region: RegionCN}); err == nil {
		t.Fatal("expected invalid config to be preserved and reported")
	}
}

func TestParseRegionRejectsUnknownValues(t *testing.T) {
	t.Parallel()
	if _, err := ParseRegion("us"); err == nil {
		t.Fatal("expected unknown region to fail")
	}
}
