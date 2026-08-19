package command

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWebsiteBindingKeepsStableSourceIdentity(t *testing.T) {
	filename := filepath.Join(t.TempDir(), ".viceme", "website.json")
	want := websiteBinding{
		SchemaVersion: 1,
		ClientWorkID:  "11111111-1111-4111-8111-111111111111",
		WorkID:        "22222222-2222-4222-8222-222222222222",
		WorkKey:       "wrk_stable_work_01",
		Region:        "cn",
		DisplayName:   "Dagou Tap",
		SourceURL:     "https://creator.example.com/dagou-tap",
	}
	if err := writeWebsiteBinding(filename, want); err != nil {
		t.Fatalf("writeWebsiteBinding() error = %v", err)
	}
	got, found, err := loadWebsiteBinding(filename)
	if err != nil {
		t.Fatalf("loadWebsiteBinding() error = %v", err)
	}
	if !found || got.ClientWorkID != want.ClientWorkID || got.WorkID != want.WorkID || got.WorkKey != want.WorkKey {
		t.Fatalf("binding = %#v, want stable identity %#v", got, want)
	}
}

func TestRequirePublishedWebsiteBindingRejectsMissingWorkKey(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, ".viceme", "website.json")
	if err := writeWebsiteBinding(filename, websiteBinding{
		SchemaVersion: 1,
		ClientWorkID:  "11111111-1111-4111-8111-111111111111",
		Region:        "cn",
		DisplayName:   "Pending",
		SourceURL:     "https://creator.example.com/pending",
	}); err != nil {
		t.Fatalf("writeWebsiteBinding() error = %v", err)
	}
	if _, _, err := requirePublishedWebsiteBinding(root); err == nil {
		t.Fatal("requirePublishedWebsiteBinding() error = nil")
	}
}

func TestWebsiteURLIsOptionalButValidatedWhenProvided(t *testing.T) {
	if err := validateWebsiteURL(""); err != nil {
		t.Fatalf("validateWebsiteURL(\"\") error = %v", err)
	}
	if err := validateWebsiteURL("https://creator.example.com/tool"); err != nil {
		t.Fatalf("validateWebsiteURL(valid) error = %v", err)
	}
	if err := validateWebsiteURL("creator.example.com/tool"); err == nil {
		t.Fatal("validateWebsiteURL(relative) error = nil")
	}
}

func TestWebsiteDigestChangesWithContentButIgnoresLocalBinding(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := digestWebsiteDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".viceme"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".viceme", "website.json"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignored, err := digestWebsiteDirectory(root)
	if err != nil || ignored != first {
		t.Fatalf("binding changed digest: %q, %v", ignored, err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := digestWebsiteDirectory(root)
	if err != nil || second == first {
		t.Fatalf("content did not change digest: %q, %v", second, err)
	}
}
