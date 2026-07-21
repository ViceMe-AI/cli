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
	if response.ResolutionID != "res_1" {
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

func TestCreatePublicationPreservesExplicitZeroExpectedTargetVersion(t *testing.T) {
	t.Parallel()
	var requestBody string
	client := NewClient("https://api.viceme.test", &http.Client{Transport: apiRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = string(body)
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"publication_id":"pub_zero","status":"received"}`)),
			Request:    request,
		}, nil
	})}, staticToken("secret"), "")
	zero := int64(0)
	_, err := client.CreatePublication(context.Background(), CreatePublicationRequest{
		ClientRequestID: "request-zero",
		ResolutionID:    "res_zero",
		Destination: Destination{
			Mode:                  "existing",
			TargetID:              "00000000-0000-0000-0000-000000000000",
			ExpectedTargetVersion: &zero,
		},
		Options: PublicationOptions{PublishMode: "confirm", AdmissionConfirmation: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantDestination := `"destination":{"mode":"existing","target_id":"00000000-0000-0000-0000-000000000000","expected_target_version":0}`
	if !strings.Contains(requestBody, wantDestination) {
		t.Fatalf("explicit version zero was omitted from request JSON: %s", requestBody)
	}
}

func TestAPIKeyIsNeverForwardedAcrossRedirect(t *testing.T) {
	t.Parallel()
	redirected := false
	destination := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		redirected = true
		if request.Header.Get("x-api-key") != "" {
			t.Fatal("API key was forwarded across a redirect")
		}
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("x-api-key") != "secret" {
			t.Fatal("origin did not receive the API key")
		}
		writer.Header().Set("Location", destination.URL+"/capture")
		writer.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	client := NewClient(origin.URL, origin.Client(), staticToken("secret"), "")
	if _, err := client.GetTarget(context.Background(), "target_1"); err == nil {
		t.Fatal("expected redirect response to be rejected")
	}
	if redirected {
		t.Fatal("authenticated request followed a redirect")
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

func TestServerErrorTypeControlsExitWithoutLosingTaxonomy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		status        int
		body          string
		wantCode      int
		wantType      string
		wantSubtype   string
		wantRetryable bool
	}{
		{
			name:        "target conflict overrides HTTP conflict type",
			status:      http.StatusConflict,
			body:        `{"type":"target_conflict","subtype":"target_changed","message":"target changed","retryable":false}`,
			wantCode:    output.ExitValidation,
			wantType:    "target_conflict",
			wantSubtype: "target_changed",
		},
		{
			name:          "concurrency uses retry exit",
			status:        http.StatusConflict,
			body:          `{"error":{"type":"concurrency","subtype":"publication_admission_cas_conflict","message":"changed concurrently","retryable":true}}`,
			wantCode:      output.ExitNetwork,
			wantType:      "concurrency",
			wantSubtype:   "publication_admission_cas_conflict",
			wantRetryable: true,
		},
		{
			name:        "rollout gate uses policy exit",
			status:      http.StatusServiceUnavailable,
			body:        `{"type":"rollout_gate","subtype":"skill_publication_disabled","message":"disabled","retryable":false}`,
			wantCode:    output.ExitPolicy,
			wantType:    "rollout_gate",
			wantSubtype: "skill_publication_disabled",
		},
		{
			name:        "confirmation uses confirmation exit",
			status:      http.StatusPreconditionRequired,
			body:        `{"type":"confirmation","subtype":"admission_confirmation_required","message":"confirmation required","retryable":false}`,
			wantCode:    output.ExitConfirmation,
			wantType:    "confirmation",
			wantSubtype: "admission_confirmation_required",
		},
		{
			name:          "missing type uses status fallback",
			status:        http.StatusConflict,
			body:          `{"subtype":"untyped_conflict","message":"conflict","retryable":true}`,
			wantCode:      output.ExitValidation,
			wantType:      "validation",
			wantSubtype:   "untyped_conflict",
			wantRetryable: true,
		},
		{
			name:        "unknown type is preserved with fail-safe status exit",
			status:      http.StatusConflict,
			body:        `{"type":"future_contract","subtype":"future_conflict","message":"future error","retryable":false}`,
			wantCode:    output.ExitValidation,
			wantType:    "future_contract",
			wantSubtype: "future_conflict",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := decodeServerError(test.status, []byte(test.body))
			var cliError *output.Error
			if !errors.As(err, &cliError) {
				t.Fatalf("expected typed CLI error, got %T: %v", err, err)
			}
			if cliError.Code != test.wantCode || cliError.Type != test.wantType || cliError.Subtype != test.wantSubtype || cliError.Retryable != test.wantRetryable {
				t.Fatalf("error=%#v want code=%d type=%q subtype=%q retryable=%v", cliError, test.wantCode, test.wantType, test.wantSubtype, test.wantRetryable)
			}
		})
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

func TestPutUploadDoesNotForwardPresignedHeadersAcrossRedirect(t *testing.T) {
	t.Parallel()
	redirected := false
	destination := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		redirected = true
		if request.Header.Get("X-Presigned-Secret") != "" {
			t.Fatal("presigned upload header crossed an origin redirect")
		}
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Presigned-Secret") != "upload-secret" {
			t.Fatal("origin did not receive the presigned header")
		}
		writer.Header().Set("Location", destination.URL+"/capture")
		writer.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	client := NewClient("https://api.invalid", origin.Client(), nil, "")
	err := client.PutUpload(context.Background(), UploadPrepareResponse{
		UploadURL: origin.URL,
		Headers:   map[string]string{"X-Presigned-Secret": "upload-secret"},
	}, strings.NewReader("bundle"), int64(len("bundle")))
	if err == nil {
		t.Fatal("expected redirected upload to fail")
	}
	if redirected {
		t.Fatal("upload followed a redirect")
	}
}

func TestNormalizeAPIOrigin(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		raw  string
		want string
	}{
		{raw: "HTTPS://API.VICEME.AI:443/v1/", want: "https://api.viceme.ai"},
		{raw: "https://api.viceme.ai:8443/custom", want: "https://api.viceme.ai:8443"},
		{raw: "http://[::1]:3000/api", want: "http://[::1]:3000"},
	} {
		got, err := NormalizeAPIOrigin(test.raw)
		if err != nil || got != test.want {
			t.Fatalf("NormalizeAPIOrigin(%q)=%q,%v want %q", test.raw, got, err, test.want)
		}
	}
}
