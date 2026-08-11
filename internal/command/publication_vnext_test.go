package command

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestPublicationCommandRecoversCreateResponseLossAndPublishesReviewedDraft(t *testing.T) {
	t.Parallel()
	state := &publicationAPITestState{
		publicationID: "22222222-2222-4222-8222-222222222222",
		reviewDigest:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	server := httptest.NewServer(http.HandlerFunc(state.serveHTTP))
	defer server.Close()
	state.baseURL = server.URL

	root := t.TempDir()
	source := filepath.Join(root, "skill")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(`---
name: Publish Test
description: Publish a deterministic Skill through the vNext contract.
---

# Publish Test
`), 0o644); err != nil {
		t.Fatal(err)
	}
	media, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(root, "manual-cover.png")
	if err := os.WriteFile(mediaPath, media, 0o644); err != nil {
		t.Fatal(err)
	}

	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL, config.RegionCN)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dependencies := Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: store, APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Now:         func() time.Time { return time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC) },
		NewID:       func() string { return "11111111-1111-4111-8111-111111111111" },
	}
	execute := func(arguments ...string) (int, map[string]any) {
		t.Helper()
		stdout.Reset()
		stderr.Reset()
		exit := Execute(arguments, dependencies)
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("command did not emit one JSON envelope: exit=%d stdout=%q stderr=%q err=%v", exit, stdout.String(), stderr.String(), err)
		}
		return exit, envelope
	}

	if exit, envelope := execute("skill", "publish", "--path", source, "--price-minor", "1"); exit == 0 || envelope["ok"] != false {
		t.Fatalf("simulated response loss did not fail safely: exit=%d envelope=%#v", exit, envelope)
	}
	if exit, envelope := execute("skill", "publish", "--path", source, "--price-minor", "1"); exit != 0 || envelope["ok"] != true {
		t.Fatalf("publication retry did not recover: exit=%d envelope=%#v", exit, envelope)
	}
	state.mu.Lock()
	if state.createCalls != 2 || len(state.clientRequestIDs) != 2 || state.clientRequestIDs[0] != state.clientRequestIDs[1] {
		state.mu.Unlock()
		t.Fatalf("response-loss retry changed idempotency identity: %#v", state.clientRequestIDs)
	}
	state.mu.Unlock()

	if exit, envelope := execute("publication", "asset", "upload", state.publicationID, "--role", "cover", "--path", mediaPath); exit != 0 || envelope["ok"] != true {
		t.Fatalf("manual cover upload failed: exit=%d envelope=%#v", exit, envelope)
	}
	if exit, envelope := execute("publication", "asset", "upload", state.publicationID, "--role", "gallery", "--path", mediaPath); exit != 0 || envelope["ok"] != true {
		t.Fatalf("verified media reuse failed: exit=%d envelope=%#v", exit, envelope)
	}
	if exit, envelope := execute("publication", "confirm", state.publicationID, "--review-digest", state.reviewDigest); exit != 0 || envelope["ok"] != true {
		t.Fatalf("review confirmation failed: exit=%d envelope=%#v", exit, envelope)
	}
	if exit, envelope := execute("publication", "publish", state.publicationID, "--review-digest", state.reviewDigest); exit != 0 || envelope["ok"] != true {
		t.Fatalf("publish failed: exit=%d envelope=%#v", exit, envelope)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.status != "PUBLISHED" || len(state.packageBytes) == 0 || len(state.mediaBytes) == 0 || state.draft.CoverUploadID == nil || len(state.draft.GalleryUploadIDs) != 1 {
		t.Fatalf("publication lifecycle did not close: %#v", state)
	}
	if _, err := os.Stat(filepath.Join(root, "config", "publications", state.publicationID+".json")); !os.IsNotExist(err) {
		t.Fatalf("published recovery state was not removed: %v", err)
	}
}

type publicationAPITestState struct {
	mu               sync.Mutex
	baseURL          string
	publicationID    string
	reviewDigest     string
	status           string
	createCalls      int
	clientRequestIDs []string
	manifest         api.SkillPublicationManifest
	draft            api.SkillPublicationDraft
	packageBytes     []byte
	mediaBytes       []byte
	mediaDigest      string
	packageVerified  bool
	mediaVerified    bool
}

func (state *publicationAPITestState) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if request.URL.Path != "/upload/package" && request.URL.Path != "/upload/media" && request.Header.Get("Authorization") != "Bearer vme_cli_test" {
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}
	publicationPath := "/v1/creator/skill-publications/" + state.publicationID
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/creator/skill-publications":
		var input api.CreateSkillPublicationRequest
		_ = json.NewDecoder(request.Body).Decode(&input)
		state.createCalls++
		state.clientRequestIDs = append(state.clientRequestIDs, input.ClientRequestID)
		state.manifest = input.Manifest
		state.draft = api.SkillPublicationDraft{Title: input.Manifest.Metadata.Title, Summary: input.Manifest.Metadata.Summary, Currency: "CNY", PriceMinor: input.Manifest.Spec.Sale.PriceMinor, GalleryUploadIDs: []string{}}
		state.status = "DRAFT"
		if state.createCalls == 1 {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, "{")
			return
		}
		writeJSONResponse(writer, api.CreateSkillPublicationResponse{PublicationID: state.publicationID, Status: state.status, PackageUpload: &api.UploadAuthorization{UploadID: "upload-package", Method: http.MethodPut, URL: state.baseURL + "/upload/package", Headers: map[string]string{"Content-Type": "application/zip"}}})
	case request.Method == http.MethodGet && request.URL.Path == publicationPath:
		writeJSONResponse(writer, state.publication())
	case request.Method == http.MethodPut && request.URL.Path == "/upload/package":
		state.packageBytes, _ = io.ReadAll(request.Body)
		writer.WriteHeader(http.StatusNoContent)
	case request.Method == http.MethodPut && request.URL.Path == "/upload/media":
		state.mediaBytes, _ = io.ReadAll(request.Body)
		writer.WriteHeader(http.StatusNoContent)
	case request.Method == http.MethodPost && request.URL.Path == publicationPath+"/upload-authorizations":
		var input api.UploadAuthorizationRequest
		_ = json.NewDecoder(request.Body).Decode(&input)
		state.mediaDigest = input.Digest
		writeJSONResponse(writer, api.UploadAuthorization{UploadID: "upload-media", Method: http.MethodPut, URL: state.baseURL + "/upload/media", Headers: map[string]string{"Content-Type": "image/png"}})
	case request.Method == http.MethodPost && request.URL.Path == publicationPath+"/complete-upload":
		var input api.CompleteUploadRequest
		_ = json.NewDecoder(request.Body).Decode(&input)
		if input.UploadID == "upload-package" {
			state.packageVerified = true
		} else if input.UploadID == "upload-media" {
			state.mediaVerified = true
		}
		writeJSONResponse(writer, state.publication())
	case request.Method == http.MethodPatch && request.URL.Path == publicationPath+"/listing-draft":
		_ = json.NewDecoder(request.Body).Decode(&state.draft)
		state.status = "REVIEW_REQUIRED"
		writeJSONResponse(writer, state.publication())
	case request.Method == http.MethodPost && request.URL.Path == publicationPath+"/confirm-review":
		state.status = "READY"
		writeJSONResponse(writer, state.publication())
	case request.Method == http.MethodPost && request.URL.Path == publicationPath+"/publish":
		state.status = "PUBLISHED"
		writeJSONResponse(writer, state.publication())
	default:
		writer.WriteHeader(http.StatusNotFound)
	}
}

func (state *publicationAPITestState) publication() api.SkillPublication {
	uploads := []api.SkillPublicationUpload{}
	if state.packageVerified {
		uploads = append(uploads, api.SkillPublicationUpload{ID: "upload-package", Kind: "PACKAGE", Status: "VERIFIED", Digest: "package", SortOrder: 0})
	}
	if state.mediaVerified {
		uploads = append(uploads, api.SkillPublicationUpload{ID: "upload-media", Kind: "MEDIA", Status: "VERIFIED", ContentType: "image/png", Digest: state.mediaDigest, SortOrder: 0})
	}
	result := api.SkillPublication{ID: state.publicationID, Status: state.status, Manifest: state.manifest, Draft: state.draft, ReviewRevision: 1, ReviewDigest: &state.reviewDigest, Uploads: uploads}
	if state.status == "PUBLISHED" {
		result.Product = &api.PublishedProduct{ID: "33333333-3333-4333-8333-333333333333", Slug: "publish-test", DetailURL: "https://viceme.cn/zh-CN/share/publish-test", ReleaseID: "44444444-4444-4444-8444-444444444444"}
	}
	return result
}

func writeJSONResponse(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}
