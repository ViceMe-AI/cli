package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillPurchaseRefreshesStaleSavedQuote(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	for _, code := range []string{"QUOTE_NOT_FOUND", "QUOTE_EXPIRED"} {
		t.Run(code, func(t *testing.T) {
			state := newSkillPurchaseTestServer(t)
			defer state.server.Close()
			home := t.TempDir()
			// Simulate an earlier interrupted purchase with a saved request but
			// no known order. A network error must preserve that request.
			state.orderQuoteErrors = []string{"SERVICE_UNAVAILABLE"}
			_, envelope, _ := executeSkillPurchaseCommand(t, state.server, home, "skill", "install", downloadableProductID, "--wait", "0")
			if envelope["error"].(map[string]any)["code"] != "SERVICE_UNAVAILABLE" || state.orderAttempts != 1 {
				t.Fatalf("network error refreshed or lost the request: %#v", envelope)
			}
			original := readSkillPurchaseIntent(t, home)
			state.orderQuoteErrors = []string{code}
			_, envelope, progress := executeSkillPurchaseCommand(t, state.server, home, "skill", "install", downloadableProductID, "--wait", "0")
			if envelope["error"].(map[string]any)["code"] != "SKILL_PURCHASE_REQUIRED" || state.orderAttempts != 3 || state.orderCreates != 1 {
				t.Fatalf("stale quote did not recover in the same invocation: %#v attempts=%d creates=%d", envelope, state.orderAttempts, state.orderCreates)
			}
			if state.orderInputs[0]["clientRequestId"] != state.orderInputs[1]["clientRequestId"] || state.orderInputs[0]["quoteId"] != state.orderInputs[1]["quoteId"] {
				t.Fatal("saved request was not replayed before refreshing")
			}
			if len(state.quoteRequestIDs) != 2 || state.quoteRequestIDs[0] == state.quoteRequestIDs[1] || state.orderInputs[1]["clientRequestId"] == state.orderInputs[2]["clientRequestId"] || state.orderInputs[1]["quoteId"] == state.orderInputs[2]["quoteId"] {
				t.Fatal("refresh reused the rejected quote or idempotency key")
			}
			refreshed := readSkillPurchaseIntent(t, home)
			if refreshed.OrderNo != skillPurchaseOrderNo || string(original.OrderRequest) == string(refreshed.OrderRequest) {
				t.Fatal("refreshed order was not durably saved")
			}
			details := envelope["error"].(map[string]any)["details"].(map[string]any)
			if !strings.Contains(details["paymentUrl"].(string), skillPurchaseOrderNo) || !strings.Contains(progress, "微信支付二维码") {
				t.Fatalf("payment presentation missing: %#v", envelope)
			}
			exit, envelope, _ := executeSkillPurchaseCommand(t, state.server, home, "skill", "install", downloadableProductID, "--agent", "agents", "--wait", "10m")
			if exit != 0 || state.orderAttempts != 3 {
				t.Fatalf("payment/install failed or created another order: %#v", envelope)
			}
			if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "free-test", "SKILL.md")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSkillPurchaseQuoteRefreshIsBounded(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	state := newSkillPurchaseTestServer(t)
	defer state.server.Close()
	state.orderQuoteErrors = []string{"QUOTE_NOT_FOUND", "QUOTE_EXPIRED", "QUOTE_NOT_FOUND"}
	home := t.TempDir()
	_, envelope, _ := executeSkillPurchaseCommand(t, state.server, home, "skill", "install", downloadableProductID, "--wait", "0")
	if envelope["error"].(map[string]any)["code"] != "QUOTE_EXPIRED" || state.orderAttempts != 2 || len(state.quoteRequestIDs) != 2 || state.orderCreates != 0 {
		t.Fatalf("refresh loop was not bounded: %#v attempts=%d", envelope, state.orderAttempts)
	}
	files, _ := filepath.Glob(filepath.Join(home, ".viceme-cli", "buyer-purchases", "*.json"))
	if len(files) != 0 {
		t.Fatal("definitively rejected quote still persisted")
	}
}

func TestSkillPurchaseQuoteRefreshCanRecoverExistingOrder(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	state := existingSkillPurchase(t)
	state.orderQuoteErrors = []string{"QUOTE_NOT_FOUND"}
	_, envelope, _ := executeSkillPurchaseCommand(t, state.server, t.TempDir(), "skill", "install", downloadableProductID, "--wait", "0")
	if envelope["error"].(map[string]any)["code"] != "SKILL_PURCHASE_REQUIRED" || state.orderAttempts != 2 || state.orderCreates != 0 || len(state.getOrderCalls) != 1 {
		t.Fatalf("fresh quote failed to recover the existing reservation: %#v", envelope)
	}
}

func TestSkillPurchaseDoesNotRefreshAmbiguousOrAuthorizationErrors(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	for _, code := range []string{"SERVICE_UNAVAILABLE", "QUOTE_PRINCIPAL_MISMATCH", "QUOTE_SESSION_MISMATCH", "ORDER_IDEMPOTENCY_CONFLICT"} {
		t.Run(code, func(t *testing.T) {
			state := newSkillPurchaseTestServer(t)
			defer state.server.Close()
			state.orderQuoteErrors = []string{code}
			home := t.TempDir()
			_, envelope, _ := executeSkillPurchaseCommand(t, state.server, home, "skill", "install", downloadableProductID, "--wait", "0")
			if envelope["error"].(map[string]any)["code"] != code || state.orderAttempts != 1 || len(state.quoteRequestIDs) != 1 || len(readSkillPurchaseIntent(t, home).OrderRequest) == 0 {
				t.Fatalf("unsafe quote refresh: %#v", envelope)
			}
		})
	}
}
