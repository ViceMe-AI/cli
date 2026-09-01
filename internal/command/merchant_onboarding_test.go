package command

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
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

func TestMerchantApplicationOmitsDerivedFieldsWhenFlagsAreAbsent(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	t.Setenv(processAccessTokenEnvironment, accessToken)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeMerchantOnboardingAuth(writer)
		case "/v1/cli/merchant/onboarding/applications":
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if _, exists := input["displayName"]; exists {
				t.Fatalf("derived displayName must not be sent: %#v", input)
			}
			if _, exists := input["handle"]; exists {
				t.Fatalf("derived handle must not be sent: %#v", input)
			}
			if input["clientRequestId"] == "" {
				t.Fatalf("clientRequestId missing: %#v", input)
			}
			writeJSONResponse(writer, merchantOnboardingFixture("APPLICATION", "SUBMITTED", nil))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	exit, envelope := executeMerchantOnboardingCommand(t, server,
		"merchant", "onboarding", "apply",
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("minimal application failed: exit=%d envelope=%#v", exit, envelope)
	}
}

func TestMerchantOnboardingEvidenceTextUploadsMultipartText(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	t.Setenv(processAccessTokenEnvironment, accessToken)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeMerchantOnboardingAuth(writer)
		case "/v1/cli/merchant/onboarding/66666666-6666-4666-8666-666666666666/evidence":
			if err := request.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("parse multipart: %v", err)
			}
			if got := request.FormValue("text"); got != "主页链接：https://example.com/creator" {
				t.Fatalf("unexpected text field: %q", got)
			}
			if got := request.FormValue("lockVersion"); got != "3" {
				t.Fatalf("unexpected lockVersion: %q", got)
			}
			if _, _, err := request.FormFile("image"); err == nil {
				t.Fatal("text upload must not carry an image part")
			}
			writeJSONResponse(writer, merchantOnboardingFixture("APPLICATION", "NEEDS_MORE_EVIDENCE", nil))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	exit, envelope := executeMerchantOnboardingCommand(t, server,
		"merchant", "onboarding", "evidence", "66666666-6666-4666-8666-666666666666",
		"--text", " 主页链接：https://example.com/creator ", "--lock-version", "3",
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("evidence text upload failed: exit=%d envelope=%#v", exit, envelope)
	}
}

func TestMerchantOnboardingEvidenceRejectsConflictingOrMissingInputs(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	t.Setenv(processAccessTokenEnvironment, accessToken)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeMerchantOnboardingAuth(writer)
	}))
	defer server.Close()

	exit, _ := executeMerchantOnboardingCommand(t, server,
		"merchant", "onboarding", "evidence", "66666666-6666-4666-8666-666666666666",
		"--path", "proof.png", "--text", "both given", "--lock-version", "1",
	)
	if exit == 0 {
		t.Fatal("expected validation failure when both --path and --text are given")
	}
	exit, _ = executeMerchantOnboardingCommand(t, server,
		"merchant", "onboarding", "evidence", "66666666-6666-4666-8666-666666666666",
		"--lock-version", "1",
	)
	if exit == 0 {
		t.Fatal("expected validation failure when neither --path nor --text is given")
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

func TestGithubMerchantChannelPrintsOneFallbackURLAndWaitsForVerification(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	const merchantID = "44444444-4444-4444-8444-444444444444"
	const authorizationURL = "https://github.com/login/oauth/authorize?state=one-time-state"
	const attemptID = "77777777-7777-4777-8777-777777777777"
	t.Setenv(processAccessTokenEnvironment, accessToken)
	var startCalls atomic.Int32
	var statusCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeMerchantOnboardingAuth(writer)
		case "/v1/cli/merchant/channels/github/start":
			startCalls.Add(1)
			writeJSONResponse(writer, map[string]any{
				"kind": "authorization", "authorizationUrl": authorizationURL, "attemptId": attemptID,
			})
		case "/v1/cli/merchant/channels/github/status":
			if request.URL.Query().Get("merchantAccountId") != merchantID || request.URL.Query().Get("attemptId") != attemptID {
				t.Fatalf("unexpected merchant status query: %s", request.URL.RawQuery)
			}
			if statusCalls.Add(1) == 1 {
				writeJSONResponse(writer, map[string]any{"kind": "pending"})
				return
			}
			writeJSONResponse(writer, map[string]any{"kind": "verified"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	exit := Execute([]string{"merchant", "channel", "github", merchantID}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(),
		HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()},
		Sleep:       func(context.Context, time.Duration) error { return nil },
		NewID:       func() string { return "55555555-5555-4555-8555-555555555555" },
	})
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid envelope: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	if exit != 0 || envelope["ok"] != true || startCalls.Load() != 1 || statusCalls.Load() != 2 {
		t.Fatalf("GitHub wait failed: exit=%d envelope=%#v starts=%d statuses=%d", exit, envelope, startCalls.Load(), statusCalls.Load())
	}
	if strings.Count(stderr.String(), authorizationURL) != 1 || !strings.Contains(stderr.String(), "Waiting for authorization") {
		t.Fatalf("GitHub fallback URL was not printed exactly once before waiting: %q", stderr.String())
	}
}

func TestGithubMerchantChannelStopsWhenAuthorizationIsDenied(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	const merchantID = "44444444-4444-4444-8444-444444444444"
	t.Setenv(processAccessTokenEnvironment, accessToken)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeMerchantOnboardingAuth(writer)
		case "/v1/cli/merchant/channels/github/start":
			writeJSONResponse(writer, map[string]any{
				"kind": "authorization", "authorizationUrl": "https://github.example/authorize", "attemptId": "77777777-7777-4777-8777-777777777777",
			})
		case "/v1/cli/merchant/channels/github/status":
			writeJSONResponse(writer, map[string]any{"kind": "denied"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	exit, envelope := executeMerchantOnboardingCommand(t, server, "merchant", "channel", "github", merchantID)
	errorBody, _ := envelope["error"].(map[string]any)
	if exit != output.ExitAuthentication || errorBody["code"] != "GITHUB_AUTHORIZATION_DENIED" {
		t.Fatalf("authorization denial did not stop immediately: exit=%d envelope=%#v", exit, envelope)
	}
}

func TestGithubAuthorizationTimeoutAndCancelAreStableDuringStatusRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()
	client := api.NewClient(server.URL, server.Client(), processTokenSource("token"), "test")
	runtime := &Runtime{deps: Dependencies{Sleep: sleepContext}}

	timeoutContext, cancelTimeout := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelTimeout()
	timeoutError := output.AsError(finishGithubChannelAuthorization(timeoutContext, runtime, client, "merchant", "attempt", time.Minute, time.Second))
	if timeoutError.Subtype != "GITHUB_AUTHORIZATION_PENDING" || !timeoutError.Retryable {
		t.Fatalf("unexpected timeout error: %#v", timeoutError)
	}

	cancelContext, cancel := context.WithCancel(context.Background())
	cancel()
	cancelError := output.AsError(finishGithubChannelAuthorization(cancelContext, runtime, client, "merchant", "attempt", time.Minute, time.Second))
	if cancelError.Subtype != "GITHUB_AUTHORIZATION_CANCELLED" || cancelError.Retryable {
		t.Fatalf("unexpected cancellation error: %#v", cancelError)
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
