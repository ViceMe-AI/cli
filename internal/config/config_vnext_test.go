package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileRoundTripContainsNoCredentialFields(t *testing.T) {
	directory := t.TempDir()
	configured := Default(RegionCN)
	profile, err := configured.AddProfile("global", RegionGlobal, "")
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
	for _, forbidden := range []string{"accessToken", "refreshToken", "credential", "apiBaseUrl"} {
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

func TestCustomProfileAPIBaseURLRoundTripsCanonically(t *testing.T) {
	directory := t.TempDir()
	configured := Default(RegionCN)
	profile, err := configured.AddProfile("shop-dev", RegionCN, "HTTPS://SHOP-DEV.EXAMPLE.COM:443/api/")
	if err != nil {
		t.Fatal(err)
	}
	if profile.APIBaseURL != "https://shop-dev.example.com/api" || profile.ResolvedAPIBaseURL() != profile.APIBaseURL {
		t.Fatalf("custom endpoint was not canonicalized: %#v", profile)
	}
	configured.CurrentProfile = profile.Name
	if _, err := Save(directory, configured); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOrDefault(directory)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := loaded.Resolve("shop-dev")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.APIBaseURL != "https://shop-dev.example.com/api" {
		t.Fatalf("custom endpoint did not round-trip: %#v", resolved)
	}
}

func TestCustomProfileWebBaseURLRequiresExplicitConfigurationAndRoundTripsCanonically(t *testing.T) {
	directory := t.TempDir()
	configured := Default(RegionCN)
	profile, err := configured.AddProfile("shop-dev", RegionCN, "https://api.shop-dev.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if profile.ResolvedWebBaseURL() != "" {
		t.Fatalf("custom API profile guessed a Web origin: %#v", profile)
	}
	if err := configured.SetProfileWebBaseURL(profile.Name, "HTTPS://WEB.SHOP-DEV.EXAMPLE.COM:443/"); err != nil {
		t.Fatal(err)
	}
	if profile.WebBaseURL != "https://web.shop-dev.example.com" || profile.ResolvedWebBaseURL() != profile.WebBaseURL {
		t.Fatalf("custom Web endpoint was not canonicalized: %#v", profile)
	}
	configured.CurrentProfile = profile.Name
	if _, err := Save(directory, configured); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOrDefault(directory)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := loaded.Resolve(profile.Name)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.WebBaseURL != "https://web.shop-dev.example.com" {
		t.Fatalf("custom Web endpoint did not round-trip: %#v", resolved)
	}
	defaultProfile, err := loaded.Resolve(DefaultProfileName)
	if err != nil {
		t.Fatal(err)
	}
	if defaultProfile.ResolvedWebBaseURL() != "https://viceme.cn" {
		t.Fatalf("regional default profile lost its Web origin: %#v", defaultProfile)
	}
}

func TestNormalizeAPIBaseURLRejectsUnsafePersistentEndpoints(t *testing.T) {
	t.Parallel()
	invalid := []string{
		"", "http://shop-dev.example.com/api", "ftp://shop-dev.example.com/api",
		"https://user:secret@shop-dev.example.com/api", "https://shop-dev.example.com/api?token=value",
		"https://shop-dev.example.com/api#fragment", "https://shop-dev.example.com/api/../admin",
		`https://shop-dev.example.com/api\v1`,
	}
	for _, value := range invalid {
		if _, err := NormalizeAPIBaseURL(value); err == nil {
			t.Errorf("unsafe API base URL was accepted: %q", value)
		}
	}
	for _, value := range []string{"http://localhost:3001/api/", "http://127.0.0.1:3001/api", "http://[::1]:3001/api"} {
		if _, err := NormalizeAPIBaseURL(value); err != nil {
			t.Errorf("loopback development endpoint was rejected: %q: %v", value, err)
		}
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
