package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

const skillPurchaseAccessToken = "vme_cli_1234567890123456789012345678901234567890123"
const skillPurchaseOrderNo = "VM1234567890"
const skillSubscriptionOrderNo = "VMSUB123456"
const skillPurchaseSkuID = "44444444-4444-4444-8444-444444444444"
const skillPurchaseQuoteID = "55555555-5555-4555-8555-555555555555"

type skillPurchaseTestServer struct {
	mu sync.Mutex
	// paymentStatus is the payment state returned by the order status
	// endpoint; tests flip it to PAID to complete the QR payment loop.
	paymentStatus        string
	subscriptionState    string
	orderCreates         int
	orderStatusCalls     int
	getOrderCalls        []string
	paidAfterStatusCalls int
	server               *httptest.Server
	archiveDigest        string
	archive              []byte
}

func newSkillPurchaseTestServer(t *testing.T) *skillPurchaseTestServer {
	t.Helper()
	state := &skillPurchaseTestServer{
		paymentStatus:        "PENDING",
		subscriptionState:    "PENDING",
		paidAfterStatusCalls: 2,
	}
	state.archive = downloadableSkillArchive(t)
	state.archiveDigest = hex.EncodeToString(sha256Sum256ForTest(state.archive))
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(state.serveHTTP))
	state.server = server
	return state
}

func sha256Sum256ForTest(data []byte) []byte {
	digest := sha256.Sum256(data)
	return digest[:]
}

func (s *skillPurchaseTestServer) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	if strings.HasPrefix(request.URL.Path, "/v1/products/") ||
		strings.HasPrefix(request.URL.Path, "/v1/product-quotes") ||
		strings.HasPrefix(request.URL.Path, "/v1/orders") ||
		strings.HasPrefix(request.URL.Path, "/v1/creator-subscription-orders") ||
		strings.HasPrefix(request.URL.Path, "/v1/cli/skills/") {
		if request.Header.Get("Authorization") != "Bearer "+skillPurchaseAccessToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
	}
	switch {
	case request.URL.Path == "/v1/skills/"+downloadableProductID+"/access":
		writeJSONResponse(writer, skillAccessFixture(false, false, s.archiveDigest, s.server.URL+"/purchase"))
	case request.URL.Path == "/v1/cli/auth/status":
		writeJSONResponse(writer, map[string]any{
			"authenticated": true,
			"user":          map[string]any{"id": "33333333-3333-4333-8333-333333333333", "displayName": "Buyer", "avatarUrl": nil},
			"scopes":        []string{"profile:read", "skill-use:read"}, "expiresAt": "2027-08-27T00:00:00Z",
		})
	case request.URL.Path == "/v1/cli/skills/"+downloadableProductID+"/access":
		s.mu.Lock()
		paid := s.paymentStatus == "PAID"
		s.mu.Unlock()
		writeJSONResponse(writer, skillAccessFixture(false, paid, s.archiveDigest, s.server.URL+"/purchase"))
	case request.URL.Path == "/v1/products/"+downloadableProductID:
		writeJSONResponse(writer, map[string]any{
			"id": downloadableProductID, "slug": "paid-skill", "title": "Paid Skill",
			"summary": "Paid summary", "description": "Paid description", "usageInstructions": "Run it",
			"status": "ACTIVE", "visibility": "PUBLIC", "revision": 1,
			"salesSpec": map[string]any{
				"id": "66666666-6666-4666-8666-666666666666", "version": 1, "digest": "d", "quantity": nil,
				"skus": []map[string]any{{
					"id": skillPurchaseSkuID, "code": "standard", "title": "Standard",
					"currency": "CNY", "priceCents": 990, "status": "ACTIVE",
					"inventoryPolicy": "UNLIMITED", "attributes": map[string]any{}, "selectedOptions": map[string]string{},
				}},
			},
		})
	case request.URL.Path == "/v1/product-quotes" && request.Method == http.MethodPost:
		var body struct {
			ClientRequestID string `json:"clientRequestId"`
			SkuID           string `json:"skuId"`
			Quantity        int    `json:"quantity"`
		}
		_ = json.NewDecoder(request.Body).Decode(&body)
		if body.SkuID != skillPurchaseSkuID || body.Quantity != 1 {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writeJSONResponse(writer, map[string]any{
			"id":          skillPurchaseQuoteID,
			"product":     map[string]any{"id": downloadableProductID, "slug": "paid-skill", "title": "Paid Skill"},
			"attribution": map[string]any{"subjectWorkId": "77777777-7777-4777-8777-777777777777", "entryWorkId": nil, "commerceApplicationId": nil},
			"sku":         map[string]any{"id": skillPurchaseSkuID, "code": "standard", "title": "Standard", "selectedOptions": map[string]string{}},
			"currency":    "CNY", "unitAmountCents": 990, "quantity": 1,
			"subtotalAmountCents": 990, "shippingAmountCents": 0, "totalAmountCents": 990,
			"contractSummary": map[string]any{"publicFields": map[string]any{}, "sensitiveFieldKeys": []string{}, "assetCount": 0},
			"fulfillment":     map[string]any{"capabilities": []string{}, "estimatedState": "NONE"},
			"paymentOptions":  []map[string]any{{"provider": "WECHAT_PAY", "scenes": []string{"NATIVE"}}},
			"expiresAt":       "2027-08-27T00:00:00Z",
		})
	case request.URL.Path == "/v1/orders" && request.Method == http.MethodPost:
		s.mu.Lock()
		s.orderCreates++
		s.mu.Unlock()
		writeJSONResponse(writer, map[string]any{"order": s.orderFixture("PENDING")})
	case request.URL.Path == "/v1/orders/"+skillPurchaseOrderNo && request.Method == http.MethodGet:
		s.mu.Lock()
		s.getOrderCalls = append(s.getOrderCalls, skillPurchaseOrderNo)
		s.mu.Unlock()
		writeJSONResponse(writer, map[string]any{"order": s.orderFixture("PENDING")})
	case request.URL.Path == "/v1/orders/"+skillPurchaseOrderNo+"/status":
		s.mu.Lock()
		s.orderStatusCalls++
		// The scan payment lands once the configured poll count is reached.
		if s.orderStatusCalls >= s.paidAfterStatusCalls {
			s.paymentStatus = "PAID"
		}
		status := s.paymentStatus
		s.mu.Unlock()
		writeJSONResponse(writer, map[string]any{
			"orderNo": skillPurchaseOrderNo, "payment": map[string]any{"status": status},
			"fulfillment": nil, "serviceCase": nil,
		})
	case request.URL.Path == "/v1/cli/skills/"+downloadableProductID+"/download":
		writeJSONResponse(writer, map[string]any{
			"url": s.server.URL + "/artifact", "fileName": "paid.zip",
			"releaseId": downloadableReleaseID, "artifactDigest": s.archiveDigest, "expiresAt": "2027-08-27T00:00:00Z",
		})
	case request.URL.Path == "/artifact":
		writer.Header().Set("Content-Type", "application/zip")
		_, _ = writer.Write(s.archive)
	case request.URL.Path == "/v1/creator-subscription-orders" && request.Method == http.MethodPost:
		writeJSONResponse(writer, map[string]any{
			"order":  map[string]any{"orderNo": skillSubscriptionOrderNo, "status": "PENDING", "productId": nil, "provider": "WECHAT_PAY", "currency": "CNY", "amountCents": 2990, "expiresAt": "2027-08-27T00:00:00Z"},
			"action": map[string]any{"type": "QR_CODE", "content": "weixin://wxpay/bizpayurl?pr=sub"},
		})
	case request.URL.Path == "/v1/creator-subscription-orders/"+skillSubscriptionOrderNo:
		s.mu.Lock()
		state := s.subscriptionState
		s.mu.Unlock()
		var paidAt any
		if state == "PAID" {
			paidAt = "2026-09-01T09:00:00Z"
		}
		writeJSONResponse(writer, map[string]any{
			"order":  map[string]any{"orderNo": skillSubscriptionOrderNo, "status": state, "productId": nil, "provider": "WECHAT_PAY", "currency": "CNY", "amountCents": 2990, "expiresAt": "2027-08-27T00:00:00Z"},
			"paidAt": paidAt, "action": nil,
		})
	default:
		http.NotFound(writer, request)
	}
}

func (s *skillPurchaseTestServer) orderFixture(status string) map[string]any {
	var paidAt, closedAt any
	if status == "PAID" {
		paidAt = "2026-09-01T09:00:00Z"
	}
	return map[string]any{
		"orderNo": skillPurchaseOrderNo, "kind": "PRODUCT_PURCHASE", "status": status,
		"region": "CN", "currency": "CNY", "amountCents": 990, "paymentProvider": "WECHAT_PAY",
		"principalKind": "USER",
		"item": map[string]any{
			"id": "88888888-8888-4888-8888-888888888888", "productId": downloadableProductID,
			"productSlug": "paid-skill", "workSlug": "paid-skill", "canonicalPath": "/creator/paid-skill",
			"skuId": skillPurchaseSkuID, "merchantAccountId": "99999999-9999-4999-8999-999999999999",
			"originAuthoringTemplateCode": "downloadable-skill",
			"attribution":                 map[string]any{"subjectWorkId": "77777777-7777-4777-8777-777777777777", "entryWorkId": nil, "commerceApplicationId": nil},
			"edition":                     nil, "quantity": 1, "unitAmountCents": 990, "totalAmountCents": 990,
		},
		"paymentAction": map[string]any{"type": "QR_CODE", "content": "weixin://wxpay/bizpayurl?pr=skill"},
		"expiresAt":     "2027-08-27T00:00:00Z", "paidAt": paidAt, "closedAt": closedAt,
		"createdAt": "2026-09-01T08:00:00Z",
	}
}

func (s *skillPurchaseTestServer) setSubscriptionState(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptionState = state
}

func (s *skillPurchaseTestServer) setPaidAfterStatusCalls(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paidAfterStatusCalls = count
}

func executeSkillPurchaseCommand(t *testing.T, server *httptest.Server, home string, arguments ...string) (int, map[string]any, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exit := Execute(arguments, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(),
		HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{
			Home: home, CodexHome: filepath.Join(home, ".codex"), ConfigDir: filepath.Join(home, ".viceme-cli"),
		},
	})
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid envelope: exit=%d stdout=%q stderr=%q err=%v", exit, stdout.String(), stderr.String(), err)
	}
	return exit, envelope, stderr.String()
}

func TestPaidSkillInstallOpensWeChatQROrderAndInstallsAfterPayment(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	state := newSkillPurchaseTestServer(t)
	defer state.server.Close()

	home := t.TempDir()
	exit, envelope, stderr := executeSkillPurchaseCommand(t, state.server, home,
		"skill", "install", downloadableProductID, "--agent", "agents", "--wait", "10m",
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("paid install with QR payment failed: exit=%d envelope=%#v stderr=%q", exit, envelope, stderr)
	}
	state.mu.Lock()
	creates := state.orderCreates
	statusCalls := state.orderStatusCalls
	state.mu.Unlock()
	if creates != 1 {
		t.Fatalf("expected exactly one order creation, got %d", creates)
	}
	if statusCalls < 2 {
		t.Fatalf("payment status was not polled through the scan payment: %d", statusCalls)
	}
	if !strings.Contains(stderr, "微信支付二维码") {
		t.Fatalf("the payment QR was not announced to the user: %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "free-test", "SKILL.md")); err != nil {
		t.Fatalf("paid Skill was not installed after payment: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".viceme-cli", "skill-purchases", downloadableProductID+".json")); !os.IsNotExist(err) {
		t.Fatalf("purchase intent survived a completed payment: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(home, ".viceme-cli", "payment-presentations", "wechat-*.png"))
	if len(matches) != 0 {
		t.Fatalf("payment QR image survived a completed payment: %v", matches)
	}
}

func TestPaidSkillInstallWaitTimeoutKeepsPendingOrderForRecovery(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	state := newSkillPurchaseTestServer(t)
	defer state.server.Close()
	state.setPaidAfterStatusCalls(1000)

	home := t.TempDir()
	exit, envelope, _ := executeSkillPurchaseCommand(t, state.server, home,
		"skill", "install", downloadableProductID, "--agent", "agents", "--wait", "30ms",
	)
	if exit == 0 || envelope["ok"] != false {
		t.Fatalf("unpaid wait timeout was reported as success: exit=%d envelope=%#v", exit, envelope)
	}
	errorBody, _ := envelope["error"].(map[string]any)
	if errorBody["code"] != "SKILL_PURCHASE_PENDING" {
		t.Fatalf("unexpected timeout error: %#v", envelope)
	}
	if _, err := os.Stat(filepath.Join(home, ".viceme-cli", "skill-purchases", downloadableProductID+".json")); err != nil {
		t.Fatalf("pending purchase intent was not retained for recovery: %v", err)
	}

	// The user is still scanning; rerunning the same command must recover the
	// pending order instead of opening a duplicate one, and the in-run wait
	// observes the payment landing on the next poll.
	state.setPaidAfterStatusCalls(2)
	exit, envelope, stderr := executeSkillPurchaseCommand(t, state.server, home,
		"skill", "install", downloadableProductID, "--agent", "agents", "--wait", "10m",
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("recovered paid install failed: exit=%d envelope=%#v stderr=%q", exit, envelope, stderr)
	}
	state.mu.Lock()
	creates := state.orderCreates
	getOrderCalls := len(state.getOrderCalls)
	state.mu.Unlock()
	if creates != 1 {
		t.Fatalf("recovery opened a duplicate order: creations=%d", creates)
	}
	if getOrderCalls == 0 {
		t.Fatalf("recovery never read the pending order")
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "free-test", "SKILL.md")); err != nil {
		t.Fatalf("paid Skill was not installed after recovery: %v", err)
	}
}

func TestSubscriptionSubscribePaysWithWeChatQR(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	state := newSkillPurchaseTestServer(t)
	defer state.server.Close()

	home := t.TempDir()
	exit, envelope, _ := executeSkillPurchaseCommand(t, state.server, home,
		"subscription", "subscribe", "dogtiti", "--wait", "0",
	)
	if exit == 0 || envelope["ok"] != false {
		t.Fatalf("subscribe without waiting was reported as success: exit=%d envelope=%#v", exit, envelope)
	}
	errorBody, _ := envelope["error"].(map[string]any)
	if errorBody["code"] != "CREATOR_SUBSCRIPTION_PURCHASE_REQUIRED" {
		t.Fatalf("unexpected subscribe error: %#v", envelope)
	}

	state.setSubscriptionState("PAID")
	exit, envelope, stderr := executeSkillPurchaseCommand(t, state.server, home,
		"subscription", "subscribe", "dogtiti", "--wait", "10m",
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("subscribe with QR payment failed: exit=%d envelope=%#v stderr=%q", exit, envelope, stderr)
	}
	data, _ := envelope["data"].(map[string]any)
	if data["subscribed"] != true || data["creatorHandle"] != "dogtiti" {
		t.Fatalf("subscription result was not reported: %#v", envelope)
	}
	if !strings.Contains(stderr, "微信支付二维码") {
		t.Fatalf("the subscription QR was not announced to the user: %q", stderr)
	}
	matches, _ := filepath.Glob(filepath.Join(home, ".viceme-cli", "payment-presentations", "wechat-*.png"))
	if len(matches) != 0 {
		t.Fatalf("subscription QR image survived a completed payment: %v", matches)
	}
}
