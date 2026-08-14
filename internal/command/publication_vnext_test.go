package command

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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

	if exit, envelope := execute("skill", "publish", "--path", source); exit != 0 || envelope["ok"] != true {
		t.Fatalf("preview-first publish preparation failed: exit=%d envelope=%#v", exit, envelope)
	} else {
		data, _ := envelope["data"].(map[string]any)
		if data["listingId"] != "66666666-6666-4666-8666-666666666666" || data["ownerPreviewUrl"] == "" || data["requiresPrice"] != true {
			t.Fatalf("first business result was not the stable owner preview: %#v", envelope)
		}
	}
	state.mu.Lock()
	if state.createCalls != 0 {
		state.mu.Unlock()
		t.Fatal("preview-first preparation created a Publication before price and upload authorization")
	}
	state.mu.Unlock()
	if _, err := os.Stat(filepath.Join(source, ".viceme", "skill.json")); err != nil {
		t.Fatalf("workspace binding was not persisted beside the source: %v", err)
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
	if exit, envelope := execute("publication", "review", state.publicationID); exit != 0 || envelope["ok"] != true {
		t.Fatalf("publication review failed: exit=%d envelope=%#v", exit, envelope)
	} else {
		data, _ := envelope["data"].(map[string]any)
		draft, _ := data["draft"].(map[string]any)
		if draft["summaryZhCn"] != "发布测试" || draft["summaryEnUs"] != "Publish test" || draft["usageInstructionsZhCn"] != "按 SKILL.md 中的步骤运行。" || draft["usageInstructionsEnUs"] != "Follow the steps in SKILL.md." {
			t.Fatalf("review omitted listing copy: %#v", envelope)
		}
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
	if state.listingID != "66666666-6666-4666-8666-666666666666" {
		t.Fatalf("Publication was not attached to the prepared Listing: %q", state.listingID)
	}
	if _, err := os.Stat(filepath.Join(root, "config", "publications", state.publicationID+".json")); !os.IsNotExist(err) {
		t.Fatalf("published recovery state was not removed: %v", err)
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
	exit := Execute([]string{"skill", "publish", "--path", missingSource, "--price-minor", "1"}, Dependencies{
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
name: Candidate Test
description: Resolve an ambiguous stable Skill listing identity.
---

# Candidate Test
`), 0o644); err != nil {
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
	}{
		{name: "expired upload authorization", putFailures: 1},
		{name: "lost completion response", loseCompleteResponse: true},
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
					Title: "Publish Test", SummaryZhCN: stringPointer("发布测试"), SummaryEnUS: stringPointer("Publish test"), UsageInstructionsZhCN: stringPointer("按 SKILL.md 中的步骤运行。"), UsageInstructionsEnUS: stringPointer("Follow the steps in SKILL.md."),
					Currency: "CNY", PriceMinor: 1, GalleryUploadIDs: []string{},
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
			}
			execute := func() (int, map[string]any) {
				t.Helper()
				stdout.Reset()
				stderr.Reset()
				exit := Execute([]string{"publication", "asset", "upload", state.publicationID, "--role", "cover", "--path", mediaPath}, dependencies)
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
			if state.draft.CoverUploadID == nil || *state.draft.CoverUploadID != "upload-media" {
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
		SourcePath: "/tmp/source", PriceMinor: 1, ArtifactDigest: strings.Repeat("b", 64),
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
	err = retirePublicationRecovery(store, pending, "PUBLISHED")
	cliError := output.AsError(err)
	if cliError.Subtype != "PUBLICATION_RECOVERY_RETIRE_FAILED" {
		t.Fatalf("terminal cleanup failure was hidden: %#v", cliError)
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
	if err := retirePublicationRecovery(store, pending, "PUBLISHED"); err != nil {
		t.Fatalf("terminal cleanup retry did not converge: %v", err)
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
	analysisPolls            int
	ambiguousPrepare         bool
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
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/auth/status":
		writeJSONResponse(writer, map[string]any{
			"authenticated": true,
			"user":          map[string]any{"id": "55555555-5555-4555-8555-555555555555", "displayName": "Creator", "avatarUrl": nil},
			"scopes":        []string{"profile:read", "skill-publication:read", "skill-publication:write"},
			"expiresAt":     "2027-08-12T08:00:00Z",
		})
	case request.Method == http.MethodPost && request.URL.Path == "/v1/creator/skill-listings/prepare":
		if state.ambiguousPrepare {
			writer.WriteHeader(http.StatusConflict)
			writeJSONResponse(writer, map[string]any{"statusCode": 409, "code": "SKILL_LISTING_SOURCE_AMBIGUOUS", "message": "Multiple listings match this package digest"})
			return
		}
		writeJSONResponse(writer, api.PrepareSkillListingResponse{
			ListingID: listingID, Status: "DRAFT", DraftRevision: 1,
			OwnerPreviewURL: state.baseURL + "/zh-CN/creator/skills/" + listingID + "/preview",
			BindingReceipt:  "binding-receipt", Resolution: "CREATED",
			Preview:     api.SkillListingPreviewViewModel{SchemaVersion: "preview.viceme.ai/v1", ListingID: listingID, DraftRevision: 1, State: "SHELL", FallbackURL: state.baseURL + "/preview"},
			NextActions: []string{"OPEN_PREVIEW", "SET_PRICE", "AUTHORIZE_UPLOAD"},
		})
	case request.Method == http.MethodPost && request.URL.Path == "/v1/creator/skill-listings/candidates":
		writeJSONResponse(writer, api.SkillListingCandidatesResponse{Candidates: []api.SkillListingCandidate{
			{ListingID: "77777777-7777-4777-8777-777777777777", UpdatedAt: "2026-08-14T08:00:00Z", OwnerPreviewURL: state.baseURL + "/preview/one"},
			{ListingID: "88888888-8888-4888-8888-888888888888", UpdatedAt: "2026-08-14T07:00:00Z", OwnerPreviewURL: state.baseURL + "/preview/two"},
		}})
	case request.Method == http.MethodGet && request.URL.Path == "/v1/creator/skill-listings/"+listingID+"/preview":
		writeJSONResponse(writer, api.SkillListingPreview{ListingID: listingID, Status: "DRAFT", DraftRevision: 1, Publication: &api.SkillListingPublicationPreview{ID: state.publicationID, Status: state.status}, Preview: api.SkillListingPreviewViewModel{SchemaVersion: "preview.viceme.ai/v1", ListingID: listingID, DraftRevision: 1, State: "SHELL", FallbackURL: state.baseURL + "/preview"}})
	case request.Method == http.MethodPost && request.URL.Path == "/v1/creator/skill-publications":
		var input api.CreateSkillPublicationRequest
		_ = json.NewDecoder(request.Body).Decode(&input)
		state.createCalls++
		state.clientRequestIDs = append(state.clientRequestIDs, input.ClientRequestID)
		state.listingID = input.ListingID
		state.manifest = input.Manifest
		state.draft = api.SkillPublicationDraft{
			Title: input.Manifest.Metadata.Title, SummaryZhCN: stringPointer("发布测试"), SummaryEnUS: stringPointer("Publish test"), UsageInstructionsZhCN: stringPointer("按 SKILL.md 中的步骤运行。"), UsageInstructionsEnUS: stringPointer("Follow the steps in SKILL.md."),
			Currency: "CNY", PriceMinor: input.Manifest.Spec.Sale.PriceMinor, GalleryUploadIDs: []string{},
		}
		state.status = "DRAFT"
		if state.createCalls == 1 {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, "{")
			return
		}
		writeJSONResponse(writer, api.CreateSkillPublicationResponse{PublicationID: state.publicationID, ListingID: listingID, DraftRevision: 1, Status: state.status, PackageUpload: &api.UploadAuthorization{UploadID: "upload-package", Method: http.MethodPut, URL: state.baseURL + "/upload/package", Headers: map[string]string{"Content-Type": "application/zip"}}})
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
		uploads = append(uploads, api.SkillPublicationUpload{ID: "upload-media", Kind: "MEDIA", Status: "VERIFIED", FileName: state.mediaFileName, ContentType: state.mediaContentType, SizeBytes: state.mediaSizeBytes, Digest: state.mediaDigest, SortOrder: state.mediaSortOrder})
	} else if state.mediaPending {
		uploads = append(uploads, api.SkillPublicationUpload{ID: "upload-media", Kind: "MEDIA", Status: "PENDING", FileName: state.mediaFileName, ContentType: state.mediaContentType, SizeBytes: state.mediaSizeBytes, Digest: state.mediaDigest, SortOrder: state.mediaSortOrder})
	}
	result := api.SkillPublication{ID: state.publicationID, ListingID: "66666666-6666-4666-8666-666666666666", DraftRevision: 1, Status: state.status, Manifest: state.manifest, Draft: state.draft, ReviewRevision: 1, ReviewDigest: &state.reviewDigest, Uploads: uploads}
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
