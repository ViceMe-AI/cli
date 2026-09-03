package command

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
)

func formatCentsAsYuan(cents int) string { return fmt.Sprintf("¥%.2f", float64(cents)/100) }

// The storefront's checkout recovery route resumes this exact buyer order.
func skillOrderPaymentURL(runtime *Runtime, orderNo string) string {
	base := runtime.profile.ResolvedWebBaseURL()
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/checkout?" + url.Values{"orderNo": {orderNo}}.Encode()
}

func localeForRuntimeMarket(runtime *Runtime) string {
	if runtime.region == config.RegionGlobal {
		return "en-US"
	}
	return "zh-CN"
}

func openSkillPurchaseOrder(ctx context.Context, runtime *Runtime, productID string) (api.CommerceOrder, error) {
	unlock, err := lockBuyerPurchase(ctx, runtime, "skill", productID)
	if err != nil {
		return api.CommerceOrder{}, err
	}
	defer unlock()
	intent, err := loadBuyerPurchaseIntent(runtime, "skill", productID)
	if err != nil {
		return api.CommerceOrder{}, err
	}
	if intent != nil && intent.OrderNo != "" {
		order, err := runtime.client().GetOrder(ctx, intent.OrderNo)
		if err != nil {
			return api.CommerceOrder{}, err
		} // Preserve state on authentication/network failures.
		if order.Status == "PENDING" || order.Status == "PAID" {
			return order, nil
		}
		if err := os.Remove(buyerPurchaseIntentPath(runtime, "skill", productID)); err != nil {
			return api.CommerceOrder{}, err
		}
		intent = nil
	}
	if intent == nil {
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
			return api.CommerceOrder{}, output.Policy("SKILL_PURCHASE_SKU_UNAVAILABLE", "this Skill edition has no purchasable SKU right now")
		}
		quoteRequest, _ := json.Marshal(map[string]any{"clientRequestId": runtime.deps.NewID(), "skuId": skuID, "quantity": 1, "contractInput": map[string]any{}})
		intent = &buyerPurchaseIntent{QuoteRequest: quoteRequest}
		if err := saveBuyerPurchaseIntent(runtime, "skill", productID, *intent); err != nil {
			return api.CommerceOrder{}, err
		}
	}
	if len(intent.OrderRequest) == 0 {
		quote, err := runtime.client().CreateBuyerQuote(ctx, intent.QuoteRequest)
		if err != nil {
			return api.CommerceOrder{}, err
		}
		intent.OrderRequest, _ = json.Marshal(map[string]any{"quoteId": quote.ID, "clientRequestId": runtime.deps.NewID(), "paymentProvider": "WECHAT_PAY", "paymentScene": "NATIVE", "locale": localeForRuntimeMarket(runtime)})
		if err := saveBuyerPurchaseIntent(runtime, "skill", productID, *intent); err != nil {
			return api.CommerceOrder{}, err
		}
	}
	created, err := runtime.client().CreateBuyerOrder(ctx, intent.OrderRequest)
	if err != nil {
		// This definitive rejection is returned only when no order exists for the saved request.
		if output.AsError(err).Subtype == "QUOTE_EXPIRED" {
			if removeErr := os.Remove(buyerPurchaseIntentPath(runtime, "skill", productID)); removeErr != nil {
				return api.CommerceOrder{}, removeErr
			}
		}
		return api.CommerceOrder{}, err
	}
	intent.OrderNo = created.Order.OrderNo
	if err := saveBuyerPurchaseIntent(runtime, "skill", productID, *intent); err != nil {
		return api.CommerceOrder{}, err
	}
	return created.Order, nil
}

// presentSkillPaymentQR renders the order's WeChat NATIVE QR code as a local
// image and tells the user to scan it. The provider URI never reaches stdout.
func presentSkillPaymentQR(runtime *Runtime, order *api.CommerceOrder) (*api.CommercePaymentPresentation, error) {
	if order.Status != "PENDING" {
		return nil, nil
	}
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
		status, err := runtime.client().GetBuyerOrderStatus(ctx, orderNo)
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
			if err := completeBuyerPurchase(ctx, runtime, "skill", productID, orderNo); err != nil {
				return err
			}
			if err := removeCommercePaymentPresentation(runtime, orderNo); err != nil {
				return output.Internal("COMMERCE_PAYMENT_PRESENTATION_CLEANUP_FAILED", "the payment is terminal but its local QR image could not be removed", err)
			}
			return nil
		case "CLOSED":
			if err := completeBuyerPurchase(ctx, runtime, "skill", productID, orderNo); err != nil {
				return err
			}
			if err := removeCommercePaymentPresentation(runtime, orderNo); err != nil {
				return output.Internal("COMMERCE_PAYMENT_PRESENTATION_CLEANUP_FAILED", "the payment is terminal but its local QR image could not be removed", err)
			}
			return output.Policy("SKILL_PURCHASE_ORDER_CLOSED", "the pending purchase order expired or was closed before payment").
				WithDetails(map[string]any{"orderNo": orderNo}).
				WithHint("rerun the same install command to open a fresh order")
		}
		if !runtime.deps.Now().Before(deadline) {
			return output.Confirmation("SKILL_PURCHASE_PENDING", "payment was not observed before the wait deadline").
				WithDetails(map[string]any{"orderNo": orderNo, "paymentUrl": skillOrderPaymentURL(runtime, orderNo)}).
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
