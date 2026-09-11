package command

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestReplicaSelfPurchaseRejectionAllowsAnonymousRetryAtSameTarget(t *testing.T) {
	fixture := newReplicaRejectedPurchaseFixture(t, "PRODUCT_PURCHASE_SELF_REJECTED", false)
	otherTarget := filepath.Join(filepath.Dir(fixture.target), "other-site")
	fixture.run(output.ExitConfirmation, "REPLICA_PURCHASE_CONFIRMATION_REQUIRED", "replica", "install", fixture.code, "--target", otherTarget)
	otherMarker := replicaTargetReservationPath(otherTarget)
	otherReservation, err := os.ReadFile(otherMarker)
	if err != nil {
		t.Fatal(err)
	}
	fixture.quote()
	fixture.run(output.ExitAuthentication, "PRODUCT_PURCHASE_SELF_REJECTED", "replica", "install", fixture.code, "--target", fixture.target, "--confirm")
	if _, err := os.Stat(fixture.stateFile()); !os.IsNotExist(err) {
		t.Errorf("self-rejected purchase retained recovery state: %v", err)
	}
	if _, err := os.Stat(replicaTargetReservationPath(fixture.target)); !os.IsNotExist(err) {
		t.Errorf("self-rejected purchase retained target reservation: %v", err)
	}
	if after, err := os.ReadFile(otherMarker); err != nil || !bytes.Equal(after, otherReservation) {
		t.Fatalf("self rejection changed another target reservation: %v", err)
	}
	fixture.run(0, "", "auth", "logout")
	// Reuse the exact directory and persistently logged-out identity. Discovery
	// must still require price consent before creating an anonymous order.
	fixture.run(output.ExitConfirmation, "REPLICA_PURCHASE_CONFIRMATION_REQUIRED", "replica", "install", fixture.code, "--anonymous", "--target", fixture.target)
	if fixture.anonymousOrders.Load() != 0 {
		t.Fatal("anonymous retry created an order before price consent")
	}
	fixture.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED", "replica", "install", fixture.code, "--anonymous", "--target", fixture.target, "--accept-price-cents", "990")
	if fixture.anonymousOrders.Load() != 1 || fixture.accountOrders.Load() != 1 {
		t.Fatalf("unexpected checkout attempts: account=%d anonymous=%d", fixture.accountOrders.Load(), fixture.anonymousOrders.Load())
	}
	if _, err := os.Stat(fixture.target); !os.IsNotExist(err) {
		t.Fatalf("unpaid retry created the source directory: %v", err)
	}
}

func TestReplicaPurchaseFailurePreservesUncertainAndExistingOrders(t *testing.T) {
	for _, test := range []struct {
		name, code string
		existing   bool
		exit       int
	}{
		{"unknown denial", "PURCHASE_DENIED", false, output.ExitAuthentication},
		{"provider failure", "DEPENDENCY_UNAVAILABLE", false, output.ExitNetwork},
		{"already presented pending order", "REPLICA_PAYMENT_INTERRUPTED", true, output.ExitNetwork},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newReplicaRejectedPurchaseFixture(t, test.code, test.existing)
			fixture.quote()
			args := []string{"replica", "install", fixture.code, "--target", fixture.target, "--confirm"}
			if test.existing {
				fixture.run(output.ExitConfirmation, "REPLICA_PAYMENT_REQUIRED", args...)
				// A displayed order now resumes status polling without another order
				// request. Interrupt the wait instead of waiting three real minutes.
				fixture.deps.Sleep = func(context.Context, time.Duration) error { return context.Canceled }
			}
			fixture.run(test.exit, test.code, args...)
			data, err := os.ReadFile(fixture.stateFile())
			if err != nil {
				t.Fatalf("rejection discarded recoverable purchase: %v", err)
			}
			var state replicaPurchaseState
			if err := json.Unmarshal(data, &state); err != nil || state.OrderRequestID == "" || state.QuoteID == "" {
				t.Fatalf("rejection lost the original idempotency identity: %v", err)
			}
			if test.existing && fixture.accountOrders.Load() != 1 {
				t.Fatal("resuming a presented payment made another order request")
			}
			if test.existing && state.OrderNo != "VMO-EXISTING" {
				t.Fatalf("existing order identity changed: %q", state.OrderNo)
			}
			if _, err := os.Stat(replicaTargetReservationPath(fixture.target)); err != nil {
				t.Fatalf("rejection discarded the target reservation: %v", err)
			}
			if fixture.anonymousOrders.Load() != 0 {
				t.Fatal("rejection silently started anonymous checkout")
			}
		})
	}
}

type replicaRejectedPurchaseFixture struct {
	t                              *testing.T
	deps                           Dependencies
	code, shortCode, target        string
	apiOrigin                      string
	accountOrders, anonymousOrders atomic.Int32
}

func newReplicaRejectedPurchaseFixture(t *testing.T, rejectionCode string, existingOrder bool) *replicaRejectedPurchaseFixture {
	t.Helper()
	t.Setenv(processAccessTokenEnvironment, "")
	const token = "vme_cli_1234567890123456789012345678901234567890123"
	const replicaID = "11111111-1111-4111-8111-111111111111"
	const quoteID = "22222222-2222-4222-8222-222222222222"
	const sessionID = "44444444-4444-4444-8444-444444444444"
	const sessionToken = "vcs_1234567890123456789012345678901234567890123"
	root := t.TempDir()
	fixture := &replicaRejectedPurchaseFixture{
		t: t, code: "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST", shortCode: "VMR-ABCDEFGHIJKLMNOPQRST", target: filepath.Join(root, "site"),
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/discovery") {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeReplicaAuthStatus(writer)
		case "/v1/cli/auth/logout":
			if request.Header.Get("Authorization") != "Bearer "+token {
				t.Error("logout did not revoke the registered credential")
			}
			writer.WriteHeader(http.StatusNoContent)
		case "/v1/website-replicas/resolve":
			writeJSONResponse(writer, replicaResolutionResponse(replicaID, fixture.shortCode))
		case "/v1/website-replicas/quotes":
			writeJSONResponse(writer, replicaQuoteResponse(quoteID))
		case "/v1/website-replicas/orders":
			if existingOrder && fixture.accountOrders.Add(1) == 1 {
				writeJSONResponse(writer, map[string]any{"orderNo": "VMO-EXISTING", "status": "PENDING", "paymentAction": map[string]any{"type": "QR_CODE", "content": "weixin://pay/test-only"}, "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
				return
			}
			if !existingOrder {
				fixture.accountOrders.Add(1)
			}
			status := http.StatusForbidden
			if rejectionCode == "DEPENDENCY_UNAVAILABLE" {
				status = http.StatusServiceUnavailable
			}
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(status)
			writeJSONResponse(writer, map[string]any{"statusCode": status, "code": rejectionCode, "message": "purchase rejected", "requestId": "test-request"})
		case "/v1/website-replicas/orders/VMO-EXISTING/status":
			writeJSONResponse(writer, map[string]any{"orderNo": "VMO-EXISTING", "payment": map[string]any{"status": "PENDING", "paidAt": nil, "closedAt": nil}, "fulfillment": nil})
		case "/v1/website-replica-sessions":
			if request.Header.Get("Authorization") != "" {
				t.Error("logged-out retry reused the registered credential")
			}
			writeJSONResponse(writer, map[string]any{"sessionId": sessionID, "token": sessionToken, "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "recovered": false, "replica": replicaResolutionResponse(replicaID, fixture.shortCode)})
		case "/v1/website-replica-sessions/" + sessionID + "/checkout":
			fixture.anonymousOrders.Add(1)
			if request.Header.Get("Authorization") != "Bearer "+sessionToken {
				t.Error("anonymous checkout used the wrong identity")
			}
			writeJSONResponse(writer, map[string]any{"orderNo": "VMO-ANONYMOUS", "status": "PENDING", "paymentAction": map[string]any{"type": "QR_CODE", "content": "weixin://pay/test-only"}, "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "checkoutUrl": serverURL(request) + "/checkout"})
		default:
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	fixture.apiOrigin = server.URL
	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: config.DefaultProfileName, ProfileName: config.DefaultProfileName, Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: token, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	fixture.deps = Dependencies{Store: store, HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionCN, Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")}}
	return fixture
}

func (fixture *replicaRejectedPurchaseFixture) run(expectedExit int, expectedCode string, args ...string) {
	fixture.t.Helper()
	var stdout bytes.Buffer
	fixture.deps.Out, fixture.deps.ErrOut = &stdout, &bytes.Buffer{}
	exit := Execute(args, fixture.deps)
	var result struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || exit != expectedExit || result.Error.Code != expectedCode {
		fixture.t.Fatalf("command %v: expected exit=%d code=%s, got exit=%d output=%s err=%v", args, expectedExit, expectedCode, exit, stdout.String(), err)
	}
}

func (fixture *replicaRejectedPurchaseFixture) quote() {
	fixture.t.Helper()
	fixture.run(output.ExitConfirmation, "REPLICA_PURCHASE_CONFIRMATION_REQUIRED", "replica", "install", fixture.code, "--target", fixture.target)
}

func (fixture *replicaRejectedPurchaseFixture) stateFile() string {
	fixture.t.Helper()
	target, err := validateReplicaTarget(fixture.target)
	if err != nil {
		fixture.t.Fatal(err)
	}
	store, err := newReplicaPurchaseStore(&Runtime{apiBaseURL: fixture.apiOrigin, configBase: fixture.deps.Environment.ConfigDir, deps: defaults(fixture.deps)}, fixture.shortCode, target)
	if err != nil {
		fixture.t.Fatal(err)
	}
	return store.filename
}
