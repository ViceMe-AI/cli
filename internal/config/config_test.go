package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveCreatesPreservesAndUpdatesProfileConfig(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	config := Default(RegionCN)
	result, err := Save(base, config)
	if err != nil || result.Status != "created" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if result.Path != filepath.Join(base, "config.json") {
		t.Fatalf("unexpected path: %s", result.Path)
	}
	info, err := os.Stat(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config permissions are too broad: %o", info.Mode().Perm())
	}
	result, err = Save(base, config)
	if err != nil || result.Status != "unchanged" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	config.Profiles[0].Region = RegionGlobal
	result, err = Save(base, config)
	if err != nil || result.Status != "updated" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	loaded, err := LoadOrDefault(base)
	if err != nil || loaded.Profiles[0].Region != RegionGlobal {
		t.Fatalf("config=%#v err=%v", loaded, err)
	}
	data, err := os.ReadFile(filepath.Join(base, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"currentProfile": "default"`, `"id": "default"`, `"name": "default"`, `"region": "global"`} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("config lacks %s: %s", expected, data)
		}
	}
}

func TestExplicitLocalProfileOverridesRemainLocalAndRequirePrivatePermissions(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	configured := Default(RegionCN)
	configured.Profiles[0].APIBaseURL = "http://localhost:8090"
	configured.Profiles[0].AccessToken = "local-operator-secret"
	if _, err := Save(base, configured); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOrDefault(base)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := loaded.Resolve("default")
	if err != nil || profile.APIBaseURL != "http://localhost:8090" || profile.AccessToken != "local-operator-secret" {
		t.Fatalf("profile=%#v err=%v", profile, err)
	}
	filename := filepath.Join(base, "config.json")
	if err := os.Chmod(filename, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrDefault(base); err == nil || !strings.Contains(err.Error(), "permissions 0600") {
		t.Fatalf("broad secret config permissions were accepted: %v", err)
	}
}

func TestLocalAccessTokenValidationDoesNotEchoSecret(t *testing.T) {
	t.Parallel()
	configured := Default(RegionCN)
	configured.Profiles[0].APIBaseURL = "http://localhost:8090"
	configured.Profiles[0].AccessToken = " secret-value "
	_, err := Save(t.TempDir(), configured)
	if err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("invalid token error=%v", err)
	}
}

func TestLoadDefaultsToCN(t *testing.T) {
	t.Parallel()
	loaded, err := LoadOrDefault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	profile, err := loaded.Resolve("")
	if err != nil || profile.Region != RegionCN || profile.Name != DefaultProfileName {
		t.Fatalf("profile=%#v err=%v", profile, err)
	}
	if APIBaseURL(profile.Region) != "https://api.viceme.cn" || APIBaseURL(RegionGlobal) != "https://api.viceme.ai" {
		t.Fatal("region endpoint mapping is incorrect")
	}
}

func TestLoadRefusesInvalidExistingConfig(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "config.json"), []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrDefault(base); err == nil {
		t.Fatal("expected invalid config to be preserved and reported")
	}
}

func TestLoadDoesNotAcceptLegacySingleRegionDocument(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "config.json"), []byte("{\"region\":\"global\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrDefault(base); err == nil {
		t.Fatal("legacy single-region config must not be migrated or accepted")
	}
}

func TestProfilesResolveAndRemainStableAcrossRename(t *testing.T) {
	t.Parallel()
	config := Default(RegionCN)
	profile, err := config.AddProfile("work", RegionGlobal)
	if err != nil {
		t.Fatal(err)
	}
	id := profile.ID
	config.CurrentProfile = profile.Name
	profile.Name = "company"
	config.CurrentProfile = profile.Name
	resolved, err := config.Resolve("company")
	if err != nil || resolved.ID != id || resolved.Region != RegionGlobal {
		t.Fatalf("profile=%#v err=%v", resolved, err)
	}
	if _, err := config.AddProfile("company", RegionCN); err == nil {
		t.Fatal("expected duplicate profile name to fail")
	}
}

func TestParseRegionAndProfileNameRejectUnknownValues(t *testing.T) {
	t.Parallel()
	if _, err := ParseRegion("us"); err == nil {
		t.Fatal("expected unknown region to fail")
	}
	for _, name := range []string{"", "has space", "../escape", "shell$var"} {
		if err := ValidateProfileName(name); err == nil {
			t.Fatalf("expected profile name %q to fail", name)
		}
	}
}
