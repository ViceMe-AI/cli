package config

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestGatewayProfileLoadPreservesFileAndIdentity(t *testing.T) {
	for _, region := range []Region{RegionCN, RegionGlobal} {
		t.Run(string(region), func(t *testing.T) {
			directory := t.TempDir()
			original := Default(region)
			original.Profiles[0].APIBaseURL = APIStateBaseURL(APIBaseURL(region))
			original.Profiles[0].UserID = "existing-user"
			data, _ := json.Marshal(original)
			if err := os.WriteFile(ConfigPath(directory), data, 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadOrDefault(directory)
			if err != nil {
				t.Fatal(err)
			}
			want := original.Profiles[0]
			want.APIBaseURL = APIBaseURL(region)
			if loaded.Profiles[0] != want || loaded.CurrentProfile != original.CurrentProfile {
				t.Fatalf("profile identity changed: %#v", loaded)
			}
			current, _ := os.ReadFile(ConfigPath(directory))
			if !bytes.Equal(data, current) {
				t.Fatal("read rewrote the user's profile")
			}
			if _, err := Save(directory, loaded); err != nil {
				t.Fatal(err)
			}
			current, _ = os.ReadFile(ConfigPath(directory))
			if bytes.Contains(current, []byte(original.Profiles[0].APIBaseURL)) || !bytes.Contains(current, []byte(APIBaseURL(region))) {
				t.Fatal("explicit save did not persist Gateway URL")
			}
		})
	}
}

func TestGatewayAliasBoundaries(t *testing.T) {
	for input, want := range map[string]string{
		"HTTPS://API.VICEME.CN:443/":        "https://viceme.cn/api",
		"https://api.viceme.ai/":            "https://viceme.ai/api",
		"https://viceme.cn/api/":            "https://viceme.cn/api",
		"https://dev.viceme.cn/api":         "https://dev.viceme.cn/api",
		"http://localhost:3001":             "http://localhost:3001",
		"https://api.viceme.cn:8443":        "https://api.viceme.cn:8443",
		"https://api.viceme.cn/custom":      "https://api.viceme.cn/custom",
		"https://api.viceme.cn.example.com": "https://api.viceme.cn.example.com",
	} {
		got, err := NormalizeAPIBaseURL(input)
		if err != nil || got != want {
			t.Fatalf("%q: got %q, %v; want %q", input, got, err, want)
		}
	}
	for _, other := range []string{"https://viceme.ai/api", "https://viceme.cn", "https://viceme.cn/api/custom", "https://viceme.cn:8443/api", "https://api.viceme.cn:8443", "https://api.viceme.cn/custom", "https://api.viceme.cn.example.com"} {
		if EquivalentAPIBaseURLs("https://api.viceme.cn", other) {
			t.Fatalf("unrelated API accepted: %s", other)
		}
	}
}
