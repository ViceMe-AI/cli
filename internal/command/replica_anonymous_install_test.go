package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestReplicaInspectAndAnonymousFreeInstall(t *testing.T) {
	const (
		fullCode     = "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST"
		shortCode    = "VMR-ABCDEFGHIJKLMNOPQRST"
		replicaID    = "11111111-1111-4111-8111-111111111111"
		versionID    = "33333333-3333-4333-8333-333333333333"
		sessionID    = "44444444-4444-4444-8444-444444444444"
		sessionToken = "vcs_1234567890123456789012345678901234567890123"
		orderNo      = "VMO-20260903-000001"
	)
	archive := replicaTestZIP(t, map[string]string{"index.html": "<h1>Anonymous copy</h1>"})
	digestBytes := sha256.Sum256(archive)
	digest := hex.EncodeToString(digestBytes[:])
	signer := newReplicaTestSigner(t, "replica-anonymous-v1")
	trustReplicaTestSigner(t, signer)
	license := signedReplicaTestLicense(t, signer, replicaID, versionID, 1, orderNo, digest)

	objectServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Length", strconv.Itoa(len(archive)))
		_, _ = writer.Write(archive)
	}))
	defer objectServer.Close()

	var sessionRequestID, quoteRequestID, orderRequestID, recoverySecret string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		resolution := replicaResolutionResponse(replicaID, shortCode)
		switch request.URL.Path {
		case "/v1/website-replicas/resolve":
			if request.Header.Get("Authorization") != "" {
				t.Fatalf("public resolution unexpectedly authenticated")
			}
			writeJSONResponse(writer, resolution)
		case "/v1/website-replica-sessions":
			if request.Header.Get("Authorization") != "" {
				t.Fatalf("anonymous session unexpectedly authenticated")
			}
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			sessionRequestID, _ = body["clientRequestId"].(string)
			writeJSONResponse(writer, map[string]any{
				"sessionId": sessionID, "token": sessionToken,
				"expiresAt": time.Now().UTC().Add(time.Hour).Format(time.RFC3339), "recovered": false, "replica": resolution,
			})
		case "/v1/website-replica-sessions/" + sessionID + "/checkout":
			if request.Header.Get("Authorization") != "Bearer "+sessionToken {
				t.Fatalf("checkout did not use the anonymous session token")
			}
			var body struct {
				AcceptedPriceCents     int    `json:"acceptedPriceCents"`
				QuoteClientRequestID   string `json:"quoteClientRequestId"`
				OrderClientRequestID   string `json:"orderClientRequestId"`
				DownloadRecoverySecret string `json:"downloadRecoverySecret"`
			}
			_ = json.NewDecoder(request.Body).Decode(&body)
			if body.AcceptedPriceCents != 990 {
				t.Fatalf("accepted price = %d", body.AcceptedPriceCents)
			}
			quoteRequestID, orderRequestID, recoverySecret = body.QuoteClientRequestID, body.OrderClientRequestID, body.DownloadRecoverySecret
			writeJSONResponse(writer, map[string]any{
				"orderNo": orderNo, "status": "PAID", "paymentAction": nil,
				"expiresAt":   time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
				"checkoutUrl": serverURL(request) + "/replica-checkout/" + sessionID + "#token=hidden",
			})
		case "/v1/website-replica-sessions/recover-download":
			var body api.RecoverWebsiteReplicaDownloadRequest
			_ = json.NewDecoder(request.Body).Decode(&body)
			if request.Header.Get("Authorization") != "" || body.OrderNo != orderNo || body.RecoverySecret != recoverySecret {
				t.Fatalf("download recovery was not bound to the anonymous order")
			}
			writeJSONResponse(writer, map[string]any{
				"replicaId": replicaID, "versionId": versionID, "version": 1, "fileName": "source.zip",
				"sizeBytes": len(archive), "artifactDigest": digest, "downloadUrl": objectServer.URL,
				"expiresAt": time.Now().UTC().Add(time.Hour).Format(time.RFC3339), "license": license,
			})
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	ids := []string{
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		"cccccccc-cccc-4ccc-8ccc-cccccccccccc",
	}
	deps := Dependencies{
		HTTPClient: server.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, APIBaseURL: server.URL,
		NewID: func() string { id := ids[0]; ids = ids[1:]; return id },
		Sleep: func(context.Context, time.Duration) error { return nil },
	}
	var inspectOutput bytes.Buffer
	deps.Out, deps.ErrOut = &inspectOutput, &bytes.Buffer{}
	if exit := Execute([]string{"replica", "inspect", fullCode}, deps); exit != 0 || !bytes.Contains(inspectOutput.Bytes(), []byte(`"nextAction": "OPEN_WORK_PREVIEW"`)) {
		t.Fatalf("inspect failed: exit=%d output=%q", exit, inspectOutput.String())
	}

	target := filepath.Join(root, strings.ToLower(shortCode))
	var installOutput bytes.Buffer
	deps.Out = &installOutput
	if exit := Execute([]string{"replica", "install", fullCode, "--target", target, "--accept-price-cents", "990"}, deps); exit != 0 {
		t.Fatalf("anonymous install failed: exit=%d output=%q", exit, installOutput.String())
	}
	if sessionRequestID == "" || quoteRequestID == "" || orderRequestID == "" {
		t.Fatalf("idempotency identities were not sent: session=%q quote=%q order=%q", sessionRequestID, quoteRequestID, orderRequestID)
	}
	if content, err := os.ReadFile(filepath.Join(target, "index.html")); err != nil || string(content) != "<h1>Anonymous copy</h1>" {
		t.Fatalf("installed content mismatch: %q %v", content, err)
	}
}

func TestDefaultReplicaTargetUsesAChildOfTheCurrentWorkspace(t *testing.T) {
	workspace := t.TempDir()
	if target := defaultReplicaTarget(workspace, "VMR-ABCDEFGHIJKLMNOPQRST"); target != filepath.Join(workspace, "vmr-abcdefghijklmnopqrst") {
		t.Fatalf("default target = %q", target)
	}
}

func TestReplicaInspectFailureStopsInsteadOfLaunchingDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotImplemented)
	}))
	defer server.Close()

	root := t.TempDir()
	var stdout bytes.Buffer
	exit := Execute([]string{"replica", "inspect", "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST"}, Dependencies{
		Out: &stdout, ErrOut: &bytes.Buffer{}, HTTPClient: server.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, APIBaseURL: server.URL,
	})
	if exit != output.ExitInternal {
		t.Fatalf("inspect failure exit = %d, output=%q", exit, stdout.String())
	}
	var envelope struct {
		Error struct {
			Code      string         `json:"code"`
			Retryable bool           `json:"retryable"`
			Details   map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "HTTP_501" || envelope.Error.Retryable ||
		envelope.Error.Details["nextAction"] != "STOP_AND_REPORT" ||
		envelope.Error.Details["stage"] != "INSPECT_REPLICA" {
		t.Fatalf("inspect failure did not stop deterministically: %#v", envelope.Error)
	}
}

func TestAnonymousPaidReplicaOpensHostedPaymentPageThenWaitsThreeMinutes(t *testing.T) {
	const (
		fullCode     = "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST"
		replicaID    = "11111111-1111-4111-8111-111111111111"
		sessionID    = "44444444-4444-4444-8444-444444444444"
		sessionToken = "vcs_1234567890123456789012345678901234567890123"
		orderNo      = "VMO-20260903-000002"
		paymentURI   = "weixin://pay/not-for-command-output"
	)
	var sessionCalls, checkoutCalls, statusCalls int
	var downloadRecoverySecret string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		resolution := replicaResolutionResponse(replicaID, "VMR-ABCDEFGHIJKLMNOPQRST")
		switch request.URL.Path {
		case "/v1/website-replicas/resolve":
			writeJSONResponse(writer, resolution)
		case "/v1/website-replica-sessions":
			sessionCalls++
			writeJSONResponse(writer, map[string]any{
				"sessionId": sessionID, "token": sessionToken,
				"expiresAt": time.Now().UTC().Add(time.Hour).Format(time.RFC3339), "recovered": false, "replica": resolution,
			})
		case "/v1/website-replica-sessions/" + sessionID + "/checkout":
			checkoutCalls++
			var body api.CheckoutWebsiteReplicaRequest
			_ = json.NewDecoder(request.Body).Decode(&body)
			if checkoutCalls == 1 {
				downloadRecoverySecret = body.DownloadRecoverySecret
			}
			if len(body.DownloadRecoverySecret) != 43 {
				t.Fatalf("checkout recovery credential length = %d", len(body.DownloadRecoverySecret))
			}
			writeJSONResponse(writer, map[string]any{
				"orderNo": orderNo, "status": "PENDING",
				"paymentAction": map[string]any{"type": "QR_CODE", "content": paymentURI},
				"expiresAt":     time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
				"checkoutUrl":   serverURL(request) + "/replica-checkout/" + sessionID + "#token=hosted-capability&orderNo=" + orderNo,
			})
		case "/v1/website-replica-sessions/" + sessionID + "/orders/" + orderNo + "/status":
			statusCalls++
			writeJSONResponse(writer, map[string]any{
				"orderNo":     orderNo,
				"payment":     map[string]any{"status": "PENDING", "paidAt": nil, "closedAt": nil},
				"fulfillment": nil, "serviceCase": nil,
			})
		case "/v1/website-replica-sessions/recover-download":
			var body api.RecoverWebsiteReplicaDownloadRequest
			_ = json.NewDecoder(request.Body).Decode(&body)
			if body.OrderNo != orderNo || body.RecoverySecret != downloadRecoverySecret {
				t.Fatalf("download recovery did not preserve the original order credential")
			}
			writer.WriteHeader(http.StatusNotFound)
			writeJSONResponse(writer, map[string]any{
				"statusCode": 404, "code": "WEBSITE_REPLICA_NOT_FOUND", "message": "Website replica not found", "requestId": "test-request",
			})
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	ids := []string{
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		"cccccccc-cccc-4ccc-8ccc-cccccccccccc",
		"dddddddd-dddd-4ddd-8ddd-dddddddddddd",
		"eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
		"ffffffff-ffff-4fff-8fff-ffffffffffff",
	}
	var stdout bytes.Buffer
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	var sleeps []time.Duration
	deps := Dependencies{
		Out: &stdout, ErrOut: &bytes.Buffer{}, HTTPClient: server.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, APIBaseURL: server.URL,
		NewID: func() string { id := ids[0]; ids = ids[1:]; return id },
		Now:   func() time.Time { return now },
		Sleep: func(_ context.Context, delay time.Duration) error {
			sleeps = append(sleeps, delay)
			now = now.Add(delay)
			return nil
		},
	}
	exit := Execute([]string{
		"replica", "install", fullCode, "--target", filepath.Join(root, "copy"), "--accept-price-cents", "990",
	}, deps)
	if exit != output.ExitConfirmation {
		t.Fatalf("payment page was not requested: exit=%d output=%q", exit, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"nextAction": "OPEN_PAYMENT_PAGE"`)) ||
		!bytes.Contains(stdout.Bytes(), []byte(`"checkoutUrl"`)) {
		t.Fatalf("unexpected payment page response: %q", stdout.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte(paymentURI)) || bytes.Contains(stdout.Bytes(), []byte(`"sessionToken"`)) {
		t.Fatalf("payment response leaked a provider or session capability: %q", stdout.String())
	}
	var paymentEnvelope struct {
		Error struct {
			Details struct {
				CheckoutURL string `json:"checkoutUrl"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &paymentEnvelope); err != nil {
		t.Fatal(err)
	}
	checkoutURL := paymentEnvelope.Error.Details.CheckoutURL
	if !strings.Contains(checkoutURL, "/replica-checkout/"+sessionID) || !strings.Contains(checkoutURL, "#token=hosted-capability") {
		t.Fatalf("payment response did not include the hosted checkout page: %q", checkoutURL)
	}

	stdout.Reset()
	if exit := Execute([]string{
		"replica", "install", fullCode, "--target", filepath.Join(root, "copy"), "--accept-price-cents", "990",
	}, deps); exit != output.ExitConfirmation {
		t.Fatalf("unacknowledged payment page was not presented again: exit=%d output=%q", exit, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"nextAction": "OPEN_PAYMENT_PAGE"`)) || sessionCalls != 1 || checkoutCalls != 1 {
		t.Fatalf("payment page replay created another checkout: sessions=%d checkouts=%d output=%q", sessionCalls, checkoutCalls, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(checkoutURL)) {
		t.Fatalf("payment page replay did not reuse the hosted checkout: %q", stdout.String())
	}

	stdout.Reset()
	if exit := Execute([]string{
		"replica", "install", fullCode, "--target", filepath.Join(root, "copy"), "--accept-price-cents", "990", "--payment-presented",
	}, deps); exit != output.ExitNetwork {
		t.Fatalf("pending payment did not end at the bounded deadline: exit=%d output=%q", exit, stdout.String())
	}
	if statusCalls != 3 || len(sleeps) != 3 {
		t.Fatalf("payment wait did not poll once per minute for three minutes: statusCalls=%d sleeps=%v", statusCalls, sleeps)
	}
	for index, delay := range sleeps {
		if delay != time.Minute {
			t.Fatalf("payment sleep %d = %s, want 1m", index, delay)
		}
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"code": "REPLICA_PAYMENT_TIMEOUT"`)) {
		t.Fatalf("pending payment returned the wrong result: %q", stdout.String())
	}

	stdout.Reset()
	if exit := Execute([]string{
		"replica", "install", fullCode, "--target", filepath.Join(root, "another-copy"), "--accept-price-cents", "990",
	}, deps); exit != output.ExitConfirmation {
		t.Fatalf("another target did not create its own checkout: exit=%d output=%q", exit, stdout.String())
	}
	if sessionCalls != 2 || checkoutCalls != 2 {
		t.Fatalf("another target recovered historical checkout: sessions=%d checkouts=%d", sessionCalls, checkoutCalls)
	}
}

func serverURL(request *http.Request) string {
	return "http://" + request.Host
}
