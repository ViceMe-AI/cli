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
