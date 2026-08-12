package paymentconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigRoundTripContainsOnlyNonSecretContext(t *testing.T) {
	root := t.TempDir()
	want := Config{
		SchemaVersion: 1, CapabilitySpace: "space-id", ApplicationID: "app-id",
		ApplicationSlug: "demo-app", Environment: "sandbox", MarketRegion: "CN",
		EnvironmentID: "environment-id", InstallationID: "installation-id",
	}
	filename, err := Save(root, want)
	if err != nil {
		t.Fatal(err)
	}
	got, loadedPath, err := Load(root)
	if err != nil || loadedPath != filename || got != want {
		t.Fatalf("unexpected round trip: got=%#v path=%q err=%v", got, loadedPath, err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".viceme", "payment.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range [][]byte{[]byte("vcp_sandbox_"), []byte("whsec_"), []byte("accessToken")} {
		if bytesContains(data, forbidden) {
			t.Fatalf("project config contains secret field %q: %s", forbidden, data)
		}
	}
}

func bytesContains(value, part []byte) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		match := true
		for offset := range part {
			if value[index+offset] != part[offset] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
