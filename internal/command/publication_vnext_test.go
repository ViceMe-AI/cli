package command

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/publication"
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
name: publish-test
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
	scope, err := credentialScopeForAPIBase(server.URL)
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
		Out: &stdout, ErrOut: &stderr, Store: store, APIBaseURL: server.URL, Region: config.RegionGlobal,
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
	lastPreviewOpenURL := ""
	expectFreshPreview := func(envelope map[string]any) {
		t.Helper()
		data, _ := envelope["data"].(map[string]any)
		presentation, _ := data["presentation"].(map[string]any)
		openURL, _ := presentation["openUrl"].(string)
		if presentation["intent"] != "OPEN_OWNER_PREVIEW" || presentation["mode"] != "ONE_TIME_LAUNCH" || openURL == "" || presentation["fallbackUrl"] == "" {
			t.Fatalf("owner preview presentation was not actionable with a stable fallback: %#v", envelope)
		}
		if openURL == lastPreviewOpenURL {
			t.Fatalf("content update reused the consumed preview launch: %#v", envelope)
		}
		lastPreviewOpenURL = openURL
	}

	if exit, envelope := execute("skill", "publish", "--path", source, "--edition-key", "standard", "--edition-order", "0"); exit == 0 || envelope["ok"] != false {
		t.Fatalf("simulated response loss did not fail safely: exit=%d envelope=%#v", exit, envelope)
	}
	previewStartedAt := time.Now()
	if exit, envelope := execute("skill", "publish", "--path", source, "--edition-key", "standard", "--edition-order", "0"); exit != 0 || envelope["ok"] != true {
		t.Fatalf("private package upload retry did not recover: exit=%d envelope=%#v", exit, envelope)
	} else {
		data, _ := envelope["data"].(map[string]any)
		if data["listingId"] != "66666666-6666-4666-8666-666666666666" || data["publicationId"] != state.publicationID || data["requiresPrice"] != true {
			t.Fatalf("first business result was not the uploaded private draft: %#v", envelope)
		}
		expectFreshPreview(envelope)
	}
	if elapsed := time.Since(previewStartedAt); elapsed >= 10*time.Second {
		t.Fatalf("private package preview fast path took %s", elapsed)
	}
	state.mu.Lock()
	if state.createCalls != 2 || !state.packageVerified || state.mediaVerified || len(state.mediaBytes) != 0 || len(state.clientRequestIDs) != 2 || state.clientRequestIDs[0] != state.clientRequestIDs[1] {
		state.mu.Unlock()
		t.Fatalf("private upload did not recover one Publication intent: %#v", state.clientRequestIDs)
	}
	state.mu.Unlock()
	if _, err := os.Stat(filepath.Join(source, ".viceme", "skill.json")); err != nil {
		t.Fatalf("workspace binding was not persisted beside the source: %v", err)
	}

	if exit, envelope := execute("skill", "publish", "--resume", state.publicationID, "--price-minor", "1"); exit != 0 || envelope["ok"] != true {
		t.Fatalf("priced publication continuation failed: exit=%d envelope=%#v", exit, envelope)
	} else if data, _ := envelope["data"].(map[string]any); data["requiresPrice"] != false {
		t.Fatalf("price update was not reflected in the progressive preview: %#v", envelope)
	} else {
		expectFreshPreview(envelope)
	}
	state.mu.Lock()
	if state.createCalls != 2 {
		state.mu.Unlock()
		t.Fatalf("resume created another Publication: calls=%d", state.createCalls)
	}
	state.mu.Unlock()

	if exit, envelope := execute("publication", "asset", "upload", state.publicationID, "--role", "cover", "--path", mediaPath); exit != 0 || envelope["ok"] != true {
		t.Fatalf("manual cover upload failed: exit=%d envelope=%#v", exit, envelope)
	} else {
		expectFreshPreview(envelope)
	}
	if exit, envelope := execute("publication", "asset", "upload", state.publicationID, "--role", "gallery", "--path", mediaPath); exit != 0 || envelope["ok"] != true {
		t.Fatalf("verified media reuse failed: exit=%d envelope=%#v", exit, envelope)
	} else {
		expectFreshPreview(envelope)
	}
	suggestionPath := filepath.Join(root, "agent-suggestion.json")
	suggestion := api.SuggestSkillPublicationDraftRequest{
		BaseDraftRevision: 1,
		Patch: api.SkillPublicationAgentSuggestionPatch{
			SummaryZhCN:           "发布测试",
			UsageInstructionsZhCN: "按 SKILL.md 中的步骤运行。",
			CoverUploadID:         stringPointer("upload-media"), GalleryUploadIDs: []string{"upload-media"},
		},
	}
	suggestionJSON, err := json.Marshal(suggestion)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(suggestionPath, suggestionJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if exit, envelope := execute("publication", "suggest", state.publicationID, "--input", suggestionPath); exit != 0 || envelope["ok"] != true {
		t.Fatalf("Agent suggestion failed: exit=%d envelope=%#v", exit, envelope)
	} else {
		expectFreshPreview(envelope)
	}
	if exit, envelope := execute("publication", "review", state.publicationID); exit != 0 || envelope["ok"] != true {
		t.Fatalf("publication review failed: exit=%d envelope=%#v", exit, envelope)
	} else {
		data, _ := envelope["data"].(map[string]any)
		draft, _ := data["draft"].(map[string]any)
		if draft["summaryZhCn"] != "发布测试" || draft["usageInstructionsZhCn"] != "按 SKILL.md 中的步骤运行。" {
			t.Fatalf("review omitted listing copy: %#v", envelope)
		}
		if data["draftRevision"] != float64(1) {
			t.Fatalf("review omitted the Agent CAS revision: %#v", envelope)
		}
		expectFreshPreview(envelope)
	}
	if exit, envelope := execute("publication", "confirm", state.publicationID, "--review-digest", state.reviewDigest); exit != 0 || envelope["ok"] != true {
		t.Fatalf("review confirmation failed: exit=%d envelope=%#v", exit, envelope)
	} else {
		expectFreshPreview(envelope)
	}
	if exit, envelope := execute("publication", "publish", state.publicationID, "--review-digest", state.reviewDigest); exit != 0 || envelope["ok"] != true {
		t.Fatalf("publish failed: exit=%d envelope=%#v", exit, envelope)
	} else {
		expectFreshPreview(envelope)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.status != "PUBLISHED" || len(state.packageBytes) == 0 || len(state.mediaBytes) == 0 || state.draft.CoverUploadID == nil || len(state.draft.GalleryUploadIDs) != 1 || state.suggestionCalls != 1 || state.analysisCalls != 0 {
		t.Fatalf("publication lifecycle did not close: %#v", state)
	}
	if len(state.lastDraftPatchFields) != 1 || state.lastDraftPatchFields[0] != "galleryUploadIds" {
		t.Fatalf("asset upload rewrote unrelated user fields: %#v", state.lastDraftPatchFields)
	}
	if state.listingID != "66666666-6666-4666-8666-666666666666" {
		t.Fatalf("Publication was not attached to the prepared Listing: %q", state.listingID)
	}
	if _, err := os.Stat(filepath.Join(root, "config", "publications", state.publicationID+".json")); !os.IsNotExist(err) {
		t.Fatalf("published recovery state was not removed: %v", err)
	}
}

func TestSkillPublishBindsAdditionalEditionToExplicitListing(t *testing.T) {
	t.Parallel()
	state := &publicationAPITestState{
		publicationID: "22222222-2222-4222-8222-222222222226",
		reviewDigest:  strings.Repeat("f", 64),
	}
	server := httptest.NewServer(http.HandlerFunc(state.serveHTTP))
	defer server.Close()
	state.baseURL = server.URL

	root := t.TempDir()
	source := filepath.Join(root, "pro-skill")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(`---
name: pro-edition
description: A separate package for the same Work.
---

# Pro Edition
`), 0o644); err != nil {
		t.Fatal(err)
	}

	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	listingID := "66666666-6666-4666-8666-666666666666"
	exit := Execute([]string{
		"skill", "publish", "--path", source,
		"--listing", listingID,
		"--edition-key", "pro",
		"--edition-title", "Pro",
		"--edition-order", "1",
		"--edition-highlight", "Advanced workflow",
	}, Dependencies{
		Out: &stdout, ErrOut: io.Discard, Store: store, APIBaseURL: server.URL, Region: config.RegionGlobal,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Now:         func() time.Time { return time.Date(2026, 8, 27, 8, 0, 0, 0, time.UTC) },
		NewID:       func() string { return "11111111-1111-4111-8111-111111111111" },
	})
	if exit == 0 {
		t.Fatalf("the response-loss fixture unexpectedly completed: %s", stdout.String())
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.prepareResolution == nil || state.prepareResolution["mode"] != "BIND_EXISTING" || state.prepareResolution["listingId"] != listingID {
		t.Fatalf("additional edition did not bind the requested Listing: %#v", state.prepareResolution)
	}
}

func TestXiaohongshuSearchRequiresExplicitSelectionForMultipleMatches(t *testing.T) {
	const merchantID = "99999999-9999-4999-8999-999999999999"
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	t.Setenv(processAccessTokenEnvironment, accessToken)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "55555555-5555-4555-8555-555555555555", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        []string{"profile:read", "skill-publication:read", "skill-publication:write"},
				"expiresAt":     "2027-08-27T00:00:00Z",
			})
		case "/v1/cli/merchant/accounts":
			writeJSONResponse(writer, api.MerchantAccountsResponse{Items: []api.MerchantAccount{{ID: merchantID, DisplayName: "Creator", Status: "ACTIVE", StatusVersion: 1}}})
		case "/v1/cli/skill-sources/xiaohongshu/search":
			writeJSONResponse(writer, map[string]any{"items": []any{
				map[string]any{"skillId": "xhs-a", "skillName": "Poster A", "authorDisplayName": "Creator", "artifactVersion": "v1", "artifactDigest": strings.Repeat("a", 64), "artifactSizeBytes": 100, "observedAt": "2026-08-27T00:00:00Z"},
				map[string]any{"skillId": "xhs-b", "skillName": "Poster B", "authorDisplayName": "Creator", "artifactVersion": "v1", "artifactDigest": strings.Repeat("b", 64), "artifactSizeBytes": 200, "observedAt": "2026-08-27T00:00:00Z"},
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	var stdout bytes.Buffer
	exit := Execute([]string{"skill", "publish", "--xiaohongshu-search", "Poster", "--merchant", merchantID, "--edition-key", "standard", "--edition-order", "0"}, Dependencies{
		Out: &stdout, ErrOut: io.Discard, APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()},
	})
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if exit == 0 || envelope["ok"] != false {
		t.Fatalf("ambiguous Xiaohongshu search unexpectedly continued: exit=%d envelope=%#v", exit, envelope)
	}
	errorBody, _ := envelope["error"].(map[string]any)
	details, _ := errorBody["details"].(map[string]any)
	candidates, _ := details["candidates"].([]any)
	if errorBody["type"] != "confirmation" || errorBody["code"] != "XIAOHONGSHU_SKILL_SELECTION_REQUIRED" || len(candidates) != 2 {
		t.Fatalf("ambiguous Xiaohongshu search did not expose candidates: %#v", envelope)
	}
}

func TestSkillPublishResumeWithoutPriceUploadsMediaWithoutStartingPlatformAnalysis(t *testing.T) {
	t.Parallel()
	state := &publicationAPITestState{
		publicationID: "22222222-2222-4222-8222-222222222223",
		reviewDigest:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
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
name: progressive-publish-test
description: Verify unpriced media and analysis continuation.
---

# Progressive Publish Test
`), 0o644); err != nil {
		t.Fatal(err)
	}
	media, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "preview.png"), media, 0o644); err != nil {
		t.Fatal(err)
	}

	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
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
		Now:         func() time.Time { return time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC) },
		NewID:       func() string { return "11111111-1111-4111-8111-111111111112" },
	}
	execute := func(arguments ...string) (int, map[string]any) {
		t.Helper()
		stdout.Reset()
		stderr.Reset()
		exit := Execute(arguments, dependencies)
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("command did not emit JSON: exit=%d stdout=%q stderr=%q err=%v", exit, stdout.String(), stderr.String(), err)
		}
		return exit, envelope
	}

	if exit, _ := execute("skill", "publish", "--path", source, "--edition-key", "standard", "--edition-order", "0"); exit == 0 {
		t.Fatal("simulated create response loss unexpectedly succeeded")
	}
	if exit, envelope := execute("skill", "publish", "--path", source, "--edition-key", "standard", "--edition-order", "0"); exit != 0 || envelope["ok"] != true {
		t.Fatalf("private preview recovery failed: exit=%d envelope=%#v", exit, envelope)
	}
	state.mu.Lock()
	if !state.packageVerified || state.mediaVerified || state.analysisCalls != 0 {
		state.mu.Unlock()
		t.Fatalf("first preview did not stop after the verified package: %#v", state)
	}
	state.mu.Unlock()

	if exit, envelope := execute("skill", "publish", "--resume", state.publicationID); exit != 0 || envelope["ok"] != true {
		t.Fatalf("unpriced continuation failed: exit=%d envelope=%#v", exit, envelope)
	} else if data, _ := envelope["data"].(map[string]any); data["requiresPrice"] != true {
		t.Fatalf("unpriced continuation lost its Draft completeness signal: %#v", envelope)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.mediaVerified || len(state.mediaBytes) == 0 || state.analysisCalls != 0 || state.draft.PriceMinor != nil {
		t.Fatalf("unpriced continuation did not stop after deterministic media upload: %#v", state)
	}
}

func TestPublicationAnalyzeIsAnExplicitPlatformFallback(t *testing.T) {
	t.Parallel()
	state := &publicationAPITestState{
		publicationID: "22222222-2222-4222-8222-222222222224",
		status:        "DRAFT",
	}
	server := httptest.NewServer(http.HandlerFunc(state.serveHTTP))
	defer server.Close()
	state.baseURL = server.URL
	root := t.TempDir()
	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	exit := Execute([]string{"publication", "analyze", state.publicationID}, Dependencies{
		Out: &stdout, ErrOut: io.Discard, Store: store, APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit != 0 {
		t.Fatalf("explicit analysis fallback failed: exit=%d stdout=%s", exit, stdout.String())
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.analysisCalls != 1 {
		t.Fatalf("explicit analysis fallback was not requested exactly once: %#v", state)
	}
}

func TestSkillPublishValidatesLocallyBeforeLoginWithoutCreatingRecoveryState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	missingSource := filepath.Join(root, "does-not-exist")
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requestCount++
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"skill", "publish", "--path", missingSource, "--edition-key", "standard", "--edition-order", "0", "--price-minor", "1"}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit != output.ExitValidation {
		t.Fatalf("local validation used the wrong exit class: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	errorData, _ := envelope["error"].(map[string]any)
	if errorData["code"] != "SKILL_PATH_NOT_FOUND" {
		t.Fatalf("publication did not report the local source problem first: %#v", envelope)
	}
	if requestCount != 0 {
		t.Fatalf("unauthenticated publication called the API before login: requests=%d", requestCount)
	}
	if _, err := os.Stat(filepath.Join(root, "config", "publications")); !os.IsNotExist(err) {
		t.Fatalf("unauthenticated publication created recovery state before login: %v", err)
	}
}

func TestSkillPublishRequiresExplicitMerchantWhenMultipleAreActive(t *testing.T) {
	t.Parallel()
	state := &publicationAPITestState{merchantAccounts: []api.MerchantAccount{
		{ID: "99999999-9999-4999-8999-999999999999", DisplayName: "Merchant One", Status: "ACTIVE", StatusVersion: 1},
		{ID: "88888888-8888-4888-8888-888888888888", DisplayName: "Merchant Two", Status: "ACTIVE", StatusVersion: 1},
	}}
	server := httptest.NewServer(http.HandlerFunc(state.serveHTTP))
	defer server.Close()
	state.baseURL = server.URL
	root := t.TempDir()
	source := filepath.Join(root, "skill")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: merchant-select\ndescription: Require an explicit Merchant selection.\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	exit := Execute([]string{"skill", "publish", "--path", source, "--edition-key", "standard", "--edition-order", "0"}, Dependencies{
		Out: &stdout, ErrOut: io.Discard, Store: store, APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit != output.ExitValidation {
		t.Fatalf("multiple Merchants used the wrong exit: exit=%d stdout=%s", exit, stdout.String())
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "MERCHANT_SELECTION_REQUIRED" {
		t.Fatalf("multiple Merchants were not surfaced for explicit selection: %#v", envelope)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.createCalls != 0 || state.listingID != "" {
		t.Fatalf("publication started before Merchant selection: %#v", state)
	}
}

func TestSkillPrepareReturnsOwnedCandidatesForAmbiguousDigest(t *testing.T) {
	t.Parallel()
	state := &publicationAPITestState{ambiguousPrepare: true}
	server := httptest.NewServer(http.HandlerFunc(state.serveHTTP))
	defer server.Close()
	state.baseURL = server.URL
	root := t.TempDir()
	source := filepath.Join(root, "skill")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(`---
name: candidate-test
description: Resolve an ambiguous stable Skill listing identity.
---

# Candidate Test
`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	exit := Execute([]string{"skill", "listing", "prepare", "--path", source}, Dependencies{
		Out: &stdout, ErrOut: io.Discard, Store: store, APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		NewID:       func() string { return "11111111-1111-4111-8111-111111111111" },
	})
	if exit != output.ExitValidation {
		t.Fatalf("ambiguous prepare used the wrong exit: exit=%d stdout=%s", exit, stdout.String())
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				Candidates []api.SkillListingCandidate `json:"candidates"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "SKILL_LISTING_SOURCE_AMBIGUOUS" || len(envelope.Error.Details.Candidates) != 2 {
		t.Fatalf("ambiguous candidates were not returned: %#v", envelope)
	}
}

func TestPublicationWaitKeepsPollingWithoutAnotherUserConfirmation(t *testing.T) {
	t.Parallel()
	state := &publicationAPITestState{
		publicationID: "22222222-2222-4222-8222-222222222222",
		status:        "REVIEW_REQUIRED",
		analysisPolls: 2,
	}
	server := httptest.NewServer(http.HandlerFunc(state.serveHTTP))
	defer server.Close()
	state.baseURL = server.URL
	root := t.TempDir()
	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	sleeps := 0
	exit := Execute([]string{"publication", "wait", state.publicationID, "--timeout", "1m", "--interval", "1s"}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: store, APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Sleep:       func(context.Context, time.Duration) error { sleeps++; return nil },
	})
	if exit != 0 {
		t.Fatalf("wait failed: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data, _ := envelope["data"].(map[string]any)
	analysis, _ := data["analysis"].(map[string]any)
	if analysis["status"] != "SUCCEEDED" || sleeps != 2 {
		t.Fatalf("wait did not converge automatically: sleeps=%d envelope=%#v", sleeps, envelope)
	}
}

func TestPublicationWaitTimeoutPreservesThePublicationIdentity(t *testing.T) {
	t.Parallel()
	state := &publicationAPITestState{
		publicationID: "22222222-2222-4222-8222-222222222222",
		status:        "REVIEW_REQUIRED",
		analysisPolls: 100,
	}
	server := httptest.NewServer(http.HandlerFunc(state.serveHTTP))
	defer server.Close()
	state.baseURL = server.URL
	root := t.TempDir()
	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"publication", "wait", state.publicationID, "--timeout", "1ms", "--interval", "1s"}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: store, APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Sleep:       func(ctx context.Context, _ time.Duration) error { <-ctx.Done(); return ctx.Err() },
	})
	if exit != output.ExitNetwork {
		t.Fatalf("wait timeout used the wrong exit class: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	errorData, _ := envelope["error"].(map[string]any)
	details, _ := errorData["details"].(map[string]any)
	if errorData["code"] != "PUBLICATION_ANALYSIS_WAIT_TIMEOUT" || errorData["retryable"] != true || details["publicationId"] != state.publicationID {
		t.Fatalf("timeout did not preserve safe resume identity: %#v", envelope)
	}
}

func TestPublicationAssetUploadRecoversWithoutBurningMediaSlots(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name                 string
		putFailures          int
		loseCompleteResponse bool
		candidateOnly        bool
	}{
		{name: "expired upload authorization", putFailures: 1},
		{name: "lost completion response", loseCompleteResponse: true},
		{name: "Agent candidate without user selection", putFailures: 1, candidateOnly: true},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			state := &publicationAPITestState{
				publicationID:        "22222222-2222-4222-8222-222222222222",
				reviewDigest:         "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				status:               "DRAFT",
				mediaPutFailures:     scenario.putFailures,
				loseCompleteResponse: scenario.loseCompleteResponse,
				draft: api.SkillPublicationDraft{
					Title: "Publish Test", SummaryZhCN: stringPointer("发布测试"), UsageInstructionsZhCN: stringPointer("按 SKILL.md 中的步骤运行。"),
					Currency: "CNY", PriceMinor: intPointer(1), GalleryUploadIDs: []string{},
				},
			}
			server := httptest.NewServer(http.HandlerFunc(state.serveHTTP))
			defer server.Close()
			state.baseURL = server.URL

			root := t.TempDir()
			media, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
			if err != nil {
				t.Fatal(err)
			}
			mediaPath := filepath.Join(root, "manual-cover.png")
			if err := os.WriteFile(mediaPath, media, 0o644); err != nil {
				t.Fatal(err)
			}
			store := securestore.NewMemory()
			scope, err := credentialScopeForAPIBase(server.URL)
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
			}
			execute := func() (int, map[string]any) {
				t.Helper()
				stdout.Reset()
				stderr.Reset()
				arguments := []string{"publication", "asset", "upload", state.publicationID, "--role", "cover", "--path", mediaPath}
				if scenario.candidateOnly {
					arguments = append(arguments, "--candidate-only")
				}
				exit := Execute(arguments, dependencies)
				var envelope map[string]any
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatalf("command did not emit one JSON envelope: exit=%d stdout=%q stderr=%q err=%v", exit, stdout.String(), stderr.String(), err)
				}
				return exit, envelope
			}

			if exit, envelope := execute(); exit == 0 || envelope["ok"] != false {
				t.Fatalf("simulated upload interruption did not fail: exit=%d envelope=%#v", exit, envelope)
			}
			if exit, envelope := execute(); exit != 0 || envelope["ok"] != true {
				t.Fatalf("upload retry did not recover: exit=%d envelope=%#v", exit, envelope)
			}
			state.mu.Lock()
			defer state.mu.Unlock()
			if state.mediaSortOrder != 0 || state.uploadAuthorizationCalls > 2 {
				t.Fatalf("recovery burned another media slot: sort=%d authorizations=%d", state.mediaSortOrder, state.uploadAuthorizationCalls)
			}
			if scenario.loseCompleteResponse && state.uploadAuthorizationCalls != 1 {
				t.Fatalf("verified upload should be reused without a new authorization: %d", state.uploadAuthorizationCalls)
			}
			if scenario.candidateOnly {
				if state.draft.CoverUploadID != nil || len(state.lastDraftPatchFields) != 0 {
					t.Fatalf("Agent candidate was incorrectly recorded as a user selection: draft=%#v fields=%#v", state.draft, state.lastDraftPatchFields)
				}
			} else if state.draft.CoverUploadID == nil || *state.draft.CoverUploadID != "upload-media" {
				t.Fatalf("recovered upload was not selected as cover: %#v", state.draft)
			}
		})
	}
}

func TestTerminalPublicationRetirementFailureIsRecoverable(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	store := publication.PendingStore{Directory: directory, Now: time.Now}
	fingerprint := strings.Repeat("a", 64)
	clientRequestID := "11111111-1111-4111-8111-111111111111"
	publicationID := "22222222-2222-4222-8222-222222222222"
	intent, err := store.LoadOrCreateIntent(fingerprint, func() string { return clientRequestID })
	if err != nil {
		t.Fatal(err)
	}
	intent.PublicationID = publicationID
	if err := store.SaveIntent(intent); err != nil {
		t.Fatal(err)
	}
	pending := publication.Pending{
		PublicationID: publicationID, ClientRequestID: clientRequestID, Fingerprint: fingerprint,
		MerchantAccountID: "99999999-9999-4999-8999-999999999999",
		SourcePath:        "/tmp/source", PriceMinor: intPointer(1), ArtifactDigest: strings.Repeat("b", 64),
	}
	if err := store.Save(pending); err != nil {
		t.Fatal(err)
	}

	intentPath := filepath.Join(directory, "intent-"+fingerprint+".json")
	if err := os.Remove(intentPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(intentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{deps: Dependencies{Out: io.Discard, ErrOut: &bytes.Buffer{}}}
	warnings := retirePublicationRecovery(runtime, store, pending, "PUBLISHED")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "PUBLICATION_RECOVERY_RETIRE_FAILED") {
		t.Fatalf("terminal cleanup failure was hidden: %#v", warnings)
	}
	if _, err := os.Stat(filepath.Join(directory, publicationID+".json")); err != nil {
		t.Fatalf("pending recovery was not retained for retry: %v", err)
	}

	if err := os.Remove(intentPath); err != nil {
		t.Fatal(err)
	}
	intent, err = store.LoadOrCreateIntent(fingerprint, func() string { return clientRequestID })
	if err != nil {
		t.Fatal(err)
	}
	intent.PublicationID = publicationID
	if err := store.SaveIntent(intent); err != nil {
		t.Fatal(err)
	}
	if warnings := retirePublicationRecovery(runtime, store, pending, "PUBLISHED"); len(warnings) != 0 {
		t.Fatalf("terminal cleanup retry did not converge: %#v", warnings)
	}
	if _, err := os.Stat(filepath.Join(directory, publicationID+".json")); !os.IsNotExist(err) {
		t.Fatalf("pending recovery was not retired: %v", err)
	}
	next, err := store.LoadOrCreateIntent(fingerprint, func() string { return "33333333-3333-4333-8333-333333333333" })
	if err != nil {
		t.Fatal(err)
	}
	if next.PublicationID != "" || next.ClientRequestID == clientRequestID {
		t.Fatalf("terminal intent still trapped a future publication: %#v", next)
	}
}

type publicationAPITestState struct {
	mu                       sync.Mutex
	baseURL                  string
	publicationID            string
	reviewDigest             string
	status                   string
	createCalls              int
	clientRequestIDs         []string
	listingID                string
	manifest                 api.SkillPublicationManifest
	draft                    api.SkillPublicationDraft
	packageBytes             []byte
	packageDigest            string
	mediaBytes               []byte
	mediaDigest              string
	packageVerified          bool
	mediaVerified            bool
	mediaPending             bool
	mediaFileName            string
	mediaContentType         string
	mediaSizeBytes           int64
	mediaSortOrder           int
	mediaPutFailures         int
	loseCompleteResponse     bool
	uploadAuthorizationCalls int
	authorizedSortOrders     []int
	slotConflictOnFirst      bool
	rivalVisible             bool
	occupiedMediaSlot        int
	previewLaunchCalls       int
	analysisPolls            int
	analysisCalls            int
	suggestionCalls          int
	lastDraftPatchFields     []string
	ambiguousPrepare         bool
	prepareResolution        map[string]any
	merchantAccounts         []api.MerchantAccount
}

func (state *publicationAPITestState) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if request.URL.Path != "/upload/package" && request.URL.Path != "/upload/media" && request.Header.Get("Authorization") != "Bearer vme_cli_test" {
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}
	publicationPath := "/v1/creator/skill-publications/" + state.publicationID
	listingID := "66666666-6666-4666-8666-666666666666"
	merchantAccountID := "99999999-9999-4999-8999-999999999999"
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/auth/status":
		writeJSONResponse(writer, map[string]any{
			"authenticated": true,
			"user":          map[string]any{"id": "55555555-5555-4555-8555-555555555555", "displayName": "Creator", "avatarUrl": nil},
			"scopes":        []string{"profile:read", "skill-publication:read", "skill-publication:write"},
			"expiresAt":     "2027-08-12T08:00:00Z",
		})
	case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/merchant/accounts":
		items := state.merchantAccounts
		if items == nil {
			items = []api.MerchantAccount{{ID: merchantAccountID, DisplayName: "Test Merchant", Status: "ACTIVE", StatusVersion: 1}}
		}
		writeJSONResponse(writer, api.MerchantAccountsResponse{Items: items})
	case request.Method == http.MethodPost && request.URL.Path == "/v1/creator/skill-listings/prepare":
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, clientSelectedMarket := input["market"]; clientSelectedMarket {
			writer.WriteHeader(http.StatusBadRequest)
			writeJSONResponse(writer, map[string]any{"statusCode": 400, "code": "CLIENT_MARKET_FORBIDDEN", "message": "market is owned by the API endpoint"})
			return
		}
		if resolution, ok := input["resolution"].(map[string]any); ok {
			state.prepareResolution = resolution
		}
		if state.ambiguousPrepare {
			writer.WriteHeader(http.StatusConflict)
			writeJSONResponse(writer, map[string]any{"statusCode": 409, "code": "SKILL_LISTING_SOURCE_AMBIGUOUS", "message": "Multiple listings match this package digest"})
			return
		}
		writeJSONResponse(writer, api.PrepareSkillListingResponse{
			ListingID: listingID, Market: "CN", Status: "DRAFT", DraftRevision: 1,
			OwnerPreviewURL: state.baseURL + "/creator/skills/" + listingID + "/preview",
			BindingReceipt:  "binding-receipt", Resolution: "CREATED",
			Preview:     api.SkillListingPreviewViewModel{SchemaVersion: "preview.viceme.ai/v1", ListingID: listingID, DraftRevision: 1, State: "SHELL", FallbackURL: state.baseURL + "/preview"},
			NextActions: []string{"OPEN_PREVIEW", "SET_PRICE", "AUTHORIZE_UPLOAD"},
		})
	case request.Method == http.MethodPost && request.URL.Path == "/v1/creator/skill-listings/candidates":
		writeJSONResponse(writer, api.SkillListingCandidatesResponse{Candidates: []api.SkillListingCandidate{
			{ListingID: "77777777-7777-4777-8777-777777777777", UpdatedAt: "2026-08-14T08:00:00Z", OwnerPreviewURL: state.baseURL + "/preview/one"},
			{ListingID: "88888888-8888-4888-8888-888888888888", UpdatedAt: "2026-08-14T07:00:00Z", OwnerPreviewURL: state.baseURL + "/preview/two"},
		}})
	case request.Method == http.MethodPost && request.URL.Path == "/v1/creator/skill-listings/"+listingID+"/preview-launch":
		state.previewLaunchCalls++
		writeJSONResponse(writer, api.CreateSkillPreviewLaunchResponse{
			LaunchURL: state.baseURL + fmt.Sprintf("/v1/creator/skill-preview-launches/one-time-code-%d", state.previewLaunchCalls),
			ExpiresAt: "2026-08-14T08:01:00Z",
		})
	case request.Method == http.MethodGet && request.URL.Path == "/v1/creator/skill-listings/"+listingID+"/preview":
		writeJSONResponse(writer, api.SkillListingPreview{ListingID: listingID, Status: "DRAFT", DraftRevision: 1, Publication: &api.SkillListingPublicationPreview{ID: state.publicationID, Status: state.status}, Preview: api.SkillListingPreviewViewModel{SchemaVersion: "preview.viceme.ai/v1", ListingID: listingID, DraftRevision: 1, State: "SHELL", FallbackURL: state.baseURL + "/preview"}})
	case request.Method == http.MethodPost && request.URL.Path == "/v1/creator/skill-publications":
		var input api.CreateSkillPublicationRequest
		_ = json.NewDecoder(request.Body).Decode(&input)
		if input.ContractVersion != "2026-08-27" || input.MerchantAccountID != merchantAccountID || input.Manifest.Spec.Sale.PriceMinor != nil || input.Manifest.Spec.PublishMode != "DOWNLOADABLE_SKILL" || input.Manifest.Spec.Edition.Key == "" {
			writer.WriteHeader(http.StatusBadRequest)
			writeJSONResponse(writer, map[string]any{"statusCode": 400, "code": "PUBLICATION_CONTRACT_INVALID", "message": "expected a Merchant-bound unpriced downloadable edition"})
			return
		}
		state.createCalls++
		state.clientRequestIDs = append(state.clientRequestIDs, input.ClientRequestID)
		state.listingID = input.ListingID
		state.manifest = input.Manifest
		state.packageDigest = input.Artifact.Digest
		state.draft = api.SkillPublicationDraft{
			Title: input.Manifest.Metadata.Title, SummaryZhCN: stringPointer("发布测试"), UsageInstructionsZhCN: stringPointer("按 SKILL.md 中的步骤运行。"),
			Currency: "CNY", PriceMinor: input.Manifest.Spec.Sale.PriceMinor, GalleryUploadIDs: []string{},
		}
		state.status = "DRAFT"
		if state.createCalls == 1 {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, "{")
			return
		}
		writeJSONResponse(writer, api.CreateSkillPublicationResponse{PublicationID: state.publicationID, ListingID: listingID, MerchantAccountID: merchantAccountID, DraftRevision: 1, Status: state.status, PackageUpload: &api.UploadAuthorization{UploadID: "upload-package", Method: http.MethodPut, URL: state.baseURL + "/upload/package", Headers: map[string]string{"Content-Type": "application/zip"}}})
	case request.Method == http.MethodGet && request.URL.Path == publicationPath:
		writeJSONResponse(writer, state.publication())
	case request.Method == http.MethodPut && request.URL.Path == "/upload/package":
		state.packageBytes, _ = io.ReadAll(request.Body)
		writer.WriteHeader(http.StatusNoContent)
	case request.Method == http.MethodPut && request.URL.Path == "/upload/media":
		state.mediaBytes, _ = io.ReadAll(request.Body)
		if state.mediaPutFailures > 0 {
			state.mediaPutFailures--
			writer.WriteHeader(http.StatusForbidden)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	case request.Method == http.MethodPost && request.URL.Path == publicationPath+"/upload-authorizations":
		var input api.UploadAuthorizationRequest
		_ = json.NewDecoder(request.Body).Decode(&input)
		state.uploadAuthorizationCalls++
		state.authorizedSortOrders = append(state.authorizedSortOrders, input.SortOrder)
		if state.slotConflictOnFirst && !state.rivalVisible && input.SortOrder == state.occupiedMediaSlot {
			state.rivalVisible = true
			writer.WriteHeader(http.StatusConflict)
			_, _ = writer.Write([]byte(`{"statusCode":409,"code":"SKILL_PUBLICATION_UPLOAD_SLOT_CONFLICT","message":"The requested media upload slot already contains another file"}`))
			return
		}
		state.mediaDigest = input.Digest
		state.mediaFileName = input.FileName
		state.mediaContentType = input.ContentType
		state.mediaSizeBytes = input.SizeBytes
		state.mediaSortOrder = input.SortOrder
		state.mediaPending = true
		writeJSONResponse(writer, api.UploadAuthorization{UploadID: "upload-media", Method: http.MethodPut, URL: state.baseURL + "/upload/media", Headers: map[string]string{"Content-Type": "image/png"}})
	case request.Method == http.MethodPost && request.URL.Path == publicationPath+"/complete-upload":
		var input api.CompleteUploadRequest
		_ = json.NewDecoder(request.Body).Decode(&input)
		if input.UploadID == "upload-package" {
			state.packageVerified = true
		} else if input.UploadID == "upload-media" {
			state.mediaVerified = true
			state.mediaPending = false
		}
		if input.UploadID == "upload-media" && state.loseCompleteResponse {
			state.loseCompleteResponse = false
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, "{")
			return
		}
		writeJSONResponse(writer, state.publication())
	case request.Method == http.MethodPatch && request.URL.Path == publicationPath+"/listing-draft":
		var raw map[string]json.RawMessage
		_ = json.NewDecoder(request.Body).Decode(&raw)
		state.lastDraftPatchFields = state.lastDraftPatchFields[:0]
		for field := range raw {
			state.lastDraftPatchFields = append(state.lastDraftPatchFields, field)
		}
		if len(raw) == 1 && raw["priceMinor"] != nil {
			var price int
			_ = json.Unmarshal(raw["priceMinor"], &price)
			state.draft.PriceMinor = &price
		} else {
			encoded, _ := json.Marshal(raw)
			_ = json.Unmarshal(encoded, &state.draft)
		}
		state.status = "REVIEW_REQUIRED"
		writeJSONResponse(writer, state.publication())
	case request.Method == http.MethodPatch && request.URL.Path == publicationPath+"/listing-suggestion":
		var input api.SuggestSkillPublicationDraftRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil || input.BaseDraftRevision != 1 {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		state.suggestionCalls++
		state.draft.SummaryZhCN = &input.Patch.SummaryZhCN
		state.draft.UsageInstructionsZhCN = &input.Patch.UsageInstructionsZhCN
		state.draft.CoverUploadID = input.Patch.CoverUploadID
		state.draft.GalleryUploadIDs = input.Patch.GalleryUploadIDs
		state.status = "REVIEW_REQUIRED"
		writeJSONResponse(writer, state.publication())
	case request.Method == http.MethodPost && request.URL.Path == publicationPath+"/analyze-listing":
		state.analysisCalls++
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
		uploads = append(uploads, api.SkillPublicationUpload{ID: "upload-package", Kind: "PACKAGE", Status: "VERIFIED", Digest: state.packageDigest, SortOrder: 0})
	}
	if state.rivalVisible {
		// 模拟并发对手的媒体写入:首次 GET 时尚不可见,撞槽后的重读才出现。
		uploads = append(uploads, api.SkillPublicationUpload{ID: "upload-rival", Kind: "MEDIA", Status: "VERIFIED", FileName: "rival.png", ContentType: "image/png", SizeBytes: 1, Digest: strings.Repeat("c", 64), SortOrder: state.occupiedMediaSlot})
	}
	if state.mediaVerified {
		uploads = append(uploads, api.SkillPublicationUpload{ID: "upload-media", Kind: "MEDIA", Status: "VERIFIED", FileName: state.mediaFileName, ContentType: state.mediaContentType, SizeBytes: state.mediaSizeBytes, Digest: state.mediaDigest, SortOrder: state.mediaSortOrder})
	} else if state.mediaPending {
		uploads = append(uploads, api.SkillPublicationUpload{ID: "upload-media", Kind: "MEDIA", Status: "PENDING", FileName: state.mediaFileName, ContentType: state.mediaContentType, SizeBytes: state.mediaSizeBytes, Digest: state.mediaDigest, SortOrder: state.mediaSortOrder})
	}
	result := api.SkillPublication{ID: state.publicationID, ListingID: "66666666-6666-4666-8666-666666666666", MerchantAccountID: "99999999-9999-4999-8999-999999999999", DraftRevision: 1, Status: state.status, Manifest: state.manifest, Draft: state.draft, ReviewRevision: 1, ReviewDigest: &state.reviewDigest, Uploads: uploads}
	if state.analysisPolls > 0 {
		state.analysisPolls--
		result.Analysis = &api.PublicationAnalysis{Status: "PENDING"}
	} else if state.status == "REVIEW_REQUIRED" {
		result.Analysis = &api.PublicationAnalysis{Status: "SUCCEEDED"}
	}
	if state.status == "PUBLISHED" {
		result.Product = &api.PublishedProduct{ID: "33333333-3333-4333-8333-333333333333", Slug: "publish-test", DetailURL: "https://viceme.cn/zh-CN/share/publish-test", ReleaseID: "44444444-4444-4444-8444-444444444444"}
	}
	return result
}

func writeJSONResponse(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func stringPointer(value string) *string {
	return &value
}

func intPointer(value int) *int {
	return &value
}

func TestPublicationAssetUploadRetriesSlotConflictWithFreshSlot(t *testing.T) {
	t.Parallel()
	state := &publicationAPITestState{
		publicationID:       "22222222-2222-4222-8222-222222222222",
		reviewDigest:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		status:              "DRAFT",
		slotConflictOnFirst: true,
		occupiedMediaSlot:   0,
	}
	server := httptest.NewServer(http.HandlerFunc(state.serveHTTP))
	defer server.Close()
	state.baseURL = server.URL

	root := t.TempDir()
	media, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(root, "cover.png")
	if err := os.WriteFile(mediaPath, media, 0o644); err != nil {
		t.Fatal(err)
	}

	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	exit := Execute([]string{"publication", "asset", "upload", state.publicationID, "--role", "cover", "--path", mediaPath, "--candidate-only"}, Dependencies{
		Out: &stdout, ErrOut: &bytes.Buffer{}, Store: store, APIBaseURL: server.URL, Region: config.RegionGlobal,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit != 0 {
		t.Fatalf("slot-conflict retry did not succeed: %s", stdout.String())
	}
	var envelope struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK {
		t.Fatalf("unexpected envelope: %s err=%v", stdout.String(), err)
	}
	if state.uploadAuthorizationCalls != 2 {
		t.Fatalf("expected authorize to run twice (conflict then fresh slot), got %d", state.uploadAuthorizationCalls)
	}
	// 重试必须换到未被并发对手占用的槽位
	if state.authorizedSortOrders[0] == state.authorizedSortOrders[1] {
		t.Fatalf("retry reused the conflicted slot %d", state.authorizedSortOrders[1])
	}
}
