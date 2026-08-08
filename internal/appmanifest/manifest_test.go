package appmanifest

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	directory := t.TempDir()
	expected := Manifest{
		SchemaVersion:  SchemaVersion,
		AppID:          "550e8400-e29b-41d4-a716-446655440000",
		HostingMode:    "EXTERNAL",
		Environment:    "TEST",
		PublishableKey: "app_pk_test_abcdefghijklmnopqrstuvwxyz123456",
		Origin:         "http://localhost:3000",
		Capabilities: map[string]Capability{
			"commerce": {ContractVersion: "1.0.0", SDKPackage: "@viceme/web-sdk", SDKVersion: "0.1.0"},
		},
	}
	filename, err := Save(directory, expected)
	if err != nil {
		t.Fatal(err)
	}
	if filename != filepath.Join(directory, ".viceme", "app.json") {
		t.Fatalf("unexpected path %q", filename)
	}
	actual, err := Load(directory)
	if err != nil {
		t.Fatal(err)
	}
	if actual.AppID != expected.AppID || actual.Capabilities["commerce"] != expected.Capabilities["commerce"] {
		t.Fatalf("manifest mismatch: %#v", actual)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(Path(directory)), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"schemaVersion":1,"appId":"550e8400-e29b-41d4-a716-446655440000","hostingMode":"EXTERNAL","environment":"TEST","publishableKey":"app_pk_test_abcdefghijklmnopqrstuvwxyz123456","capabilities":{},"secret":"no"}`)
	if err := os.WriteFile(Path(directory), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(directory); err == nil {
		t.Fatal("expected unknown field rejection")
	}
}

func TestLoadRejectsMissingOrNullCapabilities(t *testing.T) {
	for _, body := range []string{
		`{"schemaVersion":1,"appId":"550e8400-e29b-41d4-a716-446655440000","hostingMode":"EXTERNAL","environment":"TEST","publishableKey":"app_pk_test_abcdefghijklmnopqrstuvwxyz123456"}`,
		`{"schemaVersion":1,"appId":"550e8400-e29b-41d4-a716-446655440000","hostingMode":"EXTERNAL","environment":"TEST","publishableKey":"app_pk_test_abcdefghijklmnopqrstuvwxyz123456","capabilities":null}`,
	} {
		directory := t.TempDir()
		if err := os.MkdirAll(filepath.Dir(Path(directory)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(Path(directory), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(directory); err == nil {
			t.Fatalf("manifest accepted missing/null capabilities: %s", body)
		}
	}
}

func TestLoadRejectsTrailingDataAndInconsistentPublicBinding(t *testing.T) {
	base := Manifest{
		SchemaVersion:  SchemaVersion,
		AppID:          "550e8400-e29b-41d4-a716-446655440000",
		HostingMode:    "EXTERNAL",
		Environment:    "TEST",
		PublishableKey: "app_pk_test_abcdefghijklmnopqrstuvwxyz123456",
		Capabilities:   map[string]Capability{},
	}
	directory := t.TempDir()
	if _, err := Save(directory, base); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(Path(directory), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{}\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(directory); err == nil {
		t.Fatal("manifest with trailing JSON was accepted")
	}

	base.PublishableKey = "app_pk_live_abcdefghijklmnopqrstuvwxyz123456"
	if _, err := Save(t.TempDir(), base); err == nil {
		t.Fatal("environment/key mismatch was accepted")
	}
	base.PublishableKey = "app_pk_test_abcdefghijklmnopqrstuvwxyz123456"
	base.Origin = "https://example.com/path"
	if _, err := Save(t.TempDir(), base); err == nil {
		t.Fatal("non-origin URL was accepted")
	}
}

func TestLoadReportsMissingManifest(t *testing.T) {
	_, err := Load(t.TempDir())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestOriginAcceptsCanonicalLoopbackAndRejectsPublicHTTP(t *testing.T) {
	base := Manifest{
		SchemaVersion:  SchemaVersion,
		AppID:          "550e8400-e29b-41d4-a716-446655440000",
		HostingMode:    "EXTERNAL",
		Environment:    "TEST",
		PublishableKey: "app_pk_test_abcdefghijklmnopqrstuvwxyz123456",
		Capabilities:   map[string]Capability{},
	}

	base.Origin = "http://127.0.0.2:3000"
	if _, err := Save(t.TempDir(), base); err != nil {
		t.Fatalf("canonical loopback origin rejected: %v", err)
	}
	base.Origin = "http://example.com"
	if _, err := Save(t.TempDir(), base); err == nil {
		t.Fatal("public HTTP origin was accepted")
	}
}

func TestValidateAcceptsCanonicalAndLegacySDKPackages(t *testing.T) {
	base := Manifest{
		SchemaVersion:  SchemaVersion,
		AppID:          "550e8400-e29b-41d4-a716-446655440000",
		HostingMode:    "VICEME_HOSTED",
		Environment:    "TEST",
		PublishableKey: "app_pk_test_abcdefghijklmnopqrstuvwxyz123456",
		Capabilities:   map[string]Capability{},
	}
	// The canonical @viceme-ai/* namespace must be accepted for both web-sdk
	// (EXTERNAL apps) and app-sdk (VICEME_HOSTED managed apps).
	for _, pkg := range []string{"@viceme-ai/web-sdk", "@viceme-ai/app-sdk"} {
		m := base
		m.Capabilities = map[string]Capability{
			"runtime": {ContractVersion: "1.0.0", SDKPackage: pkg, SDKVersion: "0.1.0"},
		}
		if _, err := Save(t.TempDir(), m); err != nil {
			t.Fatalf("canonical SDK package %s rejected: %v", pkg, err)
		}
	}
	// Legacy @viceme/* names are accepted for backward compatibility.
	for _, pkg := range []string{"@viceme/web-sdk", "@viceme/app-sdk"} {
		m := base
		m.Capabilities = map[string]Capability{
			"runtime": {ContractVersion: "1.0.0", SDKPackage: pkg, SDKVersion: "0.1.0"},
		}
		if _, err := Save(t.TempDir(), m); err != nil {
			t.Fatalf("legacy SDK package %s rejected: %v", pkg, err)
		}
	}
	// An unknown package name must be rejected.
	m := base
	m.Capabilities = map[string]Capability{
		"runtime": {ContractVersion: "1.0.0", SDKPackage: "@evil/sdk", SDKVersion: "0.1.0"},
	}
	if _, err := Save(t.TempDir(), m); err == nil {
		t.Fatal("unknown SDK package was accepted")
	}
}
