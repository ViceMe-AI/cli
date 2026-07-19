package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
)

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }

type apiRoundTripFunc func(*http.Request) (*http.Response, error)

func (f apiRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestInspectUsesAPIKeyAndAcceptsEnvelope(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skill-agent-publications/inspect" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("x-api-key") != "secret" {
			t.Fatalf("missing API key: %q", request.Header.Get("x-api-key"))
		}
		if request.Header.Get("Authorization") != "" {
			t.Fatalf("API key must not be sent as Bearer: %q", request.Header.Get("Authorization"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"ok":true,"data":{"resolution_id":"res_1","destination":{"mode":"new_auto"}}}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client(), staticToken("secret"), "viceme/test")
	response, err := client.Inspect(context.Background(), InspectRequest{Source: Source{Kind: "expression", Value: "https://github.com/acme/skill"}})
	if err != nil {
		t.Fatal(err)
	}
	if Document(response).StringValue("resolution_id") != "res_1" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestDeviceAuthorizationPrefersCompleteVerificationURL(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		response string
		wantURL  string
	}{
		{
			name:     "complete URL",
			response: `{"verification_url":"https://viceme.test/cli/auth","verification_url_complete":"https://viceme.test/cli/auth?user_code=ABCD-EFGH","device_code":"device-public","expires_at":"2030-01-01T00:00:00Z","interval_seconds":5}`,
			wantURL:  "https://viceme.test/cli/auth?user_code=ABCD-EFGH",
		},
		{
			name:     "base URL fallback",
			response: `{"verification_url":"https://viceme.test/cli/auth","device_code":"device-public","expires_at":"2030-01-01T00:00:00Z","interval_seconds":5}`,
			wantURL:  "https://viceme.test/cli/auth",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/v1/cli/auth/device" {
					t.Fatalf("unexpected path: %s", request.URL.Path)
				}
				_, _ = io.WriteString(writer, test.response)
			}))
			defer server.Close()

			client := NewClient(server.URL, server.Client(), nil, "viceme/test")
			authorization, err := client.StartDeviceAuthorization(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if authorization.VerificationURL != test.wantURL {
				t.Fatalf("verification_url=%q want=%q", authorization.VerificationURL, test.wantURL)
			}
		})
	}
}

func TestAuthenticatedRequestRejectsRemoteHTTPBeforeSendingCredential(t *testing.T) {
	t.Parallel()
	transportCalled := false
	client := NewClient("http://api.viceme.example", &http.Client{Transport: apiRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		transportCalled = true
		return nil, errors.New("request must not be sent")
	})}, staticToken("secret"), "")

	_, err := client.GetTarget(context.Background(), "target_1")
	var cliError *output.Error
	if !errors.As(err, &cliError) || cliError.Subtype != "api_base_url" {
		t.Fatalf("expected api_base_url validation error, got %T: %v", err, err)
	}
	if transportCalled {
		t.Fatal("HTTP transport was called for a non-loopback plaintext API URL")
	}
}

func TestAPIBaseURLAllowsOnlyHTTPSOrLoopbackHTTP(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		raw     string
		wantErr bool
	}{
		{raw: "https://api.viceme.cn"},
		{raw: "http://localhost:3991"},
		{raw: "http://127.0.0.1:3991"},
		{raw: "http://[::1]:3991"},
		{raw: "http://10.0.0.8:3991", wantErr: true},
		{raw: "http://api.viceme.ai", wantErr: true},
		{raw: "ftp://api.viceme.ai", wantErr: true},
		{raw: "https://user:password@api.viceme.ai", wantErr: true},
	} {
		_, err := validateAPIBaseURL(test.raw)
		if (err != nil) != test.wantErr {
			t.Fatalf("validateAPIBaseURL(%q) error = %v, wantErr=%v", test.raw, err, test.wantErr)
		}
	}
}

func TestCredentialHeaderIsInjectable(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer compatibility-token" {
			t.Fatalf("custom credential header was not applied: %#v", request.Header)
		}
		_, _ = io.WriteString(writer, `{"target_id":"target_1"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client(), staticToken("compatibility-token"), "")
	client.CredentialHeader = func(request *http.Request, credential string) {
		request.Header.Set("Authorization", "Bearer "+credential)
	}
	if _, err := client.GetTarget(context.Background(), "target_1"); err != nil {
		t.Fatal(err)
	}
}

func TestServerErrorPreservesTypedContract(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(writer, `{"error":{"type":"authorization","subtype":"target_owner","message":"not yours","retryable":false,"hint":"choose your target"}}`)
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client(), staticToken("secret"), "")
	_, err := client.GetTarget(context.Background(), "target_1")
	var cliError *output.Error
	if !errors.As(err, &cliError) {
		t.Fatalf("expected typed CLI error, got %T: %v", err, err)
	}
	if cliError.Code != output.ExitAuthentication || cliError.Type != "authorization" || cliError.Subtype != "target_owner" || cliError.Hint == "" {
		t.Fatalf("unexpected typed error: %#v", cliError)
	}
}

func TestExpiredAndUnsupportedStatusFallbackToValidation(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusGone, http.StatusUnsupportedMediaType} {
		err := decodeServerError(status, []byte(`{"message":"request cannot be accepted"}`))
		var cliError *output.Error
		if !errors.As(err, &cliError) {
			t.Fatalf("status %d: expected typed CLI error, got %T: %v", status, err, err)
		}
		if cliError.Code != output.ExitValidation || cliError.Type != "validation" {
			t.Fatalf("status %d: unexpected classification: %#v", status, cliError)
		}
	}
}

func TestTypedAuthenticationOverridesGoneFallback(t *testing.T) {
	t.Parallel()
	err := decodeServerError(http.StatusGone, []byte(`{"type":"authentication","subtype":"expired_token","message":"device code expired","retryable":false}`))
	var cliError *output.Error
	if !errors.As(err, &cliError) {
		t.Fatalf("expected typed CLI error, got %T: %v", err, err)
	}
	if cliError.Code != output.ExitAuthentication || cliError.Type != "authentication" || cliError.Subtype != "expired_token" {
		t.Fatalf("unexpected classification: %#v", cliError)
	}
}

func TestGetTargetExtractsShareCodeLocally(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		url  string
		code string
	}{
		{name: "stable v path", url: "https://app.viceme.ai/v/stable123", code: "stable123"},
		{name: "legacy share path", url: "https://app.viceme.ai/share/legacy123", code: "legacy123"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				wantPath := "/v1/skill-agent-publish-targets/by-share-code/" + test.code
				if request.URL.Path != wantPath {
					t.Fatalf("unexpected path: %s; want %s", request.URL.Path, wantPath)
				}
				_, _ = io.WriteString(writer, `{"target_id":"target_1"}`)
			}))
			defer server.Close()

			client := NewClient(server.URL, server.Client(), staticToken("secret"), "")
			response, err := client.GetTarget(context.Background(), test.url)
			if err != nil {
				t.Fatal(err)
			}
			if Document(response).StringValue("target_id") != "target_1" {
				t.Fatalf("unexpected target: %#v", response)
			}
		})
	}
}

func TestPutUploadUsesPreparedMethodAndHeaders(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.Header.Get("X-Upload-Test") != "ok" {
			t.Fatalf("unexpected upload request: %s %#v", request.Method, request.Header)
		}
		body, _ := io.ReadAll(request.Body)
		if string(body) != "bundle" {
			t.Fatalf("unexpected body: %q", body)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := NewClient("https://api.invalid", server.Client(), nil, "")
	err := client.PutUpload(context.Background(), UploadPrepareResponse{
		UploadURL: server.URL,
		Headers:   map[string]string{"X-Upload-Test": "ok"},
	}, strings.NewReader("bundle"), int64(len("bundle")))
	if err != nil {
		t.Fatal(err)
	}
}
