package command

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestReplicaInstallPersistsAndPresentsQuoteBeforeCreatingOrder(t *testing.T) {
	const (
		accessToken = "vme_cli_1234567890123456789012345678901234567890123"
		fullCode    = "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST"
		replicaID   = "11111111-1111-4111-8111-111111111111"
		quoteID     = "22222222-2222-4222-8222-222222222222"
		requestID   = "33333333-3333-4333-8333-333333333333"
	)
	var resolveCalls, quoteCalls, orderCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			t.Fatalf("request was not authenticated: %q", request.Header.Get("Authorization"))
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/auth/status":
			writeReplicaAuthStatus(writer)
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/resolve":
			resolveCalls++
			assertJSONFields(t, request, map[string]any{"instruction": fullCode})
			writeJSONResponse(writer, replicaResolutionResponse(replicaID, "VMR-ABCDEFGHIJKLMNOPQRST"))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/quotes":
			quoteCalls++
			assertJSONFields(t, request, map[string]any{"instruction": fullCode, "clientRequestId": requestID})
			writeJSONResponse(writer, replicaQuoteResponse(quoteID))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/orders":
			orderCalls++
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"statusCode": 500, "code": "ORDER_REACHED", "message": "confirmation reached order creation", "requestId": "request-id",
			})
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, accessToken)
	root := t.TempDir()
	target := filepath.Join(root, "site")
	baseArguments := []string{"replica", "install", fullCode, "--target", target}
	dependencies := Dependencies{
		ErrOut: io.Discard, HTTPClient: server.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, APIBaseURL: server.URL, NewID: func() string { return requestID },
	}

	for attempt := 0; attempt < 2; attempt++ {
		var stdout bytes.Buffer
		dependencies.Out = &stdout
		exit := Execute(baseArguments, dependencies)
		if exit != output.ExitConfirmation {
			t.Fatalf("attempt %d did not stop at quote confirmation: exit=%d output=%q", attempt+1, exit, stdout.String())
		}
		assertReplicaQuoteConfirmation(t, stdout.Bytes(), quoteID, 990)
	}
	if resolveCalls != 1 || quoteCalls != 1 || orderCalls != 0 {
		t.Fatalf("unconfirmed retries changed the transaction: resolve=%d quotes=%d orders=%d", resolveCalls, quoteCalls, orderCalls)
	}
	states, err := filepath.Glob(filepath.Join(root, "config", "replica-purchases", "*.json"))
	if err != nil || len(states) != 1 {
		t.Fatalf("quote recovery state was not persisted exactly once: states=%v err=%v", states, err)
	}

	var confirmedOutput bytes.Buffer
	dependencies.Out = &confirmedOutput
	confirmedArguments := append(append([]string{}, baseArguments...), "--confirm")
	if exit := Execute(confirmedArguments, dependencies); exit == output.ExitConfirmation {
		t.Fatalf("confirmed quote did not reach order creation: output=%q", confirmedOutput.String())
	}
	if quoteCalls != 1 || orderCalls != 1 {
		t.Fatalf("confirmation did not consume the persisted quote: quotes=%d orders=%d", quoteCalls, orderCalls)
	}
}

func TestReplicaInstallCannotBlindlyConfirmANewQuote(t *testing.T) {
	const (
		accessToken = "vme_cli_1234567890123456789012345678901234567890123"
		fullCode    = "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST"
		replicaID   = "11111111-1111-4111-8111-111111111111"
		quoteID     = "22222222-2222-4222-8222-222222222222"
		requestID   = "33333333-3333-4333-8333-333333333333"
	)
	var orderCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/auth/status":
			writeReplicaAuthStatus(writer)
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/resolve":
			writeJSONResponse(writer, replicaResolutionResponse(replicaID, "VMR-ABCDEFGHIJKLMNOPQRST"))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/quotes":
			writeJSONResponse(writer, replicaQuoteResponse(quoteID))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/orders":
			orderCalls++
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, accessToken)
	root := t.TempDir()
	var stdout bytes.Buffer
	exit := Execute([]string{
		"replica", "install", fullCode, "--target", filepath.Join(root, "site"), "--confirm",
	}, Dependencies{
		Out: &stdout, ErrOut: io.Discard, HTTPClient: server.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, APIBaseURL: server.URL, NewID: func() string { return requestID },
	})
	if exit != output.ExitConfirmation || orderCalls != 0 {
		t.Fatalf("blind confirmation created an order: exit=%d orders=%d output=%q", exit, orderCalls, stdout.String())
	}
	assertReplicaQuoteConfirmation(t, stdout.Bytes(), quoteID, 990)
}

func TestReplicaInstallReplacesExpiredQuoteAndRequiresFreshConfirmation(t *testing.T) {
	const (
		accessToken     = "vme_cli_1234567890123456789012345678901234567890123"
		fullCode        = "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST"
		replicaID       = "11111111-1111-4111-8111-111111111111"
		firstQuoteID    = "22222222-2222-4222-8222-222222222222"
		secondQuoteID   = "33333333-3333-4333-8333-333333333333"
		firstRequestID  = "44444444-4444-4444-8444-444444444444"
		secondRequestID = "55555555-5555-4555-8555-555555555555"
	)
	now := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	var resolveCalls, quoteCalls, orderCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/auth/status":
			writeReplicaAuthStatus(writer)
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/resolve":
			resolveCalls++
			response := replicaResolutionResponse(replicaID, "VMR-ABCDEFGHIJKLMNOPQRST")
			if resolveCalls == 2 {
				response["product"].(map[string]any)["priceCents"] = 1290
			}
			writeJSONResponse(writer, response)
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/quotes":
			quoteCalls++
			response := replicaQuoteResponse(firstQuoteID)
			if quoteCalls == 1 {
				response["expiresAt"] = now.Add(time.Minute).Format(time.RFC3339)
			} else {
				response["id"] = secondQuoteID
				response["unitAmountCents"] = 1290
				response["subtotalAmountCents"] = 1290
				response["totalAmountCents"] = 1290
				response["expiresAt"] = now.Add(10 * time.Minute).Format(time.RFC3339)
			}
			writeJSONResponse(writer, response)
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/orders":
			orderCalls++
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, accessToken)
	root := t.TempDir()
	arguments := []string{"replica", "install", fullCode, "--target", filepath.Join(root, "site")}
	requestIDs := []string{firstRequestID, secondRequestID}
	dependencies := Dependencies{
		ErrOut: io.Discard, HTTPClient: server.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, APIBaseURL: server.URL, Now: func() time.Time { return now },
		NewID: func() string {
			if len(requestIDs) == 0 {
				t.Fatal("unexpected request identity creation")
			}
			value := requestIDs[0]
			requestIDs = requestIDs[1:]
			return value
		},
	}

	var firstOutput bytes.Buffer
	dependencies.Out = &firstOutput
	if exit := Execute(arguments, dependencies); exit != output.ExitConfirmation {
		t.Fatalf("initial quote was not presented: exit=%d output=%q", exit, firstOutput.String())
	}
	assertReplicaQuoteConfirmation(t, firstOutput.Bytes(), firstQuoteID, 990)

	now = now.Add(2 * time.Minute)
	var replacementOutput bytes.Buffer
	dependencies.Out = &replacementOutput
	if exit := Execute(arguments, dependencies); exit != output.ExitConfirmation {
		t.Fatalf("unconfirmed expired quote did not require fresh confirmation: exit=%d output=%q", exit, replacementOutput.String())
	}
	assertReplicaQuoteConfirmation(t, replacementOutput.Bytes(), secondQuoteID, 1290)
	if resolveCalls != 2 || quoteCalls != 2 || orderCalls != 0 {
		t.Fatalf("expired quote crossed the confirmation boundary: resolve=%d quotes=%d orders=%d", resolveCalls, quoteCalls, orderCalls)
	}
}

func TestReplicaInstallReplacesQuoteRejectedAtOrderCreation(t *testing.T) {
	const (
		accessToken   = "vme_cli_1234567890123456789012345678901234567890123"
		fullCode      = "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST"
		replicaID     = "11111111-1111-4111-8111-111111111111"
		firstQuoteID  = "22222222-2222-4222-8222-222222222222"
		secondQuoteID = "33333333-3333-4333-8333-333333333333"
	)
	requestIDs := []string{
		"44444444-4444-4444-8444-444444444444",
		"55555555-5555-4555-8555-555555555555",
		"66666666-6666-4666-8666-666666666666",
	}
	var quoteCalls, orderCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/auth/status":
			writeReplicaAuthStatus(writer)
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/resolve":
			writeJSONResponse(writer, replicaResolutionResponse(replicaID, "VMR-ABCDEFGHIJKLMNOPQRST"))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/quotes":
			quoteCalls++
			quoteID := firstQuoteID
			if quoteCalls == 2 {
				quoteID = secondQuoteID
			}
			writeJSONResponse(writer, replicaQuoteResponse(quoteID))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/orders":
			orderCalls++
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"statusCode": 409, "code": "QUOTE_EXPIRED", "message": "Quote expired", "requestId": "request-id",
			})
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, accessToken)
	root := t.TempDir()
	arguments := []string{"replica", "install", fullCode, "--target", filepath.Join(root, "site")}
	dependencies := Dependencies{
		ErrOut: io.Discard, HTTPClient: server.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, APIBaseURL: server.URL,
		NewID: func() string {
			if len(requestIDs) == 0 {
				t.Fatal("unexpected request identity creation")
			}
			value := requestIDs[0]
			requestIDs = requestIDs[1:]
			return value
		},
	}

	dependencies.Out = io.Discard
	if exit := Execute(arguments, dependencies); exit != output.ExitConfirmation {
		t.Fatalf("initial quote was not presented: exit=%d", exit)
	}
	var replacementOutput bytes.Buffer
	dependencies.Out = &replacementOutput
	if exit := Execute(append(append([]string{}, arguments...), "--confirm"), dependencies); exit != output.ExitConfirmation {
		t.Fatalf("server-expired quote did not require fresh confirmation: exit=%d output=%q", exit, replacementOutput.String())
	}
	assertReplicaQuoteConfirmation(t, replacementOutput.Bytes(), secondQuoteID, 990)
	if quoteCalls != 2 || orderCalls != 1 {
		t.Fatalf("replacement quote was consumed without confirmation: quotes=%d orders=%d", quoteCalls, orderCalls)
	}
}

func assertReplicaQuoteConfirmation(t *testing.T, outputBytes []byte, quoteID string, amountCents float64) {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(outputBytes, &envelope); err != nil {
		t.Fatalf("decode confirmation output: %v: %s", err, outputBytes)
	}
	errorBody, _ := envelope["error"].(map[string]any)
	details, _ := errorBody["details"].(map[string]any)
	if errorBody["type"] != "confirmation" || errorBody["code"] != "REPLICA_PURCHASE_CONFIRMATION_REQUIRED" ||
		details["quoteId"] != quoteID || details["productId"] != replicaTestProductID ||
		details["currency"] != "CNY" || details["totalAmountCents"] != amountCents || details["expiresAt"] == "" {
		t.Fatalf("unexpected quote confirmation: %#v", envelope)
	}
}
