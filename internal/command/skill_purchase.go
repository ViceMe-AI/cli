package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
)

// skillPurchaseIntentDirectory stores one purchase intent per downloadable
// Skill edition so an interrupted --wait recovers the same pending order
// instead of opening a duplicate one.
const skillPurchaseIntentDirectory = "skill-purchases"

type skillPurchaseIntent struct {
	ProductID      string    `json:"productId"`
	OrderNo        string    `json:"orderNo"`
	QuoteRequestID string    `json:"quoteClientRequestId"`
	OrderRequestID string    `json:"orderClientRequestId"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

func formatCentsAsYuan(cents int) string {
	return fmt.Sprintf("¥%.2f", float64(cents)/100)
}

func localeForRuntimeMarket(runtime *Runtime) string {
	if runtime.region == config.RegionGlobal {
		return "en-US"
	}
	return "zh-CN"
}

func skillPurchaseIntentPath(runtime *Runtime, productID string) string {
	return filepath.Join(runtime.configBase, skillPurchaseIntentDirectory, productID+".json")
}

func loadSkillPurchaseIntent(runtime *Runtime, productID string) (*skillPurchaseIntent, error) {
	raw, err := os.ReadFile(skillPurchaseIntentPath(runtime, productID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, output.Internal("SKILL_PURCHASE_INTENT_READ_FAILED", "could not read the local Skill purchase intent", err)
	}
	var intent skillPurchaseIntent
	if err := json.Unmarshal(raw, &intent); err != nil {
		return nil, output.Internal("SKILL_PURCHASE_INTENT_INVALID", "the local Skill purchase intent is invalid", err)
	}
	return &intent, nil
}

func saveSkillPurchaseIntent(runtime *Runtime, intent skillPurchaseIntent) error {
	directory := filepath.Join(runtime.configBase, skillPurchaseIntentDirectory)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return output.Internal("SKILL_PURCHASE_INTENT_WRITE_FAILED", "could not create the local Skill purchase intent directory", err)
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return output.Internal("SKILL_PURCHASE_INTENT_WRITE_FAILED", "could not encode the local Skill purchase intent", err)
	}
	temporary, err := os.CreateTemp(directory, ".intent-*.tmp")
	if err != nil {
		return output.Internal("SKILL_PURCHASE_INTENT_WRITE_FAILED", "could not stage the local Skill purchase intent", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(encoded); err != nil {
		temporary.Close()
		return output.Internal("SKILL_PURCHASE_INTENT_WRITE_FAILED", "could not write the local Skill purchase intent", err)
	}
	if err := temporary.Close(); err != nil {
		return output.Internal("SKILL_PURCHASE_INTENT_WRITE_FAILED", "could not close the local Skill purchase intent", err)
	}
	if err := os.Chmod(name, 0o600); err != nil {
		return output.Internal("SKILL_PURCHASE_INTENT_WRITE_FAILED", "could not secure the local Skill purchase intent", err)
	}
	if err := os.Rename(name, skillPurchaseIntentPath(runtime, intent.ProductID)); err != nil {
		return output.Internal("SKILL_PURCHASE_INTENT_WRITE_FAILED", "could not activate the local Skill purchase intent", err)
	}
	return nil
}

func clearSkillPurchaseIntent(runtime *Runtime, productID string) {
	_ = os.Remove(skillPurchaseIntentPath(runtime, productID))
}

// recoverPendingSkillPurchaseOrder returns the still-pending order recorded
// by an earlier attempt so the same QR and order number are reused. Terminal,
// expired, or unreadable intents are discarded; a nil order means a fresh
// order is due.
func recoverPendingSkillPurchaseOrder(ctx context.Context, runtime *Runtime, productID string) (*api.CommerceOrder, error) {
	intent, err := loadSkillPurchaseIntent(runtime, productID)
	if err != nil || intent == nil {
		return nil, err
	}
	order, err := runtime.client().GetOrder(ctx, intent.OrderNo)
	if err != nil {
		// A stale intent must never block a fresh purchase; order creation
		// surfaces any real connectivity problem on its own.
		clearSkillPurchaseIntent(runtime, productID)
		return nil, nil
	}
	if order.Status == "PENDING" && runtime.deps.Now().Before(parseCommercePaymentExpiry(order.ExpiresAt)) {
		return &order, nil
	}
	clearSkillPurchaseIntent(runtime, productID)
	return nil, nil
}

// createSkillPurchaseOrder opens a WeChat NATIVE order for one downloadable
// Skill edition from its active sales-spec SKU and records the local intent.
func createSkillPurchaseOrder(ctx context.Context, runtime *Runtime, productID string) (api.CommerceOrder, error) {
	product, err := runtime.client().GetProduct(ctx, productID)
	if err != nil {
		return api.CommerceOrder{}, err
	}
	var skuID string
	for _, sku := range product.SalesSpec.SKUs {
		if sku.Status == "ACTIVE" {
			skuID = sku.ID
			break
		}
	}
	if skuID == "" {
		return api.CommerceOrder{}, output.Policy(
			"SKILL_PURCHASE_SKU_UNAVAILABLE",
			"this Skill edition has no purchasable SKU right now",
		).WithDetails(map[string]any{"productId": productID})
	}
	quoteRequestID := runtime.deps.NewID()
	quoteRequest, _ := json.Marshal(map[string]any{
		"clientRequestId": quoteRequestID,
		"skuId":           skuID,
		"quantity":        1,
		"contractInput":   map[string]any{},
	})
	quote, err := runtime.client().CreateProductQuote(ctx, quoteRequest, "@stored")
	if err != nil {
		return api.CommerceOrder{}, err
	}
	orderRequestID := runtime.deps.NewID()
	orderRequest, _ := json.Marshal(map[string]any{
		"quoteId":         quote.ID,
		"clientRequestId": orderRequestID,
		"paymentProvider": "WECHAT_PAY",
		"paymentScene":    "NATIVE",
		"locale":          localeForRuntimeMarket(runtime),
	})
	created, err := runtime.client().CreateCommerceOrder(ctx, orderRequest, "@stored")
	if err != nil {
		return api.CommerceOrder{}, err
	}
	if err := saveSkillPurchaseIntent(runtime, skillPurchaseIntent{
		ProductID:      productID,
		OrderNo:        created.Order.OrderNo,
		QuoteRequestID: quoteRequestID,
		OrderRequestID: orderRequestID,
		ExpiresAt:      parseCommercePaymentExpiry(created.Order.ExpiresAt),
	}); err != nil {
		return api.CommerceOrder{}, err
	}
	return created.Order, nil
}

// presentSkillPaymentQR renders the order's WeChat NATIVE QR code as a local
// image and tells the user to scan it. The provider URI never reaches stdout.
func presentSkillPaymentQR(runtime *Runtime, order *api.CommerceOrder) (*api.CommercePaymentPresentation, error) {
	if err := prepareCommercePaymentPresentation(runtime, order); err != nil {
		return nil, err
	}
	if order.PaymentPresentation != nil {
		amount := formatCentsAsYuan(order.AmountCents)
		progress(runtime, fmt.Sprintf("微信支付二维码已生成（订单 %s，%s，%s 前有效）：%s",
			order.OrderNo, amount, order.ExpiresAt, order.PaymentPresentation.ImagePath))
		progress(runtime, "请扫码完成支付；支付到账后安装会自动继续")
	}
	return order.PaymentPresentation, nil
}

// waitForSkillOrderPayment polls the order's payment status until it is paid,
// closed, or the bounded wait deadline passes. Terminal states clear the
// local intent; a timeout leaves it in place so rerunning the same install
// command recovers this order.
func waitForSkillOrderPayment(ctx context.Context, runtime *Runtime, productID, orderNo string, timeout time.Duration) error {
	if timeout <= 0 {
		return output.Validation("SKILL_PURCHASE_WAIT_INVALID", "--wait must be positive to wait for the QR payment")
	}
	deadline := runtime.deps.Now().Add(timeout)
	for {
		status, err := runtime.client().GetCommerceOrderStatus(ctx, orderNo, "@stored")
		if err != nil {
			return err
		}
		var payment struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(status.Payment, &payment) != nil {
			return output.Internal("SKILL_PURCHASE_STATUS_INVALID", "the order payment status could not be decoded", nil)
		}
		switch payment.Status {
		case "PAID":
			clearSkillPurchaseIntent(runtime, productID)
			if err := removeCommercePaymentPresentation(runtime, orderNo); err != nil {
				return output.Internal("COMMERCE_PAYMENT_PRESENTATION_CLEANUP_FAILED", "the payment is terminal but its local QR image could not be removed", err)
			}
			return nil
		case "CLOSED":
			clearSkillPurchaseIntent(runtime, productID)
			if err := removeCommercePaymentPresentation(runtime, orderNo); err != nil {
				return output.Internal("COMMERCE_PAYMENT_PRESENTATION_CLEANUP_FAILED", "the payment is terminal but its local QR image could not be removed", err)
			}
			return output.Policy("SKILL_PURCHASE_ORDER_CLOSED", "the pending purchase order expired or was closed before payment").
				WithDetails(map[string]any{"orderNo": orderNo}).
				WithHint("rerun the same install command to open a fresh order")
		}
		if !runtime.deps.Now().Before(deadline) {
			return output.Confirmation("SKILL_PURCHASE_PENDING", "payment was not observed before the wait deadline").
				WithDetails(map[string]any{"orderNo": orderNo}).
				WithHint("after the user finishes the scan payment, rerun the same install command; the pending order is recovered instead of duplicated")
		}
		remaining := deadline.Sub(runtime.deps.Now())
		delay := 2 * time.Second
		if remaining < delay {
			delay = remaining
		}
		if err := runtime.deps.Sleep(ctx, delay); err != nil {
			return output.Network("SKILL_PURCHASE_WAIT_INTERRUPTED", "the Skill purchase wait was interrupted", err)
		}
	}
}
