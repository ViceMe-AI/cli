package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestDefaultRegionUsesChinaEndpoint(t *testing.T) {
	t.Setenv("VICEME_API_BASE_URL", "")
	_, runtime, err := NewRoot(Dependencies{
		Store:       securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.region != config.RegionCN || runtime.apiBaseURL != "https://api.viceme.cn" {
		t.Fatalf("region=%q API base URL=%q", runtime.region, runtime.apiBaseURL)
	}
}

func TestAPIBaseURLEnvironmentOverride(t *testing.T) {
	t.Setenv("VICEME_API_BASE_URL", "http://localhost:3000")
	_, runtime, err := NewRoot(Dependencies{
		Store:       securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.apiBaseURL != "http://localhost:3000" {
		t.Fatalf("environment API base URL = %q", runtime.apiBaseURL)
	}
	if !strings.HasPrefix(runtime.credentialScope, "custom:") {
		t.Fatalf("custom API origin did not receive an isolated credential scope: %q", runtime.credentialScope)
	}
}

func TestCanonicalAPIOverridesPreserveLegacyRegionCredentialScope(t *testing.T) {
	for _, test := range []struct {
		name    string
		region  config.Region
		baseURL string
	}{
		{name: "china exact", region: config.RegionCN, baseURL: "https://api.viceme.cn"},
		{name: "china normalized default port and path", region: config.RegionCN, baseURL: "https://API.VICEME.CN:443/internal/base/"},
		{name: "global exact", region: config.RegionGlobal, baseURL: "https://api.viceme.ai"},
		{name: "global normalized default port and path", region: config.RegionGlobal, baseURL: "https://API.VICEME.AI:443/v1/"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			store := securestore.NewMemory()
			legacy := &credentialauth.Manager{Store: store, Region: string(test.region)}
			if err := legacy.Save(credentialauth.Credential{AccessToken: "legacy-token"}); err != nil {
				t.Fatal(err)
			}
			_, runtime, err := NewRoot(Dependencies{
				APIBaseURL: test.baseURL,
				Region:     test.region,
				Store:      store,
				Environment: skillcontent.Environment{
					Home:      t.TempDir(),
					ConfigDir: t.TempDir(),
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if runtime.credentialScope != "" {
				t.Fatalf("canonical origin selected custom scope %q", runtime.credentialScope)
			}
			credential, err := runtime.manager().Load()
			if err != nil || credential.AccessToken != "legacy-token" {
				t.Fatalf("canonical override lost the legacy credential: %#v err=%v", credential, err)
			}
		})
	}
}

func TestTrulyCustomAPIOriginsUseStableIsolatedScopes(t *testing.T) {
	first, err := credentialScopeForAPIBase("https://DEV.VICEME.EXAMPLE:443/api/one", config.RegionCN)
	if err != nil {
		t.Fatal(err)
	}
	sameOrigin, err := credentialScopeForAPIBase("https://dev.viceme.example/api/two", config.RegionCN)
	if err != nil {
		t.Fatal(err)
	}
	differentPort, err := credentialScopeForAPIBase("https://dev.viceme.example:8443/api/one", config.RegionCN)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first, "custom:") || first != sameOrigin || first == differentPort {
		t.Fatalf("custom origin scopes are not stable and isolated: first=%q same=%q port=%q", first, sameOrigin, differentPort)
	}
}

func TestProfileSelectionRecomputesScopeAgainstFixedAPIOrigin(t *testing.T) {
	t.Parallel()
	configBase := t.TempDir()
	configured := config.Default(config.RegionCN)
	work, err := configured.AddProfile("work", config.RegionGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Save(configBase, configured); err != nil {
		t.Fatal(err)
	}
	store := securestore.NewMemory()
	defaultManager := &credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default"}
	if err := defaultManager.Save(credentialauth.Credential{AccessToken: "default-token"}); err != nil {
		t.Fatal(err)
	}
	_, runtime, err := NewRoot(Dependencies{
		APIBaseURL: "https://api.viceme.cn",
		Store:      store,
		Environment: skillcontent.Environment{
			Home:      t.TempDir(),
			ConfigDir: configBase,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.credentialScope != "" {
		t.Fatalf("default cn profile unexpectedly used scope %q", runtime.credentialScope)
	}
	if err := runtime.selectProfile(work.Name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(runtime.credentialScope, "custom:") {
		t.Fatalf("global profile did not isolate fixed cn endpoint: %q", runtime.credentialScope)
	}
	if _, err := runtime.manager().Load(); err == nil {
		t.Fatal("selected profile reused the default profile credential")
	}
}

func TestCustomAPIOriginCannotReadProductionCredentials(t *testing.T) {
	t.Setenv("VICEME_API_BASE_URL", "http://localhost:3000/dev")
	store := securestore.NewMemory()
	production := &credentialauth.Manager{Store: store, Region: "cn"}
	if err := production.Save(credentialauth.Credential{AccessToken: "production-token"}); err != nil {
		t.Fatal(err)
	}
	_, runtime, err := NewRoot(Dependencies{
		Store:       store,
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := runtime.manager().CurrentStatus()
	if err != nil || status.Authenticated {
		t.Fatalf("custom origin reused production login: %#v err=%v", status, err)
	}
	if err := runtime.manager().Save(credentialauth.Credential{AccessToken: "custom-token"}); err != nil {
		t.Fatal(err)
	}
	credential, err := production.Load()
	if err != nil || credential.AccessToken != "production-token" {
		t.Fatalf("custom login overwrote production credential: %#v err=%v", credential, err)
	}
}

func TestStoredGlobalRegionUsesGlobalEndpoint(t *testing.T) {
	t.Setenv("VICEME_API_BASE_URL", "")
	configBase := t.TempDir()
	if _, err := config.Save(configBase, config.Default(config.RegionGlobal)); err != nil {
		t.Fatal(err)
	}
	_, runtime, err := NewRoot(Dependencies{
		Store:       securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: configBase},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.region != config.RegionGlobal || runtime.apiBaseURL != "https://api.viceme.ai" {
		t.Fatalf("region=%q API base URL=%q", runtime.region, runtime.apiBaseURL)
	}
}

func TestVersionJSONEnvelope(t *testing.T) {
	t.Parallel()
	code, stdout, stderr, _ := runCLI(t, nil, nil, "--version")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["ok"] != true {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
	meta := envelope["meta"].(map[string]any)
	if meta["full_skill_bundle_digest"] == "" || meta["embedded_content_digest"] == "" {
		t.Fatalf("missing digest metadata: %#v", meta)
	}
}

func TestPublishRequiresConfirmationAndExplicitUploadTarget(t *testing.T) {
	t.Parallel()
	code, _, stderr, _ := runCLI(t, nil, nil, "skill", "publish", "https://github.com/acme/skill")
	if code != 10 || !strings.Contains(stderr, "confirmation_required") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	code, _, stderr, _ = runCLI(t, nil, nil, "skill", "publish", "--file", "missing.zip", "--yes")
	if code != 2 || !strings.Contains(stderr, "upload_target") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	code, stdout, stderr, _ := runCLI(t, nil, nil, "skill", "publish", "--file", "missing.zip", "--new-target", "--yes", "--dry-run")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"source_mode":"file"`) ||
		!strings.Contains(stdout, `"publish_mode":"confirm"`) ||
		!strings.Contains(stdout, `"confirmation_scope":"publication_admission/v1"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, _, stderr, _ = runCLI(t, nil, nil, "skill", "publish", "https://github.com/acme/skill", "--target-id", "target_1", "--yes")
	if code != 2 || !strings.Contains(stderr, "target_version") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	code, stdout, stderr, _ = runCLI(t, nil, nil, "skill", "publish", "https://github.com/acme/skill", "--target-id", "target_1", "--expected-target-version", "4", "--yes", "--dry-run")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"expected_target_version":4`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runCLI(t, nil, nil, "skill", "publish", "https://github.com/acme/skill", "--target-id", "target_0", "--expected-target-version", "0", "--yes", "--dry-run")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"expected_target_version":0`) {
		t.Fatalf("explicit version zero was not preserved: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, _, stderr, _ = runCLI(t, nil, nil, "skill", "publish", "https://github.com/acme/skill", "--target-id", "target_0", "--expected-target-version", "-1", "--yes", "--dry-run")
	if code != 2 || !strings.Contains(stderr, "target_version") {
		t.Fatalf("negative target version was accepted: code=%d stderr=%s", code, stderr)
	}
}

func TestPublishTargetAliasCanonicality(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		alias     string
		newTarget bool
		wantCode  int
	}{
		{name: "explicit empty", alias: "", newTarget: true, wantCode: 2},
		{name: "surrounding whitespace", alias: " poster ", newTarget: true, wantCode: 2},
		{name: "more than 191 Unicode characters", alias: strings.Repeat("技", 192), newTarget: true, wantCode: 2},
		{name: "invalid UTF-8", alias: string([]byte{0xff}), newTarget: true, wantCode: 2},
		{name: "alias without new target", alias: "poster", wantCode: 2},
		{name: "legal Unicode alias", alias: strings.Repeat("技", 191), newTarget: true, wantCode: 0},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			args := []string{"skill", "publish", "https://github.com/acme/skill", "--target-alias", test.alias, "--yes", "--dry-run"}
			if test.newTarget {
				args = append(args, "--new-target")
			}
			code, stdout, stderr, _ := runCLI(t, nil, nil, args...)
			if code != test.wantCode {
				t.Fatalf("code=%d want=%d stdout=%s stderr=%s", code, test.wantCode, stdout, stderr)
			}
			if test.wantCode == 0 {
				if stderr != "" || !strings.Contains(stdout, `"alias":"`+test.alias+`"`) {
					t.Fatalf("legal alias was not preserved: stdout=%s stderr=%s", stdout, stderr)
				}
			} else if !strings.Contains(stderr, "target_alias") {
				t.Fatalf("invalid alias returned the wrong error: %s", stderr)
			}
		})
	}
}

func TestAuthNoWaitJSONNeverReturnsToken(t *testing.T) {
	t.Parallel()
	completeURL := "https://viceme.test/cli/auth?user_code=ABCD-EFGH"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/device" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"verification_url":"https://viceme.test/cli/auth","verification_url_complete":"`+completeURL+`","device_code":"device-public","user_code":"ABCD-EFGH","expires_at":"2030-01-01T00:00:00Z","interval_seconds":2}`)
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLI(t, server, nil, "auth", "login", "--no-wait", "--json")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "device-public") || !strings.Contains(stdout, `"profile":"default"`) || !strings.Contains(stdout, `"region":"cn"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "access_token") || strings.Contains(stderr, "access_token") {
		t.Fatal("login start leaked an access token field")
	}
	var envelope struct {
		Data struct {
			VerificationURL         string `json:"verification_url"`
			VerificationURLComplete string `json:"verification_url_complete"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.VerificationURL != completeURL || envelope.Data.VerificationURLComplete != completeURL {
		t.Fatalf("CLI did not return the direct browser URL: %#v", envelope.Data)
	}
}

func TestAuthLoginGuidesHumanAndWaitsForAuthorization(t *testing.T) {
	t.Parallel()
	completeURL := "https://viceme.test/cli/auth?user_code=ABCD-EFGH"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/auth/device":
			_, _ = io.WriteString(writer, `{"verification_url":"https://viceme.test/cli/auth","verification_url_complete":"`+completeURL+`","device_code":"device-public","user_code":"ABCD-EFGH","expires_at":"2030-01-01T00:00:00Z","interval_seconds":0}`)
		case "/v1/cli/auth/token":
			_, _ = io.WriteString(writer, `{"access_token":"top-secret","refresh_token":"refresh-secret","user_id":"user_1","expires_at":"2030-01-01T00:00:00Z"}`)
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	store := securestore.NewMemory()
	code, stdout, stderr, _ := runCLI(t, server, store, "auth", "login")
	if code != 0 || stdout != "" {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	for _, expected := range []string{completeURL, "ABCD-EFGH", "Waiting for authorization...", "Authorization successful.", "Profile: default", "Region: cn"} {
		if !strings.Contains(stderr, expected) {
			t.Fatalf("human login output lacks %q: %s", expected, stderr)
		}
	}
	for _, secret := range []string{"device-public", "top-secret", "refresh-secret", `{"ok":`} {
		if strings.Contains(stderr, secret) {
			t.Fatalf("human login leaked %q: %s", secret, stderr)
		}
	}
	scope, err := customCredentialScope(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := &credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", Scope: scope}
	credential, err := manager.Load()
	if err != nil || credential.AccessToken != "top-secret" {
		t.Fatalf("credential was not stored: credential=%#v err=%v", credential, err)
	}
}

func TestAgentLoginFlowRequiresJSON(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args    []string
		subtype string
	}{
		{args: []string{"auth", "login", "--no-wait"}, subtype: "auth_json_required"},
		{args: []string{"auth", "login", "--device-code", "device-public"}, subtype: "auth_json_required"},
		{args: []string{"auth", "login", "--json"}, subtype: "auth_json_flow"},
	} {
		code, stdout, stderr, _ := runCLI(t, nil, nil, test.args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, `"subtype":"`+test.subtype+`"`) {
			t.Fatalf("args=%v code=%d stdout=%s stderr=%s", test.args, code, stdout, stderr)
		}
	}
}

func TestDeviceLoginStoresTokenButDoesNotPrintIt(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/token" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"access_token":"top-secret","refresh_token":"refresh-secret","user_id":"user_1","expires_at":"2030-01-01T00:00:00Z"}`)
	}))
	defer server.Close()
	store := securestore.NewMemory()
	code, stdout, stderr, _ := runCLI(t, server, store, "auth", "login", "--device-code", "device-public", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "top-secret") || strings.Contains(stdout, "refresh-secret") {
		t.Fatal("completed login leaked credentials")
	}
	scope, err := customCredentialScope(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := &credentialauth.Manager{Store: store, Region: "cn", Scope: scope}
	credential, err := manager.Load()
	if err != nil {
		t.Fatal(err)
	}
	if credential.AccessToken != "top-secret" {
		t.Fatal("credential was not stored")
	}
}

func TestProcessCredentialUsesStandardAPIKeyWithoutPersistenceOrOutput(t *testing.T) {
	const processToken = "vpa_process_only_secret"
	t.Setenv(processAccessTokenEnvironment, processToken)
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("x-api-key") != processToken {
			t.Fatalf("standard API key header = %q", request.Header.Get("x-api-key"))
		}
		return jsonHTTPResponse(request, http.StatusAccepted, `{"publication_id":"pub_process","status":"received"}`), nil
	})
	store := securestore.NewMemory()
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, store, "", Dependencies{
		HTTPClient: &http.Client{Transport: transport},
		APIBaseURL: "https://api.viceme.test",
	}, "skill", "publish", "--resolution-id", "res_process", "--yes")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "pub_process") || strings.Contains(stdout, processToken) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if _, err := (&credentialauth.Manager{Store: store, Region: "cn"}).Load(); err == nil {
		t.Fatal("process credential was persisted to the keychain")
	}

	code, stdout, stderr, _ = runCLIWithDependencies(t, nil, store, "", Dependencies{}, "auth", "status")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"source":"process"`) ||
		!strings.Contains(stdout, `"persistent":false`) || strings.Contains(stdout, processToken) {
		t.Fatalf("status code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, _, stderr, _ = runCLIWithDependencies(t, nil, store, "", Dependencies{}, "auth", "logout")
	if code == 0 || !strings.Contains(stderr, "process_credential_active") || strings.Contains(stderr, processToken) {
		t.Fatalf("logout code=%d stderr=%s", code, stderr)
	}
}

func TestProcessCredentialRejectsNonCanonicalSecret(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, " secret-with-space ")
	_, _, err := NewRoot(Dependencies{
		Store:       securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()},
	})
	if err == nil || !strings.Contains(err.Error(), "process publication credential is invalid") {
		t.Fatalf("error = %v", err)
	}
}

func TestInspectAndPublishRequestContracts(t *testing.T) {
	t.Parallel()
	var inspected atomic.Bool
	var published atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("x-api-key") != "test-token" {
			t.Fatalf("missing API key: %q", request.Header.Get("x-api-key"))
		}
		switch request.URL.Path {
		case "/v1/skill-agent-publications/inspect":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			source := body["source"].(map[string]any)
			if source["kind"] != "expression" || source["value"] != "copied expression" {
				t.Fatalf("unexpected inspect body: %#v", body)
			}
			inspected.Store(true)
			_, _ = io.WriteString(writer, `{"resolution_id":"res_1","destination":{"mode":"new_auto"}}`)
		case "/v1/skill-agent-publications":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			if body["client_request_id"] != "request-fixed" {
				t.Fatalf("unexpected request id: %#v", body)
			}
			destination := body["destination"].(map[string]any)
			if destination["mode"] != "auto" {
				t.Fatalf("unexpected destination: %#v", destination)
			}
			options := body["options"].(map[string]any)
			if options["publish_mode"] != "confirm" || options["admission_confirmation"] != true {
				t.Fatalf("unexpected publication admission: %#v", options)
			}
			published.Store(true)
			writer.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"received","status_url":"/v1/skill-agent-publications/pub_1"}`)
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	store := authenticatedStore(t)
	code, stdout, stderr, _ := runCLIWithInput(t, server, store, "copied expression", "skill", "inspect", "--expression-stdin")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "res_1") || !inspected.Load() {
		t.Fatalf("inspect code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runCLI(t, server, store, "skill", "publish", "https://github.com/acme/skill", "--yes")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "pub_1") || !published.Load() {
		t.Fatalf("publish code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestPublishRetriesAmbiguousTransportWithSameRequestID(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("x-api-key") != "test-token" {
			t.Fatal("retry changed or omitted the standard credential")
		}
		if calls.Add(1) == 1 {
			return nil, fmt.Errorf("connection closed after write")
		}
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		if body["client_request_id"] != "request-fixed" {
			t.Fatalf("retry changed request id: %#v", body)
		}
		options := body["options"].(map[string]any)
		if options["publish_mode"] != "confirm" || options["admission_confirmation"] != true {
			t.Fatalf("retry changed publication admission: %#v", options)
		}
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"publication_id":"pub_retry","status":"received"}`)),
			Request:    request,
		}, nil
	})
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, authenticatedStore(t), "", Dependencies{
		HTTPClient: &http.Client{Transport: transport},
		APIBaseURL: "https://api.viceme.test",
	}, "skill", "publish", "--resolution-id", "res_retry", "--client-request-id", "request-fixed", "--yes")
	if code != 0 || stderr != "" || calls.Load() != 2 || !strings.Contains(stdout, "pub_retry") {
		t.Fatalf("code=%d calls=%d stdout=%s stderr=%s", code, calls.Load(), stdout, stderr)
	}
}

func TestJobWaitReturnsBusinessFailureWithExitZero(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skill-agent-publications/pub_1" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"compiling"}`)
			return
		}
		_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"unsupported","failure":{"code":"UNSUPPORTED"}}`)
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t), "job", "wait", "pub_1")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"status":"unsupported"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobRetryRequiresConfirmationAndUsesExplicitRetryEndpoint(t *testing.T) {
	t.Parallel()
	code, _, stderr, _ := runCLI(t, nil, nil, "job", "retry", "pub_1")
	if code != 10 || !strings.Contains(stderr, "confirmation_required") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/skill-agent-publications/pub_1/retry" {
			t.Fatalf("unexpected retry request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("x-api-key") != "test-token" {
			t.Fatalf("missing API key: %q", request.Header.Get("x-api-key"))
		}
		writer.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"source_resolved","retry_ordinal":1}`)
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t), "job", "retry", "pub_1", "--yes")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"retry_ordinal":1`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestSkillsInstallAndDoctorCommands(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, nil, "", Dependencies{
		Environment: skillcontent.Environment{Home: home},
	}, "skills", "install", "--target", "codex")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"all_succeeded":true`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runCLIWithDependencies(t, nil, nil, "", Dependencies{
		Environment: skillcontent.Environment{Home: home},
	}, "skills", "doctor", "--target", "codex")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"healthy":true`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestSkillInstallPartialFailureIsTyped(t *testing.T) {
	t.Parallel()
	code, stdout, stderr, _ := runCLI(t, nil, nil, "skills", "install", "--target", "unknown")
	if code != 5 || stdout != "" || !strings.Contains(stderr, "skill_install_partial") || !strings.Contains(stderr, "all_succeeded") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobWaitTimeoutReturnsLastStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"compiling"}`)
	}))
	defer server.Close()
	base := time.Now()
	var nowCalls atomic.Int32
	code, stdout, stderr, _ := runCLIWithDependencies(t, server, authenticatedStore(t), "", Dependencies{
		Now: func() time.Time {
			if nowCalls.Add(1) == 1 {
				return base
			}
			return base.Add(2 * time.Second)
		},
	}, "job", "wait", "pub_1", "--timeout", "1s")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"wait_timed_out":true`) || !strings.Contains(stdout, `"status":"compiling"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func authenticatedStore(t *testing.T) *securestore.MemoryStore {
	t.Helper()
	store := securestore.NewMemory()
	manager := &credentialauth.Manager{Store: store, Region: "cn"}
	if err := manager.Save(credentialauth.Credential{AccessToken: "test-token", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return store
}

func runCLI(t *testing.T, server *httptest.Server, store securestore.Store, args ...string) (int, string, string, securestore.Store) {
	t.Helper()
	return runCLIWithInput(t, server, store, "", args...)
}

func runCLIWithInput(t *testing.T, server *httptest.Server, store securestore.Store, input string, args ...string) (int, string, string, securestore.Store) {
	t.Helper()
	return runCLIWithDependencies(t, server, store, input, Dependencies{}, args...)
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
		extra.NewID = func() string { return "request-fixed" }
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
	// Tests that seed the default-profile canonical login explicitly mirror it into the
	// custom httptest origin. Production code never performs this migration.
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
