package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type replicaDiagnosticsEnvelope struct {
	Data  replicaSupportResult `json:"data"`
	Error struct {
		Code      string         `json:"code"`
		Retryable bool           `json:"retryable"`
		Details   map[string]any `json:"details"`
	} `json:"error"`
}

type replicaRecoveryDiagnosticsFixture struct {
	invitationMu          sync.Mutex
	invitationEvents      []map[string]string
	invitationStatus      int
	t                     *testing.T
	deps                  Dependencies
	target, code, orderNo string
	account               bool
	checkoutCalls         atomic.Int32
	downloadFailure       atomic.Bool
	statusFailure         atomic.Bool
	paymentStatus         atomic.Value
	paidObserved          atomic.Bool
	validLicense          atomic.Bool
	downloadCalls         atomic.Int32
}

func newReplicaRecoveryDiagnosticsFixture(t *testing.T, immediate bool) *replicaRecoveryDiagnosticsFixture {
	t.Helper()
	t.Setenv(processAccessTokenEnvironment, "")
	const replicaID = "11111111-1111-4111-8111-111111111111"
	const versionID = "33333333-3333-4333-8333-333333333333"
	const sessionID = "44444444-4444-4444-8444-444444444444"
	const sessionToken = "vcs_1234567890123456789012345678901234567890123"
	const shortCode = "VMR-ABCDEFGHIJKLMNOPQRST"
	root := t.TempDir()
	f := &replicaRecoveryDiagnosticsFixture{t: t, target: filepath.Join(root, "site"), code: "VICEME-REPLICA:" + shortCode, orderNo: "VMO-DIAGNOSTICS"}
	archive := replicaTestZIP(t, map[string]string{"index.html": "<h1>Recovered</h1>"})
	digestBytes := sha256.Sum256(archive)
	digest := hex.EncodeToString(digestBytes[:])
	signer := newReplicaTestSigner(t, "replica-diagnostics-v1")
	trustReplicaTestSigner(t, signer)
	license := signedReplicaTestLicense(t, signer, replicaID, versionID, 1, f.orderNo, digest)
	validLicense := license
	license.Signature = base64.RawURLEncoding.EncodeToString(make([]byte, 64))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/invitation-flow") || strings.HasPrefix(r.URL.Path, "/v1/website-replica-invitation-flows") {
			var event map[string]string
			if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
				t.Error(err)
			}
			event["path"] = r.URL.Path
			f.invitationMu.Lock()
			f.invitationEvents = append(f.invitationEvents, event)
			status := f.invitationStatus
			f.invitationMu.Unlock()
			if status != 0 {
				w.WriteHeader(status)
				return
			}
			writeJSONResponse(w, map[string]any{"recorded": true})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/discovery") {
			w.WriteHeader(404)
			return
		}
		switch r.URL.Path {
		case "/v1/cli/auth/status":
			writeReplicaAuthStatus(w)
		case "/v1/website-replicas/quotes":
			writeJSONResponse(w, replicaQuoteResponse("22222222-2222-4222-8222-222222222222"))
		case "/v1/website-replicas/resolve":
			writeJSONResponse(w, replicaResolutionResponse(replicaID, shortCode))
		case "/v1/website-replica-sessions":
			writeJSONResponse(w, map[string]any{"sessionId": sessionID, "token": sessionToken, "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "recovered": false, "replica": replicaResolutionResponse(replicaID, shortCode)})
		case "/v1/website-replica-sessions/" + sessionID + "/checkout", "/v1/website-replicas/orders":
			f.checkoutCalls.Add(1)
			status := "PENDING"
			var action any = map[string]any{"type": "QR_CODE", "content": "weixin://pay/test-only"}
			if immediate {
				status = "PAID"
				action = nil
				f.paidObserved.Store(true)
			}
			response := map[string]any{"orderNo": f.orderNo, "status": status, "paymentAction": action, "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}
			if r.URL.Path != "/v1/website-replicas/orders" {
				response["checkoutUrl"] = serverURL(r) + "/checkout"
			}
			writeJSONResponse(w, response)
		case "/v1/website-replica-sessions/" + sessionID + "/orders/" + f.orderNo + "/status", "/v1/website-replica-sessions/recover-status", "/v1/website-replicas/orders/" + f.orderNo + "/status":
			if f.statusFailure.Load() {
				w.WriteHeader(503)
				writeJSONResponse(w, map[string]any{"statusCode": 503, "code": "DEPENDENCY_UNAVAILABLE", "message": "Status unavailable", "requestId": "test"})
				return
			}
			status := "PAID"
			if override := f.paymentStatus.Load(); override != nil {
				status = override.(string)
			}
			var paidAt, closedAt any
			if status == "PAID" {
				paidAt = "2026-09-08T14:38:37.000Z"
				f.paidObserved.Store(true)
			} else if status == "CLOSED" {
				closedAt = "2026-09-10T00:00:00.000Z"
			}
			writeJSONResponse(w, map[string]any{"orderNo": f.orderNo, "payment": map[string]any{"status": status, "paidAt": paidAt, "closedAt": closedAt}, "fulfillment": nil})
		case "/v1/website-replica-sessions/recover-download", "/v1/website-replicas/" + shortCode + "/download":
			f.downloadCalls.Add(1)
			if !f.paidObserved.Load() {
				w.WriteHeader(404)
				writeJSONResponse(w, map[string]any{"statusCode": 404, "code": "WEBSITE_REPLICA_NOT_FOUND", "message": "No entitlement yet", "requestId": "test"})
				return
			}
			if f.downloadFailure.Load() {
				w.WriteHeader(503)
				writeJSONResponse(w, map[string]any{"statusCode": 503, "code": "DEPENDENCY_UNAVAILABLE", "message": "Download unavailable", "requestId": "test"})
				return
			}
			deliveryLicense := license
			if f.validLicense.Load() {
				deliveryLicense = validLicense
			}
			writeJSONResponse(w, map[string]any{"replicaId": replicaID, "versionId": versionID, "version": 1, "fileName": "source.zip", "sizeBytes": len(archive), "artifactDigest": digest, "downloadUrl": serverURL(r) + "/source.zip?secret=download-capability", "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "license": deliveryLicense})
		case "/source.zip":
			_, _ = w.Write(archive)
		case "/v1/website-replicas/installations":
			writeJSONResponse(w, map[string]any{"replicaId": replicaID, "versionId": versionID, "version": 1, "installedAt": "2026-09-10T00:00:00.000Z"})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(server.Close)
	f.deps = Dependencies{HTTPClient: server.Client(), Store: securestore.NewMemory(), APIBaseURL: server.URL, Region: config.RegionCN, Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")}, Sleep: func(context.Context, time.Duration) error { return nil }}
	return f
}

func (f *replicaRecoveryDiagnosticsFixture) run(expectedExit int, code string, extra ...string) replicaDiagnosticsEnvelope {
	f.t.Helper()
	var stdout bytes.Buffer
	f.deps.Out, f.deps.ErrOut = &stdout, &bytes.Buffer{}
	args := []string{"replica", "install", f.code, "--target", f.target}
	if !f.account {
		args = append(args, "--anonymous")
	}
	recoveryOnly := false
	for _, arg := range extra {
		if arg == "--recovery-only" {
			recoveryOnly = true
		}
	}
	if !recoveryOnly && !f.account {
		args = append(args, "--accept-price-cents", "990")
	}
	args = append(args, extra...)
	exit := Execute(args, f.deps)
	var result replicaDiagnosticsEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || exit != expectedExit || result.Error.Code != code {
		f.t.Fatalf("expected %d/%s, got %d/%s: %s", expectedExit, code, exit, result.Error.Code, stdout.String())
	}
	for _, secret := range []string{"download-capability", "vcs_1234567890123456789012345678901234567890123", "weixin://pay/test-only"} {
		if bytes.Contains(stdout.Bytes(), []byte(secret)) {
			f.t.Fatal("failure exposed a private capability")
		}
	}
	return result
}
