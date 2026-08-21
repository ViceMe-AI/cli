package command

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
)

func TestWebsiteBindingKeepsStableSourceIdentity(t *testing.T) {
	filename := filepath.Join(t.TempDir(), ".viceme", "website.json")
	want := websiteBinding{
		SchemaVersion: 1,
		ClientWorkID:  "11111111-1111-4111-8111-111111111111",
		ProfileID:     "profile-a",
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
		ProfileID:     "profile-a",
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

func TestWebsitePublishRejectsAnotherProfileBeforeMutation(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, ".viceme", "website.json")
	if err := writeWebsiteBinding(filename, websiteBinding{
		SchemaVersion: 1,
		ClientWorkID:  "11111111-1111-4111-8111-111111111111",
		ProfileID:     "profile-a",
		WorkID:        "22222222-2222-4222-8222-222222222222",
		WorkKey:       "wrk_stable_work_01",
		Region:        "cn",
		DisplayName:   "Dagou Tap",
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = publishWebsite(context.Background(), &Runtime{
		profile: config.Profile{ID: "profile-b"},
		region:  config.RegionCN,
	}, publishWebsiteInput{SourcePath: root, DisplayName: "Dagou Tap"})
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "WEBSITE_PROFILE_MISMATCH" {
		t.Fatalf("publishWebsite() error = %#v", err)
	}
	after, readErr := os.ReadFile(filename)
	if readErr != nil || string(after) != string(before) {
		t.Fatalf("profile mismatch modified binding: err=%v before=%q after=%q", readErr, before, after)
	}
}

func TestWebsitePublishMigratesVerifiedBeta6Binding(t *testing.T) {
	const clientWorkID = "11111111-1111-4111-8111-111111111111"
	const workID = "22222222-2222-4222-8222-222222222222"
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		publication := map[string]any{
			"clientWorkId": clientWorkID, "sourceDigest": strings.Repeat("a", 64), "sourceUrl": nil,
			"releaseId": "33333333-3333-4333-8333-333333333333", "version": 1,
			"publishedAt": "2026-08-21T00:00:00Z", "unchanged": true,
		}
		if request.Method == http.MethodPost {
			var input api.PublishCreatorWebsiteRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input.ClientWorkID != clientWorkID {
				t.Fatalf("clientWorkId = %q", input.ClientWorkID)
			}
			publication["sourceDigest"] = input.SourceDigest
		}
		writeJSONResponse(writer, map[string]any{
			"creatorWorkId": workID, "workKey": "wrk_stable_work_01", "displayName": "Dagou Tap",
			"status": "DRAFT", "configVersion": 1, "publication": publication,
			"offer": nil, "features": []any{}, "capabilities": []any{},
			"createdAt": "2026-08-21T00:00:00Z", "updatedAt": "2026-08-21T00:00:00Z",
		})
	}))
	defer server.Close()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("website"), 0o644); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, ".viceme", "website.json")
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{
  "schemaVersion": 1,
  "clientWorkId": "` + clientWorkID + `",
  "workId": "` + workID + `",
  "workKey": "wrk_stable_work_01",
  "region": "cn",
  "displayName": "Dagou Tap",
  "sourceUrl": ""
}
`
	if err := os.WriteFile(filename, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{
		deps:              Dependencies{HTTPClient: server.Client(), Store: securestore.NewMemory()},
		profile:           config.Profile{ID: "profile-poc", Name: "poc"},
		region:            config.RegionCN,
		apiBaseURL:        server.URL,
		processCredential: &publicationCredential{raw: "vme_cli_test"},
	}
	if _, _, err := publishWebsite(context.Background(), runtime, publishWebsiteInput{SourcePath: root, DisplayName: "Dagou Tap"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(requests, "\n"); got != "GET /v1/cli/sdk-works/wrk_stable_work_01\nPOST /v1/cli/sdk-works/publish" {
		t.Fatalf("requests = %q", got)
	}
	binding, found, err := loadWebsiteBinding(filename)
	if err != nil || !found || binding.ProfileID != "profile-poc" {
		t.Fatalf("migrated binding = %#v, found=%t, err=%v", binding, found, err)
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

func TestWebsitePublishAcceptsFirstCreatorDisplayName(t *testing.T) {
	command := newWebsitePublishCommand(&Runtime{})
	flag := command.Flags().Lookup("creator-display-name")
	if flag == nil || flag.DefValue != "" {
		t.Fatalf("creator-display-name flag = %#v", flag)
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
