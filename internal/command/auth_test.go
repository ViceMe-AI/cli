package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestAgentDeviceLoginContinuesAcrossCLIInvocations(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC()
	refreshExpiresAt := time.Now().Add(24 * time.Hour).UTC()
	var exchanges atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/auth/device":
			writeCommandJSON(t, writer, map[string]any{
				"verification_url":          "https://shop.example/cli/authorize",
				"verification_url_complete": "https://shop.example/cli/authorize?user_code=ABCD-EFGH",
				"device_code":               "vcm_dc_cross_turn",
				"user_code":                 "ABCD-EFGH",
				"expires_at":                expiresAt.Format(time.RFC3339Nano),
				"interval_seconds":          5,
			})
		case "/v1/cli/auth/token":
			var input map[string]string
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input["device_code"] != "vcm_dc_cross_turn" {
				t.Fatalf("unexpected device code %#v", input)
			}
			if exchanges.Add(1) == 1 {
				writer.WriteHeader(http.StatusBadRequest)
				writeCommandJSON(t, writer, map[string]any{
					"statusCode": 400,
					"code":       "AUTHORIZATION_PENDING",
					"message":    "pending",
					"requestId":  "550e8400-e29b-41d4-a716-446655440099",
				})
				return
			}
			writeCommandJSON(t, writer, map[string]any{
				"access_token":       "vcm_at_cross_turn",
				"refresh_token":      "vcm_rt_cross_turn",
				"token_type":         "api_key",
				"expires_at":         expiresAt.Format(time.RFC3339Nano),
				"refresh_expires_at": refreshExpiresAt.Format(time.RFC3339Nano),
				"user_id":            "550e8400-e29b-41d4-a716-446655440000",
				"scope":              []string{"app:read", "app:write"},
			})
		default:
			http.Error(writer, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv(apiBaseURLEnvironment, server.URL)

	var store securestore.Store = securestore.NewMemory()
	environment := skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()}
	dependencies := Dependencies{
		Region:      config.RegionCN,
		Environment: environment,
		HTTPClient:  server.Client(),
	}
	var slept []time.Duration

	code, stdout, stderr, store := runCLIWithDependencies(
		t,
		nil,
		store,
		"",
		dependencies,
		"auth",
		"login",
		"--no-wait",
		"--json",
	)
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"device_code": "vcm_dc_cross_turn"`) || !strings.Contains(stdout, `"continue_args"`) {
		t.Fatalf("start code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}

	code, stdout, stderr, store = runCLIWithDependencies(
		t,
		nil,
		store,
		"",
		Dependencies{
			Region:      config.RegionCN,
			Environment: environment,
			HTTPClient:  server.Client(),
			Sleep: func(_ context.Context, duration time.Duration) error {
				slept = append(slept, duration)
				return nil
			},
		},
		"--profile",
		"default",
		"auth",
		"login",
		"--device-code",
		"vcm_dc_cross_turn",
		"--json",
	)
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"authenticated": true`) {
		t.Fatalf("finish code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if len(slept) != 1 || slept[0] != 5*time.Second {
		t.Fatalf("continuation ignored the server polling interval: %#v", slept)
	}

	code, stdout, stderr, _ = runCLIWithDependencies(
		t,
		nil,
		store,
		"",
		dependencies,
		"auth",
		"status",
	)
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"authenticated": true`) {
		t.Fatalf("status code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestAgentDeviceLoginRejectsProfileOrEndpointSwitchBeforeExchange(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).UTC()
	var tokenCalls atomic.Int32
	startServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/device" {
			http.Error(writer, "not found", http.StatusNotFound)
			return
		}
		writeCommandJSON(t, writer, map[string]any{
			"verification_url": "https://shop.example/cli/authorize",
			"device_code":      "vcm_dc_bound_context",
			"expires_at":       expiresAt.Format(time.RFC3339Nano),
			"interval_seconds": 7,
		})
	}))
	defer startServer.Close()
	wrongServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		tokenCalls.Add(1)
		http.Error(writer, "must not be called", http.StatusInternalServerError)
	}))
	defer wrongServer.Close()

	home := t.TempDir()
	environment := skillcontent.Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	configured := config.Default(config.RegionCN)
	configured.Profiles = append(configured.Profiles, config.Profile{ID: "profile-work", Name: "work", Region: config.RegionCN, APIBaseURL: startServer.URL})
	configured.Profiles[0].APIBaseURL = startServer.URL
	if _, err := config.Save(environment.ConfigDir, configured); err != nil {
		t.Fatal(err)
	}
	var store securestore.Store = securestore.NewMemory()
	dependencies := Dependencies{Environment: environment}
	code, _, stderr, store := runCLIWithDependencies(t, startServer, store, "", dependencies,
		"--profile", "default", "auth", "login", "--no-wait", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("start code=%d stderr=%s", code, stderr)
	}

	code, _, stderr, _ = runCLIWithDependencies(t, startServer, store, "", dependencies,
		"--profile", "work", "auth", "login", "--device-code", "vcm_dc_bound_context", "--json")
	if code != 3 || !strings.Contains(stderr, "device_authorization_context_mismatch") {
		t.Fatalf("profile switch code=%d stderr=%s", code, stderr)
	}

	code, _, stderr, _ = runCLIWithDependencies(t, wrongServer, store, "", Dependencies{Region: config.RegionCN, Environment: environment},
		"auth", "login", "--device-code", "vcm_dc_bound_context", "--json")
	if code != 3 || !strings.Contains(stderr, "device_authorization_context_mismatch") {
		t.Fatalf("endpoint switch code=%d stderr=%s", code, stderr)
	}
	if tokenCalls.Load() != 0 {
		t.Fatalf("mismatched continuation contacted the wrong API %d times", tokenCalls.Load())
	}
}

func TestProfileAndEnvironmentEndpointRemainExclusiveOutsideDeviceContinuation(t *testing.T) {
	t.Setenv(apiBaseURLEnvironment, "https://api.example.com/staging")
	code, _, stderr, _ := runCLIWithDependencies(
		t,
		nil,
		securestore.NewMemory(),
		"",
		Dependencies{Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()}},
		"--profile",
		"default",
		"auth",
		"status",
	)
	if code != 2 || !strings.Contains(stderr, "profile_api_base_url_conflict") {
		t.Fatalf("profile/environment conflict code=%d stderr=%s", code, stderr)
	}
}

func TestHumanDeviceLoginUsesCompleteVerificationURL(t *testing.T) {
	var output bytes.Buffer
	writeHumanLoginStart(&output, api.DeviceAuthorization{
		VerificationURL:         "https://shop.example/cli/authorize",
		VerificationURLComplete: "https://shop.example/cli/authorize?user_code=ABCD-EFGH",
		UserCode:                "ABCD-EFGH",
	})

	if !strings.Contains(output.String(), "https://shop.example/cli/authorize?user_code=ABCD-EFGH") {
		t.Fatalf("complete verification URL missing: %s", output.String())
	}
}

func TestLogoutUsesStoredAccessCredentialAndKeepsItOnRemoteFailure(t *testing.T) {
	now := time.Now().UTC()
	var revokeCalls atomic.Int32
	var failRevoke atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/auth/revoke":
			revokeCalls.Add(1)
			if request.Header.Get("x-api-key") != "vcm_at_expired_logout" {
				t.Fatalf("logout did not use the stored access credential: %q", request.Header.Get("x-api-key"))
			}
			if failRevoke.Load() {
				writer.WriteHeader(http.StatusServiceUnavailable)
				writeCommandJSON(t, writer, map[string]any{"code": "REVOKE_UNAVAILABLE", "message": "retry", "requestId": "550e8400-e29b-41d4-a716-446655440099"})
				return
			}
			writeCommandJSON(t, writer, map[string]any{"revoked": true})
		default:
			http.Error(writer, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	store := securestore.NewMemory()
	scope, err := customCredentialScope(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", Scope: scope}
	saveExpired := func() {
		t.Helper()
		if err := manager.Save(credentialauth.Credential{
			AccessToken: "vcm_at_expired_logout", RefreshToken: "vcm_rt_logout",
			ExpiresAt: now.Add(-time.Minute), RefreshExpiresAt: now.Add(24 * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	saveExpired()
	dependencies := Dependencies{Region: config.RegionCN, APIBaseURL: server.URL, HTTPClient: server.Client(), Now: func() time.Time { return now }}
	code, _, stderr, _ := runCLIWithDependencies(t, nil, store, "", dependencies, "auth", "logout")
	if code != 0 || stderr != "" || revokeCalls.Load() != 1 {
		t.Fatalf("logout code=%d stderr=%s revoke=%d", code, stderr, revokeCalls.Load())
	}
	if _, err := manager.Load(); err == nil {
		t.Fatal("successful remote revoke left local credentials")
	}

	saveExpired()
	failRevoke.Store(true)
	code, _, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies, "auth", "logout")
	if code == 0 || !strings.Contains(stderr, "revoke_unavailable") {
		t.Fatalf("failed revoke code=%d stderr=%s", code, stderr)
	}
	if _, err := manager.Load(); err != nil {
		t.Fatalf("remote revoke failure deleted the recoverable local credential: %v", err)
	}
}

func TestLogoutConvergesAfterRevocationResponseLoss(t *testing.T) {
	var revokeCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/revoke" || request.Header.Get("x-api-key") != "vcm_at_logout_retry" {
			t.Fatalf("unexpected revoke request %s token=%q", request.URL.Path, request.Header.Get("x-api-key"))
		}
		if revokeCalls.Add(1) == 1 {
			connection, _, err := writer.(http.Hijacker).Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = connection.Close()
			return
		}
		writeCommandJSON(t, writer, map[string]any{"revoked": true})
	}))
	defer server.Close()

	store := securestore.NewMemory()
	scope, err := customCredentialScope(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vcm_at_logout_retry"}); err != nil {
		t.Fatal(err)
	}
	dependencies := Dependencies{Region: config.RegionCN, APIBaseURL: server.URL, HTTPClient: server.Client()}
	code, _, _, _ := runCLIWithDependencies(t, nil, store, "", dependencies, "auth", "logout")
	if code == 0 {
		t.Fatal("lost revoke response unexpectedly reported success")
	}
	if _, err := manager.Load(); err != nil {
		t.Fatalf("lost revoke response removed local recovery credential: %v", err)
	}
	code, _, stderr, _ := runCLIWithDependencies(t, nil, store, "", dependencies, "auth", "logout")
	if code != 0 || stderr != "" || revokeCalls.Load() != 2 {
		t.Fatalf("retry code=%d stderr=%s revoke calls=%d", code, stderr, revokeCalls.Load())
	}
	if _, err := manager.Load(); err == nil {
		t.Fatal("converged logout left local credentials")
	}
}

func TestLogoutRecoversAPendingRefreshBeforeRevocation(t *testing.T) {
	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/auth/refresh":
			var input api.RefreshTokenRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input.RefreshToken != "vcm_rt_logout_pending" || input.ClientRequestID != "550e8400-e29b-41d4-a716-446655440060" {
				t.Fatalf("logout changed pending refresh state %#v", input)
			}
			writeCommandJSON(t, writer, map[string]any{
				"access_token":       "vcm_at_logout_recovered",
				"refresh_token":      "vcm_rt_logout_recovered",
				"token_type":         "api_key",
				"expires_at":         now.Add(time.Hour).Format(time.RFC3339Nano),
				"refresh_expires_at": now.Add(24 * time.Hour).Format(time.RFC3339Nano),
				"user_id":            "550e8400-e29b-41d4-a716-446655440000",
				"scope":              []string{"app:read"},
			})
		case "/v1/cli/auth/revoke":
			if request.Header.Get("x-api-key") != "vcm_at_logout_recovered" {
				t.Fatalf("logout did not revoke the recovered Session: %q", request.Header.Get("x-api-key"))
			}
			writeCommandJSON(t, writer, map[string]any{"revoked": true})
		default:
			http.Error(writer, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	store := securestore.NewMemory()
	scope, err := customCredentialScope(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{
		AccessToken:      "vcm_at_logout_stale",
		RefreshToken:     "vcm_rt_logout_pending",
		RefreshRequestID: "550e8400-e29b-41d4-a716-446655440060",
		ExpiresAt:        now.Add(-time.Minute),
		RefreshExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	dependencies := Dependencies{Region: config.RegionCN, APIBaseURL: server.URL, HTTPClient: server.Client(), Now: func() time.Time { return now }}
	code, _, stderr, _ := runCLIWithDependencies(t, nil, store, "", dependencies, "auth", "logout")
	if code != 0 || stderr != "" {
		t.Fatalf("pending refresh logout code=%d stderr=%s", code, stderr)
	}
	if _, err := manager.Load(); err == nil {
		t.Fatal("pending refresh logout left local credentials")
	}
}

func TestLogoutRetriesRemoteRevocationAfterLocalDeleteFailure(t *testing.T) {
	var revokeCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/revoke" || request.Header.Get("x-api-key") != "vcm_at_logout_delete_retry" {
			t.Fatalf("unexpected revoke request %s token=%q", request.URL.Path, request.Header.Get("x-api-key"))
		}
		revokeCalls.Add(1)
		writeCommandJSON(t, writer, map[string]any{"revoked": true})
	}))
	defer server.Close()

	backing := securestore.NewMemory()
	store := &deleteFailOnceStore{Store: backing}
	scope, err := customCredentialScope(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vcm_at_logout_delete_retry"}); err != nil {
		t.Fatal(err)
	}
	dependencies := Dependencies{Region: config.RegionCN, APIBaseURL: server.URL, HTTPClient: server.Client()}
	code, _, stderr, _ := runCLIWithDependencies(t, nil, store, "", dependencies, "auth", "logout")
	if code == 0 || !strings.Contains(stderr, "credential_store_unavailable") {
		t.Fatalf("delete failure code=%d stderr=%s", code, stderr)
	}
	if _, err := manager.Load(); err != nil {
		t.Fatalf("delete failure lost local credential: %v", err)
	}
	code, _, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies, "auth", "logout")
	if code != 0 || stderr != "" || revokeCalls.Load() != 2 {
		t.Fatalf("retry code=%d stderr=%s revoke calls=%d", code, stderr, revokeCalls.Load())
	}
}

type deleteFailOnceStore struct {
	securestore.Store
	failed atomic.Bool
}

func (store *deleteFailOnceStore) Delete(key string) error {
	if !store.failed.Swap(true) {
		return errors.New("simulated secure-store delete failure")
	}
	return store.Store.Delete(key)
}
