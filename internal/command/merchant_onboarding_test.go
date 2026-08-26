package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestMerchantApplicationUsesTheUnifiedOnboardingRoute(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	t.Setenv(processAccessTokenEnvironment, accessToken)
	var applicationCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeMerchantOnboardingAuth(writer)
		case "/v1/cli/merchant/onboarding/applications":
			applicationCalls.Add(1)
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input["displayName"] != "Creator Shop" || input["handle"] != "creator-shop" || input["clientRequestId"] == "" {
				t.Fatalf("unexpected application input: %#v", input)
			}
			writeJSONResponse(writer, merchantOnboardingFixture("APPLICATION", "SUBMITTED", nil))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	exit, envelope := executeMerchantOnboardingCommand(t, server,
		"merchant", "onboarding", "apply", "--display-name", "Creator Shop", "--handle", "creator-shop",
	)
	if exit != 0 || envelope["ok"] != true || applicationCalls.Load() != 1 {
		t.Fatalf("merchant application failed: exit=%d envelope=%#v calls=%d", exit, envelope, applicationCalls.Load())
	}
}

func TestGithubMerchantClaimReturnsOnlyTheConfiguredOAuthRoute(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	const merchantID = "44444444-4444-4444-8444-444444444444"
	t.Setenv(processAccessTokenEnvironment, accessToken)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeMerchantOnboardingAuth(writer)
		case "/v1/cli/merchant/onboarding/github/" + merchantID + "/start":
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input["returnTo"] != "/cli/github-result" {
				t.Fatalf("unexpected GitHub return route: %#v", input)
			}
			writeJSONResponse(writer, map[string]any{
				"kind": "authorization", "authorizationUrl": "https://github.com/login/oauth/authorize?state=test", "onboarding": nil,
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	exit, envelope := executeMerchantOnboardingCommand(t, server,
		"merchant", "onboarding", "claim-github", merchantID,
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("GitHub Merchant claim failed: exit=%d envelope=%#v", exit, envelope)
	}
}

func executeMerchantOnboardingCommand(t *testing.T, server *httptest.Server, arguments ...string) (int, map[string]any) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exit := Execute(arguments, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(),
		HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()},
		NewID:       func() string { return "55555555-5555-4555-8555-555555555555" },
	})
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid envelope: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	return exit, envelope
}

func writeMerchantOnboardingAuth(writer http.ResponseWriter) {
	writeJSONResponse(writer, map[string]any{
		"authenticated": true,
		"user":          map[string]any{"id": "33333333-3333-4333-8333-333333333333", "displayName": "Creator", "avatarUrl": nil},
		"scopes":        []string{"profile:read", "skill-publication:read", "skill-publication:write"},
		"expiresAt":     "2027-08-27T00:00:00Z",
	})
}

func merchantOnboardingFixture(kind, status string, merchantAccountID *string) map[string]any {
	return map[string]any{
		"id": "66666666-6666-4666-8666-666666666666", "kind": kind,
		"merchantAccountId": merchantAccountID, "provider": nil, "requestedHandle": "creator-shop",
		"displayName": "Creator Shop", "status": status, "lockVersion": 0,
		"publicAccountName": nil, "profileUrl": nil, "reservationExpiresAt": nil,
		"reasonCode": nil, "reviewNote": nil, "submittedAt": "2026-08-27T00:00:00Z",
		"reviewedAt": nil, "evidence": []any{},
	}
}
