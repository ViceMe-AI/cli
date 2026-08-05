package command

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/appmanifest"
	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

const (
	testAppID           = "550e8400-e29b-41d4-a716-446655440000"
	testEnvironmentID   = "550e8400-e29b-41d4-a716-446655440001"
	testClientRequestID = "550e8400-e29b-41d4-a716-446655440010"
	testPublishableKey  = "app_pk_test_abcdefghijklmnopqrstuvwxyz123456"
	livePublishableKey  = "app_pk_live_abcdefghijklmnopqrstuvwxyz123456"
)

func TestAppLinkRetriesCreateWithStableRequestIDAfterResponseLoss(t *testing.T) {
	var requestIDs []string
	var mutex sync.Mutex
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/creator-apps" || request.Method != http.MethodPost {
			http.Error(writer, "not found", http.StatusNotFound)
			return
		}
		var input api.CreateCreatorAppRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		mutex.Lock()
		requestIDs = append(requestIDs, input.ClientRequestID)
		mutex.Unlock()
		if requests.Add(1) == 1 {
			hijacker, ok := writer.(http.Hijacker)
			if !ok {
				t.Fatal("test server does not support hijacking")
			}
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = connection.Close()
			return
		}
		writeCommandJSON(t, writer, creatorAppFixture("Retry App", nil, nil))
	}))
	defer server.Close()

	project := filepath.Join(t.TempDir(), "Retry App")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	var idCalls atomic.Int32
	dependencies.NewID = func() string {
		idCalls.Add(1)
		return testClientRequestID
	}

	code, _, stderr, _ := runCLIWithDependencies(t, server, store, "", dependencies, "app", "link", "--dir", project)
	if code == 0 || !strings.Contains(stderr, "transport") {
		t.Fatalf("lost response code=%d stderr=%s", code, stderr)
	}
	if _, err := os.Stat(appmanifest.LinkIntentPath(project)); err != nil {
		t.Fatalf("retry intent was not retained: %v", err)
	}
	code, _, stderr, _ = runCLIWithDependencies(t, server, store, "", dependencies, "app", "link", "--dir", project)
	if code != 0 || stderr != "" {
		t.Fatalf("retry code=%d stderr=%s", code, stderr)
	}
	if len(requestIDs) != 2 || requestIDs[0] != testClientRequestID || requestIDs[1] != requestIDs[0] || idCalls.Load() != 1 {
		t.Fatalf("App creation was not idempotent: ids=%#v generator_calls=%d", requestIDs, idCalls.Load())
	}
	if _, err := os.Stat(appmanifest.LinkIntentPath(project)); !os.IsNotExist(err) {
		t.Fatalf("committed App link left pending intent: %v", err)
	}
	manifest, err := appmanifest.Load(project)
	if err != nil || manifest.AppID != testAppID {
		t.Fatalf("manifest=%#v err=%v", manifest, err)
	}
}

func TestAppLinkSerializesConcurrentFirstCreation(t *testing.T) {
	firstCreateStarted := make(chan struct{})
	releaseFirstCreate := make(chan struct{})
	var createRequests atomic.Int32
	var getRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/creator-apps":
			var input api.CreateCreatorAppRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			if input.ClientRequestID != testClientRequestID {
				http.Error(writer, "unexpected request ID", http.StatusBadRequest)
				return
			}
			if createRequests.Add(1) == 1 {
				close(firstCreateStarted)
				<-releaseFirstCreate
			}
			writeCommandJSON(t, writer, creatorAppFixture("Concurrent App", nil, nil))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/creator-apps/"+testAppID:
			getRequests.Add(1)
			writeCommandJSON(t, writer, creatorAppFixture("Concurrent App", nil, nil))
		default:
			http.Error(writer, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	project := filepath.Join(t.TempDir(), "Concurrent App")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	var idCalls atomic.Int32
	dependencies.NewID = func() string {
		idCalls.Add(1)
		return testClientRequestID
	}
	type result struct {
		code   int
		stderr string
	}
	results := make(chan result, 2)
	run := func() {
		code, _, stderr, _ := runCLIWithDependencies(t, server, store, "", dependencies, "app", "link", "--dir", project)
		results <- result{code: code, stderr: stderr}
	}
	go run()
	<-firstCreateStarted
	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		run()
	}()
	<-secondStarted
	time.Sleep(50 * time.Millisecond)
	if createRequests.Load() != 1 {
		t.Fatalf("second App link reached remote creation before the first transaction completed: creates=%d", createRequests.Load())
	}
	close(releaseFirstCreate)

	for range 2 {
		result := <-results
		if result.code != 0 || result.stderr != "" {
			t.Fatalf("concurrent link code=%d stderr=%s", result.code, result.stderr)
		}
	}
	if createRequests.Load() != 1 || getRequests.Load() != 1 || idCalls.Load() != 1 {
		t.Fatalf("concurrent App link did not converge: creates=%d gets=%d id_calls=%d", createRequests.Load(), getRequests.Load(), idCalls.Load())
	}
	manifest, err := appmanifest.Load(project)
	if err != nil || manifest.AppID != testAppID {
		t.Fatalf("manifest=%#v err=%v", manifest, err)
	}
}

func TestAppLinkExplicitSameAppPreservesEnvironmentOriginAndProtocols(t *testing.T) {
	capability := api.CreatorAppCapability{
		Type: "COMMERCE", Status: "DRAFT", ConfigVersion: 1,
		ContractVersion: "1.0.0", SDKPackage: "@viceme/web-sdk", SDKVersion: "0.1.0",
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/creator-apps/"+testAppID:
			writeCommandJSON(t, writer, creatorAppFixture("Existing", []string{"https://existing.example"}, []api.CreatorAppCapability{capability}))
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/origins"):
			writeCommandJSON(t, writer, map[string]any{"id": "550e8400-e29b-41d4-a716-446655440003", "origin": "https://existing.example"})
		default:
			http.Error(writer, "unexpected", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	project := t.TempDir()
	expected := appmanifest.Manifest{
		SchemaVersion: 1, AppID: testAppID, HostingMode: "EXTERNAL", Environment: "LIVE",
		PublishableKey: livePublishableKey, Origin: "https://existing.example",
		Capabilities: map[string]appmanifest.Capability{"commerce": capabilityBinding(capability)},
	}
	if _, err := appmanifest.Save(project, expected); err != nil {
		t.Fatal(err)
	}
	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	code, _, stderr, _ := runCLIWithDependencies(t, server, store, "", dependencies,
		"app", "link", "--dir", project, "--app", testAppID)
	if code != 0 || stderr != "" {
		t.Fatalf("link code=%d stderr=%s", code, stderr)
	}
	actual, err := appmanifest.Load(project)
	if err != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("same-App link lost binding state: actual=%#v expected=%#v err=%v", actual, expected, err)
	}
}

func TestDoctorsPreserveStructuredRemoteErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
		writeCommandJSON(t, writer, map[string]any{
			"statusCode": 503, "code": "CONTROL_PLANE_UNAVAILABLE", "message": "retry later",
			"requestId": "550e8400-e29b-41d4-a716-446655440099",
		})
	}))
	defer server.Close()
	project := t.TempDir()
	if _, err := appmanifest.Save(project, appmanifest.Manifest{
		SchemaVersion: 1, AppID: testAppID, HostingMode: "EXTERNAL", Environment: "TEST",
		PublishableKey: testPublishableKey, Origin: "http://localhost:3000",
		Capabilities: map[string]appmanifest.Capability{
			"commerce": {ContractVersion: "1.0.0", SDKPackage: "@viceme/web-sdk", SDKVersion: "0.1.0"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	for _, args := range [][]string{
		{"app", "doctor", "--dir", project},
		{"capability", "doctor", "commerce", "--dir", project},
	} {
		code, _, stderr, _ := runCLIWithDependencies(t, server, store, "", dependencies, args...)
		if code != 4 || !strings.Contains(stderr, "control_plane_unavailable") ||
			!strings.Contains(stderr, "550e8400-e29b-41d4-a716-446655440099") ||
			strings.Contains(stderr, "doctor_unhealthy") {
			t.Fatalf("doctor squashed remote error: args=%v code=%d stderr=%s", args, code, stderr)
		}
	}
}

func creatorAppFixture(name string, liveOrigins []string, liveCapabilities []api.CreatorAppCapability) map[string]any {
	return map[string]any{
		"id": testAppID, "name": name, "hostingMode": "EXTERNAL", "status": "DRAFT",
		"creatorChannelAccountId": nil, "skillProductId": nil,
		"createdAt": "2026-08-06T00:00:00Z", "updatedAt": "2026-08-06T00:00:00Z",
		"environments": []any{
			map[string]any{"id": testEnvironmentID, "type": "TEST", "status": "ACTIVE", "publishableKey": testPublishableKey, "allowedOrigins": []string{}, "capabilities": []any{}},
			map[string]any{"id": "550e8400-e29b-41d4-a716-446655440002", "type": "LIVE", "status": "SUSPENDED", "publishableKey": livePublishableKey, "allowedOrigins": liveOrigins, "capabilities": liveCapabilities},
		},
	}
}

func authenticatedDependencies(t *testing.T, server *httptest.Server, store securestore.Store) Dependencies {
	t.Helper()
	scope, err := customCredentialScope(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vcm_at_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return Dependencies{
		Region: config.RegionCN, APIBaseURL: server.URL, HTTPClient: server.Client(),
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()},
	}
}
