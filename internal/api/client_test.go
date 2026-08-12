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

func TestPaymentRuntimeUsesEnvironmentCredentialAndIdempotencyHeader(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/checkout/v1/sessions" {
			t.Fatalf("unexpected runtime request: %s %s", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer vcp_sandbox_secret" {
			t.Fatalf("runtime request used the wrong credential: %q", got)
		}
		if got := request.Header.Get("Idempotency-Key"); got != "checkout-1" {
			t.Fatalf("missing idempotency key: %q", got)
		}
		_, _ = io.WriteString(writer, `{"id":"checkout-id"}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client(), staticToken("must-not-be-used"), "")
	var result map[string]any
	if err := client.PaymentRuntime(context.Background(), http.MethodPost, "/v1/checkout/v1/sessions", map[string]string{"externalOrderNo": "order-1"}, &result, "vcp_sandbox_secret", "checkout-1"); err != nil {
		t.Fatal(err)
	}
	if result["id"] != "checkout-id" {
		t.Fatalf("unexpected runtime response: %#v", result)
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
