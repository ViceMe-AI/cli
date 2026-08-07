package managedrelease

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	state := State{
		AppID:                 "11111111-1111-4111-8111-111111111111",
		ReleaseID:             "22222222-2222-4222-8222-222222222222",
		CandidateID:           "cand_test",
		Environment:           "TEST",
		PublishableKey:        "app_pk_test_abcdefghijklmnopqrstuvwxyz123456",
		RuntimeReleaseID:      "33333333-3333-4333-8333-333333333333",
		RuntimeContractDigest: "sha256:" + strings.Repeat("a", 64),
		TemplateName:          "image-tool-starter",
		TemplateVersion:       "1.0.0",
		TemplateDigest:        "sha256:" + strings.Repeat("b", 64),
		AppSDKVersion:         "0.1.0",
		SourceDigest:          "sha256:" + strings.Repeat("c", 64),
		BuildDigest:           "sha256:" + strings.Repeat("d", 64),
	}
	path, err := Save(dir, state)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if path != Path(dir) {
		t.Fatalf("unexpected path %q want %q", path, Path(dir))
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	// Save fills in the schema version on the caller's behalf.
	state.SchemaVersion = SchemaVersion
	if loaded != state {
		t.Fatalf("round-trip mismatch:\n got  %+v\n want %+v", loaded, state)
	}
}

func TestLoadReturnsNotFoundWhenAbsent(t *testing.T) {
	_, err := Load(t.TempDir())
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSaveRejectsMalformedDigest(t *testing.T) {
	dir := t.TempDir()
	err := os.MkdirAll(filepath.Join(dir, ".viceme"), 0o755)
	if err != nil {
		t.Fatal(err)
	}
	state := State{
		AppID:            "11111111-1111-4111-8111-111111111111",
		CandidateID:      "cand_test",
		PublishableKey:   "app_pk_test_abcdefghijklmnopqrstuvwxyz123456",
		RuntimeReleaseID: "33333333-3333-4333-8333-333333333333",
		TemplateDigest:   "not-a-digest",
	}
	if _, err := Save(dir, state); err == nil {
		t.Fatalf("Save should reject a malformed digest")
	}
}

func TestSaveRejectsMissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name  string
		state State
	}{
		{"missing appId", State{CandidateID: "c", PublishableKey: "app_pk_test_abcdefghijklmnopqrstuvwxyz123456", RuntimeReleaseID: "r"}},
		{"missing candidateId", State{AppID: "a", PublishableKey: "app_pk_test_abcdefghijklmnopqrstuvwxyz123456", RuntimeReleaseID: "r"}},
		{"missing publishableKey", State{AppID: "a", CandidateID: "c", RuntimeReleaseID: "r"}},
		{"missing runtimeReleaseId", State{AppID: "a", CandidateID: "c", PublishableKey: "app_pk_test_abcdefghijklmnopqrstuvwxyz123456"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Save(dir, tc.state); err == nil {
				t.Fatalf("Save should reject %s", tc.name)
			}
		})
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := Path(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{
  "schemaVersion": 1,
  "appId": "11111111-1111-4111-8111-111111111111",
  "releaseId": "22222222-2222-4222-8222-222222222222",
  "candidateId": "cand_test",
  "environment": "TEST",
  "publishableKey": "app_pk_test_abcdefghijklmnopqrstuvwxyz123456",
  "runtimeReleaseId": "33333333-3333-4333-8333-333333333333",
  "unexpectedField": true
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatalf("Load should reject unknown fields")
	}
}
