package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
	"github.com/ViceMe-AI/cli/internal/replicapreview"
	"github.com/ViceMe-AI/cli/internal/replicapublication"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

const (
	replicaPublicationTestAccessToken   = "vme_cli_1234567890123456789012345678901234567890123"
	replicaPublicationTestID            = "11111111-1111-4111-8111-111111111111"
	replicaPublicationTestRequestID     = "22222222-2222-4222-8222-222222222222"
	replicaPublicationTestMerchantID    = "33333333-3333-4333-8333-333333333333"
	replicaPublicationTestWorkID        = "44444444-4444-4444-8444-444444444444"
	replicaPublicationTestReplicaID     = "55555555-5555-4555-8555-555555555555"
	replicaPublicationTestVersionID     = "66666666-6666-4666-8666-666666666666"
	replicaPublicationTestProductID     = "77777777-7777-4777-8777-777777777777"
	replicaPublicationTestSKUID         = "88888888-8888-4888-8888-888888888888"
	replicaPublicationTestCreatorID     = "99999999-9999-4999-8999-999999999999"
	replicaPublicationTestSourceDigest  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	replicaPublicationTestProjectDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestReplicaPublicationRejectsUnsupportedGlobalMarketBeforeLocalWork(t *testing.T) {
	project := newReplicaPublicationTestProject(t)
	previewCalled := false
	runtime := &Runtime{
		apiBaseURL: "https://api.viceme.ai",
		configBase: t.TempDir(),
		profile:    config.Profile{MarketRegion: config.RegionGlobal},
		deps: Dependencies{
			Now:    func() time.Time { return time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC) },
			ErrOut: io.Discard,
			StartReplicaPreview: func(context.Context, replicapreview.Options) (replicapreview.Running, error) {
				previewCalled = true
				return nil, errors.New("preview must not start")
			},
		},
	}
	_, err := publishWebsiteReplica(context.Background(), runtime, replicaPublishOptions{
		ProjectPath: project, Slug: "replica-site", Title: "Replica title", Summary: "Replica summary", PriceCents: 990,
	})
	if cliErr := output.AsError(err); err == nil || cliErr.Subtype != "REPLICA_PUBLICATION_MARKET_UNSUPPORTED" || cliErr.Type != "policy" {
		t.Fatalf("GLOBAL publication was not rejected by policy: %#v", cliErr)
	}
	if previewCalled {
		t.Fatal("GLOBAL publication reached local preview")
	}
}

func TestReplicaStatusPresentsProcessingAsSubmittedButNotPublished(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/website-replica-publications/"+replicaPublicationTestID {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer "+replicaPublicationTestAccessToken {
			t.Fatalf("status request was not authenticated: %q", request.Header.Get("Authorization"))
		}
		writeJSONResponse(writer, replicaPublicationAPIResponse(now, "PROCESSING", "VERIFIED"))
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	root := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"replica", "status", replicaPublicationTestID}, Dependencies{
		Out: &stdout, ErrOut: &stderr, HTTPClient: server.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, APIBaseURL: server.URL, Now: func() time.Time { return now },
	})
	if exit != 0 {
		t.Fatalf("replica status failed: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Status        string `json:"status"`
			Phase         string `json:"phase"`
			Message       string `json:"message"`
			PublicationID string `json:"publicationId"`
			StatusURL     string `json:"statusUrl"`
			Resume        struct {
				Command string `json:"command"`
			} `json:"resume"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid status output: %v: %s", err, stdout.String())
	}
	if !envelope.OK || envelope.Data.Status != "PROCESSING" || envelope.Data.Phase != "SUBMITTED_NOT_PUBLISHED" ||
		envelope.Data.PublicationID != replicaPublicationTestID || envelope.Data.StatusURL == "" ||
		envelope.Data.Resume.Command != "viceme replica resume "+replicaPublicationTestID {
		t.Fatalf("unexpected processing presentation: %#v", envelope)
	}
	message := strings.ToLower(envelope.Data.Message)
	if !strings.Contains(message, "submitted") || !strings.Contains(message, "not published") || strings.Contains(message, "publication complete") {
		t.Fatalf("PROCESSING was described as published: %q", envelope.Data.Message)
	}
}

func TestReplicaStatusPresentsOnlyPublishedTerminalStatesAsComplete(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	for _, status := range []string{"PUBLISHED", "PUBLISHED_DEGRADED"} {
		status := status
		t.Run(status, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writeJSONResponse(writer, replicaPublicationAPIResponse(now, status, "ACTIVATED"))
			}))
			defer server.Close()
			t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
			root := t.TempDir()
			dependencies := replicaPublicationTestDependencies(t, root, server, now)
			var stdout bytes.Buffer
			dependencies.Out = &stdout
			if exit := Execute([]string{"replica", "status", replicaPublicationTestID}, dependencies); exit != 0 {
				t.Fatalf("terminal status failed: exit=%d output=%s", exit, stdout.String())
			}
			var envelope struct {
				Data replicaPublicationPresentation `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Data.Status != status || !strings.Contains(strings.ToLower(envelope.Data.Message), "publication complete") {
				t.Fatalf("published terminal was not presented as complete: %#v", envelope.Data)
			}
			if status == "PUBLISHED" && !strings.Contains(envelope.Data.Message, "no hosted HTML page is active") {
				t.Fatalf("source-only publication claimed hosted completion: %q", envelope.Data.Message)
			}
			if status == "PUBLISHED_DEGRADED" && (!strings.Contains(envelope.Data.Message, "source is published") ||
				!strings.Contains(envelope.Data.Message, "hosting failed") || !strings.Contains(envelope.Data.Message, "native Work page")) {
				t.Fatalf("degraded terminal omitted its boundaries: %q", envelope.Data.Message)
			}
		})
	}
}

func TestReplicaStatusIncludesActivatedPageReleaseForHostedPublication(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	response := replicaPublicationAPIResponse(now, "PUBLISHED", "ACTIVATED")
	response["page"] = map[string]any{
		"fileName": "page.zip", "contentType": "application/zip", "sizeBytes": 512,
		"digest": strings.Repeat("d", 64), "status": "ACTIVATED", "verifiedAt": now.Add(-time.Minute).Format(time.RFC3339),
	}
	response["result"].(map[string]any)["pageRelease"] = map[string]any{
		"id": "abababab-abab-4bab-8bab-abababababab", "version": 2,
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSONResponse(writer, response)
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	root := t.TempDir()
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	var stdout bytes.Buffer
	dependencies.Out = &stdout
	if exit := Execute([]string{"replica", "status", replicaPublicationTestID}, dependencies); exit != 0 {
		t.Fatalf("hosted terminal status failed: exit=%d output=%s", exit, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"pageRelease": {`) || !strings.Contains(stdout.String(), `"version": 2`) ||
		!strings.Contains(stdout.String(), `"page": {`) || !strings.Contains(stdout.String(), `"status": "ACTIVATED"`) {
		t.Fatalf("hosted terminal status omitted the Page Release: %s", stdout.String())
	}
}

func TestReplicaPublishPreviewsConfirmsUploadsAndRecordsProcessingBinding(t *testing.T) {
	for _, projectStorage := range []bool{false, true} {
		t.Run(fmt.Sprintf("project-storage-%t", projectStorage), func(t *testing.T) { testReplicaPublicationStorageLifecycle(t, projectStorage) })
	}
}

func testReplicaPublicationStorageLifecycle(t *testing.T, projectStorage bool) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := filepath.Join(t.TempDir(), "site")
	if err := os.MkdirAll(filepath.Join(project, "node_modules"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "package.json"), []byte(`{
  "packageManager": "pnpm@9.15.0",
  "scripts": {"dev": "vite"},
  "dependencies": {"vite": "latest"}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "index.html"), []byte("<h1>Replica source</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "dist"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("<h1>Hosted page</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}

	var previewProbed atomic.Bool
	previewSession := &replicaPreviewSessionStub{result: replicapreview.Result{
		TargetURL: "http://127.0.0.1:4173/", Reused: true, ServiceKind: replicapreview.ServiceExisting,
	}}
	var uploaded []byte
	var uploadedPage []byte
	objectServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || (request.URL.Path != "/source.zip" && request.URL.Path != "/page.zip") {
			t.Fatalf("unexpected object request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "" {
			t.Fatalf("presigned upload received API authorization: %q", request.Header.Get("Authorization"))
		}
		var err error
		data, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if request.URL.Path == "/page.zip" {
			uploadedPage = data
		} else {
			uploaded = data
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer objectServer.Close()

	confirmationVersion := "wrv1-cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	var createCalls int
	var firstRequest map[string]any
	submitted := false
	controlServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !previewProbed.Load() {
			t.Fatal("remote publication request happened before local preview")
		}
		if request.Header.Get("Authorization") != "Bearer "+replicaPublicationTestAccessToken {
			t.Fatalf("control request was not authenticated: %q", request.Header.Get("Authorization"))
		}
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications":
			createCalls++
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input["protocolVersion"] != float64(2) || input["clientRequestId"] != replicaPublicationTestRequestID ||
				input["market"] != "CN" || input["title"] != "Replica title" || input["summary"] != "Replica summary" ||
				input["allowAutomaticDegradation"] != false || input["priceCents"] != float64(990) || input["projectFingerprint"] == "" || input["canonicalOrigin"] != "https://example.com" {
				t.Fatalf("unexpected create request: %#v", input)
			}
			target, _ := input["target"].(map[string]any)
			if target["kind"] != "NEW_WORK" || target["slug"] != "replica-site" {
				t.Fatalf("unexpected publication target: %#v", target)
			}
			source, _ := input["source"].(map[string]any)
			if source["contentType"] != "application/zip" || source["digest"] == "" || source["sizeBytes"] == float64(0) {
				t.Fatalf("missing frozen source summary: %#v", source)
			}
			page, _ := input["page"].(map[string]any)
			if page["fileName"] != "page.zip" || page["contentType"] != "application/zip" || page["digest"] == "" || page["sizeBytes"] == float64(0) {
				t.Fatalf("missing frozen page summary: %#v", page)
			}
			if createCalls == 1 {
				if input["confirmation"] != nil {
					t.Fatalf("first request crossed confirmation boundary: %#v", input["confirmation"])
				}
				firstRequest = input
				writeJSONResponse(writer, replicaConfirmationRequiredResponse(now, input, confirmationVersion))
				return
			}
			if input["projectFingerprint"] != firstRequest["projectFingerprint"] || !mapsHaveEqualJSON(input["source"], firstRequest["source"]) ||
				!mapsHaveEqualJSON(input["page"], firstRequest["page"]) {
				t.Fatalf("confirmation did not use the frozen request: first=%#v confirmed=%#v", firstRequest, input)
			}
			confirmation, _ := input["confirmation"].(map[string]any)
			if confirmation["version"] != confirmationVersion || confirmation["confirmedAt"] == nil {
				t.Fatalf("confirmed request did not bind the challenge: %#v", confirmation)
			}
			writeJSONResponse(writer, map[string]any{
				"outcome": "ACTION_REQUIRED", "clientRequestId": replicaPublicationTestRequestID, "market": "CN",
				"nextAction": map[string]any{"kind": "AUTHORIZE_SOURCE_UPLOAD", "publicationId": replicaPublicationTestID},
			})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID:
			if submitted {
				writeJSONResponse(writer, replicaPublicationForArtifacts(now, "PROCESSING", "VERIFIED", firstRequest["source"].(map[string]any), "VERIFIED", firstRequest["page"].(map[string]any)))
				return
			}
			writeJSONResponse(writer, replicaPublicationForArtifacts(now, "DRAFT", "WAITING_UPLOAD", firstRequest["source"].(map[string]any), "WAITING_UPLOAD", firstRequest["page"].(map[string]any)))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID+"/page/upload-authorizations":
			assertEmptyJSONObject(t, request)
			writeJSONResponse(writer, map[string]any{
				"publicationId": replicaPublicationTestID,
				"upload": map[string]any{
					"method": "PUT", "url": objectServer.URL + "/page.zip",
					"headers":   map[string]string{"Content-Type": "application/zip"},
					"expiresAt": now.Add(5 * time.Minute).Format(time.RFC3339),
				},
			})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID+"/page/complete-upload":
			assertEmptyJSONObject(t, request)
			writeJSONResponse(writer, replicaPublicationForArtifacts(now, "DRAFT", "WAITING_UPLOAD", firstRequest["source"].(map[string]any), "VERIFIED", firstRequest["page"].(map[string]any)))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID+"/source/upload-authorizations":
			assertEmptyJSONObject(t, request)
			writeJSONResponse(writer, map[string]any{
				"publicationId": replicaPublicationTestID,
				"upload": map[string]any{
					"method": "PUT", "url": objectServer.URL + "/source.zip",
					"headers":   map[string]string{"Content-Type": "application/zip"},
					"expiresAt": now.Add(5 * time.Minute).Format(time.RFC3339),
				},
			})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID+"/source/complete-upload":
			assertEmptyJSONObject(t, request)
			writeJSONResponse(writer, replicaPublicationForArtifacts(now, "DRAFT", "VERIFIED", firstRequest["source"].(map[string]any), "VERIFIED", firstRequest["page"].(map[string]any)))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID+"/submit":
			submitted = true
			assertEmptyJSONObject(t, request)
			writeJSONResponse(writer, replicaPublicationForArtifacts(now, "PROCESSING", "VERIFIED", firstRequest["source"].(map[string]any), "VERIFIED", firstRequest["page"].(map[string]any)))
		default:
			t.Fatalf("unexpected control request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer controlServer.Close()

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	root := t.TempDir()
	ids := []string{replicaPublicationTestRequestID}
	dependencies := Dependencies{
		ErrOut: &bytes.Buffer{}, HTTPClient: controlServer.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, APIBaseURL: controlServer.URL, Now: func() time.Time { return now },
		NewID: func() string {
			if len(ids) == 0 {
				t.Fatal("publication allocated more than one main clientRequestId")
			}
			value := ids[0]
			ids = ids[1:]
			return value
		},
		StartReplicaPreview: func(_ context.Context, options replicapreview.Options) (replicapreview.Running, error) {
			if options.ExistingURL != previewSession.Result().TargetURL {
				t.Fatalf("unexpected local preview URL: %q", options.ExistingURL)
			}
			previewProbed.Store(true)
			return previewSession, nil
		},
		OpenURL: func(context.Context, string) error {
			t.Fatal("publication must reuse creator approval without opening another browser")
			return nil
		},
	}
	arguments := []string{
		"replica", "publish", "--path", project, "--slug", "replica-site",
		"--title", "Replica title", "--summary", "Replica summary", "--price-cents", "990",
		"--preview-url", "http://127.0.0.1:4173/", "--preview-reviewed",
		"--canonical-origin", "HTTPS://Example.COM:443/",
	}

	if projectStorage {
		arguments = append(arguments, "--state-project", project)
		original := privatefile.ReplaceFile
		privatefile.ReplaceFile = func(from, to string) error {
			if strings.HasPrefix(to, filepath.Join(root, "config")+string(filepath.Separator)) {
				return syscall.EPERM
			}
			return original(from, to)
		}
		t.Cleanup(func() { privatefile.ReplaceFile = original })
	}
	var reviewOutput bytes.Buffer
	dependencies.Out = &reviewOutput
	if exit := Execute(arguments, dependencies); exit != output.ExitConfirmation {
		t.Fatalf("first publication did not stop at final review: exit=%d output=%q", exit, reviewOutput.String())
	}
	var reviewEnvelope map[string]any
	if err := json.Unmarshal(reviewOutput.Bytes(), &reviewEnvelope); err != nil {
		t.Fatalf("invalid review output: %v: %s", err, reviewOutput.String())
	}
	reviewError, _ := reviewEnvelope["error"].(map[string]any)
	details, _ := reviewError["details"].(map[string]any)
	if projectStorage && !strings.Contains(details["confirmCommand"].(string), "--state-project ") {
		t.Fatal("confirmation lost project storage")
	}
	review, _ := details["review"].(map[string]any)
	pageArtifact, hasPageArtifact := review["pageArtifact"].(map[string]any)
	if reviewError["code"] != "REPLICA_PUBLICATION_CONFIRMATION_REQUIRED" || details["confirmationVersion"] != confirmationVersion ||
		review["resolution"] != "CREATE" || review["merchantAccountId"] != replicaPublicationTestMerchantID ||
		review["merchantDisplayName"] != "Replica Studio" || review["creatorAccountId"] != replicaPublicationTestCreatorID ||
		review["creatorHandle"] != "replica-maker" || review["creatorDisplayName"] != "Replica Maker" ||
		review["workUrl"] != "https://viceme.cn/replica-maker/replica-site" || review["hosting"] != "HOSTED" ||
		review["title"] != "Replica title" || review["summary"] != "Replica summary" || review["priceCents"] != float64(990) ||
		!hasPageArtifact || pageArtifact["fileName"] != "page.zip" || pageArtifact["sizeBytes"] == float64(0) || pageArtifact["digest"] == "" ||
		review["automaticDegradation"] != false || review["immutableVersions"] != true ||
		review["existingBuyerVersionsRetained"] != true || review["automaticCreatorApplication"] != false ||
		review["confirmationTtlSeconds"] != float64(1800) || review["confirmationExpiresAt"] != now.Add(30*time.Minute).Format(time.RFC3339) ||
		review["sourceArchive"] == nil || review["exclusions"] == nil || review["preview"] == nil {
		t.Fatalf("final review omitted required publication facts: %#v", reviewEnvelope)
	}
	if len(uploaded) != 0 || len(uploadedPage) != 0 || createCalls != 1 {
		t.Fatalf("unconfirmed publication uploaded artifacts: create=%d source=%d page=%d", createCalls, len(uploaded), len(uploadedPage))
	}
	if err := os.WriteFile(filepath.Join(project, "index.html"), []byte("<h1>Changed after final review</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}

	var submittedOutput bytes.Buffer
	dependencies.Out = &submittedOutput
	confirmedArguments := append(append([]string{}, arguments...), "--confirm", confirmationVersion)
	if exit := Execute(confirmedArguments, dependencies); exit != 0 {
		t.Fatalf("confirmed publication failed: exit=%d output=%q", exit, submittedOutput.String())
	}
	if len(uploaded) == 0 || len(uploadedPage) == 0 || createCalls != 2 {
		t.Fatalf("confirmed publication did not upload both artifacts exactly once: create=%d source=%d page=%d", createCalls, len(uploaded), len(uploadedPage))
	}
	for name := range readReplicaZIP(t, uploaded) {
		if strings.Contains(name, ".viceme/") {
			t.Fatalf("recovery state leaked into source: %s", name)
		}
	}
	if projectStorage {
		if !strings.Contains(submittedOutput.String(), "--state-project ") {
			t.Fatal("resume lost project storage")
		}
		for _, operation := range []string{"status", "resume"} {
			var resumed bytes.Buffer
			dependencies.Out = &resumed
			if exit := Execute([]string{"replica", operation, replicaPublicationTestID, "--state-project", project}, dependencies); exit != 0 {
				t.Fatalf("%s failed: %s", operation, resumed.String())
			}
		}
	}
	if contents := readReplicaZIP(t, uploaded); string(contents["index.html"]) != "<h1>Replica source</h1>" {
		t.Fatalf("working-tree changes replaced the confirmed frozen source: %q", contents["index.html"])
	}
	if contents := readReplicaZIP(t, uploadedPage); string(contents["dist/index.html"]) != "<h1>Hosted page</h1>" || len(contents["viceme-page.json"]) == 0 {
		t.Fatalf("hosted page package did not preserve the frozen static output: %#v", contents)
	}
	if !previewSession.closed.Load() {
		t.Fatal("publication preview session was not cleaned")
	}
	if !strings.Contains(submittedOutput.String(), `"status": "PROCESSING"`) ||
		!strings.Contains(submittedOutput.String(), `"phase": "SUBMITTED_NOT_PUBLISHED"`) {
		t.Fatalf("submission did not return machine-readable PROCESSING: %s", submittedOutput.String())
	}

	bindingPath := filepath.Join(project, ".viceme", "website-replica.json")
	bindingData, err := os.ReadFile(bindingPath)
	if err != nil {
		t.Fatalf("platform takeover binding was not recorded: %v", err)
	}
	var binding map[string]any
	if err := json.Unmarshal(bindingData, &binding); err != nil {
		t.Fatalf("invalid local binding: %v", err)
	}
	publicationBinding, _ := binding["publication"].(map[string]any)
	merchantBinding, _ := binding["merchant"].(map[string]any)
	frozenSource, _ := binding["frozenSource"].(map[string]any)
	if binding["projectFingerprint"] != firstRequest["projectFingerprint"] || publicationBinding["id"] != replicaPublicationTestID ||
		merchantBinding["id"] != replicaPublicationTestMerchantID || frozenSource["digest"] != firstRequest["source"].(map[string]any)["digest"] {
		t.Fatalf("platform takeover binding is incomplete: %#v", binding)
	}
	if binding["work"] != nil || binding["replica"] != nil || binding["product"] != nil || binding["version"] != nil {
		t.Fatalf("processing binding exposed terminal associations: %#v", binding)
	}
	for _, forbidden := range []string{replicaPublicationTestAccessToken, objectServer.URL, "Content-Type"} {
		if bytes.Contains(bindingData, []byte(forbidden)) {
			t.Fatalf("local binding persisted a credential or upload capability %q: %s", forbidden, bindingData)
		}
	}
	stagedBindings, err := filepath.Glob(filepath.Join(project, ".viceme", ".website-replica-*.tmp"))
	if err != nil || len(stagedBindings) != 0 {
		t.Fatalf("atomic binding write left staging files: files=%v err=%v", stagedBindings, err)
	}
	storageRoot := filepath.Join(root, "config", "replica-publications")
	if projectStorage {
		storageRoot = filepath.Join(project, ".viceme", "publications")
	}
	artifacts, err := filepath.Glob(filepath.Join(storageRoot, "*", "artifacts", "*", "source.zip"))
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("frozen source was retained after platform takeover: files=%v err=%v", artifacts, err)
	}
}

func TestReplicaPublishRejectsUploadCapabilityBeforeLocalFinalConfirmation(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name, outcome string
		nextAction    map[string]any
	}{
		{
			name: "action-required-upload", outcome: "ACTION_REQUIRED",
			nextAction: map[string]any{"kind": "AUTHORIZE_SOURCE_UPLOAD", "publicationId": replicaPublicationTestID},
		},
		{
			name: "publication-ready-upload", outcome: "PUBLICATION_READY",
			nextAction: map[string]any{"kind": "AUTHORIZE_SOURCE_UPLOAD", "publicationId": replicaPublicationTestID},
		},
		{
			name: "action-required-creator-application", outcome: "ACTION_REQUIRED",
			nextAction: map[string]any{"kind": "APPLY_CREATOR", "applicationUrl": "https://viceme.cn/me/creator-center"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			project := newReplicaPublicationTestProject(t)
			var followupCalls int
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost || request.URL.Path != "/v1/website-replica-publications" {
					followupCalls++
					t.Fatalf("unconfirmed publication followed an upload action: %s %s", request.Method, request.URL.Path)
				}
				var input map[string]any
				if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
					t.Fatal(err)
				}
				source := input["source"].(map[string]any)
				if test.outcome == "ACTION_REQUIRED" {
					writeJSONResponse(writer, map[string]any{
						"outcome": "ACTION_REQUIRED", "clientRequestId": input["clientRequestId"], "market": "CN",
						"nextAction": test.nextAction,
					})
					return
				}
				writeJSONResponse(writer, map[string]any{
					"outcome": "PUBLICATION_READY",
					"target": map[string]any{
						"resolution": "CREATE", "merchantAccountId": replicaPublicationTestMerchantID,
						"workId": replicaPublicationTestWorkID, "replicaId": replicaPublicationTestReplicaID,
						"productId": nil, "workUrl": "https://viceme.cn/replica-maker/replica-site",
					},
					"publication": replicaPublicationForSource(now, "DRAFT", "WAITING_UPLOAD", source),
					"nextAction":  test.nextAction,
				})
			}))
			defer server.Close()

			t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
			root := t.TempDir()
			dependencies := replicaPublicationTestDependencies(t, root, server, now)
			dependencies.NewID = func() string { return replicaPublicationTestRequestID }
			var stdout bytes.Buffer
			dependencies.Out = &stdout
			exit := Execute(replicaPublicationTestArguments(project, "replica-site"), dependencies)
			if exit != output.ExitInternal || !strings.Contains(stdout.String(), `"code": "RESPONSE_INVALID"`) {
				t.Fatalf("unconfirmed upload capability was accepted: exit=%d output=%s", exit, stdout.String())
			}
			if followupCalls != 0 {
				t.Fatalf("unconfirmed publication made %d follow-up calls", followupCalls)
			}
		})
	}
}

func TestReplicaPublishResumesSameRequestAfterLogin(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	var createCalls int
	var clientRequestID string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/website-replica-publications" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		createCalls++
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if createCalls == 1 {
			if request.Header.Get("Authorization") != "" {
				t.Fatalf("anonymous create leaked authorization: %q", request.Header.Get("Authorization"))
			}
			clientRequestID, _ = input["clientRequestId"].(string)
			writeJSONResponse(writer, map[string]any{
				"outcome": "ACTION_REQUIRED", "clientRequestId": clientRequestID, "market": "CN",
				"nextAction": map[string]any{"kind": "AUTHENTICATE_CREATOR", "authUrl": "https://viceme.cn/login"},
			})
			return
		}
		if request.Header.Get("Authorization") != "Bearer "+replicaPublicationTestAccessToken {
			t.Fatalf("resumed create was not authenticated: %q", request.Header.Get("Authorization"))
		}
		if input["clientRequestId"] != clientRequestID {
			t.Fatalf("login recovery changed main clientRequestId: first=%q next=%#v", clientRequestID, input["clientRequestId"])
		}
		writeJSONResponse(writer, replicaConfirmationRequiredResponse(now, input, "wrv1-dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"))
	}))
	defer server.Close()

	root := t.TempDir()
	newIDCalls := 0
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	previewURL := "http://127.0.0.1:4173/"
	previewCalls := 0
	dependencies.StartReplicaPreview = func(_ context.Context, options replicapreview.Options) (replicapreview.Running, error) {
		previewCalls++
		if options.ExistingURL != previewURL || options.ProjectPath != "" {
			t.Fatalf("login recovery lost the explicit preview URL: %#v", options)
		}
		return &replicaPreviewSessionStub{result: replicapreview.Result{
			TargetURL: previewURL, Reused: true, ServiceKind: replicapreview.ServiceExisting,
		}}, nil
	}
	dependencies.NewID = func() string {
		newIDCalls++
		return replicaPublicationTestRequestID
	}
	arguments := append(replicaPublicationTestArguments(project, "replica-site"), "--preview-url", previewURL)
	t.Setenv(processAccessTokenEnvironment, "")
	var firstOutput bytes.Buffer
	dependencies.Out = &firstOutput
	if exit := Execute(arguments, dependencies); exit != output.ExitAuthentication ||
		!strings.Contains(firstOutput.String(), "REPLICA_PUBLICATION_AUTHENTICATION_REQUIRED") ||
		!strings.Contains(firstOutput.String(), "--preview-url "+previewURL) {
		t.Fatalf("anonymous publication did not return AUTHENTICATE_CREATOR: exit=%d output=%s", exit, firstOutput.String())
	}

	if err := os.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken); err != nil {
		t.Fatal(err)
	}
	var resumedOutput bytes.Buffer
	dependencies.Out = &resumedOutput
	if exit := Execute(arguments, dependencies); exit != output.ExitConfirmation || !strings.Contains(resumedOutput.String(), "REPLICA_PUBLICATION_CONFIRMATION_REQUIRED") {
		t.Fatalf("login recovery did not reach final review: exit=%d output=%s", exit, resumedOutput.String())
	}
	if createCalls != 2 || newIDCalls != 1 || previewCalls != 2 {
		t.Fatalf("login recovery did not reuse one request and preview target: creates=%d ids=%d previews=%d", createCalls, newIDCalls, previewCalls)
	}
}

func TestReplicaPublishStopsForCreatorReviewWithoutUpload(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	objectCalls := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/website-replica-publications" {
			objectCalls.Add(1)
			t.Fatalf("qualification stop reached another endpoint: %s %s", request.Method, request.URL.Path)
		}
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		writeJSONResponse(writer, map[string]any{
			"outcome": "ACTION_REQUIRED", "clientRequestId": input["clientRequestId"], "market": "CN",
			"nextAction": map[string]any{
				"kind": "WAIT_CREATOR_REVIEW", "onboardingId": replicaPublicationTestCreatorID,
				"statusUrl": "https://api.viceme.cn/v1/cli/merchant/onboarding/current",
			},
		})
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	root := t.TempDir()
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	dependencies.NewID = func() string { return replicaPublicationTestRequestID }
	var stdout bytes.Buffer
	dependencies.Out = &stdout
	if exit := Execute(replicaPublicationTestArguments(project, "replica-site"), dependencies); exit != output.ExitAuthentication ||
		!strings.Contains(stdout.String(), "REPLICA_CREATOR_REVIEW_PENDING") {
		t.Fatalf("creator review did not stop publication: exit=%d output=%s", exit, stdout.String())
	}
	if objectCalls.Load() != 0 {
		t.Fatalf("creator review stop performed %d unexpected remote operations", objectCalls.Load())
	}
	artifacts, err := filepath.Glob(filepath.Join(root, "config", "replica-publications", "*", "artifacts", "*", "source.zip"))
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("recoverable qualification stop did not retain one TTL-bound artifact: files=%v err=%v", artifacts, err)
	}
}

func TestReplicaPublishReusesMainRequestAfterCreatorReviewOutlivesFrozenSource(t *testing.T) {
	startedAt := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	currentTime := startedAt
	project := newReplicaPublicationTestProject(t)
	requestIDs := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		requestIDs = append(requestIDs, input["clientRequestId"].(string))
		writeJSONResponse(writer, map[string]any{
			"outcome": "ACTION_REQUIRED", "clientRequestId": input["clientRequestId"], "market": "CN",
			"nextAction": map[string]any{
				"kind": "WAIT_CREATOR_REVIEW", "onboardingId": replicaPublicationTestCreatorID,
				"statusUrl": "https://api.viceme.cn/v1/cli/merchant/onboarding/current",
			},
		})
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	root := t.TempDir()
	dependencies := replicaPublicationTestDependencies(t, root, server, startedAt)
	dependencies.Now = func() time.Time { return currentTime }
	allocated := []string{replicaPublicationTestRequestID, "abababab-abab-4bab-8bab-abababababab"}
	dependencies.NewID = func() string {
		value := allocated[0]
		allocated = allocated[1:]
		return value
	}
	arguments := replicaPublicationTestArguments(project, "replica-site")
	for attempt := 0; attempt < 2; attempt++ {
		dependencies.Out = &bytes.Buffer{}
		if exit := Execute(arguments, dependencies); exit != output.ExitAuthentication {
			t.Fatalf("creator review attempt %d did not stop: exit=%d", attempt+1, exit)
		}
		currentTime = startedAt.Add(31 * time.Minute)
	}
	if len(requestIDs) != 2 || requestIDs[0] != replicaPublicationTestRequestID || requestIDs[1] != requestIDs[0] {
		t.Fatalf("creator review recovery changed the main request identity: %v", requestIDs)
	}
	if len(allocated) != 1 {
		t.Fatalf("creator review recovery allocated a second request identity: remaining=%v", allocated)
	}
}

func TestReplicaPublishStopsForCreatorEvidenceOrRejectionWithoutUpload(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		kind, code string
		exit       int
	}{
		{kind: "SUPPLY_CREATOR_INFO", code: "REPLICA_CREATOR_INFO_REQUIRED", exit: output.ExitAuthentication},
		{kind: "CREATOR_APPLICATION_REJECTED", code: "REPLICA_CREATOR_APPLICATION_REJECTED", exit: output.ExitPolicy},
	} {
		t.Run(test.kind, func(t *testing.T) {
			project := newReplicaPublicationTestProject(t)
			remoteCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				remoteCalls++
				if request.URL.Path != "/v1/website-replica-publications" {
					t.Fatalf("qualification stop reached another endpoint: %s %s", request.Method, request.URL.Path)
				}
				var input map[string]any
				if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
					t.Fatal(err)
				}
				writeJSONResponse(writer, map[string]any{
					"outcome": "ACTION_REQUIRED", "clientRequestId": input["clientRequestId"], "market": "CN",
					"nextAction": map[string]any{
						"kind": test.kind, "onboardingId": replicaPublicationTestCreatorID,
						"statusUrl": "https://api.viceme.cn/v1/cli/merchant/onboarding/current",
					},
				})
			}))
			defer server.Close()

			t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
			root := t.TempDir()
			dependencies := replicaPublicationTestDependencies(t, root, server, now)
			dependencies.NewID = func() string { return replicaPublicationTestRequestID }
			var stdout bytes.Buffer
			dependencies.Out = &stdout
			if exit := Execute(replicaPublicationTestArguments(project, "replica-site"), dependencies); exit != test.exit ||
				!strings.Contains(stdout.String(), test.code) {
				t.Fatalf("qualification state did not stop publication: exit=%d output=%s", exit, stdout.String())
			}
			if remoteCalls != 1 {
				t.Fatalf("qualification stop performed %d remote operations", remoteCalls)
			}
			artifacts, err := filepath.Glob(filepath.Join(root, "config", "replica-publications", "*", "artifacts", "*", "source.zip"))
			if err != nil || len(artifacts) != 1 {
				t.Fatalf("qualification stop did not retain one recoverable source: files=%v err=%v", artifacts, err)
			}
		})
	}
}

func TestReplicaPublishRequiresFreshConfirmationAfterSlugChanges(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	versions := []string{
		"wrv1-eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		"wrv1-ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}
	var createCalls int
	var requestID string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		createCalls++
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if createCalls == 1 {
			requestID, _ = input["clientRequestId"].(string)
		}
		if input["clientRequestId"] != requestID {
			t.Fatalf("slug refresh changed clientRequestId: first=%q next=%#v", requestID, input["clientRequestId"])
		}
		target := input["target"].(map[string]any)
		expectedSlug := "replica-site"
		workURL := "https://viceme.cn/replica-maker/replica-site"
		if createCalls == 2 {
			expectedSlug = "replica-site-2"
			workURL += "-2"
		}
		if target["slug"] != expectedSlug {
			t.Fatalf("unexpected refreshed slug: %#v", target)
		}
		response := replicaConfirmationRequiredResponse(now, input, versions[createCalls-1])
		response["nextAction"].(map[string]any)["confirmation"].(map[string]any)["review"].(map[string]any)["workUrl"] = workURL
		writeJSONResponse(writer, response)
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	root := t.TempDir()
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	newIDCalls := 0
	dependencies.NewID = func() string {
		newIDCalls++
		return replicaPublicationTestRequestID
	}
	first := replicaPublicationTestArguments(project, "replica-site")
	dependencies.Out = &bytes.Buffer{}
	if exit := Execute(first, dependencies); exit != output.ExitConfirmation {
		t.Fatalf("first slug did not reach review: exit=%d", exit)
	}

	changed := replicaPublicationTestArguments(project, "replica-site-2")
	staleConfirmation := append(append([]string{}, changed...), "--confirm", versions[0])
	var staleOutput bytes.Buffer
	dependencies.Out = &staleOutput
	if exit := Execute(staleConfirmation, dependencies); exit != output.ExitConfirmation ||
		!strings.Contains(staleOutput.String(), "REPLICA_PUBLICATION_CONFIRMATION_CHANGED") {
		t.Fatalf("changed slug accepted stale review: exit=%d output=%s", exit, staleOutput.String())
	}
	if createCalls != 1 {
		t.Fatalf("stale local confirmation reached Shop: creates=%d", createCalls)
	}

	var refreshedOutput bytes.Buffer
	dependencies.Out = &refreshedOutput
	if exit := Execute(changed, dependencies); exit != output.ExitConfirmation || !strings.Contains(refreshedOutput.String(), versions[1]) {
		t.Fatalf("changed slug did not produce a fresh review: exit=%d output=%s", exit, refreshedOutput.String())
	}
	if createCalls != 2 || newIDCalls != 1 {
		t.Fatalf("slug refresh did not reuse the main request: creates=%d ids=%d", createCalls, newIDCalls)
	}
}

func TestReplicaPublicationReadyTargetMustMatchConfirmedRequest(t *testing.T) {
	productID := replicaPublicationTestProductID
	review := api.WebsiteReplicaPublicationReview{
		Resolution: "UPDATE", MerchantAccountID: replicaPublicationTestMerchantID,
		WorkURL: "https://viceme.cn/replica-maker/replica-site",
	}
	confirmation := &api.WebsiteReplicaPublicationConfirmationChallenge{Review: review}
	request := api.CreateWebsiteReplicaPublicationRequest{Target: api.WebsiteReplicaPublicationTarget{
		Kind: "MANAGED_BINDING", WorkID: replicaPublicationTestWorkID,
		ReplicaID: replicaPublicationTestReplicaID, ProductID: replicaPublicationTestProductID,
	}}
	target := api.WebsiteReplicaPublicationResolvedTarget{
		Resolution: "UPDATE", MerchantAccountID: replicaPublicationTestMerchantID,
		WorkID: replicaPublicationTestWorkID, ReplicaID: replicaPublicationTestReplicaID,
		ProductID: &productID, WorkURL: review.WorkURL,
	}
	if !replicaResolvedTargetMatchesRequest(target, confirmation, request) {
		t.Fatal("matching managed target was rejected")
	}
	for _, mutate := range []func(*api.WebsiteReplicaPublicationResolvedTarget){
		func(value *api.WebsiteReplicaPublicationResolvedTarget) {
			value.WorkID = replicaPublicationTestCreatorID
		},
		func(value *api.WebsiteReplicaPublicationResolvedTarget) {
			value.ReplicaID = replicaPublicationTestCreatorID
		},
		func(value *api.WebsiteReplicaPublicationResolvedTarget) { value.ProductID = nil },
		func(value *api.WebsiteReplicaPublicationResolvedTarget) {
			wrong := replicaPublicationTestCreatorID
			value.ProductID = &wrong
		},
	} {
		candidate := target
		mutate(&candidate)
		if replicaResolvedTargetMatchesRequest(candidate, confirmation, request) {
			t.Fatalf("mismatched managed target was accepted: %#v", candidate)
		}
	}

	newRequest := api.CreateWebsiteReplicaPublicationRequest{Target: api.WebsiteReplicaPublicationTarget{Kind: "NEW_WORK", Slug: "replica-site"}}
	newTarget := target
	newTarget.Resolution = "CREATE"
	newConfirmation := &api.WebsiteReplicaPublicationConfirmationChallenge{Review: review}
	newConfirmation.Review.Resolution = "CREATE"
	if !replicaResolvedTargetMatchesRequest(newTarget, newConfirmation, newRequest) {
		t.Fatal("matching new-Work slug was rejected")
	}
	newTarget.WorkURL = "https://viceme.cn/replica-maker/different-site"
	newConfirmation.Review.WorkURL = newTarget.WorkURL
	if replicaResolvedTargetMatchesRequest(newTarget, newConfirmation, newRequest) {
		t.Fatal("resolved new-Work slug that differs from the request was accepted")
	}
}

func TestReplicaPublicationRecoveryRejectsResultMetadataMismatch(t *testing.T) {
	digest := strings.Repeat("a", 64)
	pending := replicapublication.Pending{
		Market: "CN", ClientRequestID: replicaPublicationTestRequestID,
		Request:       api.CreateWebsiteReplicaPublicationRequest{Title: "Replica title", PriceCents: 990},
		SourceArchive: replicacontent.SourceArchiveSummary{Digest: digest, SizeBytes: 1024},
		Publication:   &replicapublication.PublicationReference{ID: replicaPublicationTestID},
		Confirmation: &api.WebsiteReplicaPublicationConfirmationChallenge{Review: api.WebsiteReplicaPublicationReview{
			MerchantAccountID: replicaPublicationTestMerchantID, WorkURL: "https://viceme.cn/replica-maker/replica-site",
		}},
	}
	publication := api.WebsiteReplicaPublication{
		ID: replicaPublicationTestID, ClientRequestID: replicaPublicationTestRequestID, Market: "CN",
		MerchantAccountID: replicaPublicationTestMerchantID,
		Source:            api.WebsiteReplicaPublicationSource{Digest: digest, SizeBytes: 1024},
		Result: &api.WebsiteReplicaPublicationResult{
			WorkURL: "https://viceme.cn/replica-maker/replica-site",
			Product: api.WebsiteReplicaProduct{Title: "Replica title", Currency: "CNY", PriceCents: 990},
		},
	}
	if err := validateReplicaPublicationRecovery(pending, publication); err != nil {
		t.Fatalf("matching terminal metadata was rejected: %v", err)
	}
	validResult := *publication.Result
	for _, mutate := range []func(*api.WebsiteReplicaPublicationResult){
		func(result *api.WebsiteReplicaPublicationResult) { result.Product.Title = "Different title" },
		func(result *api.WebsiteReplicaPublicationResult) { result.Product.Currency = "USD" },
		func(result *api.WebsiteReplicaPublicationResult) { result.Product.PriceCents = 1 },
	} {
		candidate := validResult
		mutate(&candidate)
		publication.Result = &candidate
		if err := validateReplicaPublicationRecovery(pending, publication); err == nil {
			t.Fatalf("mismatched terminal metadata was accepted: %#v", candidate.Product)
		}
	}
}

func TestReplicaPublishRequiresExplicitMerchantSelectionBeforeFinalReview(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	var createCalls int
	var requestID string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		createCalls++
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if createCalls == 1 {
			requestID, _ = input["clientRequestId"].(string)
			if _, selected := input["merchantAccountId"]; selected {
				t.Fatalf("first request unexpectedly selected a Merchant: %#v", input)
			}
			writeJSONResponse(writer, map[string]any{
				"outcome": "ACTION_REQUIRED", "clientRequestId": requestID, "market": "CN",
				"nextAction": map[string]any{
					"kind": "SELECT_MERCHANT",
					"merchants": []map[string]any{
						{"id": replicaPublicationTestMerchantID, "displayName": "Replica Studio", "creatorHandle": "replica-maker"},
						{"id": replicaPublicationTestCreatorID, "displayName": "Second Studio", "creatorHandle": "second-maker"},
					},
				},
			})
			return
		}
		if input["clientRequestId"] != requestID || input["merchantAccountId"] != replicaPublicationTestMerchantID {
			t.Fatalf("Merchant recovery changed the request or selected the wrong Merchant: %#v", input)
		}
		writeJSONResponse(writer, replicaConfirmationRequiredResponse(now, input, "wrv1-9090909090909090909090909090909090909090909090909090909090909090"))
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	root := t.TempDir()
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	dependencies.NewID = func() string { return replicaPublicationTestRequestID }
	arguments := replicaPublicationTestArguments(project, "replica-site")
	var selectionOutput bytes.Buffer
	dependencies.Out = &selectionOutput
	if exit := Execute(arguments, dependencies); exit != output.ExitConfirmation ||
		!strings.Contains(selectionOutput.String(), "REPLICA_PUBLICATION_MERCHANT_REQUIRED") {
		t.Fatalf("multiple Merchants did not stop for selection: exit=%d output=%s", exit, selectionOutput.String())
	}
	var reviewOutput bytes.Buffer
	dependencies.Out = &reviewOutput
	selected := append(append([]string{}, arguments...), "--merchant-id", replicaPublicationTestMerchantID)
	if exit := Execute(selected, dependencies); exit != output.ExitConfirmation ||
		!strings.Contains(reviewOutput.String(), "REPLICA_PUBLICATION_CONFIRMATION_REQUIRED") {
		t.Fatalf("selected Merchant did not reach final review: exit=%d output=%s", exit, reviewOutput.String())
	}
	if createCalls != 2 {
		t.Fatalf("Merchant selection created an unexpected request count: %d", createCalls)
	}
}

func TestReplicaPublishPresentsDifferentPendingPublicationFromCheckStatusAction(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	existingPublicationID := "abababab-abab-4bab-8bab-abababababab"
	existingRequestID := "bcbcbcbc-bcbc-4cbc-8cbc-bcbcbcbcbcbc"
	createCalls, getCalls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications":
			createCalls++
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			writeJSONResponse(writer, map[string]any{
				"outcome": "ACTION_REQUIRED", "clientRequestId": input["clientRequestId"], "market": "CN",
				"nextAction": map[string]any{
					"kind": "CHECK_STATUS", "publicationId": existingPublicationID,
					"statusUrl": "https://viceme.cn/me/website-replica-publications/" + existingPublicationID,
				},
			})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/website-replica-publications/"+existingPublicationID:
			getCalls++
			publication := replicaPublicationAPIResponse(now, "PROCESSING", "VERIFIED")
			setReplicaPublicationIdentity(publication, existingPublicationID, existingRequestID)
			writeJSONResponse(writer, publication)
		default:
			t.Fatalf("unexpected CHECK_STATUS request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	root := t.TempDir()
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	dependencies.NewID = func() string { return replicaPublicationTestRequestID }
	var stdout bytes.Buffer
	dependencies.Out = &stdout
	if exit := Execute(replicaPublicationTestArguments(project, "replica-site"), dependencies); exit != 0 ||
		!strings.Contains(stdout.String(), existingPublicationID) ||
		!strings.Contains(stdout.String(), `"phase": "SUBMITTED_NOT_PUBLISHED"`) {
		t.Fatalf("existing pending Publication was not presented: exit=%d output=%s", exit, stdout.String())
	}
	if createCalls != 1 || getCalls != 1 {
		t.Fatalf("unexpected CHECK_STATUS flow: creates=%d gets=%d", createCalls, getCalls)
	}
	states, err := filepath.Glob(filepath.Join(root, "config", "replica-publications", "*", "pending-*.json"))
	if err != nil || len(states) != 1 {
		t.Fatalf("new local request identity was not retained separately: files=%v err=%v", states, err)
	}
	state, err := os.ReadFile(states[0])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(state, []byte(existingPublicationID)) || !bytes.Contains(state, []byte(replicaPublicationTestRequestID)) {
		t.Fatalf("different pending Publication replaced the local request identity: %s", state)
	}
}

func TestReplicaPublishRejectsSensitiveSourceBeforePreviewOrRemoteRequest(t *testing.T) {
	project := newReplicaPublicationTestProject(t)
	writeReplicaSourceFile(t, project, "src/config.ts", `const apiKey = "sk-proj-abcdefghijklmnopqrstuvwxyz"`)
	root := t.TempDir()
	remoteCalls := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		remoteCalls.Add(1)
	}))
	defer server.Close()

	previewCalls := atomic.Int32{}
	dependencies := replicaPublicationTestDependencies(t, root, server, time.Now().UTC())
	dependencies.StartReplicaPreview = func(context.Context, replicapreview.Options) (replicapreview.Running, error) {
		previewCalls.Add(1)
		return nil, errors.New("preview must not start for unsafe source")
	}
	dependencies.NewID = func() string {
		t.Fatal("unsafe source reached request identity creation")
		return ""
	}
	var stdout bytes.Buffer
	dependencies.Out = &stdout
	if exit := Execute(replicaPublicationTestArguments(project, "replica-site"), dependencies); exit != output.ExitValidation ||
		!strings.Contains(stdout.String(), "REPLICA_SENSITIVE_CONTENT") {
		t.Fatalf("sensitive source was not rejected locally: exit=%d output=%s", exit, stdout.String())
	}
	if previewCalls.Load() != 0 || remoteCalls.Load() != 0 {
		t.Fatalf("unsafe source crossed a boundary: previews=%d remote=%d", previewCalls.Load(), remoteCalls.Load())
	}
}

func TestReplicaPublishAcceptsValidatedZIPWithAgentPreview(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "replica-source.zip")
	archive := replicaTestZIP(t, map[string]string{
		"package.json": `{"scripts":{"dev":"vite"}}`,
		"index.html":   "<h1>ZIP Replica</h1>",
	})
	if err := os.WriteFile(sourcePath, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	confirmationVersion := "wrv1-b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2"
	remoteCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		remoteCalls++
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		writeJSONResponse(writer, replicaConfirmationRequiredResponse(now, input, confirmationVersion))
	}))
	defer server.Close()

	root := t.TempDir()
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	dependencies.NewID = func() string { return replicaPublicationTestRequestID }
	previewCalled := false
	dependencies.StartReplicaPreview = func(_ context.Context, options replicapreview.Options) (replicapreview.Running, error) {
		previewCalled = true
		if options.ProjectPath != "" || options.ExistingURL != "http://127.0.0.1:4173/" {
			t.Fatalf("CLI attempted to infer ZIP entry: %#v", options)
		}
		return &replicaPreviewSessionStub{result: replicapreview.Result{TargetURL: options.ExistingURL, Reused: true, ServiceKind: replicapreview.ServiceExisting}}, nil
	}
	var stdout bytes.Buffer
	dependencies.Out = &stdout
	if exit := Execute(replicaPublicationTestArguments(sourcePath, "replica-site"), dependencies); exit != output.ExitConfirmation {
		t.Fatalf("validated ZIP did not reach final review: exit=%d output=%s", exit, stdout.String())
	}
	if remoteCalls != 1 || !previewCalled {
		t.Fatalf("unexpected calls: %d", remoteCalls)
	}
	artifacts, err := filepath.Glob(filepath.Join(root, "config", "replica-publications", "*", "artifacts", "*", "source.zip"))
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("ZIP final review did not retain one frozen source: files=%v err=%v", artifacts, err)
	}
	frozen, err := os.ReadFile(artifacts[0])
	if err != nil || !bytes.Equal(frozen, archive) {
		t.Fatalf("ZIP publication did not freeze the exact validated bytes: equal=%v err=%v", bytes.Equal(frozen, archive), err)
	}
}

func TestReplicaPublishRejectsUnsafeZIPBeforePreviewOrRemoteRequest(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	sourcePath := filepath.Join(t.TempDir(), "unsafe.zip")
	if err := os.WriteFile(sourcePath, rawReplicaTestZIP(t, map[string]string{
		"../escape.txt":                   "escape",
		replicacontent.ProjectHandoffFile: replicaTestProjectHandoff,
	}), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unsafe ZIP reached the Shop API")
	}))
	defer server.Close()

	root := t.TempDir()
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	dependencies.NewID = func() string { return replicaPublicationTestRequestID }
	previewCalls := 0
	dependencies.StartReplicaPreview = func(context.Context, replicapreview.Options) (replicapreview.Running, error) {
		previewCalls++
		return nil, errors.New("unsafe ZIP reached preview")
	}
	var stdout bytes.Buffer
	dependencies.Out = &stdout
	if exit := Execute(replicaPublicationTestArguments(sourcePath, "replica-site"), dependencies); exit != output.ExitValidation ||
		!strings.Contains(stdout.String(), "REPLICA_ARCHIVE_INVALID") {
		t.Fatalf("unsafe ZIP was not rejected locally: exit=%d output=%s", exit, stdout.String())
	}
	if previewCalls != 0 {
		t.Fatalf("unsafe ZIP reached preview %d times", previewCalls)
	}
}

func TestReplicaPublishRequiresExplicitReplicaOnlyFallbackWhenPreviewFails(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	remoteCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		remoteCalls++
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		writeJSONResponse(writer, replicaConfirmationRequiredResponse(now, input, "wrv1-a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1"))
	}))
	defer server.Close()

	root := t.TempDir()
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	dependencies.NewID = func() string { return replicaPublicationTestRequestID }
	dependencies.StartReplicaPreview = func(context.Context, replicapreview.Options) (replicapreview.Running, error) {
		return nil, &replicapreview.StartError{
			Code: "REPLICA_PREVIEW_COMMAND_FAILED", Stage: replicapreview.StageStarting,
			Message: "test project could not start", Cause: errors.New("missing dependency"),
		}
	}
	arguments := replicaPublicationTestArguments(project, "replica-site")
	var blockedOutput bytes.Buffer
	dependencies.Out = &blockedOutput
	if exit := Execute(arguments, dependencies); exit != output.ExitConfirmation ||
		!strings.Contains(blockedOutput.String(), "CONFIRM_UNVERIFIED_REPLICA_ONLY") ||
		!strings.Contains(blockedOutput.String(), "--confirm-unverified-replica-only") {
		t.Fatalf("failed preview did not require explicit fallback: exit=%d output=%s", exit, blockedOutput.String())
	}
	if remoteCalls != 0 {
		t.Fatalf("failed preview reached Shop without fallback authorization: calls=%d", remoteCalls)
	}

	var reviewOutput bytes.Buffer
	dependencies.Out = &reviewOutput
	fallback := append(append([]string{}, arguments...), "--preview-reviewed=false", "--confirm-unverified-replica-only")
	if exit := Execute(fallback, dependencies); exit != output.ExitConfirmation ||
		!strings.Contains(reviewOutput.String(), `"hosting": "REPLICA_ONLY"`) ||
		!strings.Contains(reviewOutput.String(), `"verified": false`) {
		t.Fatalf("explicit Replica-only fallback did not reach final review: exit=%d output=%s", exit, reviewOutput.String())
	}
	if remoteCalls != 1 {
		t.Fatalf("explicit fallback made %d Shop requests", remoteCalls)
	}
}

func TestReplicaResumeContinuesInterruptedUploadFromAuthoritativeSourceState(t *testing.T) {
	for _, projectStorage := range []bool{false, true} {
		t.Run(fmt.Sprintf("project-storage-%t", projectStorage), func(t *testing.T) {
			testReplicaResumeContinuesInterruptedUploadFromAuthoritativeSourceStateStorage(t, projectStorage)
		})
	}
}
func testReplicaResumeContinuesInterruptedUploadFromAuthoritativeSourceStateStorage(t *testing.T, projectStorage bool) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	confirmationVersion := "wrv1-1212121212121212121212121212121212121212121212121212121212121212"
	var uploadBodies [][]byte
	objectServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.Header.Get("Authorization") != "" {
			t.Fatalf("invalid presigned upload request: %s auth=%q", request.Method, request.Header.Get("Authorization"))
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		uploadBodies = append(uploadBodies, body)
		if len(uploadBodies) == 1 {
			http.Error(writer, "temporary interruption", http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer objectServer.Close()

	var createCalls, authorizationCalls, completeCalls, submitCalls int
	var source map[string]any
	controlServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications":
			createCalls++
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			source = input["source"].(map[string]any)
			if createCalls == 1 {
				writeJSONResponse(writer, replicaConfirmationRequiredResponse(now, input, confirmationVersion))
				return
			}
			writeJSONResponse(writer, map[string]any{
				"outcome": "PUBLICATION_READY",
				"target": map[string]any{
					"resolution": "CREATE", "merchantAccountId": replicaPublicationTestMerchantID,
					"workId": replicaPublicationTestWorkID, "replicaId": replicaPublicationTestReplicaID,
					"productId": nil, "workUrl": "https://viceme.cn/replica-maker/replica-site",
				},
				"publication": replicaPublicationForSource(now, "DRAFT", "WAITING_UPLOAD", source),
				"nextAction":  map[string]any{"kind": "AUTHORIZE_SOURCE_UPLOAD", "publicationId": replicaPublicationTestID},
			})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID:
			writeJSONResponse(writer, replicaPublicationForSource(now, "DRAFT", "WAITING_UPLOAD", source))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID+"/source/upload-authorizations":
			authorizationCalls++
			writeJSONResponse(writer, map[string]any{
				"publicationId": replicaPublicationTestID,
				"upload": map[string]any{
					"method": "PUT", "url": objectServer.URL + "/source.zip",
					"headers":   map[string]string{"Content-Type": "application/zip"},
					"expiresAt": now.Add(5 * time.Minute).Format(time.RFC3339),
				},
			})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID+"/source/complete-upload":
			completeCalls++
			writeJSONResponse(writer, replicaPublicationForSource(now, "DRAFT", "VERIFIED", source))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID+"/submit":
			submitCalls++
			writeJSONResponse(writer, replicaPublicationForSource(now, "PROCESSING", "VERIFIED", source))
		default:
			t.Fatalf("unexpected recovery request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer controlServer.Close()

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	root := t.TempDir()
	storageRoot := filepath.Join(root, "config", "replica-publications")
	storageArgs := []string{}
	if projectStorage {
		storageRoot = filepath.Join(project, ".viceme", "publications")
		storageArgs = []string{"--state-project", project}
		original := privatefile.ReplaceFile
		privatefile.ReplaceFile = func(from, to string) error {
			if strings.HasPrefix(to, filepath.Join(root, "config")+string(filepath.Separator)) {
				return syscall.EPERM
			}
			return original(from, to)
		}
		t.Cleanup(func() { privatefile.ReplaceFile = original })
	}
	dependencies := replicaPublicationTestDependencies(t, root, controlServer, now)
	dependencies.NewID = func() string { return replicaPublicationTestRequestID }
	arguments := append(replicaPublicationTestArguments(project, "replica-site"), storageArgs...)
	dependencies.Out = &bytes.Buffer{}
	if exit := Execute(arguments, dependencies); exit != output.ExitConfirmation {
		t.Fatalf("publication did not reach final review: exit=%d", exit)
	}
	var interruptedOutput bytes.Buffer
	dependencies.Out = &interruptedOutput
	confirmed := append(append([]string{}, arguments...), "--confirm", confirmationVersion)
	if exit := Execute(confirmed, dependencies); exit != output.ExitNetwork ||
		!strings.Contains(interruptedOutput.String(), `"command": "viceme replica resume `+replicaPublicationTestID) {
		t.Fatalf("interrupted upload was not recoverable: exit=%d output=%s", exit, interruptedOutput.String())
	}
	artifacts, err := filepath.Glob(filepath.Join(storageRoot, "*", "artifacts", "*", "source.zip"))
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("interrupted upload did not retain one frozen source: files=%v err=%v", artifacts, err)
	}
	assertReplicaSecretsAbsentFromFiles(t, root, replicaPublicationTestAccessToken, objectServer.URL, "Content-Type")

	dependencies.StartReplicaPreview = func(context.Context, replicapreview.Options) (replicapreview.Running, error) {
		t.Fatal("replica resume must not restart local preview")
		return nil, nil
	}
	var resumedOutput bytes.Buffer
	dependencies.Out = &resumedOutput
	if exit := Execute(append([]string{"replica", "resume", replicaPublicationTestID}, storageArgs...), dependencies); exit != 0 ||
		!strings.Contains(resumedOutput.String(), `"status": "PROCESSING"`) {
		t.Fatalf("upload resume did not reach PROCESSING: exit=%d output=%s", exit, resumedOutput.String())
	}
	if createCalls != 2 || authorizationCalls != 2 || completeCalls != 1 || submitCalls != 1 || len(uploadBodies) != 2 ||
		!bytes.Equal(uploadBodies[0], uploadBodies[1]) {
		t.Fatalf("resume repeated or changed a completed step: creates=%d auth=%d complete=%d submit=%d uploads=%d equal=%v",
			createCalls, authorizationCalls, completeCalls, submitCalls, len(uploadBodies), len(uploadBodies) == 2 && bytes.Equal(uploadBodies[0], uploadBodies[1]))
	}
	artifacts, err = filepath.Glob(filepath.Join(storageRoot, "*", "artifacts", "*", "source.zip"))
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("platform takeover did not clean the frozen source: files=%v err=%v", artifacts, err)
	}
}

func TestReplicaCancelRemovesRecoverableDraftAndFrozenSource(t *testing.T) {
	for _, projectStorage := range []bool{false, true} {
		t.Run(fmt.Sprintf("project-storage-%t", projectStorage), func(t *testing.T) { testReplicaCancelRemovesRecoverableDraftAndFrozenSourceStorage(t, projectStorage) })
	}
}
func testReplicaCancelRemovesRecoverableDraftAndFrozenSourceStorage(t *testing.T, projectStorage bool) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	confirmationVersion := "wrv1-3434343434343434343434343434343434343434343434343434343434343434"
	objectServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		http.Error(writer, "keep draft recoverable", http.StatusServiceUnavailable)
	}))
	defer objectServer.Close()

	var createCalls, cancelCalls int
	var source map[string]any
	controlServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications":
			createCalls++
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			source = input["source"].(map[string]any)
			if createCalls == 1 {
				writeJSONResponse(writer, replicaConfirmationRequiredResponse(now, input, confirmationVersion))
				return
			}
			writeJSONResponse(writer, map[string]any{
				"outcome": "PUBLICATION_READY",
				"target": map[string]any{
					"resolution": "CREATE", "merchantAccountId": replicaPublicationTestMerchantID,
					"workId": replicaPublicationTestWorkID, "replicaId": replicaPublicationTestReplicaID,
					"productId": nil, "workUrl": "https://viceme.cn/replica-maker/replica-site",
				},
				"publication": replicaPublicationForSource(now, "DRAFT", "WAITING_UPLOAD", source),
				"nextAction":  map[string]any{"kind": "AUTHORIZE_SOURCE_UPLOAD", "publicationId": replicaPublicationTestID},
			})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID+"/source/upload-authorizations":
			writeJSONResponse(writer, map[string]any{
				"publicationId": replicaPublicationTestID,
				"upload": map[string]any{
					"method": "PUT", "url": objectServer.URL + "/source.zip", "headers": map[string]string{},
					"expiresAt": now.Add(5 * time.Minute).Format(time.RFC3339),
				},
			})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID+"/cancel":
			cancelCalls++
			writeJSONResponse(writer, replicaPublicationForSource(now, "CANCELLED", "WAITING_UPLOAD", source))
		default:
			t.Fatalf("unexpected cancellation request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer controlServer.Close()

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	root := t.TempDir()
	storageRoot := filepath.Join(root, "config", "replica-publications")
	storageArgs := []string{}
	if projectStorage {
		storageRoot = filepath.Join(project, ".viceme", "publications")
		storageArgs = []string{"--state-project", project}
		original := privatefile.ReplaceFile
		privatefile.ReplaceFile = func(from, to string) error {
			if strings.HasPrefix(to, filepath.Join(root, "config")+string(filepath.Separator)) {
				return syscall.EPERM
			}
			return original(from, to)
		}
		t.Cleanup(func() { privatefile.ReplaceFile = original })
	}
	dependencies := replicaPublicationTestDependencies(t, root, controlServer, now)
	dependencies.NewID = func() string { return replicaPublicationTestRequestID }
	arguments := append(replicaPublicationTestArguments(project, "replica-site"), storageArgs...)
	dependencies.Out = &bytes.Buffer{}
	if exit := Execute(arguments, dependencies); exit != output.ExitConfirmation {
		t.Fatalf("publication did not reach final review: exit=%d", exit)
	}
	dependencies.Out = &bytes.Buffer{}
	if exit := Execute(append(append([]string{}, arguments...), "--confirm", confirmationVersion), dependencies); exit != output.ExitNetwork {
		t.Fatalf("test publication did not stop as a recoverable draft: exit=%d", exit)
	}

	if projectStorage {
		original := privatefile.ReplaceFile
		privatefile.ReplaceFile = func(string, string) error { return syscall.EPERM }
		var denied bytes.Buffer
		dependencies.Out = &denied
		exit := Execute(append([]string{"replica", "cancel", replicaPublicationTestID}, storageArgs...), dependencies)
		privatefile.ReplaceFile = original
		if exit != output.ExitPolicy || cancelCalls != 0 {
			t.Fatalf("denied cancellation mutated remote state: exit=%d calls=%d %s", exit, cancelCalls, denied.String())
		}
	}
	var cancelledOutput bytes.Buffer
	dependencies.Out = &cancelledOutput
	if exit := Execute(append([]string{"replica", "cancel", replicaPublicationTestID}, storageArgs...), dependencies); exit != 0 ||
		!strings.Contains(cancelledOutput.String(), `"status": "CANCELLED"`) {
		t.Fatalf("draft cancellation failed: exit=%d output=%s", exit, cancelledOutput.String())
	}
	if cancelCalls != 1 {
		t.Fatalf("cancel endpoint was called %d times", cancelCalls)
	}
	artifacts, err := filepath.Glob(filepath.Join(storageRoot, "*", "artifacts", "*", "source.zip"))
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("cancelled draft retained frozen source: files=%v err=%v", artifacts, err)
	}
	states, err := filepath.Glob(filepath.Join(storageRoot, "*", "pending-*.json"))
	if err != nil || len(states) != 0 {
		t.Fatalf("cancelled draft retained pending state: files=%v err=%v", states, err)
	}
	if _, err := os.Stat(filepath.Join(project, ".viceme", "website-replica.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-takeover cancellation created a managed binding: err=%v", err)
	}
}

func TestReplicaCancelAfterPlatformTakeoverUpdatesBindingAndCleansRecovery(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	root := t.TempDir()
	cancelCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/website-replica-publications/"+replicaPublicationTestID+"/cancel" {
			t.Fatalf("unexpected taken-over cancellation request: %s %s", request.Method, request.URL.Path)
		}
		cancelCalls++
		writeJSONResponse(writer, replicaPublicationAPIResponse(now, "CANCELLED", "VERIFIED"))
	}))
	defer server.Close()

	fingerprint, canonicalProject, err := replicapublication.ProjectFingerprint(server.URL, "CN", project)
	if err != nil {
		t.Fatal(err)
	}
	summary := replicacontent.SourceArchiveSummary{
		Digest: replicaPublicationTestSourceDigest, SizeBytes: 1024,
		IncludedFileCount: 1, IncludedBytes: 1024, IncludedPaths: []string{"index.html"},
		ExcludedPaths: []replicacontent.SourceArchiveExclusion{},
	}
	pending := replicapublication.Pending{
		EndpointOrigin: server.URL, Market: "CN", ProjectPath: canonicalProject,
		ProjectFingerprint: fingerprint, ClientRequestID: replicaPublicationTestRequestID,
		Request: api.CreateWebsiteReplicaPublicationRequest{
			ProtocolVersion: api.WebsiteReplicaPublicationProtocolVersion, ClientRequestID: replicaPublicationTestRequestID,
			Market: "CN", ProjectFingerprint: fingerprint, Target: api.WebsiteReplicaPublicationTarget{Kind: "NEW_WORK", Slug: "replica-site"},
			Title: "Replica title", Summary: "Replica summary", PriceCents: 990,
			Source: api.WebsiteReplicaPublicationSourceArtifact{
				FileName: "source.zip", ContentType: "application/zip", SizeBytes: summary.SizeBytes, Digest: summary.Digest,
			},
		},
		SourceArchive: summary, ArtifactExpiresAt: now.Add(30 * time.Minute),
		Publication: &replicapublication.PublicationReference{
			ID: replicaPublicationTestID, Status: "PROCESSING",
			StatusURL: "https://viceme.cn/me/website-replica-publications/" + replicaPublicationTestID,
		},
		TakenOver: true,
	}
	stateStore := replicapublication.Store{
		Directory:      replicapublication.ScopedDirectory(filepath.Join(root, "config", "replica-publications"), server.URL, "CN"),
		EndpointOrigin: server.URL, Market: "CN", Now: func() time.Time { return now },
	}
	if err := stateStore.Save(&pending); err != nil {
		t.Fatal(err)
	}

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	var stdout bytes.Buffer
	dependencies.Out = &stdout
	if exit := Execute([]string{"replica", "cancel", replicaPublicationTestID}, dependencies); exit != 0 ||
		!strings.Contains(stdout.String(), `"status": "CANCELLED"`) {
		t.Fatalf("taken-over cancellation failed: exit=%d output=%s", exit, stdout.String())
	}
	if cancelCalls != 1 {
		t.Fatalf("taken-over cancellation calls=%d", cancelCalls)
	}
	bindingData, err := os.ReadFile(filepath.Join(project, ".viceme", "website-replica.json"))
	if err != nil || !bytes.Contains(bindingData, []byte(`"status": "CANCELLED"`)) {
		t.Fatalf("taken-over cancellation did not update its binding: data=%s err=%v", bindingData, err)
	}
	states, err := filepath.Glob(filepath.Join(root, "config", "replica-publications", "*", "pending-*.json"))
	if err != nil || len(states) != 0 {
		t.Fatalf("taken-over cancellation retained recovery state: files=%v err=%v", states, err)
	}
}

func TestReplicaResumeRetriesOnlyWhenAuthoritativeActionsAllowIt(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	var getCalls, retryCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID:
			getCalls++
			writeJSONResponse(writer, replicaPublicationAPIResponse(now, "FAILED", "VERIFIED"))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID+"/retry":
			retryCalls++
			assertEmptyJSONObject(t, request)
			writeJSONResponse(writer, replicaPublicationAPIResponse(now, "PROCESSING", "VERIFIED"))
		default:
			t.Fatalf("unexpected retry request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	root := t.TempDir()
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	var stdout bytes.Buffer
	dependencies.Out = &stdout
	if exit := Execute([]string{"replica", "resume", replicaPublicationTestID}, dependencies); exit != 0 ||
		!strings.Contains(stdout.String(), `"status": "PROCESSING"`) || !strings.Contains(stdout.String(), `"phase": "SUBMITTED_NOT_PUBLISHED"`) {
		t.Fatalf("retryable Publication did not resume: exit=%d output=%s", exit, stdout.String())
	}
	if getCalls != 1 || retryCalls != 1 {
		t.Fatalf("unexpected retry flow: gets=%d retries=%d", getCalls, retryCalls)
	}
}

func TestReplicaResumeDoesNotRetryNonRetryableFailureAndCleansRecoveryState(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	root := t.TempDir()
	retryCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID:
			response := replicaPublicationAPIResponse(now, "FAILED", "VERIFIED")
			response["allowedActions"] = []string{}
			response["failure"].(map[string]any)["retryable"] = false
			writeJSONResponse(writer, response)
		case strings.HasSuffix(request.URL.Path, "/retry"):
			retryCalls++
			t.Fatal("non-retryable Publication reached retry endpoint")
		default:
			t.Fatalf("unexpected non-retryable recovery request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	fingerprint, canonicalProject, err := replicapublication.ProjectFingerprint(server.URL, "CN", project)
	if err != nil {
		t.Fatal(err)
	}
	summary := replicacontent.SourceArchiveSummary{
		Digest: replicaPublicationTestSourceDigest, SizeBytes: 1024,
		IncludedFileCount: 1, IncludedBytes: 1024, IncludedPaths: []string{"index.html"},
		ExcludedPaths: []replicacontent.SourceArchiveExclusion{},
	}
	pending := replicapublication.Pending{
		EndpointOrigin: server.URL, Market: "CN", ProjectPath: canonicalProject,
		ProjectFingerprint: fingerprint, ClientRequestID: replicaPublicationTestRequestID,
		Request: api.CreateWebsiteReplicaPublicationRequest{
			ProtocolVersion: api.WebsiteReplicaPublicationProtocolVersion, ClientRequestID: replicaPublicationTestRequestID,
			Market: "CN", ProjectFingerprint: fingerprint, Target: api.WebsiteReplicaPublicationTarget{Kind: "NEW_WORK", Slug: "replica-site"},
			Title: "Replica title", Summary: "Replica summary", PriceCents: 990,
			Source: api.WebsiteReplicaPublicationSourceArtifact{
				FileName: "source.zip", ContentType: "application/zip", SizeBytes: summary.SizeBytes, Digest: summary.Digest,
			},
		},
		SourceArchive: summary, ArtifactExpiresAt: now.Add(30 * time.Minute),
		Publication: &replicapublication.PublicationReference{
			ID: replicaPublicationTestID, Status: "PROCESSING",
			StatusURL: "https://viceme.cn/me/website-replica-publications/" + replicaPublicationTestID,
		},
		TakenOver: true,
	}
	stateStore := replicapublication.Store{
		Directory:      replicapublication.ScopedDirectory(filepath.Join(root, "config", "replica-publications"), server.URL, "CN"),
		EndpointOrigin: server.URL, Market: "CN", Now: func() time.Time { return now },
	}
	if err := stateStore.Save(&pending); err != nil {
		t.Fatal(err)
	}

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	var stdout bytes.Buffer
	dependencies.Out = &stdout
	if exit := Execute([]string{"replica", "resume", replicaPublicationTestID}, dependencies); exit != 0 ||
		!strings.Contains(stdout.String(), `"status": "FAILED"`) {
		t.Fatalf("non-retryable failure was not presented: exit=%d output=%s", exit, stdout.String())
	}
	if retryCalls != 0 {
		t.Fatalf("non-retryable Publication was retried %d times", retryCalls)
	}
	states, err := filepath.Glob(filepath.Join(root, "config", "replica-publications", "*", "pending-*.json"))
	if err != nil || len(states) != 0 {
		t.Fatalf("non-retryable terminal failure retained recovery state: files=%v err=%v", states, err)
	}
}

func TestReplicaStatusCompletesStableBindingAndUpdateDefaultsCurrentPrice(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	root := t.TempDir()
	var updateCreateCalls int
	updateRequestID := "abababab-abab-4bab-8bab-abababababab"
	updatePublicationID := "bcbcbcbc-bcbc-4cbc-8cbc-bcbcbcbcbcbc"
	confirmationVersion := "wrv1-5656565656565656565656565656565656565656565656565656565656565656"
	var updateSource map[string]any
	var uploaded []byte
	objectServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.Path != "/source.zip" {
			t.Fatalf("unexpected update object request: %s %s", request.Method, request.URL.Path)
		}
		var err error
		uploaded, err = io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer objectServer.Close()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID:
			writeJSONResponse(writer, replicaPublicationAPIResponse(now, "PUBLISHED", "ACTIVATED"))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications":
			updateCreateCalls++
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			target, _ := input["target"].(map[string]any)
			if target["kind"] != "MANAGED_BINDING" || target["workId"] != replicaPublicationTestWorkID ||
				target["replicaId"] != replicaPublicationTestReplicaID || target["productId"] != replicaPublicationTestProductID ||
				input["merchantAccountId"] != replicaPublicationTestMerchantID || input["priceCents"] != float64(990) {
				t.Fatalf("update did not reuse the stable binding and current price: %#v", input)
			}
			updateSource = input["source"].(map[string]any)
			if updateCreateCalls == 1 {
				response := replicaConfirmationRequiredResponse(now, input, confirmationVersion)
				review := response["nextAction"].(map[string]any)["confirmation"].(map[string]any)["review"].(map[string]any)
				review["resolution"] = "UPDATE"
				writeJSONResponse(writer, response)
				return
			}
			confirmation, _ := input["confirmation"].(map[string]any)
			if confirmation["version"] != confirmationVersion || confirmation["confirmedAt"] == nil {
				t.Fatalf("managed update did not submit its exact confirmation: %#v", confirmation)
			}
			publication := replicaPublicationForSource(now, "DRAFT", "WAITING_UPLOAD", updateSource)
			setReplicaPublicationIdentity(publication, updatePublicationID, updateRequestID)
			writeJSONResponse(writer, map[string]any{
				"outcome": "PUBLICATION_READY",
				"target": map[string]any{
					"resolution": "UPDATE", "merchantAccountId": replicaPublicationTestMerchantID,
					"workId": replicaPublicationTestWorkID, "replicaId": replicaPublicationTestReplicaID,
					"productId": replicaPublicationTestProductID, "workUrl": "https://viceme.cn/replica-maker/replica-site",
				},
				"publication": publication,
				"nextAction":  map[string]any{"kind": "AUTHORIZE_SOURCE_UPLOAD", "publicationId": updatePublicationID},
			})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+updatePublicationID+"/source/upload-authorizations":
			writeJSONResponse(writer, map[string]any{
				"publicationId": updatePublicationID,
				"upload": map[string]any{
					"method": "PUT", "url": objectServer.URL + "/source.zip",
					"headers": map[string]string{"Content-Type": "application/zip"}, "expiresAt": now.Add(5 * time.Minute).Format(time.RFC3339),
				},
			})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+updatePublicationID+"/source/complete-upload":
			publication := replicaPublicationForSource(now, "DRAFT", "VERIFIED", updateSource)
			setReplicaPublicationIdentity(publication, updatePublicationID, updateRequestID)
			writeJSONResponse(writer, publication)
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+updatePublicationID+"/submit":
			publication := replicaPublicationForSource(now, "PROCESSING", "VERIFIED", updateSource)
			setReplicaPublicationIdentity(publication, updatePublicationID, updateRequestID)
			writeJSONResponse(writer, publication)
		default:
			t.Fatalf("unexpected terminal binding request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	fingerprint, canonicalProject, err := replicapublication.ProjectFingerprint(server.URL, "CN", project)
	if err != nil {
		t.Fatal(err)
	}
	sourceSummary := replicacontent.SourceArchiveSummary{
		Digest: replicaPublicationTestSourceDigest, SizeBytes: 1024,
		IncludedFileCount: 1, IncludedBytes: 1024, IncludedPaths: []string{"index.html"},
		ExcludedPaths: []replicacontent.SourceArchiveExclusion{},
	}
	submittedAt := now.Add(-2 * time.Minute).Format(time.RFC3339)
	verifiedAt := now.Add(-3 * time.Minute).Format(time.RFC3339)
	processing := api.WebsiteReplicaPublication{
		ID: replicaPublicationTestID, ClientRequestID: replicaPublicationTestRequestID, Market: "CN",
		MerchantAccountID: replicaPublicationTestMerchantID, WorkID: replicaPublicationTestWorkID,
		ReplicaID: replicaPublicationTestReplicaID, Status: "PROCESSING",
		StatusURL:      "https://viceme.cn/me/website-replica-publications/" + replicaPublicationTestID,
		AllowedActions: []string{"CANCEL"},
		Retry:          api.WebsiteReplicaPublicationRetry{MaxAutomaticRetries: 3},
		Source: api.WebsiteReplicaPublicationSource{
			FileName: "source.zip", ContentType: "application/zip", SizeBytes: 1024,
			Digest: replicaPublicationTestSourceDigest, Status: "VERIFIED", VerifiedAt: &verifiedAt,
		},
		SubmittedAt: &submittedAt, CreatedAt: now.Add(-5 * time.Minute).Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339),
	}
	pending := replicapublication.Pending{
		EndpointOrigin: server.URL, Market: "CN", ProjectPath: canonicalProject,
		ProjectFingerprint: fingerprint, ClientRequestID: replicaPublicationTestRequestID,
		Request: api.CreateWebsiteReplicaPublicationRequest{
			ProtocolVersion: api.WebsiteReplicaPublicationProtocolVersion, ClientRequestID: replicaPublicationTestRequestID,
			Market: "CN", ProjectFingerprint: fingerprint, Target: api.WebsiteReplicaPublicationTarget{Kind: "NEW_WORK", Slug: "replica-site"},
			Title: "Replica title", Summary: "Replica summary", PriceCents: 990,
			Source: api.WebsiteReplicaPublicationSourceArtifact{FileName: "source.zip", ContentType: "application/zip", SizeBytes: 1024, Digest: replicaPublicationTestSourceDigest},
		},
		SourceArchive: sourceSummary, ArtifactExpiresAt: now.Add(30 * time.Minute),
		Publication: &replicapublication.PublicationReference{
			ID: processing.ID, Status: processing.Status, StatusURL: processing.StatusURL,
		},
		TakenOver: true,
	}
	stateStore := replicapublication.Store{
		Directory:      replicapublication.ScopedDirectory(filepath.Join(root, "config", "replica-publications"), server.URL, "CN"),
		EndpointOrigin: server.URL, Market: "CN", Now: func() time.Time { return now },
	}
	if err := stateStore.Save(&pending); err != nil {
		t.Fatal(err)
	}
	if err := (replicapublication.BindingStore{EndpointOrigin: server.URL, Market: "CN", Now: func() time.Time { return now }}).
		Save(canonicalProject, pending, processing); err != nil {
		t.Fatal(err)
	}

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	dependencies.NewID = func() string { return updateRequestID }
	var statusOutput bytes.Buffer
	dependencies.Out = &statusOutput
	if exit := Execute([]string{"replica", "status", replicaPublicationTestID}, dependencies); exit != 0 ||
		!strings.Contains(statusOutput.String(), `"status": "PUBLISHED"`) {
		t.Fatalf("terminal status did not complete publication: exit=%d output=%s", exit, statusOutput.String())
	}
	bindingData, err := os.ReadFile(filepath.Join(project, ".viceme", "website-replica.json"))
	if err != nil {
		t.Fatal(err)
	}
	var binding map[string]any
	if err := json.Unmarshal(bindingData, &binding); err != nil {
		t.Fatal(err)
	}
	if binding["work"] == nil || binding["replica"] == nil || binding["product"] == nil || binding["version"] == nil ||
		binding["pageRelease"] != nil || binding["product"].(map[string]any)["priceCents"] != float64(990) {
		t.Fatalf("terminal binding omitted stable associations: %#v", binding)
	}
	states, err := filepath.Glob(filepath.Join(root, "config", "replica-publications", "*", "pending-*.json"))
	if err != nil || len(states) != 0 {
		t.Fatalf("terminal publication retained pending state: files=%v err=%v", states, err)
	}

	var updateOutput bytes.Buffer
	dependencies.Out = &updateOutput
	updateArguments := []string{
		"replica", "publish", "--path", project, "--title", "Updated replica", "--summary", "Updated summary", "--replica-only",
		"--preview-url", "http://127.0.0.1:4173/", "--preview-reviewed",
	}
	if exit := Execute(updateArguments, dependencies); exit != output.ExitConfirmation ||
		!strings.Contains(updateOutput.String(), `"resolution": "UPDATE"`) || !strings.Contains(updateOutput.String(), `"priceCents": 990`) ||
		!strings.Contains(updateOutput.String(), `"path": ".viceme"`) {
		t.Fatalf("managed update did not reach review with inherited price: exit=%d output=%s", exit, updateOutput.String())
	}
	if updateCreateCalls != 1 {
		t.Fatalf("managed update create calls=%d", updateCreateCalls)
	}
	artifacts, err := filepath.Glob(filepath.Join(root, "config", "replica-publications", "*", "artifacts", "*", "source.zip"))
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("managed update did not retain one confirmed source: files=%v err=%v", artifacts, err)
	}
	archive, err := os.ReadFile(artifacts[0])
	if err != nil {
		t.Fatal(err)
	}
	for name := range readReplicaZIP(t, archive) {
		if name == ".viceme" || strings.HasPrefix(name, ".viceme/") {
			t.Fatalf("local binding entered the source ZIP as %q", name)
		}
	}

	var submittedOutput bytes.Buffer
	dependencies.Out = &submittedOutput
	confirmedArguments := append(append([]string{}, updateArguments...), "--confirm", confirmationVersion)
	if exit := Execute(confirmedArguments, dependencies); exit != 0 ||
		!strings.Contains(submittedOutput.String(), `"status": "PROCESSING"`) {
		t.Fatalf("managed update did not upload and submit: exit=%d output=%s", exit, submittedOutput.String())
	}
	if updateCreateCalls != 2 || len(uploaded) == 0 {
		t.Fatalf("managed update create calls=%d uploaded=%d", updateCreateCalls, len(uploaded))
	}
	bindingData, err = os.ReadFile(filepath.Join(project, ".viceme", "website-replica.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(bindingData, &binding); err != nil {
		t.Fatal(err)
	}
	publicationBinding := binding["publication"].(map[string]any)
	if publicationBinding["id"] != updatePublicationID || publicationBinding["status"] != "PROCESSING" {
		t.Fatalf("managed update takeover did not replace the bound Publication: %#v", binding)
	}
}

func TestReplicaPublishAppliesForCreatorOnlyAfterExplicitAuthorization(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	applicationRequestID := "cdcdcdcd-cdcd-4dcd-8dcd-cdcdcdcdcdcd"
	initialConfirmationVersion := "wrv1-3434343434343434343434343434343434343434343434343434343434343434"
	authorizedConfirmationVersion := "wrv1-5656565656565656565656565656565656565656565656565656565656565656"
	var createCalls, applicationCalls int
	var mainRequestID string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+replicaPublicationTestAccessToken {
			t.Fatalf("creator flow was not authenticated: %q", request.Header.Get("Authorization"))
		}
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications":
			createCalls++
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if createCalls == 1 {
				mainRequestID, _ = input["clientRequestId"].(string)
			}
			if input["clientRequestId"] != mainRequestID {
				t.Fatalf("creator application changed main publication request: %#v", input)
			}
			switch createCalls {
			case 1:
				if input["confirmation"] != nil {
					t.Fatalf("initial creator publication crossed confirmation boundary: %#v", input)
				}
				writeJSONResponse(writer, replicaConfirmationRequiredResponse(now, input, initialConfirmationVersion))
				return
			case 2:
				confirmation, _ := input["confirmation"].(map[string]any)
				if confirmation["version"] != initialConfirmationVersion {
					t.Fatalf("creator qualification was checked before initial final review: %#v", input)
				}
			case 3:
				if input["confirmation"] != nil {
					t.Fatalf("changed automatic-application authorization reused an old review: %#v", input)
				}
				writeJSONResponse(writer, replicaConfirmationRequiredResponse(now, input, authorizedConfirmationVersion))
				return
			case 4:
				confirmation, _ := input["confirmation"].(map[string]any)
				if confirmation["version"] != authorizedConfirmationVersion {
					t.Fatalf("creator application did not use the authorized final review: %#v", input)
				}
			default:
				t.Fatalf("creator flow made an unexpected create call: %d", createCalls)
			}
			writeJSONResponse(writer, map[string]any{
				"outcome": "ACTION_REQUIRED", "clientRequestId": mainRequestID, "market": "CN",
				"nextAction": map[string]any{"kind": "APPLY_CREATOR", "applicationUrl": "https://viceme.cn/me/creator-center"},
			})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/cli/merchant/onboarding/applications":
			applicationCalls++
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input["clientRequestId"] != applicationRequestID || len(input) != 1 {
				t.Fatalf("creator application was not minimal and idempotent: %#v", input)
			}
			writeJSONResponse(writer, merchantOnboardingFixture("APPLICATION", "SUBMITTED", nil))
		default:
			t.Fatalf("unexpected creator application request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	root := t.TempDir()
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	ids := []string{replicaPublicationTestRequestID, applicationRequestID}
	dependencies.NewID = func() string {
		if len(ids) == 0 {
			t.Fatal("creator flow allocated an unexpected identity")
		}
		value := ids[0]
		ids = ids[1:]
		return value
	}
	arguments := replicaPublicationTestArguments(project, "replica-site")
	var reviewOutput bytes.Buffer
	dependencies.Out = &reviewOutput
	if exit := Execute(arguments, dependencies); exit != output.ExitConfirmation ||
		!strings.Contains(reviewOutput.String(), "REPLICA_PUBLICATION_CONFIRMATION_REQUIRED") {
		t.Fatalf("creator flow did not start with final review: exit=%d output=%s", exit, reviewOutput.String())
	}

	var unauthorizedOutput bytes.Buffer
	dependencies.Out = &unauthorizedOutput
	confirmedArguments := append(append([]string{}, arguments...), "--confirm", initialConfirmationVersion)
	if exit := Execute(confirmedArguments, dependencies); exit != output.ExitConfirmation ||
		!strings.Contains(unauthorizedOutput.String(), "REPLICA_CREATOR_APPLICATION_AUTHORIZATION_REQUIRED") {
		t.Fatalf("unapproved creator application did not stop after final review: exit=%d output=%s", exit, unauthorizedOutput.String())
	}
	if applicationCalls != 0 {
		t.Fatalf("creator application ran without authorization: calls=%d", applicationCalls)
	}

	authorizedArguments := append(append([]string{}, arguments...), "--auto-apply-creator")
	var authorizedReviewOutput bytes.Buffer
	dependencies.Out = &authorizedReviewOutput
	if exit := Execute(authorizedArguments, dependencies); exit != output.ExitConfirmation ||
		!strings.Contains(authorizedReviewOutput.String(), "REPLICA_PUBLICATION_CONFIRMATION_REQUIRED") ||
		!strings.Contains(authorizedReviewOutput.String(), `"automaticCreatorApplication": true`) {
		t.Fatalf("changed creator application authorization did not get a fresh review: exit=%d output=%s", exit, authorizedReviewOutput.String())
	}

	var authorizedOutput bytes.Buffer
	dependencies.Out = &authorizedOutput
	authorizedConfirmedArguments := append(append([]string{}, authorizedArguments...), "--confirm", authorizedConfirmationVersion)
	if exit := Execute(authorizedConfirmedArguments, dependencies); exit != output.ExitAuthentication ||
		!strings.Contains(authorizedOutput.String(), "REPLICA_CREATOR_APPLICATION_PENDING") {
		t.Fatalf("authorized creator application did not stop for review: exit=%d output=%s", exit, authorizedOutput.String())
	}
	if createCalls != 4 || applicationCalls != 1 || len(ids) != 0 {
		t.Fatalf("unexpected creator application flow: creates=%d applications=%d remainingIds=%d", createCalls, applicationCalls, len(ids))
	}
}

func TestReplicaPublishExpiresConfirmationAndCleansFrozenSource(t *testing.T) {
	issuedAt := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	currentTime := issuedAt
	project := newReplicaPublicationTestProject(t)
	confirmationVersion := "wrv1-7878787878787878787878787878787878787878787878787878787878787878"
	createCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		createCalls++
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		writeJSONResponse(writer, replicaConfirmationRequiredResponse(issuedAt, input, confirmationVersion))
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	root := t.TempDir()
	dependencies := replicaPublicationTestDependencies(t, root, server, issuedAt)
	dependencies.Now = func() time.Time { return currentTime }
	dependencies.NewID = func() string { return replicaPublicationTestRequestID }
	arguments := replicaPublicationTestArguments(project, "replica-site")
	dependencies.Out = &bytes.Buffer{}
	if exit := Execute(arguments, dependencies); exit != output.ExitConfirmation {
		t.Fatalf("publication did not reach final review: exit=%d", exit)
	}
	currentTime = issuedAt.Add(30 * time.Minute)
	var expiredOutput bytes.Buffer
	dependencies.Out = &expiredOutput
	confirmed := append(append([]string{}, arguments...), "--confirm", confirmationVersion)
	if exit := Execute(confirmed, dependencies); exit != output.ExitConfirmation ||
		!strings.Contains(expiredOutput.String(), "REPLICA_PUBLICATION_CONFIRMATION_EXPIRED") {
		t.Fatalf("expired confirmation was not rejected: exit=%d output=%s", exit, expiredOutput.String())
	}
	if createCalls != 1 {
		t.Fatalf("expired confirmation reached Shop: creates=%d", createCalls)
	}
	artifacts, err := filepath.Glob(filepath.Join(root, "config", "replica-publications", "*", "artifacts", "*", "source.zip"))
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("expired confirmation retained frozen source: files=%v err=%v", artifacts, err)
	}
	states, err := filepath.Glob(filepath.Join(root, "config", "replica-publications", "*", "pending-*.json"))
	if err != nil || len(states) != 0 {
		t.Fatalf("expired confirmation retained pending state: files=%v err=%v", states, err)
	}
}

func TestReplicaFinalReviewGetsTheFullChallengeTTL(t *testing.T) {
	issuedAt := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	endpoint := "https://api.viceme.cn"
	fingerprint, canonicalProject, err := replicapublication.ProjectFingerprint(endpoint, "CN", project)
	if err != nil {
		t.Fatal(err)
	}
	source := api.WebsiteReplicaPublicationSourceArtifact{
		FileName: "source.zip", ContentType: "application/zip", SizeBytes: 1024,
		Digest: replicaPublicationTestSourceDigest,
	}
	pending := replicapublication.Pending{
		EndpointOrigin: endpoint, Market: "CN", ProjectPath: canonicalProject,
		ProjectFingerprint: fingerprint, ClientRequestID: replicaPublicationTestRequestID,
		Request: api.CreateWebsiteReplicaPublicationRequest{
			ProtocolVersion: api.WebsiteReplicaPublicationProtocolVersion,
			ClientRequestID: replicaPublicationTestRequestID, Market: "CN", ProjectFingerprint: fingerprint,
			Target: api.WebsiteReplicaPublicationTarget{Kind: "NEW_WORK", Slug: "replica-site"},
			Title:  "Replica title", Summary: "Replica summary", PriceCents: 990, Source: source,
		},
		SourceArchive:     replicacontent.SourceArchiveSummary{Digest: source.Digest, SizeBytes: source.SizeBytes},
		ArtifactExpiresAt: issuedAt.Add(20 * time.Minute),
	}
	confirmation := api.WebsiteReplicaPublicationConfirmationChallenge{
		Version: "wrv1-" + strings.Repeat("c", 64),
		Review: api.WebsiteReplicaPublicationReview{
			Resolution: "CREATE", MerchantAccountID: replicaPublicationTestMerchantID,
			MerchantDisplayName: "Replica Studio", CreatorAccountID: replicaPublicationTestCreatorID,
			CreatorHandle: "replica-maker", CreatorDisplayName: "Replica Maker",
			ProjectFingerprint: fingerprint, WorkURL: "https://viceme.cn/replica-maker/replica-site",
			Title: "Replica title", Summary: "Replica summary", PriceCents: 990, Source: source,
		},
		IssuedAt: issuedAt.Format(time.RFC3339), ExpiresAt: issuedAt.Add(30 * time.Minute).Format(time.RFC3339),
	}
	store := replicapublication.Store{
		Directory:      replicapublication.ScopedDirectory(filepath.Join(t.TempDir(), "replica-publications"), endpoint, "CN"),
		EndpointOrigin: endpoint, Market: "CN", Now: func() time.Time { return issuedAt },
	}
	runtime := &Runtime{deps: Dependencies{Now: func() time.Time { return issuedAt }}}
	_, err = handleReplicaPublicationNextAction(context.Background(), runtime, store, pending, api.WebsiteReplicaPublicationNextAction{
		Kind: "CONFIRM_PUBLICATION", Confirmation: &confirmation,
	})
	if cliErr := output.AsError(err); err == nil || cliErr.Subtype != "REPLICA_PUBLICATION_CONFIRMATION_REQUIRED" {
		t.Fatalf("final review was not returned: %#v", cliErr)
	}
	loaded, found, err := store.LoadProject(fingerprint)
	if err != nil || !found {
		t.Fatalf("final review state was not saved: found=%v err=%v", found, err)
	}
	if want := issuedAt.Add(30 * time.Minute); !loaded.ArtifactExpiresAt.Equal(want) {
		t.Fatalf("artifact expiration=%s want full challenge TTL %s", loaded.ArtifactExpiresAt, want)
	}
}

func TestReplicaPublishRejectsChangedFrozenSourceBeforeConfirmedRemoteRequest(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	confirmationVersion := "wrv1-b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2"
	createCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		createCalls++
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		writeJSONResponse(writer, replicaConfirmationRequiredResponse(now, input, confirmationVersion))
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	root := t.TempDir()
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	dependencies.NewID = func() string { return replicaPublicationTestRequestID }
	arguments := replicaPublicationTestArguments(project, "replica-site")
	dependencies.Out = &bytes.Buffer{}
	if exit := Execute(arguments, dependencies); exit != output.ExitConfirmation {
		t.Fatalf("publication did not reach final review: exit=%d", exit)
	}
	artifacts, err := filepath.Glob(filepath.Join(root, "config", "replica-publications", "*", "artifacts", "*", "source.zip"))
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("final review did not retain one frozen source: files=%v err=%v", artifacts, err)
	}
	artifact, err := os.OpenFile(artifacts[0], os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := artifact.WriteAt([]byte{0xff}, 0); err != nil {
		_ = artifact.Close()
		t.Fatal(err)
	}
	if err := artifact.Close(); err != nil {
		t.Fatal(err)
	}

	var confirmedOutput bytes.Buffer
	dependencies.Out = &confirmedOutput
	confirmed := append(append([]string{}, arguments...), "--confirm", confirmationVersion)
	if exit := Execute(confirmed, dependencies); exit != output.ExitValidation ||
		!strings.Contains(confirmedOutput.String(), "REPLICA_PUBLICATION_ARTIFACT_CHANGED") {
		t.Fatalf("changed frozen source was not rejected locally: exit=%d output=%s", exit, confirmedOutput.String())
	}
	if createCalls != 1 {
		t.Fatalf("changed frozen source reached the confirmed remote request: calls=%d", createCalls)
	}
	artifacts, err = filepath.Glob(filepath.Join(root, "config", "replica-publications", "*", "artifacts", "*", "source.zip"))
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("changed frozen source was retained: files=%v err=%v", artifacts, err)
	}
}

func TestReplicaResumeDoesNotReuploadAuthoritativelyVerifiedSource(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	root := t.TempDir()
	var authorizationCalls, submitCalls int
	var source map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID:
			writeJSONResponse(writer, replicaPublicationForSource(now, "DRAFT", "VERIFIED", source))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+replicaPublicationTestID+"/submit":
			submitCalls++
			writeJSONResponse(writer, replicaPublicationForSource(now, "PROCESSING", "VERIFIED", source))
		case strings.HasSuffix(request.URL.Path, "/source/upload-authorizations"):
			authorizationCalls++
			t.Fatalf("verified source requested another upload authorization")
		default:
			t.Fatalf("unexpected verified-source recovery request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	fingerprint, canonicalProject, err := replicapublication.ProjectFingerprint(server.URL, "CN", project)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := replicacontent.FreezeSourceArchive(project, replicacontent.FreezeSourceOptions{ExpiresAt: now.Add(30 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	defer frozen.Cleanup()
	source = map[string]any{
		"fileName": "source.zip", "contentType": "application/zip",
		"sizeBytes": float64(frozen.Summary.SizeBytes), "digest": frozen.Summary.Digest,
	}
	store := replicapublication.Store{
		Directory:      replicapublication.ScopedDirectory(filepath.Join(root, "config", "replica-publications"), server.URL, "CN"),
		EndpointOrigin: server.URL, Market: "CN", Now: func() time.Time { return now },
	}
	if err := store.SaveArtifact(replicaPublicationTestRequestID, frozen); err != nil {
		t.Fatal(err)
	}
	pending := replicapublication.Pending{
		EndpointOrigin: server.URL, Market: "CN", ProjectPath: canonicalProject,
		ProjectFingerprint: fingerprint, ClientRequestID: replicaPublicationTestRequestID,
		Request: api.CreateWebsiteReplicaPublicationRequest{
			ProtocolVersion: api.WebsiteReplicaPublicationProtocolVersion, ClientRequestID: replicaPublicationTestRequestID,
			Market: "CN", ProjectFingerprint: fingerprint, Target: api.WebsiteReplicaPublicationTarget{Kind: "NEW_WORK", Slug: "replica-site"},
			Title: "Replica title", Summary: "Replica summary", PriceCents: 990,
			Source: api.WebsiteReplicaPublicationSourceArtifact{
				FileName: "source.zip", ContentType: "application/zip", SizeBytes: frozen.Summary.SizeBytes, Digest: frozen.Summary.Digest,
			},
		},
		SourceArchive: frozen.Summary, ArtifactExpiresAt: now.Add(30 * time.Minute),
		Publication: &replicapublication.PublicationReference{
			ID: replicaPublicationTestID, Status: "DRAFT",
			StatusURL: "https://viceme.cn/me/website-replica-publications/" + replicaPublicationTestID,
		},
	}
	pending.Confirmation = &api.WebsiteReplicaPublicationConfirmationChallenge{
		Version: "wrv1-" + strings.Repeat("d", 64),
		Review: api.WebsiteReplicaPublicationReview{
			MerchantAccountID: replicaPublicationTestMerchantID, ProjectFingerprint: pending.ProjectFingerprint,
			Title: pending.Request.Title, Summary: pending.Request.Summary,
			PriceCents: pending.Request.PriceCents, Source: pending.Request.Source,
		},
		IssuedAt: now.Format(time.RFC3339), ExpiresAt: now.Add(30 * time.Minute).Format(time.RFC3339),
	}
	pending.ConfirmedAt = &now
	if err := store.Save(&pending); err != nil {
		t.Fatal(err)
	}

	t.Setenv(processAccessTokenEnvironment, replicaPublicationTestAccessToken)
	dependencies := replicaPublicationTestDependencies(t, root, server, now)
	var stdout bytes.Buffer
	dependencies.Out = &stdout
	if exit := Execute([]string{"replica", "resume", replicaPublicationTestID}, dependencies); exit != 0 ||
		!strings.Contains(stdout.String(), `"status": "PROCESSING"`) {
		t.Fatalf("verified source did not resume at submit: exit=%d output=%s", exit, stdout.String())
	}
	if authorizationCalls != 0 || submitCalls != 1 {
		t.Fatalf("verified-source recovery repeated work: authorizations=%d submits=%d", authorizationCalls, submitCalls)
	}
}

func replicaPublicationAPIResponse(now time.Time, status, sourceStatus string) map[string]any {
	verifiedAt := any(nil)
	if sourceStatus == "VERIFIED" || sourceStatus == "ACTIVATED" {
		verifiedAt = now.Add(-time.Minute).Format(time.RFC3339)
	}
	allowedActions := []string{}
	if status == "DRAFT" {
		switch sourceStatus {
		case "WAITING_UPLOAD":
			allowedActions = []string{"AUTHORIZE_SOURCE_UPLOAD", "COMPLETE_SOURCE_UPLOAD", "CANCEL"}
		case "UPLOADED", "VALIDATING":
			allowedActions = []string{"COMPLETE_SOURCE_UPLOAD", "CANCEL"}
		case "VERIFIED":
			allowedActions = []string{"SUBMIT", "CANCEL"}
		}
	} else if status == "PROCESSING" {
		allowedActions = []string{"CANCEL"}
	} else if status == "FAILED" {
		allowedActions = []string{"RETRY"}
	}
	submittedAt := any(nil)
	if status != "DRAFT" && status != "CANCELLED" {
		submittedAt = now.Add(-2 * time.Minute).Format(time.RFC3339)
	}
	cancelledAt := any(nil)
	if status == "CANCELLED" {
		cancelledAt = now.Format(time.RFC3339)
	}
	failedAt := any(nil)
	failure := any(nil)
	if status == "FAILED" {
		failedAt = now.Format(time.RFC3339)
		failure = map[string]any{"code": "TRANSIENT_STORAGE", "message": "temporary storage failure", "retryable": true}
	}
	if status == "PUBLISHED_DEGRADED" {
		failure = map[string]any{"code": "PAGE_VALIDATION_FAILED", "message": "hosted page could not be activated", "retryable": false}
	}
	result := any(nil)
	page := any(nil)
	if status == "PUBLISHED" || status == "PUBLISHED_DEGRADED" {
		result = map[string]any{
			"workUrl":   "https://viceme.cn/replica-maker/replica-site",
			"versionId": replicaPublicationTestVersionID, "version": 1,
			"shortCode": "VMR-ABCDEFGHIJKLMNOPQRST", "instruction": "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST",
			"product": map[string]any{
				"id": replicaPublicationTestProductID, "skuId": replicaPublicationTestSKUID,
				"title": "Replica title", "currency": "CNY", "priceCents": 990,
			},
			"pageRelease": nil,
			"publishedAt": now.Format(time.RFC3339),
		}
	}
	if status == "PUBLISHED_DEGRADED" {
		page = map[string]any{
			"fileName": "page.zip", "contentType": "application/zip", "sizeBytes": 512,
			"digest": strings.Repeat("d", 64), "status": "FAILED", "verifiedAt": nil,
		}
	}
	return map[string]any{
		"id": replicaPublicationTestID, "clientRequestId": replicaPublicationTestRequestID,
		"market": "CN", "merchantAccountId": replicaPublicationTestMerchantID,
		"workId": replicaPublicationTestWorkID, "replicaId": replicaPublicationTestReplicaID,
		"status": status, "statusUrl": "https://viceme.cn/me/website-replica-publications/" + replicaPublicationTestID,
		"allowedActions":            allowedActions,
		"allowAutomaticDegradation": false, "priceCents": 990,
		"hosting":  map[string]any{"requested": false, "status": "NOT_REQUESTED", "activePageRelease": nil, "repair": nil, "latestRepair": nil},
		"rollback": map[string]any{"activePair": nil, "availablePairs": []any{}},
		"retry":    map[string]any{"automaticRetries": 0, "maxAutomaticRetries": 3, "nextAttemptAt": nil},
		"source": map[string]any{
			"fileName": "source.zip", "contentType": "application/zip", "sizeBytes": 1024,
			"digest": replicaPublicationTestSourceDigest, "status": sourceStatus, "verifiedAt": verifiedAt,
		},
		"page":    page,
		"failure": failure, "result": result, "submittedAt": submittedAt, "failedAt": failedAt, "cancelledAt": cancelledAt,
		"createdAt": now.Add(-5 * time.Minute).Format(time.RFC3339), "updatedAt": now.Format(time.RFC3339),
	}
}

func replicaConfirmationRequiredResponse(now time.Time, input map[string]any, version string) map[string]any {
	canonicalOrigin := input["canonicalOrigin"]
	if canonicalOrigin != nil {
		canonicalOrigin = "https://example.com"
	}
	response := map[string]any{
		"outcome": "ACTION_REQUIRED", "clientRequestId": input["clientRequestId"], "market": "CN",
		"nextAction": map[string]any{
			"kind": "CONFIRM_PUBLICATION",
			"confirmation": map[string]any{
				"version": version,
				"review": map[string]any{
					"resolution": "CREATE", "merchantAccountId": replicaPublicationTestMerchantID,
					"merchantDisplayName": "Replica Studio", "creatorAccountId": replicaPublicationTestCreatorID,
					"creatorHandle": "replica-maker", "creatorDisplayName": "Replica Maker",
					"projectFingerprint": input["projectFingerprint"], "workUrl": "https://viceme.cn/replica-maker/replica-site",
					"canonicalOrigin": canonicalOrigin, "title": input["title"], "summary": input["summary"],
					"priceCents": input["priceCents"], "source": input["source"], "page": input["page"],
					"allowAutomaticDegradation": input["allowAutomaticDegradation"],
				},
				"issuedAt": now.Format(time.RFC3339), "expiresAt": now.Add(30 * time.Minute).Format(time.RFC3339),
			},
		},
	}
	if input["page"] == nil {
		delete(response["nextAction"].(map[string]any)["confirmation"].(map[string]any)["review"].(map[string]any), "page")
	}
	return response
}

func replicaPublicationForSource(now time.Time, status, sourceStatus string, source map[string]any) map[string]any {
	response := replicaPublicationAPIResponse(now, status, sourceStatus)
	responseSource := response["source"].(map[string]any)
	for _, key := range []string{"fileName", "contentType", "sizeBytes", "digest"} {
		responseSource[key] = source[key]
	}
	return response
}

func replicaPublicationForArtifacts(now time.Time, status, sourceStatus string, source map[string]any, pageStatus string, page map[string]any) map[string]any {
	response := replicaPublicationForSource(now, status, sourceStatus, source)
	verifiedAt := any(nil)
	if pageStatus == "VERIFIED" || pageStatus == "ACTIVATED" {
		verifiedAt = now.Add(-time.Minute).Format(time.RFC3339)
	}
	responsePage := map[string]any{"status": pageStatus, "verifiedAt": verifiedAt}
	for _, key := range []string{"fileName", "contentType", "sizeBytes", "digest"} {
		responsePage[key] = page[key]
	}
	response["page"] = responsePage
	response["allowAutomaticDegradation"] = false
	if status == "DRAFT" {
		actions := []string{"CANCEL"}
		switch sourceStatus {
		case "WAITING_UPLOAD":
			actions = append(actions, "AUTHORIZE_SOURCE_UPLOAD", "COMPLETE_SOURCE_UPLOAD")
		case "UPLOADED", "VALIDATING":
			actions = append(actions, "COMPLETE_SOURCE_UPLOAD")
		}
		switch pageStatus {
		case "WAITING_UPLOAD":
			actions = append(actions, "AUTHORIZE_PAGE_UPLOAD", "COMPLETE_PAGE_UPLOAD")
		case "UPLOADED", "VALIDATING":
			actions = append(actions, "COMPLETE_PAGE_UPLOAD")
		}
		if sourceStatus == "VERIFIED" && pageStatus == "VERIFIED" {
			actions = append(actions, "SUBMIT")
		}
		response["allowedActions"] = actions
	}
	return response
}

func setReplicaPublicationIdentity(publication map[string]any, publicationID, clientRequestID string) {
	publication["id"] = publicationID
	publication["clientRequestId"] = clientRequestID
	publication["statusUrl"] = "https://viceme.cn/me/website-replica-publications/" + publicationID
}

func assertEmptyJSONObject(t *testing.T, request *http.Request) {
	t.Helper()
	var value map[string]any
	if err := json.NewDecoder(request.Body).Decode(&value); err != nil || len(value) != 0 {
		t.Fatalf("expected an empty command object: value=%#v err=%v", value, err)
	}
}

func mapsHaveEqualJSON(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func newReplicaPublicationTestProject(t *testing.T) string {
	t.Helper()
	project := filepath.Join(t.TempDir(), "replica-project")
	if err := os.MkdirAll(filepath.Join(project, "node_modules"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "index.html"), []byte("<h1>Replica</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}
	return project
}

func replicaPublicationTestDependencies(t *testing.T, root string, server *httptest.Server, now time.Time) Dependencies {
	t.Helper()
	return Dependencies{
		ErrOut: &bytes.Buffer{}, HTTPClient: server.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, APIBaseURL: server.URL, Now: func() time.Time { return now },
		StartReplicaPreview: func(context.Context, replicapreview.Options) (replicapreview.Running, error) {
			return &replicaPreviewSessionStub{result: replicapreview.Result{
				TargetURL: "http://127.0.0.1:4173/", Reused: true, ServiceKind: replicapreview.ServiceExisting,
			}}, nil
		},
		OpenURL: func(context.Context, string) error { return nil },
	}
}

func replicaPublicationTestArguments(project, slug string) []string {
	return []string{
		"replica", "publish", "--path", project, "--slug", slug,
		"--title", "Replica title", "--summary", "Replica summary", "--price-cents", "990",
		"--preview-url", "http://127.0.0.1:4173/", "--preview-reviewed", "--replica-only",
	}
}

func TestReplicaPublishRejectsChangedDegradationReview(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	if err := os.MkdirAll(filepath.Join(project, "dist"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("<h1>Hosted</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/website-replica-publications" {
			t.Error("unexpected upload after changed review")
			w.WriteHeader(500)
			return
		}
		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			return
		}
		if input["allowAutomaticDegradation"] != false {
			t.Error("hosted request omitted degradation policy")
		}
		response := replicaConfirmationRequiredResponse(now, input, "wrv1-"+strings.Repeat("a", 64))
		response["nextAction"].(map[string]any)["confirmation"].(map[string]any)["review"].(map[string]any)["allowAutomaticDegradation"] = true
		writeJSONResponse(w, response)
	}))
	defer server.Close()
	deps := replicaPublicationTestDependencies(t, t.TempDir(), server, now)
	deps.NewID = func() string { return replicaPublicationTestRequestID }
	var out bytes.Buffer
	deps.Out = &out
	if exit := Execute(append(replicaPublicationTestArguments(project, "replica-site"), "--replica-only=false"), deps); exit != output.ExitInternal || !strings.Contains(out.String(), "RESPONSE_INVALID") {
		t.Fatalf("changed policy was accepted: exit=%d output=%s", exit, out.String())
	}
	if calls != 1 {
		t.Fatalf("unexpected requests: %d", calls)
	}
}

func TestReplicaPublishRequiresHostedOutputBeforeRemoteWork(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newReplicaPublicationTestProject(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		t.Error("missing hosted output reached the API")
		w.WriteHeader(500)
	}))
	defer server.Close()
	deps := replicaPublicationTestDependencies(t, t.TempDir(), server, now)
	var out bytes.Buffer
	deps.Out = &out
	args := append(replicaPublicationTestArguments(project, "replica-site"), "--replica-only=false")
	if code := Execute(args, deps); code != output.ExitValidation || !strings.Contains(out.String(), "REPLICA_HOSTED_PAGE_REQUIRED") || !strings.Contains(out.String(), "PREPARE_HOSTED_PAGE") {
		t.Fatalf("missing HTML was not rejected: exit=%d output=%s", code, out.String())
	}
	if calls.Load() != 0 {
		t.Fatalf("made %d remote calls without HTML", calls.Load())
	}
}

func TestReplicaPublishedPresentationReportsActiveHosting(t *testing.T) {
	result := presentReplicaPublication(api.WebsiteReplicaPublication{
		Status: "PUBLISHED", Hosting: api.WebsiteReplicaHostingProjection{Status: "ACTIVE"},
	})
	if result.Message != "Website Replica source and hosted HTML publication complete." {
		t.Fatalf("active hosting not reported: %s", result.Message)
	}
}

func TestReplicaConfirmationCannotSilentlySwitchSourceOnlyToDefaultHosting(t *testing.T) {
	pending := replicapublication.Pending{
		Hosting:      "REPLICA_ONLY",
		Confirmation: &api.WebsiteReplicaPublicationConfirmationChallenge{Version: "review"},
	}
	err := validateConfirmedReplicaRequest(replicaPublishOptions{ConfirmationVersion: "review"}, pending, replicapublication.Binding{}, false)
	if err == nil || !strings.Contains(err.Error(), "hosting selection changed") {
		t.Fatalf("source-only confirmation accepted without explicit selection: %v", err)
	}
}
