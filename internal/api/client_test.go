package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
)

type staticToken string

func (token staticToken) Token(context.Context) (string, error) { return string(token), nil }

func TestDeviceAuthorizationUsesNewShopContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/device" || request.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["client_id"] != "viceme-cli" {
			t.Fatalf("unexpected client_id %#v", body)
		}
		writeJSONResponse(t, writer, map[string]any{
			"verification_url":          "https://shop.example/cli/authorize",
			"verification_url_complete": "https://shop.example/cli/authorize?user_code=ABCD-EFGH",
			"device_code":               "vcm_dc_example",
			"user_code":                 "ABCD-EFGH",
			"expires_at":                "2026-08-06T00:00:00.000Z",
			"interval_seconds":          5,
		})
	}))
	defer server.Close()

	result, err := NewClient(server.URL, server.Client(), nil, "test").StartDeviceAuthorization(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.UserCode != "ABCD-EFGH" || result.IntervalSeconds != 5 {
		t.Fatalf("unexpected response %#v", result)
	}
}

func TestCreatorAppRequestsUseAPIKeyAndExactPaths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("x-api-key") != "vcm_at_test" {
			t.Fatal("missing CLI credential")
		}
		switch request.URL.Path {
		case "/v1/creator-apps":
			if request.Method == http.MethodGet {
				writeJSONResponse(t, writer, map[string]any{"items": []any{}})
				return
			}
		case "/v1/creator-apps/capabilities/catalog":
			writeJSONResponse(t, writer, map[string]any{"items": []any{}})
			return
		}
		t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client(), staticToken("vcm_at_test"), "test")

	if _, err := client.ListCreatorApps(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CapabilityCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPublicContextSendsOriginWithoutCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Origin") != "http://localhost:3000" {
			t.Fatalf("unexpected Origin %q", request.Header.Get("Origin"))
		}
		if request.Header.Get("x-api-key") != "" {
			t.Fatal("public context must not send a CLI credential")
		}
		writeJSONResponse(t, writer, map[string]any{
			"app":          map[string]string{"name": "Poster Lab"},
			"environment":  "TEST",
			"capabilities": []any{},
		})
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client(), staticToken("must-not-send"), "test")

	contextResult, err := client.GetPublicAppContext(context.Background(), "app_pk_test_example", "http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	if contextResult.App.Name != "Poster Lab" {
		t.Fatalf("unexpected context %#v", contextResult)
	}
}

func TestCanonicalErrorPreservesCodeAndRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
		writeJSONResponse(t, writer, map[string]any{
			"statusCode": 403,
			"code":       "APP_ORIGIN_NOT_ALLOWED",
			"message":    "Origin denied",
			"requestId":  "request-123",
		})
	}))
	defer server.Close()

	_, err := NewClient(server.URL, server.Client(), nil, "test").GetPublicAppContext(context.Background(), "key", "https://example.com")
	var cliError *output.Error
	if !errors.As(err, &cliError) {
		t.Fatalf("expected CLI error, got %v", err)
	}
	if cliError.Subtype != "app_origin_not_allowed" || cliError.Type != "authorization" {
		t.Fatalf("unexpected error %#v", cliError)
	}
	details, ok := cliError.Details.(map[string]any)
	if !ok || details["request_id"] != "request-123" {
		t.Fatalf("request ID not preserved: %#v", cliError.Details)
	}
}

func TestNormalizeAPIBaseURLPreservesCanonicalPathAuthority(t *testing.T) {
	for input, expected := range map[string]string{
		"https://API.Example.com:443/staging/": "https://api.example.com/staging",
		"https://api.example.com/":             "https://api.example.com",
		"http://LOCALHOST:80/v1/":              "http://localhost/v1",
	} {
		actual, err := NormalizeAPIBaseURL(input)
		if err != nil || actual != expected {
			t.Fatalf("NormalizeAPIBaseURL(%q)=%q err=%v; want %q", input, actual, err, expected)
		}
	}
}

func writeJSONResponse(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Fatal(err)
	}
}
