package command

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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

func TestRootExposesCreatorCapabilitySurfaceOnly(t *testing.T) {
	root, _, err := NewRoot(Dependencies{Store: securestore.NewMemory(), Region: config.RegionCN})
	if err != nil {
		t.Fatal(err)
	}
	paths := make(map[string]bool)
	for _, command := range BuildCommandManifest(root).Commands {
		paths[command.Path] = true
	}
	for _, expected := range []string{
		"app link", "app get", "app list", "app doctor",
		"capability catalog", "capability add", "capability get", "capability doctor",
		"auth login", "auth status", "auth logout", "skills install", "skills doctor",
		"runtime inspect", "job get", "job wait", "job cancel",
	} {
		if !paths[expected] {
			t.Errorf("missing command %q", expected)
		}
	}
	for _, removed := range []string{"skill", "skill publish"} {
		if paths[removed] {
			t.Errorf("legacy publication command %q is still exposed", removed)
		}
	}
}

func TestCredentialScopeIncludesTheCanonicalAPIBasePath(t *testing.T) {
	first, err := customCredentialScope("https://api.example.com/a/")
	if err != nil {
		t.Fatal(err)
	}
	firstEquivalent, err := customCredentialScope("https://API.example.com:443/a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := customCredentialScope("https://api.example.com/b")
	if err != nil {
		t.Fatal(err)
	}
	if first != firstEquivalent {
		t.Fatalf("equivalent API bases used different credential scopes: %q != %q", first, firstEquivalent)
	}
	if first == second {
		t.Fatalf("different API paths shared credential scope %q", first)
	}

	officialStaging, err := credentialScopeForAPIBase(config.APIBaseURL(config.RegionCN)+"/staging", config.RegionCN)
	if err != nil {
		t.Fatal(err)
	}
	if officialStaging == "" {
		t.Fatal("an API path below the official origin fell into the production credential namespace")
	}
}

func TestStableProcessLockRootIgnoresConfigDirectoryOverride(t *testing.T) {
	home := t.TempDir()
	first := stableProcessLockRoot(skillcontent.Environment{
		Home:      home,
		ConfigDir: filepath.Join(t.TempDir(), "config-a"),
	})
	second := stableProcessLockRoot(skillcontent.Environment{
		Home:      home,
		ConfigDir: filepath.Join(t.TempDir(), "config-b"),
	})
	if first != second {
		t.Fatalf("config directory override changed shared process lock root: %q != %q", first, second)
	}
}

func TestAppLinkCapabilityAddAndDoctor(t *testing.T) {
	const appID = "550e8400-e29b-41d4-a716-446655440000"
	const publishableKey = "app_pk_test_abcdefghijklmnopqrstuvwxyz123456"
	var mutex sync.Mutex
	origins := []string{}
	capabilities := []map[string]any{}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("x-api-key") == "" && !strings.HasPrefix(request.URL.Path, "/v1/creator-app-context/") {
			http.Error(writer, "missing token", http.StatusUnauthorized)
			return
		}
		mutex.Lock()
		defer mutex.Unlock()
		environment := map[string]any{
			"id":             "550e8400-e29b-41d4-a716-446655440001",
			"type":           "TEST",
			"status":         "ACTIVE",
			"publishableKey": publishableKey,
			"allowedOrigins": append([]string(nil), origins...),
			"capabilities":   append([]map[string]any(nil), capabilities...),
		}
		app := map[string]any{
			"id":                      appID,
			"name":                    "sample-project",
			"hostingMode":             "EXTERNAL",
			"status":                  "DRAFT",
			"creatorChannelAccountId": nil,
			"skillProductId":          nil,
			"createdAt":               "2026-08-06T00:00:00Z",
			"updatedAt":               "2026-08-06T00:00:00Z",
			"environments":            []any{environment},
		}
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/creator-apps":
			var input api.CreateCreatorAppRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input.ClientRequestID != "550e8400-e29b-41d4-a716-446655440010" {
				t.Fatalf("missing stable client request ID: %#v", input)
			}
			writeCommandJSON(t, writer, app)
		case request.Method == http.MethodGet && request.URL.Path == "/v1/creator-apps/"+appID:
			writeCommandJSON(t, writer, app)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/origins"):
			var input map[string]string
			_ = json.NewDecoder(request.Body).Decode(&input)
			origins = []string{input["origin"]}
			writeCommandJSON(t, writer, map[string]any{"id": "550e8400-e29b-41d4-a716-446655440002", "origin": input["origin"]})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/capabilities"):
			capability := map[string]any{
				"type": "COMMERCE", "status": "DRAFT", "configVersion": 1,
				"contractVersion": "1.0.0", "sdkPackage": "@viceme/web-sdk", "sdkVersion": "0.1.0",
				"config": map[string]any{},
			}
			capabilities = []map[string]any{capability}
			writeCommandJSON(t, writer, capability)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/capabilities/COMMERCE"):
			writeCommandJSON(t, writer, capabilities[0])
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/v1/creator-app-context/"):
			if request.Header.Get("Origin") != "http://localhost:3000" {
				http.Error(writer, "bad origin", http.StatusForbidden)
				return
			}
			writeCommandJSON(t, writer, map[string]any{"app": map[string]string{"name": "sample-project"}, "environment": "TEST", "capabilities": capabilities})
		default:
			http.Error(writer, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	project := filepath.Join(t.TempDir(), "sample-project")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	store := securestore.NewMemory()
	environment := skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()}
	dependencies := Dependencies{
		Store:       store,
		Region:      config.RegionCN,
		APIBaseURL:  server.URL,
		HTTPClient:  server.Client(),
		Environment: environment,
		Now:         time.Now,
		NewID:       func() string { return "550e8400-e29b-41d4-a716-446655440010" },
	}
	root, runtime, err := NewRoot(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.manager().Save(credentialauth.Credential{AccessToken: "vcm_at_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	root.SetArgs([]string{"app", "link", "--dir", project, "--origin", "http://localhost:3000"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	manifest, err := appmanifest.Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.AppID != appID || manifest.Origin != "http://localhost:3000" {
		t.Fatalf("unexpected binding %#v", manifest)
	}

	root, _, err = NewRoot(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	root.SetArgs([]string{"capability", "add", "commerce", "--dir", project})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	manifest, err = appmanifest.Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Capabilities["commerce"] != (appmanifest.Capability{ContractVersion: "1.0.0", SDKPackage: "@viceme/web-sdk", SDKVersion: "0.1.0"}) {
		t.Fatalf("capability not persisted %#v", manifest.Capabilities)
	}

	var stdout bytes.Buffer
	dependencies.Out = &stdout
	root, _, err = NewRoot(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	root.SetArgs([]string{"app", "doctor", "--dir", project})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"healthy": true`) {
		t.Fatalf("unexpected doctor output %s", stdout.String())
	}
}

func TestRefreshingTokenSourceRotatesExpiredCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/refresh" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		var input api.RefreshTokenRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input.RefreshToken != "refresh" || input.ClientRequestID != "550e8400-e29b-41d4-a716-446655440050" {
			t.Fatalf("unexpected refresh request %#v", input)
		}
		writeCommandJSON(t, writer, map[string]any{
			"access_token":       "vcm_at_new",
			"refresh_token":      "vcm_rt_new",
			"token_type":         "api_key",
			"expires_at":         time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano),
			"refresh_expires_at": time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339Nano),
			"user_id":            "550e8400-e29b-41d4-a716-446655440000",
			"scope":              []string{"app:read"},
		})
	}))
	defer server.Close()
	store := securestore.NewMemory()
	manager := &credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default", LockRoot: t.TempDir()}
	if err := manager.Save(credentialauth.Credential{AccessToken: "old", RefreshToken: "refresh", ExpiresAt: time.Now().Add(-time.Minute), RefreshExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	client := apiClientForTest(server)
	source := &refreshingTokenSource{
		manager: manager,
		client:  client,
		now:     time.Now,
		newID:   func() string { return "550e8400-e29b-41d4-a716-446655440050" },
	}
	token, err := source.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "vcm_at_new" {
		t.Fatalf("unexpected token %q", token)
	}
	stored, err := manager.Load()
	if err != nil || stored.RefreshToken != "vcm_rt_new" {
		t.Fatalf("refresh rotation not persisted %#v err=%v", stored, err)
	}
	if stored.RefreshRequestID != "" {
		t.Fatalf("completed refresh left recovery state %#v", stored)
	}
}

func TestRefreshingTokenSourceRetriesSameRefreshAfterResponseLoss(t *testing.T) {
	now := time.Now().UTC()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/refresh" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		var input api.RefreshTokenRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input.RefreshToken != "vcm_rt_original" {
			t.Fatalf("retry did not use the original refresh credential: %#v", input)
		}
		if input.ClientRequestID != "550e8400-e29b-41d4-a716-446655440051" {
			t.Fatalf("retry changed the refresh request identity: %#v", input)
		}
		if calls.Add(1) == 1 {
			hijacker := writer.(http.Hijacker)
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = connection.Close()
			return
		}
		writeCommandJSON(t, writer, map[string]any{
			"access_token":       "vcm_at_recovered",
			"refresh_token":      "vcm_rt_recovered",
			"token_type":         "api_key",
			"expires_at":         now.Add(time.Hour).Format(time.RFC3339Nano),
			"refresh_expires_at": now.Add(24 * time.Hour).Format(time.RFC3339Nano),
			"user_id":            "550e8400-e29b-41d4-a716-446655440000",
			"scope":              []string{"app:read"},
		})
	}))
	defer server.Close()
	store := securestore.NewMemory()
	manager := &credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", LockRoot: t.TempDir()}
	if err := manager.Save(credentialauth.Credential{
		AccessToken: "vcm_at_expired", RefreshToken: "vcm_rt_original",
		ExpiresAt: now.Add(-time.Minute), RefreshExpiresAt: now.Add(24 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	client := apiClientForTest(server)
	source := &refreshingTokenSource{
		manager: manager,
		client:  client,
		now:     func() time.Time { return now },
		newID:   func() string { return "550e8400-e29b-41d4-a716-446655440051" },
	}
	if _, err := source.Token(context.Background()); err == nil {
		t.Fatal("simulated response loss unexpectedly succeeded")
	}
	unchanged, err := manager.Load()
	if err != nil || unchanged.RefreshToken != "vcm_rt_original" || unchanged.RefreshRequestID != "550e8400-e29b-41d4-a716-446655440051" {
		t.Fatalf("response loss corrupted stored rotation state: %#v err=%v", unchanged, err)
	}
	token, err := source.Token(context.Background())
	if err != nil || token != "vcm_at_recovered" {
		t.Fatalf("refresh retry did not recover: token=%q err=%v", token, err)
	}
	completed, err := manager.Load()
	if err != nil || completed.RefreshRequestID != "" {
		t.Fatalf("recovered refresh did not clear request identity: %#v err=%v", completed, err)
	}
}

func TestRefreshingTokenSourceSerializesConcurrentCredentialTransactions(t *testing.T) {
	now := time.Now().UTC()
	requestStarted := make(chan struct{})
	releaseResponse := make(chan struct{})
	var refreshCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/refresh" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		var input api.RefreshTokenRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input.RefreshToken != "vcm_rt_concurrent" || input.ClientRequestID != "550e8400-e29b-41d4-a716-446655440070" {
			t.Fatalf("unexpected first refresh request %#v", input)
		}
		if refreshCalls.Add(1) == 1 {
			close(requestStarted)
		}
		<-releaseResponse
		writeCommandJSON(t, writer, map[string]any{
			"access_token":       "vcm_at_concurrent_new",
			"refresh_token":      "vcm_rt_concurrent_new",
			"token_type":         "api_key",
			"expires_at":         now.Add(time.Hour).Format(time.RFC3339Nano),
			"refresh_expires_at": now.Add(24 * time.Hour).Format(time.RFC3339Nano),
			"user_id":            "550e8400-e29b-41d4-a716-446655440000",
			"scope":              []string{"app:read"},
		})
	}))
	defer server.Close()

	store := &observedCredentialStore{
		MemoryStore: securestore.NewMemory(),
		thirdRead:   make(chan struct{}),
	}
	lockRoot := t.TempDir()
	initialManager := &credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", LockRoot: lockRoot}
	if err := initialManager.Save(credentialauth.Credential{
		AccessToken:      "vcm_at_concurrent_old",
		RefreshToken:     "vcm_rt_concurrent",
		ExpiresAt:        now.Add(-time.Minute),
		RefreshExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	newSource := func(requestID string) *refreshingTokenSource {
		return &refreshingTokenSource{
			manager: &credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", LockRoot: lockRoot},
			client:  apiClientForTest(server),
			now:     func() time.Time { return now },
			newID:   func() string { return requestID },
		}
	}
	results := make(chan struct {
		token string
		err   error
	}, 2)
	go func() {
		token, err := newSource("550e8400-e29b-41d4-a716-446655440070").Token(context.Background())
		results <- struct {
			token string
			err   error
		}{token: token, err: err}
	}()
	<-requestStarted
	go func() {
		token, err := newSource("550e8400-e29b-41d4-a716-446655440071").Token(context.Background())
		results <- struct {
			token string
			err   error
		}{token: token, err: err}
	}()
	<-store.thirdRead
	close(releaseResponse)

	for range 2 {
		result := <-results
		if result.err != nil || result.token != "vcm_at_concurrent_new" {
			t.Fatalf("concurrent refresh token=%q err=%v", result.token, result.err)
		}
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("concurrent credential transaction made %d refresh requests", refreshCalls.Load())
	}
}

type observedCredentialStore struct {
	*securestore.MemoryStore
	reads     atomic.Int32
	thirdRead chan struct{}
	once      sync.Once
}

func (store *observedCredentialStore) Get(key string) (string, error) {
	value, err := store.MemoryStore.Get(key)
	if store.reads.Add(1) >= 3 {
		store.once.Do(func() { close(store.thirdRead) })
	}
	return value, err
}

func apiClientForTest(server *httptest.Server) *api.Client {
	return api.NewClient(server.URL, server.Client(), nil, "test")
}

func writeCommandJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func runCLIWithDependencies(t *testing.T, server *httptest.Server, store securestore.Store, input string, extra Dependencies, args ...string) (int, string, string, securestore.Store) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if store == nil {
		store = securestore.NewMemory()
	}
	extra.In = strings.NewReader(input)
	extra.Out = &stdout
	extra.ErrOut = &stderr
	extra.Store = store
	if extra.NewID == nil {
		extra.NewID = func() string { return "550e8400-e29b-41d4-a716-446655440010" }
	}
	if extra.Sleep == nil {
		extra.Sleep = func(context.Context, time.Duration) error { return nil }
	}
	if extra.Environment.Home == "" {
		extra.Environment = skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()}
	}
	if extra.Now == nil {
		extra.Now = time.Now
	}
	if server != nil {
		extra.HTTPClient = server.Client()
		extra.APIBaseURL = server.URL
	}
	if extra.APIBaseURL != "" {
		canonical := &credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default"}
		if credential, loadErr := canonical.Load(); loadErr == nil && credential.AccessToken == "test-token" {
			scope, scopeErr := customCredentialScope(extra.APIBaseURL)
			if scopeErr != nil {
				t.Fatal(scopeErr)
			}
			if saveErr := (&credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", Scope: scope}).Save(credential); saveErr != nil {
				t.Fatal(saveErr)
			}
		}
	}
	code := Execute(args, extra)
	return code, stdout.String(), stderr.String(), store
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "../.."))
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func jsonHTTPResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}
