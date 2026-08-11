package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileRoundTripContainsNoCredentialFields(t *testing.T) {
	directory := t.TempDir()
	configured := Default(RegionCN)
	profile, err := configured.AddProfile("global", RegionGlobal)
	if err != nil {
		t.Fatal(err)
	}
	configured.CurrentProfile = profile.Name
	if _, err := Save(directory, configured); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOrDefault(directory)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CurrentProfile != "global" || len(loaded.Profiles) != 2 {
		t.Fatalf("unexpected config: %#v", loaded)
	}
	data, err := os.ReadFile(filepath.Join(directory, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"accessToken", "refreshToken", "credential"} {
		if stringContains(string(data), forbidden) {
			t.Fatalf("config contains %q: %s", forbidden, data)
		}
	}
}

func TestAPIBaseURLs(t *testing.T) {
	if APIBaseURL(RegionCN) != "https://api.viceme.cn" {
		t.Fatal("unexpected CN API URL")
	}
	if APIBaseURL(RegionGlobal) != "https://api.viceme.ai" {
		t.Fatal("unexpected global API URL")
	}
}

func stringContains(value, part string) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		if value[index:index+len(part)] == part {
			return true
		}
	}
	return false
}
