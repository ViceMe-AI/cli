package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

type loginPersistenceFailureStore struct{}

func (loginPersistenceFailureStore) Get(string) (string, error) { return "", securestore.ErrNotFound }
func (loginPersistenceFailureStore) Set(string, string) error {
	return errors.New("persistence failed")
}
func (loginPersistenceFailureStore) Delete(string) error    { return securestore.ErrNotFound }
func (loginPersistenceFailureStore) Preflight(string) error { return nil }

func TestDeviceLoginWaitsAndPersistsScopedCredentialWithoutPrintingToken(t *testing.T) {
	t.Parallel()
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	var tokenPolls atomic.Int32
	var revoked atomic.Bool
	var requestedScopes []string
	var requestedPurpose string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/device-authorizations":
			var authorizationRequest api.DeviceAuthorizationRequest
			if err := json.NewDecoder(request.Body).Decode(&authorizationRequest); err != nil {
				t.Fatalf("decode device authorization request: %v", err)
			}
			requestedScopes = append([]string(nil), authorizationRequest.Scopes...)
			requestedPurpose = authorizationRequest.Purpose
			writeJSONResponse(writer, map[string]any{
				"deviceCode": "device-code", "userCode": "ABCD-EFGH",
				"verificationUri":         "https://viceme.cn/cli/authorize",
				"verificationUriComplete": "https://viceme.cn/cli/authorize?user_code=ABCD-EFGH",
				"expiresIn":               600, "interval": 1,
			})
		case "/v1/cli/device-authorizations/token":
			if tokenPolls.Add(1) == 1 {
				writeJSONResponse(writer, map[string]any{"status": "authorization_pending", "interval": 1})
				return
			}
			writeJSONResponse(writer, map[string]any{
				"status": "authorized", "accessToken": accessToken, "tokenType": "Bearer",
				"expiresAt": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
				"scopes": []string{
					"profile:read", "skill-publication:read", "skill-publication:write",
					"merchant-commerce:read", "merchant-commerce:write",
				},
			})
		case "/v1/cli/auth/status":
			if request.Header.Get("Authorization") != "Bearer "+accessToken {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "55555555-5555-4555-8555-555555555555", "displayName": "Creator", "avatarUrl": nil},
				"scopes": []string{
					"profile:read", "skill-publication:read", "skill-publication:write",
					"merchant-commerce:read", "merchant-commerce:write",
				},
				"expiresAt": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			})
		case "/v1/cli/auth/logout":
			revoked.Store(true)
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dependencies := Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Sleep:       func(context.Context, time.Duration) error { return nil },
	}
	run := func(arguments ...string) (int, map[string]any) {
		t.Helper()
		stdout.Reset()
		stderr.Reset()
		exit := Execute(arguments, dependencies)
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("invalid CLI envelope: exit=%d stdout=%q stderr=%q err=%v", exit, stdout.String(), stderr.String(), err)
		}
		if strings.Contains(stdout.String(), accessToken) || strings.Contains(stderr.String(), accessToken) {
			t.Fatalf("access token leaked in CLI output: stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
		return exit, envelope
	}
	assertCamelAuthData := func(result map[string]any, required ...string) {
		t.Helper()
		data, ok := result["data"].(map[string]any)
		if !ok {
			t.Fatalf("auth response data is not an object: %#v", result)
		}
		for _, key := range required {
			if _, exists := data[key]; !exists {
				t.Fatalf("auth response omitted camelCase field %q: %#v", key, data)
			}
		}
		for _, key := range []string{"distribution_region", "user_id", "expires_at", "logged_out", "already_logged_out"} {
			if _, exists := data[key]; exists {
				t.Fatalf("auth response exposed obsolete snake_case field %q: %#v", key, data)
			}
		}
	}
	if exit, result := run("auth", "login"); exit != 0 || result["ok"] != true {
		t.Fatalf("device login failed: exit=%d result=%#v", exit, result)
	} else {
		assertCamelAuthData(result, "distributionRegion", "userId", "expiresAt")
	}
	if tokenPolls.Load() < 2 {
		t.Fatalf("device login returned before authorization completed: polls=%d", tokenPolls.Load())
	}
	expectedScopes := []string{
		"profile:read", "skill-publication:read", "skill-publication:write",
		"merchant-commerce:read", "merchant-commerce:write", "skill-use:read",
	}
	if strings.Join(requestedScopes, ",") != strings.Join(expectedScopes, ",") {
		t.Fatalf("device login requested wrong scopes: got=%v want=%v", requestedScopes, expectedScopes)
	}
	if requestedPurpose != "" {
		t.Fatalf("ordinary login should omit purpose for compatibility, got %q", requestedPurpose)
	}
	if !strings.Contains(stderr.String(), "https://viceme.cn/cli/authorize?user_code=ABCD-EFGH") {
		t.Fatalf("complete browser authorization URL was not shown: stderr=%q", stderr.String())
	}
	if strings.Contains(strings.ToLower(stderr.String()), "enter code") {
		t.Fatalf("login still asks the user to enter a device code: stderr=%q", stderr.String())
	}
	if exit, result := run("auth", "status"); exit != 0 || result["ok"] != true {
		t.Fatalf("remote status failed: exit=%d result=%#v", exit, result)
	} else {
		assertCamelAuthData(result, "distributionRegion", "expiresAt")
	}
	if exit, result := run("auth", "logout"); exit != 0 || result["ok"] != true || !revoked.Load() {
		t.Fatalf("logout failed: exit=%d result=%#v revoked=%v", exit, result, revoked.Load())
	} else {
		assertCamelAuthData(result, "distributionRegion")
	}
	if exit, result := run("auth", "status"); exit != 0 || result["ok"] != true {
		t.Fatalf("logged-out status failed: exit=%d result=%#v", exit, result)
	} else {
		assertCamelAuthData(result, "distributionRegion")
		if data := result["data"].(map[string]any); data["authenticated"] != false {
			t.Fatalf("logged-out status reported authenticated: %#v", data)
		}
	}
	configData := []byte(readFileString(t, filepath.Join(root, "config", "config.json")))
	if !bytes.Contains(configData, []byte("55555555-5555-4555-8555-555555555555")) {
		t.Fatalf("profile user was not persisted: config=%s", configData)
	}
}

func TestDeviceLoginSendsCreatorOnboardingPurpose(t *testing.T) {
	t.Parallel()
	var requestedPurpose string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var authorizationRequest api.DeviceAuthorizationRequest
		if err := json.NewDecoder(request.Body).Decode(&authorizationRequest); err != nil {
			t.Fatal(err)
		}
		requestedPurpose = authorizationRequest.Purpose
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	root := t.TempDir()
	_ = Execute([]string{"auth", "login", "--purpose", "creator-onboarding"}, Dependencies{
		Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}, Store: securestore.NewMemory(),
		APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if requestedPurpose != "CREATOR_ONBOARDING" {
		t.Fatalf("creator onboarding login sent the wrong purpose: %q", requestedPurpose)
	}
}

func TestAuthLogoutRetainsCredentialUntilRemoteRevocationSucceeds(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/logout" || request.Header.Get("Authorization") != "Bearer "+accessToken {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		if attempts.Add(1) == 1 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	root := t.TempDir()
	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{
		Store: store, Region: "cn", ProfileID: config.DefaultProfileName,
		ProfileName: config.DefaultProfileName, Scope: scope,
	}
	if err := manager.Save(credentialauth.Credential{AccessToken: accessToken, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	dependencies := Dependencies{
		Store: store, APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	}
	run := func() (int, map[string]any) {
		t.Helper()
		var stdout bytes.Buffer
		dependencies.Out = &stdout
		dependencies.ErrOut = &bytes.Buffer{}
		exit := Execute([]string{"auth", "logout"}, dependencies)
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("invalid logout response: exit=%d stdout=%q err=%v", exit, stdout.String(), err)
		}
		return exit, envelope
	}

	if exit, envelope := run(); exit != output.ExitNetwork || envelope["ok"] != false {
		t.Fatalf("failed revocation returned the wrong result: exit=%d envelope=%#v", exit, envelope)
	}
	if _, err := manager.Load(); err != nil {
		t.Fatalf("failed remote revocation discarded the retry credential: %v", err)
	}
	if exit, envelope := run(); exit != 0 || envelope["ok"] != true {
		t.Fatalf("retry logout failed: exit=%d envelope=%#v", exit, envelope)
	}
	if _, err := manager.Load(); err == nil {
		t.Fatal("successful remote revocation retained the local credential")
	}
}

func TestAuthLogoutRetryRemovesCredentialAfterRemoteRevocationCommitted(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/logout" || request.Header.Get("Authorization") != "Bearer "+accessToken {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		if attempts.Add(1) == 1 {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	root := t.TempDir()
	memory := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{
		Store: memory, Region: "cn", ProfileID: config.DefaultProfileName,
		ProfileName: config.DefaultProfileName, Scope: scope,
	}
	if err := manager.Save(credentialauth.Credential{AccessToken: accessToken, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	store := &deleteFailureStore{store: memory, failKey: manager.StorageKey()}
	dependencies := Dependencies{
		Store: store, APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	}
	run := func() (int, map[string]any) {
		t.Helper()
		var stdout bytes.Buffer
		dependencies.Out = &stdout
		dependencies.ErrOut = &bytes.Buffer{}
		exit := Execute([]string{"auth", "logout"}, dependencies)
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("invalid logout response: exit=%d stdout=%q err=%v", exit, stdout.String(), err)
		}
		return exit, envelope
	}

	if exit, envelope := run(); exit != output.ExitAuthentication || envelope["ok"] != false {
		t.Fatalf("local delete failure returned the wrong result: exit=%d envelope=%#v", exit, envelope)
	}
	if _, err := manager.Load(); err != nil {
		t.Fatalf("local delete failure did not restore the credential for cleanup retry: %v", err)
	}
	if exit, envelope := run(); exit != 0 || envelope["ok"] != true {
		t.Fatalf("retry did not accept the already-revoked credential: exit=%d envelope=%#v", exit, envelope)
	}
	if _, err := manager.Load(); err == nil {
		t.Fatal("logout retry retained an already-revoked local credential")
	}
}

func TestDeviceLoginDoesNotExposeSplitFlowFlags(t *testing.T) {
	t.Parallel()
	command := newAuthLoginCommand(&Runtime{})
	for _, flag := range []string{"no-wait", "device-code"} {
		if command.Flags().Lookup(flag) != nil {
			t.Fatalf("auth login still exposes removed --%s flag", flag)
		}
	}
}

func TestDeviceLoginPersistenceFailureUsesCamelCasePublicDetails(t *testing.T) {
	t.Parallel()
	var revoked atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/device-authorizations":
			writeJSONResponse(writer, map[string]any{
				"deviceCode": "device-code", "userCode": "ABCD-EFGH",
				"verificationUri": "https://viceme.cn/cli/authorize", "verificationUriComplete": "https://viceme.cn/cli/authorize?user_code=ABCD-EFGH",
				"expiresIn": 600, "interval": 1,
			})
		case "/v1/cli/device-authorizations/token":
			writeJSONResponse(writer, map[string]any{
				"status": "authorized", "accessToken": "test-access-token", "tokenType": "Bearer",
				"expiresAt": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			})
		case "/v1/cli/auth/logout":
			revoked.Store(true)
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	var stdout bytes.Buffer
	exit := Execute([]string{"auth", "login"}, Dependencies{
		Out: &stdout, ErrOut: &bytes.Buffer{}, Store: loginPersistenceFailureStore{},
		APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit == 0 || !revoked.Load() {
		t.Fatalf("persistence failure did not fail and revoke the issued credential: exit=%d revoked=%v", exit, revoked.Load())
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid failure envelope: %v stdout=%q", err, stdout.String())
	}
	errorData, ok := envelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("failure envelope omitted error object: %#v", envelope)
	}
	details, ok := errorData["details"].(map[string]any)
	if !ok || details["authorizationConsumed"] != true || details["issuedCredentialRevoked"] != true {
		t.Fatalf("persistence failure omitted camelCase details: %#v", errorData)
	}
	for _, key := range []string{"authorization_consumed", "issued_credential_revoked"} {
		if _, exists := details[key]; exists {
			t.Fatalf("persistence failure exposed obsolete snake_case detail %q: %#v", key, details)
		}
	}
}

func readFileString(t *testing.T, filename string) string {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
