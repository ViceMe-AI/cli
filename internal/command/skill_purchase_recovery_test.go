package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func existingSkillPurchase(t *testing.T) *skillPurchaseTestServer {
	t.Helper()
	state := newSkillPurchaseTestServer(t)
	t.Cleanup(state.server.Close)
	state.orderConflictRecovery = map[string]any{"resourceType": "ORDER", "resourceId": skillPurchaseOrderNo}
	return state
}

func readSkillPurchaseIntent(t *testing.T, home string) buyerPurchaseIntent {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(home, ".viceme-cli", "buyer-purchases", "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one durable purchase intent: %v %v", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var intent buyerPurchaseIntent
	if err := json.Unmarshal(raw, &intent); err != nil {
		t.Fatal(err)
	}
	return intent
}

func TestSkillPurchaseRecoversServerOrderWithoutLocalOrderNumber(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	state := existingSkillPurchase(t)
	home := t.TempDir()
	_, envelope, progress := executeSkillPurchaseCommand(t, state.server, home, "skill", "install", downloadableProductID, "--agent", "agents", "--wait", "0")
	errBody := envelope["error"].(map[string]any)
	if errBody["code"] != "SKILL_PURCHASE_REQUIRED" {
		t.Fatalf("existing order was not recovered: %#v", envelope)
	}
	details := errBody["details"].(map[string]any)
	if details["orderNo"] != skillPurchaseOrderNo || !strings.Contains(details["paymentUrl"].(string), "orderNo="+skillPurchaseOrderNo) || !strings.Contains(progress, details["paymentUrl"].(string)) {
		t.Fatalf("recovered payment link missing: %#v %s", details, progress)
	}
	qr := details["paymentPresentation"].(map[string]any)["imagePath"].(string)
	if data, err := os.ReadFile(qr); err != nil || len(data) < 8 || string(data[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("recovered QR is not a local PNG: %v", err)
	}
	if intent := readSkillPurchaseIntent(t, home); intent.OrderNo != skillPurchaseOrderNo {
		t.Fatalf("recovered order was not saved: %#v", intent)
	}
	// A fresh CLI invocation uses the saved order, waits for settlement, and
	// installs the paid asset without attempting a second order creation.
	exit, envelope, _ := executeSkillPurchaseCommand(t, state.server, home, "skill", "install", downloadableProductID, "--agent", "agents", "--wait", "10m")
	if exit != 0 || envelope["ok"] != true || state.orderAttempts != 1 || state.orderCreates != 0 || len(state.getOrderCalls) != 2 {
		t.Fatalf("recovery did not continue through payment/install: %#v attempts=%d creates=%d reads=%v", envelope, state.orderAttempts, state.orderCreates, state.getOrderCalls)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "free-test", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestSkillPurchaseRecoveryRejectsUnrelatedOrders(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	for _, field := range []string{"orderNo", "kind", "productId"} {
		t.Run(field, func(t *testing.T) {
			state := existingSkillPurchase(t)
			state.recoveredOrder = state.orderFixture("PENDING")
			if field == "productId" {
				state.recoveredOrder["item"].(map[string]any)[field] = "different-product"
			} else {
				state.recoveredOrder[field] = "different-value"
			}
			home := t.TempDir()
			_, envelope, progress := executeSkillPurchaseCommand(t, state.server, home, "skill", "install", downloadableProductID, "--wait", "0")
			if envelope["error"].(map[string]any)["code"] != "SKILL_PURCHASE_ORDER_MISMATCH" || readSkillPurchaseIntent(t, home).OrderNo != "" || strings.Contains(progress, "打开订单支付") {
				t.Fatalf("unrelated order was adopted: %#v %s", envelope, progress)
			}
		})
	}
}

func TestSkillPurchaseRecoveryReadFailurePreservesOriginalRequest(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	state := existingSkillPurchase(t)
	state.recoveryFailures = 1
	home := t.TempDir()
	_, envelope, _ := executeSkillPurchaseCommand(t, state.server, home, "skill", "install", downloadableProductID, "--wait", "0")
	before := readSkillPurchaseIntent(t, home)
	if envelope["error"].(map[string]any)["code"] != "CLI_TOKEN_INVALID" || before.OrderNo != "" {
		t.Fatalf("failed authenticated read was ignored: %#v", envelope)
	}
	_, envelope, _ = executeSkillPurchaseCommand(t, state.server, home, "skill", "install", downloadableProductID, "--wait", "0")
	after := readSkillPurchaseIntent(t, home)
	if envelope["error"].(map[string]any)["code"] != "SKILL_PURCHASE_REQUIRED" || string(before.OrderRequest) != string(after.OrderRequest) || after.OrderNo != skillPurchaseOrderNo || state.orderCreates != 0 {
		t.Fatalf("original request was lost after read failure: %#v", envelope)
	}
}

func TestSkillPurchaseRecoveryWithoutQRStillReturnsPaymentLink(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	state := existingSkillPurchase(t)
	state.recoveredOrder = state.orderFixture("PENDING")
	state.recoveredOrder["paymentAction"] = nil
	home := t.TempDir()
	for range 2 {
		_, envelope, progress := executeSkillPurchaseCommand(t, state.server, home, "skill", "install", downloadableProductID, "--wait", "0")
		errBody := envelope["error"].(map[string]any)
		if errBody["code"] != "SKILL_PAYMENT_QR_UNAVAILABLE" {
			t.Fatalf("missing payment action was not reported: %#v", envelope)
		}
		details := errBody["details"].(map[string]any)
		if details["orderNo"] != skillPurchaseOrderNo || !strings.Contains(details["paymentUrl"].(string), "orderNo="+skillPurchaseOrderNo) || !strings.Contains(progress, details["paymentUrl"].(string)) || readSkillPurchaseIntent(t, home).OrderNo != skillPurchaseOrderNo {
			t.Fatalf("no usable payment recovery: %#v", envelope)
		}
	}
	if state.orderAttempts != 1 || state.orderCreates != 0 {
		t.Fatal("QR failure retried order creation")
	}
}

func TestSkillPurchaseRecoveryRequiresValidReference(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	for _, ref := range []map[string]any{{}, {"resourceType": "ORDER", "resourceId": ""}, {"resourceType": "OTHER", "resourceId": skillPurchaseOrderNo}} {
		state := existingSkillPurchase(t)
		state.orderConflictRecovery = ref
		home := t.TempDir()
		_, envelope, _ := executeSkillPurchaseCommand(t, state.server, home, "skill", "install", downloadableProductID, "--wait", "0")
		if envelope["error"].(map[string]any)["code"] != "PRODUCT_PURCHASE_IN_PROGRESS" || len(state.getOrderCalls) != 0 || readSkillPurchaseIntent(t, home).OrderNo != "" {
			t.Fatalf("invalid reference was used: %#v", envelope)
		}
	}
}

func TestSkillPurchaseRecoveryDoesNotPayTerminalOrder(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	state := existingSkillPurchase(t)
	state.recoveredOrder = state.orderFixture("CLOSED")
	home := t.TempDir()
	_, envelope, progress := executeSkillPurchaseCommand(t, state.server, home, "skill", "install", downloadableProductID, "--wait", "0")
	if envelope["error"].(map[string]any)["code"] != "SKILL_PURCHASE_ORDER_CLOSED" || strings.Contains(progress, "微信支付二维码") || state.orderAttempts != 1 || readSkillPurchaseIntent(t, home).OrderNo != skillPurchaseOrderNo {
		t.Fatalf("terminal order was presented for payment: %#v", envelope)
	}
}
