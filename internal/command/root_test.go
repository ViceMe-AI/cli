package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

type preflightFailureStore struct{}

func (s *preflightFailureStore) Get(string) (string, error) { return "", securestore.ErrNotFound }
func (s *preflightFailureStore) Set(string, string) error   { return nil }
func (s *preflightFailureStore) Delete(string) error        { return nil }
func (s *preflightFailureStore) Preflight(string) error {
	return fmt.Errorf("sandbox cannot persist credentials")
}

type saveFailureStore struct{}

func (saveFailureStore) Get(string) (string, error) { return "", securestore.ErrNotFound }
func (saveFailureStore) Set(string, string) error   { return fmt.Errorf("disk became read-only") }
func (saveFailureStore) Delete(string) error        { return nil }
func (saveFailureStore) Preflight(string) error     { return nil }

type blockedKeyringStore struct{}

func (blockedKeyringStore) Get(string) (string, error) { return "", fmt.Errorf("keychain blocked") }
func (blockedKeyringStore) Set(string, string) error   { return fmt.Errorf("keychain blocked") }
func (blockedKeyringStore) Delete(string) error        { return fmt.Errorf("keychain blocked") }

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

func TestEnvironmentOverridesExplicitLocalProfile(t *testing.T) {
	const profileToken = "profile-secret"
	const processToken = "process-secret"
	configBase := t.TempDir()
	configured := config.Default(config.RegionCN)
	configured.Profiles[0].APIBaseURL = "http://localhost:8090"
	configured.Profiles[0].AccessToken = profileToken
	if _, err := config.Save(configBase, configured); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VICEME_API_BASE_URL", "http://localhost:9090")
	t.Setenv(processAccessTokenEnvironment, processToken)
	_, runtime, err := NewRoot(Dependencies{
		Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{
			Home:      t.TempDir(),
			ConfigDir: configBase,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.apiBaseURL != "http://localhost:9090" {
		t.Fatalf("API base URL=%q", runtime.apiBaseURL)
	}
	if token, source, persistent := runtime.overrideCredential(); token != processToken || source != "process" || persistent {
		t.Fatalf("override token=%q source=%q persistent=%v", token, source, persistent)
	}
}

func TestAPIEnvironmentOverrideDoesNotForwardLocalProfileTokenToAnotherOrigin(t *testing.T) {
	configBase := t.TempDir()
	configured := config.Default(config.RegionCN)
	configured.Profiles[0].APIBaseURL = "http://localhost:8090"
	configured.Profiles[0].AccessToken = "profile-secret"
	if _, err := config.Save(configBase, configured); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VICEME_API_BASE_URL", "http://localhost:9090")
	t.Setenv(processAccessTokenEnvironment, "")
	_, runtime, err := NewRoot(Dependencies{
		Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{
			Home:      t.TempDir(),
			ConfigDir: configBase,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if token, source, _ := runtime.overrideCredential(); token != "" || source != "" {
		t.Fatalf("profile token crossed origins: token=%q source=%q", token, source)
	}
}

func TestDeviceLoginDoesNotBackfillExplicitProfileOverrides(t *testing.T) {
	configBase := t.TempDir()
	configured := config.Default(config.RegionCN)
	configured.Profiles[0].APIBaseURL = "http://localhost:8090"
	if _, err := config.Save(configBase, configured); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/token" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"access_token":"device-secret","user_id":"user-local","expires_at":"2030-01-01T00:00:00Z"}`)
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLIWithDependencies(t, server, securestore.NewMemory(), "", Dependencies{
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: configBase},
	}, "auth", "login", "--device-code", "device-local", "--json")
	if code != 0 || stderr != "" || strings.Contains(stdout, "device-secret") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	loaded, err := config.LoadOrDefault(configBase)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := loaded.Resolve("default")
	if err != nil || profile.APIBaseURL != "http://localhost:8090" || profile.AccessToken != "" {
		t.Fatalf("normal login changed explicit profile overrides: %#v err=%v", profile, err)
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

func TestVersionWritesBareBusinessJSON(t *testing.T) {
	t.Parallel()
	code, stdout, stderr, _ := runCLI(t, nil, nil, "--version")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	for _, transportField := range []string{"ok", "data", "meta"} {
		if _, exists := result[transportField]; exists {
			t.Fatalf("version contains transport field %q: %#v", transportField, result)
		}
	}
	if result["version"] == "" || result["skill_version"] == "" ||
		result["full_skill_bundle_digest"] == "" || result["embedded_content_digest"] == "" {
		t.Fatalf("missing version diagnostics: %#v", result)
	}
	if !strings.Contains(stdout, "\n  \"version\": ") {
		t.Fatalf("version output is not indented: %q", stdout)
	}
}

func TestAuthStatusWritesBareIndentedBusinessJSON(t *testing.T) {
	t.Parallel()
	code, stdout, stderr, _ := runCLI(t, nil, nil, "auth", "status")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if result["authenticated"] != false || result["profile"] != "default" || result["region"] != "cn" {
		t.Fatalf("unexpected auth status: %#v", result)
	}
	for _, transportField := range []string{"ok", "data", "meta"} {
		if _, exists := result[transportField]; exists {
			t.Fatalf("auth status contains transport field %q: %#v", transportField, result)
		}
	}
	if !strings.Contains(stdout, "\n  \"authenticated\": false,") {
		t.Fatalf("auth status is not indented: %q", stdout)
	}
}

func TestSkillsReadWritesRawContent(t *testing.T) {
	t.Parallel()
	code, stdout, stderr, _ := runCLI(t, nil, nil, "skills", "read", "viceme")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.HasPrefix(stdout, "---\n") || !strings.Contains(stdout, "\nname: viceme\n") {
		t.Fatalf("skills read did not return raw SKILL.md: %q", stdout)
	}
	if stringContains(stdout, `"ok"`) || stringContains(stdout, `"data"`) || stringContains(stdout, `"content"`) {
		t.Fatalf("skills read wrapped raw content: %q", stdout)
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
	if code != 0 || stderr != "" || !stringContains(stdout, `"source_mode":"file"`) ||
		!stringContains(stdout, `"publish_mode":"confirm"`) ||
		!stringContains(stdout, `"confirmation_scope":"publication_admission/v1"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, _, stderr, _ = runCLI(t, nil, nil, "skill", "publish", "https://github.com/acme/skill", "--target-id", "target_1", "--yes")
	if code != 2 || !strings.Contains(stderr, "target_version") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	code, stdout, stderr, _ = runCLI(t, nil, nil, "skill", "publish", "https://github.com/acme/skill", "--target-id", "target_1", "--expected-target-version", "4", "--yes", "--dry-run")
	if code != 0 || stderr != "" || !stringContains(stdout, `"expected_target_version":4`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runCLI(t, nil, nil, "skill", "publish", "https://github.com/acme/skill", "--target-id", "target_0", "--expected-target-version", "0", "--yes", "--dry-run")
	if code != 0 || stderr != "" || !stringContains(stdout, `"expected_target_version":0`) {
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
				if stderr != "" || !stringContains(stdout, `"alias":"`+test.alias+`"`) {
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
	if code != 0 || stderr != "" || !strings.Contains(stdout, "device-public") || !stringContains(stdout, `"profile":"default"`) || !stringContains(stdout, `"region":"cn"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "access_token") || strings.Contains(stderr, "access_token") {
		t.Fatal("login start leaked an access token field")
	}
	var result struct {
		VerificationURL         string `json:"verification_url"`
		VerificationURLComplete string `json:"verification_url_complete"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if result.VerificationURL != completeURL || result.VerificationURLComplete != completeURL {
		t.Fatalf("CLI did not return the direct browser URL: %#v", result)
	}
}

func TestDeviceLoginPreflightFailureDoesNotCreateOrExchangeAuthorization(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "before create", args: []string{"auth", "login", "--no-wait", "--json"}},
		{name: "before exchange", args: []string{"auth", "login", "--device-code", "device-once", "--json"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				calls.Add(1)
			}))
			defer server.Close()
			code, stdout, stderr, _ := runCLI(t, server, &preflightFailureStore{}, test.args...)
			if code != 3 || stdout != "" || !stringContains(stderr, `"subtype":"credential_store_unavailable"`) {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			if calls.Load() != 0 {
				t.Fatalf("server was called %d times after persistence preflight failed", calls.Load())
			}
		})
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
		if code != 2 || stdout != "" || !stringContains(stderr, `"subtype":"`+test.subtype+`"`) {
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

func TestDeviceLoginSaveFailureReturnsRecoverableConsumedAuthorizationContract(t *testing.T) {
	t.Parallel()
	var exchanges atomic.Int32
	var revocations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/auth/token":
			exchanges.Add(1)
			_, _ = io.WriteString(writer, `{"access_token":"issued-secret","user_id":"user_1"}`)
		case "/v1/cli/auth/revoke":
			revocations.Add(1)
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLI(t, server, saveFailureStore{}, "auth", "login", "--device-code", "device-once", "--json")
	if code != 3 || stdout != "" || !stringContains(stderr, `"subtype":"credential_persistence_failed"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	for _, forbidden := range []string{"issued-secret", "device-once"} {
		if strings.Contains(stderr, forbidden) {
			t.Fatalf("save failure leaked %q: %s", forbidden, stderr)
		}
	}
	if !stringContains(stderr, `"authorization_consumed":true`) || !stringContains(stderr, `"issued_credential_revoked":true`) {
		t.Fatalf("save failure omitted recovery details: %s", stderr)
	}
	if exchanges.Load() != 1 || revocations.Load() != 1 {
		t.Fatalf("exchanges=%d revocations=%d", exchanges.Load(), revocations.Load())
	}
}

func TestSandboxFallbackNeverWritesDeviceTokenToProfileConfig(t *testing.T) {
	t.Parallel()
	configBase := t.TempDir()
	store := securestore.NewEncryptedFile(configBase, "viceme-cli-test", blockedKeyringStore{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/token" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"access_token":"sandbox-device-secret","user_id":"sandbox-user"}`)
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLIWithDependencies(t, server, store, "", Dependencies{
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: configBase},
	}, "auth", "login", "--device-code", "device-sandbox", "--json")
	if code != 0 || stderr != "" || strings.Contains(stdout, "sandbox-device-secret") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	configData, err := os.ReadFile(config.ConfigPath(configBase))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configData), "sandbox-device-secret") || strings.Contains(string(configData), "access_token") {
		t.Fatalf("profile config contains device credential material: %s", configData)
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
	if code != 0 || stderr != "" || !stringContains(stdout, `"source":"process"`) ||
		!stringContains(stdout, `"persistent":false`) || strings.Contains(stdout, processToken) {
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
	if code != 0 || stderr != "" || !stringContains(stdout, `"status":"unsupported"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobBindReturnsSignedBrowserAction(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skill-agent-publications/pub_binding" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"publication_id":"pub_binding","status":"binding_required","next_action":{"type":"bind_channel_account","provider":"github","binding_status":"required","binding_url":"https://viceme.example/channel-bindings/signed","expires_at":"2030-01-01T00:00:00Z","retry_mode":"new_publication","hints":[{"type":"download_source"},{"type":"fork_source"}]}}`)
	}))
	defer server.Close()

	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t), "job", "bind", "pub_binding")
	if code != 0 || stderr != "" ||
		!stringContains(stdout, `"binding_url":"https://viceme.example/channel-bindings/signed"`) ||
		!stringContains(stdout, `"retry_mode":"new_publication"`) ||
		!stringContains(stdout, `"type":"fork_source"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobBindRequiresAuthoritativeTerminalContract(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		response string
		code     string
	}{
		{
			name:     "publication is not binding required",
			response: `{"publication_id":"pub_binding","status":"failed","next_action":{"type":"bind_channel_account","binding_url":"https://viceme.example/channel-bindings/signed","retry_mode":"new_publication"}}`,
			code:     "channel_binding_not_required",
		},
		{
			name:     "retry mode is missing",
			response: `{"publication_id":"pub_binding","status":"binding_required","next_action":{"type":"bind_channel_account","binding_url":"https://viceme.example/channel-bindings/signed"}}`,
			code:     "channel_binding_retry_mode_invalid",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				_, _ = io.WriteString(writer, test.response)
			}))
			defer server.Close()

			code, _, stderr, _ := runCLI(t, server, authenticatedStore(t), "job", "bind", "pub_binding")
			if code == 0 || !strings.Contains(stderr, test.code) {
				t.Fatalf("code=%d stderr=%s", code, stderr)
			}
		})
	}
}

func TestJobMetadataReadAndResolveContract(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/metadata" && request.Method == http.MethodGet:
			_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"meta_review","title":"海报文案","description":"为产品海报写文案","author":"acme/poster","missing":[],"action_id":"meta_1","expires_at":"2030-01-01T00:00:00Z"}`)
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/metadata/resolve" && request.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode resolve body: %v", err)
			}
			if body["action_id"] != "meta_1" || body["decision"] != "confirm" || body["title"] != "探针海报" ||
				body["author"] != "acme/ops" || body["expected_payload_digest"] != "sha256:payload" {
				t.Fatalf("metadata resolve body = %#v", body)
			}
			_, _ = io.WriteString(writer, `{"action_id":"meta_1","status":"resolved","publication_status":"meta_confirmed","resolution_digest":"sha256:resolution","resolved_at":"2026-07-20T00:00:00Z"}`)
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t), "job", "metadata", "pub_1")
	if code != 0 || stderr != "" || !stringContains(stdout, `"author":"acme/poster"`) || !stringContains(stdout, `"status":"meta_review"`) {
		t.Fatalf("metadata read: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runCLI(t, server, authenticatedStore(t),
		"job", "metadata", "pub_1",
		"--action-id", "meta_1",
		"--expected-payload-digest", "sha256:payload",
		"--decision", "confirm",
		"--title", "探针海报",
		"--author", "acme/ops",
	)
	if code != 0 || stderr != "" || !stringContains(stdout, `"publication_status":"meta_confirmed"`) {
		t.Fatalf("metadata resolve: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobMetadataDecisionValidation(t *testing.T) {
	t.Parallel()
	code, _, stderr, _ := runCLI(t, nil, authenticatedStore(t),
		"job", "metadata", "pub_1", "--decision", "maybe")
	if code != 2 || !strings.Contains(stderr, "metadata_decision") {
		t.Fatalf("invalid decision: code=%d stderr=%s", code, stderr)
	}
	code, _, stderr, _ = runCLI(t, nil, authenticatedStore(t),
		"job", "metadata", "pub_1", "--decision", "confirm")
	if code != 2 || !strings.Contains(stderr, "metadata_flags") {
		t.Fatalf("missing action flags: code=%d stderr=%s", code, stderr)
	}
}

func TestHostTypedActionLoopPreviewEditRunAccept(t *testing.T) {
	t.Parallel()
	const editRequest = "把标题改成探针海报\n保留原文：$(touch /tmp/viceme-pwned) `whoami` \"quoted\""
	var edits, runs, accepts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/preview" && request.Method == http.MethodGet:
			_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"awaiting_action","preview":{"title":"海报文案","description":"为产品海报写文案","author":"acme/poster","input_method":"提供输入参数:theme(必填)","usage":"为产品海报写文案","output_description":"一句主标题和一段卖点","release_candidate_digest":"sha256:cand1","public_summary_digest":"sha256:summary1","preview_expires_at":"2030-01-01T00:00:00Z"}}`)
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/edits" && request.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode edit body: %v", err)
			}
			if body["edit_request"] != editRequest || body["current_candidate_digest"] != "sha256:cand1" {
				t.Fatalf("edit body = %#v", body)
			}
			edits.Add(1)
			_, _ = io.WriteString(writer, `{"edit_id":"edit_1","status":"pending"}`)
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/edits/edit_1" && request.Method == http.MethodGet:
			_, _ = io.WriteString(writer, `{"edit_id":"edit_1","status":"applied","class":"presentation","result_candidate_digest":"sha256:cand2"}`)
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/preview-runs" && request.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode run body: %v", err)
			}
			if body["expected_candidate_digest"] != "sha256:cand2" {
				t.Fatalf("run body = %#v", body)
			}
			runs.Add(1)
			_, _ = io.WriteString(writer, `{"preview_run_id":"run_1","status":"running"}`)
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/preview-runs/run_1" && request.Method == http.MethodGet:
			_, _ = io.WriteString(writer, `{"publication_id":"pub_1","preview_run_id":"run_1","runner_run_id":"rr_1","candidate_digest":"sha256:cand2","inputs_digest":"sha256:inputs","status":"succeeded","result":{"outcome":"succeeded","finish_report":{"title":"海报文案已生成","summary":"已生成"},"output_links":[]},"accepted":false}`)
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/preview-runs/run_1/accept" && request.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode accept body: %v", err)
			}
			if body["expected_candidate_digest"] != "sha256:cand2" || body["expected_inputs_digest"] != "sha256:inputs" {
				t.Fatalf("accept body = %#v", body)
			}
			accepts.Add(1)
			_, _ = io.WriteString(writer, `{"publication_id":"pub_1","preview_run_id":"run_1","status":"succeeded","accepted_at":"2026-07-20T00:00:00Z"}`)
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	// Host 闭环:展示摘要 → 自然语言编辑 → 新候选试跑 → 接受结果。
	// preview 原样透传 public_summary_digest,供 resume 的确认门绑定。
	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t), "job", "preview", "pub_1")
	if code != 0 || stderr != "" || !stringContains(stdout, `"author":"acme/poster"`) || !stringContains(stdout, `"input_method"`) ||
		!stringContains(stdout, `"public_summary_digest":"sha256:summary1"`) {
		t.Fatalf("preview: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	code, stdout, stderr, _ = runCLIWithInput(t, server, authenticatedStore(t), editRequest,
		"job", "edit", "pub_1", "--candidate-digest", "sha256:cand1", "--request-stdin", "--timeout", "10s")
	if code != 0 || stderr != "" || !stringContains(stdout, `"result_candidate_digest":"sha256:cand2"`) {
		t.Fatalf("edit: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	code, stdout, stderr, _ = runCLI(t, server, authenticatedStore(t),
		"job", "run", "pub_1", "--candidate-digest", "sha256:cand2", "--input", "theme=咖啡", "--timeout", "10s")
	if code != 0 || stderr != "" || !stringContains(stdout, `"status":"succeeded"`) {
		t.Fatalf("run: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	code, stdout, stderr, _ = runCLI(t, server, authenticatedStore(t),
		"job", "accept", "pub_1", "--run-id", "run_1", "--candidate-digest", "sha256:cand2", "--inputs-digest", "sha256:inputs")
	if code != 0 || stderr != "" || !stringContains(stdout, `"accepted_at"`) {
		t.Fatalf("accept: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if edits.Load() != 1 || runs.Load() != 1 || accepts.Load() != 1 {
		t.Fatalf("typed actions must fire exactly once each: edits=%d runs=%d accepts=%d", edits.Load(), runs.Load(), accepts.Load())
	}
}

func TestJobEditRequiresExplicitNonEmptyStdin(t *testing.T) {
	t.Parallel()
	code, _, stderr, _ := runCLI(t, nil, authenticatedStore(t),
		"job", "edit", "pub_1", "--candidate-digest", "sha256:cand1")
	if code != 2 || !strings.Contains(stderr, "edit_flags") || !strings.Contains(stderr, "--request-stdin") {
		t.Fatalf("missing stdin mode: code=%d stderr=%s", code, stderr)
	}
	code, _, stderr, _ = runCLIWithInput(t, nil, authenticatedStore(t), " \n\t",
		"job", "edit", "pub_1", "--candidate-digest", "sha256:cand1", "--request-stdin")
	if code != 2 || !strings.Contains(stderr, "edit_request") {
		t.Fatalf("empty stdin request: code=%d stderr=%s", code, stderr)
	}
}

func TestJobAcceptRequiresInputsDigest(t *testing.T) {
	t.Parallel()
	code, _, stderr, _ := runCLI(t, nil, authenticatedStore(t),
		"job", "accept", "pub_1", "--run-id", "run_1", "--candidate-digest", "sha256:cand2")
	if code != 2 || !strings.Contains(stderr, "accept_flags") || !strings.Contains(stderr, "--inputs-digest") {
		t.Fatalf("missing inputs digest: code=%d stderr=%s", code, stderr)
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
	if code != 0 || stderr != "" || !stringContains(stdout, `"retry_ordinal":1`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestSkillsInstallAndDoctorCommands(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, nil, "", Dependencies{
		Environment: skillcontent.Environment{Home: home},
	}, "skills", "install", "--target", "codex")
	if code != 0 || stderr != "" || !stringContains(stdout, `"all_succeeded":true`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runCLIWithDependencies(t, nil, nil, "", Dependencies{
		Environment: skillcontent.Environment{Home: home},
	}, "skills", "doctor", "--target", "codex")
	if code != 0 || stderr != "" || !stringContains(stdout, `"healthy":true`) {
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
	if code != 0 || stderr != "" || !stringContains(stdout, `"wait_timed_out":true`) || !stringContains(stdout, `"status":"compiling"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobWaitReturnsImmediatelyForMetadataReview(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"meta_review","next_action":{"type":"confirm_metadata","action_id":"meta_1","payload_digest":"sha256:payload"}}`)
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLIWithDependencies(t, server, authenticatedStore(t), "", Dependencies{
		Sleep: func(context.Context, time.Duration) error {
			t.Fatal("meta_review must return without polling again")
			return nil
		},
	}, "job", "wait", "pub_1", "--timeout", "60s")
	if code != 0 || stderr != "" || stringContains(stdout, `"wait_timed_out"`) ||
		!stringContains(stdout, `"status":"meta_review"`) ||
		!stringContains(stdout, `"type":"confirm_metadata"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobResumeConfirmPublishSendsDecisionContract(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skill-agent-publications/pub_1/actions/act_1/resolve-confirmation" || request.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode resolve body: %v", err)
		}
		if body["decision"] != "confirm" ||
			body["expected_release_candidate_digest"] != "sha256:candidate" ||
			body["expected_public_summary_digest"] != "sha256:summary" ||
			body["expected_payload_digest"] != "sha256:payload" {
			t.Fatalf("confirm_publish resolve body = %#v", body)
		}
		if _, ok := body["payload"]; ok {
			t.Fatalf("decision resolution must not carry a typed payload: %#v", body)
		}
		_, _ = io.WriteString(writer, `{"action_id":"act_1","status":"resolved","publication_status":"release_authorized","resolution_digest":"sha256:resolution","resolved_at":"2026-07-18T00:00:00Z"}`)
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t),
		"job", "resume", "pub_1",
		"--action-id", "act_1",
		"--expected-payload-digest", "sha256:payload",
		"--expected-release-candidate-digest", "sha256:candidate",
		"--expected-public-summary-digest", "sha256:summary",
		"--decision", "confirm",
	)
	if code != 0 || stderr != "" || !stringContains(stdout, `"publication_status":"release_authorized"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobResumeCancelPublishSendsDecisionContract(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skill-agent-publications/pub_1/actions/act_1/resolve-confirmation" || request.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode resolve body: %v", err)
		}
		if body["decision"] != "cancel" ||
			body["expected_release_candidate_digest"] != "sha256:candidate" ||
			body["expected_public_summary_digest"] != "sha256:summary" ||
			body["expected_payload_digest"] != "sha256:payload" {
			t.Fatalf("cancel resolve body = %#v", body)
		}
		if _, ok := body["payload"]; ok {
			t.Fatalf("cancel resolution must not carry a typed payload: %#v", body)
		}
		_, _ = io.WriteString(writer, `{"action_id":"act_1","status":"resolved","publication_status":"cancelled","resolution_digest":"sha256:resolution","resolved_at":"2026-07-18T00:00:00Z"}`)
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t),
		"job", "resume", "pub_1",
		"--action-id", "act_1",
		"--expected-payload-digest", "sha256:payload",
		"--expected-release-candidate-digest", "sha256:candidate",
		"--expected-public-summary-digest", "sha256:summary",
		"--decision", "cancel",
	)
	if code != 0 || stderr != "" || !stringContains(stdout, `"publication_status":"cancelled"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobResumeDecisionValidation(t *testing.T) {
	t.Parallel()
	code, _, stderr, _ := runCLI(t, nil, authenticatedStore(t),
		"job", "resume", "pub_1",
		"--action-id", "act_1",
		"--expected-payload-digest", "sha256:payload",
		"--decision", "confirm",
	)
	if code != 2 || !strings.Contains(stderr, "resume_flags") {
		t.Fatalf("missing candidate digest: code=%d stderr=%s", code, stderr)
	}
	code, _, stderr, _ = runCLI(t, nil, authenticatedStore(t),
		"job", "resume", "pub_1",
		"--action-id", "act_1",
		"--expected-payload-digest", "sha256:payload",
		"--expected-release-candidate-digest", "sha256:candidate",
		"--decision", "confirm",
	)
	if code != 2 || !strings.Contains(stderr, "resume_flags") || !strings.Contains(stderr, "--expected-public-summary-digest") {
		t.Fatalf("missing public summary digest: code=%d stderr=%s", code, stderr)
	}
	code, _, stderr, _ = runCLI(t, nil, authenticatedStore(t),
		"job", "resume", "pub_1",
		"--action-id", "act_1",
		"--expected-payload-digest", "sha256:payload",
		"--expected-release-candidate-digest", "sha256:candidate",
		"--expected-public-summary-digest", "sha256:summary",
		"--decision", "maybe",
	)
	if code != 2 || !strings.Contains(stderr, "resume_decision") {
		t.Fatalf("invalid decision: code=%d stderr=%s", code, stderr)
	}
	code, _, stderr, _ = runCLIWithInput(t, nil, authenticatedStore(t), `{"selector":"skills/poster"}`,
		"job", "resume", "pub_1",
		"--action-id", "act_1",
		"--expected-payload-digest", "sha256:payload",
		"--expected-release-candidate-digest", "sha256:candidate",
		"--expected-public-summary-digest", "sha256:summary",
		"--decision", "confirm",
		"--payload-stdin",
	)
	if code != 2 || !strings.Contains(stderr, "resume_flags") {
		t.Fatalf("decision with payload stdin: code=%d stderr=%s", code, stderr)
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

func TestJobEditStdinPassesNaturalLanguageVerbatim(t *testing.T) {
	t.Parallel()
	// 注入形态载荷:引号、命令替换、反引号、换行、分号都必须原样进 JSON body。
	payload := "把标题改成\"探针\"\n第二行 $(rm -rf /) `whoami`; echo pwned --candidate-digest sha256:fake"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/edits" && request.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode edit body: %v", err)
			}
			if body["edit_request"] != payload {
				t.Fatalf("edit request was not transported verbatim: %#v", body["edit_request"])
			}
			if body["current_candidate_digest"] != "sha256:cand1" {
				t.Fatalf("edit body = %#v", body)
			}
			_, _ = io.WriteString(writer, `{"edit_id":"edit_1","status":"pending"}`)
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/edits/edit_1" && request.Method == http.MethodGet:
			_, _ = io.WriteString(writer, `{"edit_id":"edit_1","status":"applied","class":"behavioral","result_candidate_digest":"sha256:cand2"}`)
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLIWithInput(t, server, authenticatedStore(t), payload,
		"job", "edit", "pub_1", "--candidate-digest", "sha256:cand1", "--request-stdin", "--timeout", "10s")
	if code != 0 || stderr != "" || !stringContains(stdout, `"result_candidate_digest":"sha256:cand2"`) {
		t.Fatalf("stdin edit: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
}

func TestJobEditRejectsArgvRequestTransport(t *testing.T) {
	t.Parallel()
	code, _, stderr, _ := runCLIWithInput(t, nil, authenticatedStore(t), "ignored",
		"job", "edit", "pub_1", "--candidate-digest", "sha256:cand1", "--request", "x", "--request-stdin")
	if code != 2 || !strings.Contains(stderr, "invalid_flag") || !strings.Contains(stderr, "--request") {
		t.Fatalf("argv request transport: code=%d stderr=%s", code, stderr)
	}
	code, _, stderr, _ = runCLIWithInput(t, nil, authenticatedStore(t), "",
		"job", "edit", "pub_1", "--candidate-digest", "sha256:cand1", "--request-stdin")
	if code != 2 || !strings.Contains(stderr, "edit_request") {
		t.Fatalf("empty stdin: code=%d stderr=%s", code, stderr)
	}
}

func TestJobEditTimeoutPreservesCreatedEditID(t *testing.T) {
	t.Parallel()
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/edits" && request.Method == http.MethodPost:
			_, _ = io.WriteString(writer, `{"edit_id":"edit_1","status":"pending"}`)
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/edits/edit_1" && request.Method == http.MethodGet:
			polls.Add(1)
			_, _ = io.WriteString(writer, `{"edit_id":"edit_1","status":"pending"}`)
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()
	// Sleep 直接推进到 deadline:第一次 poll 后超时,输出必须保留 edit_id。
	deps := Dependencies{Sleep: func(ctx context.Context, _ time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	code, stdout, stderr, _ := runCLIWithDependencies(t, server, authenticatedStore(t), "把标题改成探针海报", deps,
		"job", "edit", "pub_1", "--candidate-digest", "sha256:cand1", "--request-stdin", "--timeout", "5s")
	if code != 0 || stderr != "" {
		t.Fatalf("timeout must not fail the command: code=%d stderr=%s", code, stderr)
	}
	if !stringContains(stdout, `"edit_id":"edit_1"`) || !stringContains(stdout, `"wait_timed_out":true`) {
		t.Fatalf("timeout output must preserve the created edit id: %s", stdout)
	}
}

func TestJobRunTimeoutPreservesCreatedRunID(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/preview-runs" && request.Method == http.MethodPost:
			_, _ = io.WriteString(writer, `{"preview_run_id":"run_1","status":"running"}`)
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/preview-runs/run_1" && request.Method == http.MethodGet:
			_, _ = io.WriteString(writer, `{"publication_id":"pub_1","preview_run_id":"run_1","runner_run_id":"rr_1","candidate_digest":"sha256:cand2","status":"running","accepted":false}`)
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()
	deps := Dependencies{Sleep: func(ctx context.Context, _ time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	code, stdout, stderr, _ := runCLIWithDependencies(t, server, authenticatedStore(t), "", deps,
		"job", "run", "pub_1", "--candidate-digest", "sha256:cand2", "--input", "theme=咖啡", "--timeout", "5s")
	if code != 0 || stderr != "" {
		t.Fatalf("timeout must not fail the command: code=%d stderr=%s", code, stderr)
	}
	if !stringContains(stdout, `"preview_run_id":"run_1"`) || !stringContains(stdout, `"wait_timed_out":true`) {
		t.Fatalf("timeout output must preserve the created run id: %s", stdout)
	}
}

func TestJobMetadataEditsStdinContract(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skill-agent-publications/pub_1/metadata/resolve" || request.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode metadata body: %v", err)
		}
		if body["title"] != "海报\"标题\"\n换行" || body["author"] != "acme/poster" {
			t.Fatalf("metadata edits were not transported verbatim: %#v", body)
		}
		if _, ok := body["description"]; ok {
			t.Fatalf("absent fields must stay omitted: %#v", body)
		}
		_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"compiled"}`)
	}))
	defer server.Close()
	edits := `{"title":"海报\"标题\"\n换行","author":"acme/poster"}`
	code, stdout, stderr, _ := runCLIWithInput(t, server, authenticatedStore(t), edits,
		"job", "metadata", "pub_1", "--decision", "confirm",
		"--action-id", "act_meta", "--expected-payload-digest", "sha256:payload", "--edits-stdin")
	if code != 0 || stderr != "" || !stringContains(stdout, `"status":"compiled"`) {
		t.Fatalf("edits-stdin: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	// 与逐字段 flag 混用必须拒绝。
	code, _, stderr, _ = runCLIWithInput(t, nil, authenticatedStore(t), edits,
		"job", "metadata", "pub_1", "--decision", "confirm",
		"--action-id", "act_meta", "--expected-payload-digest", "sha256:payload",
		"--edits-stdin", "--title", "x")
	if code != 2 || !strings.Contains(stderr, "metadata_flags") {
		t.Fatalf("mixed transports: code=%d stderr=%s", code, stderr)
	}
}

func TestJobRunGetReadsSameIDWithAuthoritativeInputs(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skill-agent-publications/pub_1/preview-runs/run_1" || request.Method != http.MethodGet {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"publication_id":"pub_1","preview_run_id":"run_1","runner_run_id":"rr_1","candidate_digest":"sha256:cand2","inputs_digest":"sha256:inputs","inputs":{"theme":"咖啡"},"status":"succeeded","result":{"outcome":"succeeded","finish_report":{"summary":"已生成"},"output_links":[]},"accepted":false}`)
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t), "job", "run-get", "pub_1", "run_1")
	if code != 0 || stderr != "" {
		t.Fatalf("run-get: code=%d stderr=%s", code, stderr)
	}
	// 权威输入值必须与 digest 一并展示,CLI 不再静默丢弃 inputs。
	if !stringContains(stdout, `"inputs":{"theme":"咖啡"}`) || !stringContains(stdout, `"inputs_digest":"sha256:inputs"`) {
		t.Fatalf("authoritative inputs not surfaced: %s", stdout)
	}
}

func TestJobRunGetResumesBoundedWaitAfterProcessRestart(t *testing.T) {
	t.Parallel()
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skill-agent-publications/pub_1/preview-runs/run_1" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if polls.Add(1) < 2 {
			_, _ = io.WriteString(writer, `{"publication_id":"pub_1","preview_run_id":"run_1","runner_run_id":"rr_1","candidate_digest":"sha256:cand2","inputs_digest":"sha256:inputs","inputs":{"theme":"咖啡"},"status":"running","accepted":false}`)
			return
		}
		_, _ = io.WriteString(writer, `{"publication_id":"pub_1","preview_run_id":"run_1","runner_run_id":"rr_1","candidate_digest":"sha256:cand2","inputs_digest":"sha256:inputs","inputs":{"theme":"咖啡"},"status":"succeeded","result":{"outcome":"succeeded","finish_report":{"summary":"已生成"},"output_links":[]},"accepted":false}`)
	}))
	defer server.Close()
	// 进程重启后凭同一 run ID 续等:第一次 poll 仍 running,第二次到终态。
	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t), "job", "run-get", "pub_1", "run_1", "--timeout", "10s")
	if code != 0 || stderr != "" || !stringContains(stdout, `"status":"succeeded"`) {
		t.Fatalf("resumed wait: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if stringContains(stdout, `"wait_timed_out"`) {
		t.Fatalf("completed wait must not carry the timeout marker: %s", stdout)
	}
}

func TestJobEditGetReadsAndResumesSameLogicalEdit(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skill-agent-publications/pub_1/edits/edit_1" || request.Method != http.MethodGet {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"edit_id":"edit_1","status":"applied","class":"behavioral","base_candidate_digest":"sha256:cand1","result_candidate_digest":"sha256:cand2","error":null,"created_at":"2026-07-22T00:00:00Z","completed_at":"2026-07-22T00:01:00Z"}`)
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t), "job", "edit-get", "pub_1", "edit_1")
	if code != 0 || stderr != "" || !stringContains(stdout, `"result_candidate_digest":"sha256:cand2"`) {
		t.Fatalf("edit-get: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
}

func TestJobEditGetTimeoutKeepsSameID(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skill-agent-publications/pub_1/edits/edit_1" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"edit_id":"edit_1","status":"pending"}`)
	}))
	defer server.Close()
	deps := Dependencies{Sleep: func(ctx context.Context, _ time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	code, stdout, stderr, _ := runCLIWithDependencies(t, server, authenticatedStore(t), "", deps,
		"job", "edit-get", "pub_1", "edit_1", "--timeout", "5s")
	if code != 0 || stderr != "" {
		t.Fatalf("timeout must not fail the command: code=%d stderr=%s", code, stderr)
	}
	if !stringContains(stdout, `"edit_id":"edit_1"`) || !stringContains(stdout, `"wait_timed_out":true`) {
		t.Fatalf("timeout output must preserve the same edit id: %s", stdout)
	}
}

func TestConfirmStepsFlowUsesOnlyActionPayloadDigests(t *testing.T) {
	t.Parallel()
	// steps payload 不带 preview_url；payload_digest 位于 action 顶层，另外两个 digest 位于 payload。
	stepsPayload := `{"publication_id":"pub_1","target_id":"t_1","expected_release_candidate_digest":"sha256:cand","expected_public_summary_digest":"sha256:sum","steps":{"title":"海报","author":"acme","input_method":"theme","usage":"写海报","output_description":"一句标题"}}`
	var resolved atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/v1/skill-agent-publications/pub_1" && request.Method == http.MethodGet:
			if resolved.Load() == 0 {
				_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"awaiting_action","next_action":{"type":"confirm_steps","action_id":"act_steps","payload_digest":"sha256:payload","expires_at":"2030-01-01T00:00:00Z","payload":`+stepsPayload+`}}`)
				return
			}
			_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"awaiting_action","next_action":{"type":"confirm_publish","action_id":"act_pub","payload_digest":"sha256:payload2","expires_at":"2030-01-01T00:00:00Z","payload":{"publication_id":"pub_1","target_id":"t_1","expected_release_candidate_digest":"sha256:cand","expected_public_summary_digest":"sha256:sum","preview_url":"https://app.viceme.ai/skill-agent-publications/pub_1/preview?action_id=act_pub","preview_expires_at":"2030-01-01T00:00:00Z"}}}`)
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/actions/act_steps/resolve-confirmation" && request.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode steps resolve body: %v", err)
			}
			if body["expected_payload_digest"] != "sha256:payload" ||
				body["expected_release_candidate_digest"] != "sha256:cand" ||
				body["expected_public_summary_digest"] != "sha256:sum" ||
				body["decision"] != "confirm" {
				t.Fatalf("steps resolution does not bind the payload digests: %#v", body)
			}
			resolved.Add(1)
			_, _ = io.WriteString(writer, `{"action_id":"act_steps","status":"resolved","publication_status":"awaiting_action","resolution_digest":"sha256:res","resolved_at":"2026-07-23T00:00:00Z"}`)
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	// job get:steps action 无 preview_url；按真实 JSON 结构从三个精确路径组装 resume。
	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t), "job", "get", "pub_1")
	if code != 0 || stderr != "" {
		t.Fatalf("job get: code=%d stderr=%s", code, stderr)
	}
	var envelope struct {
		Data struct {
			NextAction struct {
				ActionID      string `json:"action_id"`
				PayloadDigest string `json:"payload_digest"`
				Payload       struct {
					ExpectedReleaseCandidateDigest string `json:"expected_release_candidate_digest"`
					ExpectedPublicSummaryDigest    string `json:"expected_public_summary_digest"`
					PreviewURL                     string `json:"preview_url"`
				} `json:"payload"`
			} `json:"next_action"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode job get output: %v\n%s", err, stdout)
	}
	action := envelope.Data.NextAction
	if action.Payload.PreviewURL != "" {
		t.Fatalf("confirm_steps must not carry a preview link: %s", stdout)
	}
	if action.PayloadDigest == "" ||
		action.Payload.ExpectedReleaseCandidateDigest == "" ||
		action.Payload.ExpectedPublicSummaryDigest == "" {
		t.Fatalf("confirm_steps digest paths are incomplete: %#v", action)
	}
	// 仅凭 action 顶层 payload_digest 与 payload 内两个 expected digest 完成确认。
	code, stdout, stderr, _ = runCLI(t, server, authenticatedStore(t),
		"job", "resume", "pub_1",
		"--action-id", action.ActionID,
		"--expected-payload-digest", action.PayloadDigest,
		"--expected-release-candidate-digest", action.Payload.ExpectedReleaseCandidateDigest,
		"--expected-public-summary-digest", action.Payload.ExpectedPublicSummaryDigest,
		"--decision", "confirm",
	)
	if code != 0 || stderr != "" || !stringContains(stdout, `"status":"resolved"`) {
		t.Fatalf("steps resolve: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	// 确认通过后才出现 confirm_publish 与 preview_url。
	code, stdout, stderr, _ = runCLI(t, server, authenticatedStore(t), "job", "get", "pub_1")
	if code != 0 || !stringContains(stdout, `"type":"confirm_publish"`) || !stringContains(stdout, `"preview_url"`) {
		t.Fatalf("confirm_publish preview must appear only after steps confirmation: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if resolved.Load() != 1 {
		t.Fatalf("steps action resolved %d times, want exactly 1", resolved.Load())
	}
}
