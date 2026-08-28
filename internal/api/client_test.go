package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
)

type staticToken string

func (token staticToken) Token(context.Context) (string, error) { return string(token), nil }

func TestPublicationClientUsesBearerAndExactContract(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/creator/skill-publications" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer vme_cli_test" {
			t.Fatalf("missing bearer credential: %q", request.Header.Get("Authorization"))
		}
		if request.Header.Get("User-Agent") != "viceme/test" {
			t.Fatalf("unexpected user-agent: %q", request.Header.Get("User-Agent"))
		}
		body, _ := io.ReadAll(request.Body)
		if !strings.Contains(string(body), `"clientRequestId":"request-1"`) {
			t.Fatalf("unexpected request body: %s", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"publicationId":"11111111-1111-4111-8111-111111111111","status":"DRAFT","packageUpload":null}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
	response, err := client.CreateSkillPublication(context.Background(), CreateSkillPublicationRequest{ClientRequestID: "request-1"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != "DRAFT" || response.PublicationID == "" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestSdkWorkClientUsesLightweightCreatorEndpoints(t *testing.T) {
	t.Parallel()
	requests := make(chan string, 5)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer vme_cli_test" {
			t.Fatalf("missing bearer credential: %q", request.Header.Get("Authorization"))
		}
		if request.URL.Path == "/v1/cli/sdk-works" && request.Method == http.MethodPost {
			body, _ := io.ReadAll(request.Body)
			if string(body) != `{"displayName":"Test"}` {
				t.Fatalf("unexpected create request: %s", body)
			}
		}
		requests <- request.Method + " " + request.URL.Path
		writer.Header().Set("Content-Type", "application/json")
		work := `{"workKey":"wrk_test","displayName":"Test","status":"DRAFT","configVersion":1,"features":[],"capabilities":[],"createdAt":"2026-08-15T00:00:00.000Z","updatedAt":"2026-08-15T00:00:00.000Z"}`
		if request.URL.Path == "/v1/cli/sdk-works" && request.Method == http.MethodGet {
			_, _ = io.WriteString(writer, `{"works":[`+work+`]}`)
			return
		}
		if request.Method == http.MethodPut {
			work = strings.Replace(work, `"configVersion":1`, `"configVersion":2`, 1)
		}
		_, _ = io.WriteString(writer, work)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
	if _, err := client.CreateSdkWork(context.Background(), CreateSdkWorkRequest{DisplayName: "Test"}); err != nil {
		t.Fatal(err)
	}
	if works, err := client.ListSdkWorks(context.Background()); err != nil {
		t.Fatal(err)
	} else if len(works.Works) != 1 || works.Works[0].WorkKey != "wrk_test" {
		t.Fatalf("unexpected Work list: %#v", works)
	}
	if _, err := client.GetSdkWork(context.Background(), "wrk_test"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ApplySdkWork(context.Background(), "wrk_test", ApplySdkWorkRequest{ExpectedConfigVersion: 1, DisplayName: "Test", Status: "DRAFT"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteSdkWork(context.Background(), "wrk_test"); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"POST /v1/cli/sdk-works",
		"GET /v1/cli/sdk-works",
		"GET /v1/cli/sdk-works/wrk_test",
		"PUT /v1/cli/sdk-works/wrk_test",
		"DELETE /v1/cli/sdk-works/wrk_test",
	}
	for _, expected := range want {
		if actual := <-requests; actual != expected {
			t.Fatalf("request = %q, want %q", actual, expected)
		}
	}
}

func TestSdkWorkMutationsRejectIncompleteSuccessfulResponses(t *testing.T) {
	t.Parallel()
	bodies := map[string]string{
		"empty":            " \n",
		"empty_object":     `{}`,
		"invalid_work_key": `{"workKey":"invalid","displayName":"Test","status":"DRAFT","configVersion":1,"features":[],"capabilities":[],"createdAt":"2026-08-15T00:00:00.000Z","updatedAt":"2026-08-15T00:00:00.000Z"}`,
		"missing_fields":   `{"workKey":"wrk_test","displayName":"Test","status":"DRAFT","configVersion":1}`,
	}
	operations := map[string]func(*Client) error{
		"create": func(client *Client) error {
			_, err := client.CreateSdkWork(context.Background(), CreateSdkWorkRequest{DisplayName: "Test"})
			return err
		},
		"apply": func(client *Client) error {
			_, err := client.ApplySdkWork(context.Background(), "wrk_test", ApplySdkWorkRequest{
				ExpectedConfigVersion: 1,
				DisplayName:           "Test",
				Features:              []SdkWorkFeatureConfig{},
				Status:                "DRAFT",
			})
			return err
		},
	}
	for operation, call := range operations {
		operation, call := operation, call
		for name, body := range bodies {
			name, body := name, body
			t.Run(operation+"/"+name, func(t *testing.T) {
				t.Parallel()
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(writer, body)
				}))
				defer server.Close()
				err := call(NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), ""))
				if err == nil {
					t.Fatal("incomplete 2xx response was accepted")
				}
				cliError := output.AsError(err)
				if cliError.Subtype != "RESPONSE_INVALID" || cliError.Retryable {
					t.Fatalf("incomplete 2xx response was accepted: err=%#v", cliError)
				}
			})
		}
	}
}

func TestCreatorAppCreateRejectsIncompleteSuccessfulResponses(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"empty":          "\n",
		"empty_object":   `{}`,
		"html_id":        `{"id":"\" onload=alert(1)","kind":"EXTERNAL","name":"Demo","domains":[],"createdAt":"2026-08-25T00:00:00Z"}`,
		"invalid_kind":   `{"id":"11111111-1111-4111-8111-111111111111","kind":"OTHER","name":"Demo","domains":[],"createdAt":"2026-08-25T00:00:00Z"}`,
		"invalid_time":   `{"id":"11111111-1111-4111-8111-111111111111","kind":"EXTERNAL","name":"Demo","domains":[],"createdAt":"yesterday"}`,
		"invalid_domain": `{"id":"11111111-1111-4111-8111-111111111111","kind":"EXTERNAL","name":"Demo","domains":[{"domain":"bad.example.com\" onload=alert(1)","verified":false,"verificationToken":null}],"createdAt":"2026-08-25T00:00:00Z"}`,
		"missing_fields": `{"id":"app-1","kind":"EXTERNAL","name":"Demo"}`,
	} {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(writer, body)
			}))
			defer server.Close()
			client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "")
			_, err := client.CreateCreatorApp(context.Background(), CreateCreatorAppRequest{Name: "Demo"})
			if err == nil {
				t.Fatal("incomplete Creator App response was accepted")
			}
			cliError := output.AsError(err)
			if cliError.Subtype != "RESPONSE_INVALID" || cliError.Retryable {
				t.Fatalf("incomplete Creator App response was accepted: err=%#v", cliError)
			}
		})
	}
}

func TestHealthReadyIsUnauthenticatedAndRedirectFree(t *testing.T) {
	t.Parallel()
	var targetCalled atomic.Bool
	var redirectResponse atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalled.Store(true)
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/health/ready" {
			t.Fatalf("unexpected readiness request: %s %s", request.Method, request.URL.Path)
		}
		if authorization := request.Header.Get("Authorization"); authorization != "" {
			t.Fatalf("readiness probe leaked a credential: %q", authorization)
		}
		if redirectResponse.Load() {
			http.Redirect(writer, request, target.URL, http.StatusTemporaryRedirect)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client(), staticToken("must-not-be-read"), "viceme/test")
	if err := client.HealthReady(context.Background()); err != nil {
		t.Fatal(err)
	}
	redirectResponse.Store(true)
	if err := client.HealthReady(context.Background()); err == nil {
		t.Fatal("readiness probe accepted a redirect")
	}
	if targetCalled.Load() {
		t.Fatal("readiness probe followed a redirect")
	}
}

func TestCreateSkillPreviewLaunchUsesBearerAndExactRoute(t *testing.T) {
	t.Parallel()
	const listingID = "66666666-6666-4666-8666-666666666666"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/creator/skill-listings/"+listingID+"/preview-launch" {
			t.Fatalf("unexpected preview launch request: %s %s", request.Method, request.URL.EscapedPath())
		}
		if request.Header.Get("Authorization") != "Bearer vme_cli_test" {
			t.Fatalf("missing bearer credential: %q", request.Header.Get("Authorization"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"launchUrl":"https://shop.example/v1/creator/skill-preview-launches/code","expiresAt":"2026-08-16T12:01:00Z"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
	result, err := client.CreateSkillPreviewLaunch(context.Background(), listingID)
	if err != nil {
		t.Fatal(err)
	}
	if result.LaunchURL == "" || result.ExpiresAt == "" {
		t.Fatalf("unexpected preview launch response: %#v", result)
	}
}

func TestClientPreservesCanonicalServerError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Request-Id", "request-from-header")
		writer.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(writer, `{"statusCode":409,"code":"PUBLICATION_CONFLICT","message":["one","two"]}`)
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "")
	_, err := client.GetSkillPublication(context.Background(), "11111111-1111-4111-8111-111111111111")
	var cliError *output.Error
	if !errors.As(err, &cliError) {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if cliError.Subtype != "PUBLICATION_CONFLICT" || cliError.Message != "one; two" || cliError.RequestID != "request-from-header" {
		t.Fatalf("unexpected typed error: %#v", cliError)
	}
}

func TestClientDoesNotRetryMissingOAuthConfiguration(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(writer, `{"statusCode":503,"code":"OAUTH_PROVIDER_NOT_CONFIGURED","message":"GITHUB OAuth is not configured"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "")
	_, err := client.StartGithubChannel(context.Background(), "11111111-1111-4111-8111-111111111111")
	var cliError *output.Error
	if !errors.As(err, &cliError) {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if cliError.Subtype != "OAUTH_PROVIDER_NOT_CONFIGURED" || cliError.Retryable {
		t.Fatalf("missing OAuth configuration was treated as retryable: %#v", cliError)
	}
}

func TestPutUploadRejectsRedirectAndInsecureRemoteURL(t *testing.T) {
	t.Parallel()
	client := NewClient("https://api.viceme.ai", nil, nil, "")
	if err := client.PutUpload(context.Background(), UploadAuthorization{Method: http.MethodPut, URL: "http://example.com/object"}, strings.NewReader("x"), 1); err == nil {
		t.Fatal("insecure remote upload URL was accepted")
	}

	redirected := false
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected = true }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	client.HTTPClient = source.Client()
	err := client.PutUpload(context.Background(), UploadAuthorization{Method: http.MethodPut, URL: source.URL}, strings.NewReader("x"), 1)
	if err == nil || redirected {
		t.Fatalf("upload redirect was followed: err=%v redirected=%v", err, redirected)
	}
}

func TestPutUploadTreatsExistingImmutableObjectAsRecoverable(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer server.Close()
	client := NewClient("https://api.viceme.ai", server.Client(), nil, "")
	if err := client.PutUpload(
		context.Background(),
		UploadAuthorization{Method: http.MethodPut, URL: server.URL},
		strings.NewReader("x"),
		1,
	); err != nil {
		t.Fatalf("existing immutable upload was not recoverable: %v", err)
	}
}

func TestPutUploadUsesDedicatedLongLivedClient(t *testing.T) {
	t.Parallel()
	var controlCalls atomic.Int32
	var uploadCalls atomic.Int32
	control := &http.Client{
		Timeout: time.Nanosecond,
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			controlCalls.Add(1)
			return nil, errors.New("control client must not be used for uploads")
		}),
	}
	upload := &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			uploadCalls.Add(1)
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	}
	client := &Client{BaseURL: "https://api.viceme.ai", HTTPClient: control, UploadHTTPClient: upload}
	if err := client.PutUpload(
		context.Background(),
		UploadAuthorization{Method: http.MethodPut, URL: "https://s3.viceme.ai/object"},
		strings.NewReader("x"),
		1,
	); err != nil {
		t.Fatalf("dedicated upload client failed: %v", err)
	}
	if controlCalls.Load() != 0 || uploadCalls.Load() != 1 {
		t.Fatalf("unexpected transport selection: control=%d upload=%d", controlCalls.Load(), uploadCalls.Load())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestNormalizeAPIOriginRejectsCredentialAndRemoteHTTP(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"http://example.com", "https://user@example.com", "https://example.com?x=1"} {
		if _, err := NormalizeAPIOrigin(value); err == nil {
			t.Fatalf("unsafe API origin accepted: %s", value)
		}
	}
	origin, err := NormalizeAPIOrigin("HTTPS://API.VICEME.AI:443/path")
	if err != nil || origin != "https://api.viceme.ai" {
		t.Fatalf("unexpected normalized origin %q: %v", origin, err)
	}
}
