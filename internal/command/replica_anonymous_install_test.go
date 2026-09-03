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
	"testing"
	"time"

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

	var sessionRequestID, quoteRequestID, orderRequestID string
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
				AcceptedPriceCents   int    `json:"acceptedPriceCents"`
				QuoteClientRequestID string `json:"quoteClientRequestId"`
				OrderClientRequestID string `json:"orderClientRequestId"`
			}
			_ = json.NewDecoder(request.Body).Decode(&body)
			if body.AcceptedPriceCents != 990 {
				t.Fatalf("accepted price = %d", body.AcceptedPriceCents)
			}
			quoteRequestID, orderRequestID = body.QuoteClientRequestID, body.OrderClientRequestID
			writeJSONResponse(writer, map[string]any{
				"orderNo": orderNo, "status": "PAID", "paymentAction": nil,
				"expiresAt":   time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
				"checkoutUrl": serverURL(request) + "/replica-checkout/" + sessionID + "#token=hidden",
			})
		case "/v1/website-replica-sessions/" + sessionID + "/download":
			if request.Header.Get("Authorization") != "Bearer "+sessionToken {
				t.Fatalf("download did not use the anonymous session token")
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

	target := filepath.Join(root, "copy")
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

func TestAnonymousPaidReplicaOpensHostedPaymentWidgetWithoutSecondConfirmation(t *testing.T) {
	const (
		fullCode     = "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST"
		replicaID    = "11111111-1111-4111-8111-111111111111"
		sessionID    = "44444444-4444-4444-8444-444444444444"
		sessionToken = "vcs_1234567890123456789012345678901234567890123"
		orderNo      = "VMO-20260903-000002"
		paymentURI   = "weixin://pay/not-for-command-output"
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		resolution := replicaResolutionResponse(replicaID, "VMR-ABCDEFGHIJKLMNOPQRST")
		switch request.URL.Path {
		case "/v1/website-replicas/resolve":
			writeJSONResponse(writer, resolution)
		case "/v1/website-replica-sessions":
			writeJSONResponse(writer, map[string]any{
				"sessionId": sessionID, "token": sessionToken,
				"expiresAt": time.Now().UTC().Add(time.Hour).Format(time.RFC3339), "recovered": false, "replica": resolution,
			})
		case "/v1/website-replica-sessions/" + sessionID + "/checkout":
			writeJSONResponse(writer, map[string]any{
				"orderNo": orderNo, "status": "PENDING",
				"paymentAction": map[string]any{"type": "QR_CODE", "content": paymentURI},
				"expiresAt":     time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
				"checkoutUrl":   serverURL(request) + "/replica-checkout/" + sessionID + "#token=hosted-capability&orderNo=" + orderNo,
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
	var stdout bytes.Buffer
	deps := Dependencies{
		Out: &stdout, ErrOut: &bytes.Buffer{}, HTTPClient: server.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, APIBaseURL: server.URL,
		NewID: func() string { id := ids[0]; ids = ids[1:]; return id },
	}
	exit := Execute([]string{
		"replica", "install", fullCode, "--target", filepath.Join(root, "copy"), "--accept-price-cents", "990",
	}, deps)
	if exit != output.ExitConfirmation {
		t.Fatalf("payment widget was not requested: exit=%d output=%q", exit, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"nextAction": "OPEN_PAYMENT_WIDGET"`)) ||
		!bytes.Contains(stdout.Bytes(), []byte(`/replica-checkout/`+sessionID)) {
		t.Fatalf("unexpected payment widget response: %q", stdout.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte(paymentURI)) || bytes.Contains(stdout.Bytes(), []byte(`"sessionToken"`)) {
		t.Fatalf("payment response leaked an internal capability: %q", stdout.String())
	}
}

func serverURL(request *http.Request) string {
	return "http://" + request.Host
}
