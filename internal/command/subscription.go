package command

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

const creatorSubscriptionPriceMinorMax = 10_000_000

// newSubscriptionCommand 暴露订阅两侧：创作者侧管理粉丝订阅计划（show/set/off），
// 买家侧 subscribe 打开一笔微信 Native 订阅订单并等待扫码支付完成。
func newSubscriptionCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "subscription", Short: "Manage fan subscriptions and subscribe to a creator"}
	command.AddCommand(newSubscriptionShowCommand(runtime))
	command.AddCommand(newSubscriptionSetCommand(runtime))
	command.AddCommand(newSubscriptionOffCommand(runtime))
	command.AddCommand(newSubscriptionSubscribeCommand(runtime))
	return command
}

func newSubscriptionSubscribeCommand(runtime *Runtime) *cobra.Command {
	var wait time.Duration
	command := &cobra.Command{
		Use:   "subscribe <creator-handle>",
		Short: "Subscribe to a creator with a WeChat QR payment and unlock every paid Skill of theirs",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireBuyerAuthentication(command.Context()); err != nil {
				return err
			}
			creatorHandle := args[0]
			order, presentation, err := openCreatorSubscriptionOrder(command.Context(), runtime, creatorHandle)
			if err != nil {
				return err
			}
			if wait <= 0 && order.Status != "PAID" {
				return output.Confirmation("CREATOR_SUBSCRIPTION_PURCHASE_REQUIRED", "complete the subscription payment before it expires").
					WithDetails(map[string]any{
						"creatorHandle": creatorHandle, "orderNo": order.OrderNo,
						"amountCents": order.AmountCents, "expiresAt": order.ExpiresAt,
						"paymentPresentation": presentation,
					}).
					WithHint("present the payment QR to the user, then rerun the same subscribe command with --wait while the payment is in progress")
			}
			if wait <= 0 {
				wait = time.Second
			}
			if err := waitForCreatorSubscriptionPayment(command.Context(), runtime, creatorHandle, order.OrderNo, wait); err != nil {
				return err
			}
			return runtime.business(map[string]any{
				"subscribed": true, "creatorHandle": creatorHandle,
				"orderNo": order.OrderNo, "amountCents": order.AmountCents,
				"nextAction": "EVERY_PAID_SKILL_OF_THIS_CREATOR_IS_NOW_UNLOCKED",
			})
		},
	}
	command.Flags().DurationVar(&wait, "wait", 5*time.Minute, "wait up to this duration for the WeChat QR payment; 0 presents the QR without waiting")
	return command
}

// openCreatorSubscriptionOrder opens a WeChat NATIVE subscription order and
// renders its payment QR as a local image; the provider URI never reaches
// stdout.
func openCreatorSubscriptionOrder(ctx context.Context, runtime *Runtime, creatorHandle string) (api.PaymentOrder, *api.CommercePaymentPresentation, error) {
	unlock, err := lockBuyerPurchase(ctx, runtime, "subscription", creatorHandle)
	if err != nil {
		return api.PaymentOrder{}, nil, err
	}
	defer unlock()
	intent, err := loadBuyerPurchaseIntent(runtime, "subscription", creatorHandle)
	if err != nil {
		return api.PaymentOrder{}, nil, err
	}
	if intent != nil && intent.OrderNo != "" {
		current, err := runtime.client().GetCreatorSubscriptionOrderStatus(ctx, intent.OrderNo)
		if err != nil {
			return api.PaymentOrder{}, nil, err
		}
		if current.Order.Status == "PAID" {
			return current.Order, nil, nil
		}
		if current.Order.Status == "CLOSED" || current.Order.Status == "CANCELLED" {
			if err := os.Remove(buyerPurchaseIntentPath(runtime, "subscription", creatorHandle)); err != nil {
				return api.PaymentOrder{}, nil, err
			}
			intent = nil
		}
	}
	if intent == nil {
		request, _ := json.Marshal(map[string]any{"creatorHandle": creatorHandle, "clientRequestId": runtime.deps.NewID(), "paymentProvider": "WECHAT_PAY", "paymentScene": "NATIVE", "locale": localeForRuntimeMarket(runtime)})
		intent = &buyerPurchaseIntent{OrderRequest: request}
		if err := saveBuyerPurchaseIntent(runtime, "subscription", creatorHandle, *intent); err != nil {
			return api.PaymentOrder{}, nil, err
		}
	}
	created, err := runtime.client().CreateCreatorSubscriptionOrder(ctx, intent.OrderRequest)
	if err != nil {
		return api.PaymentOrder{}, nil, err
	}
	intent.OrderNo = created.Order.OrderNo
	if err := saveBuyerPurchaseIntent(runtime, "subscription", creatorHandle, *intent); err != nil {
		return api.PaymentOrder{}, nil, err
	}
	order := api.CommerceOrder{
		OrderNo:         created.Order.OrderNo,
		Status:          created.Order.Status,
		AmountCents:     created.Order.AmountCents,
		PaymentProvider: created.Order.Provider,
		PaymentAction:   created.Action,
		ExpiresAt:       created.Order.ExpiresAt,
	}
	if err := prepareCommercePaymentPresentation(runtime, &order); err != nil {
		return created.Order, nil, err
	}
	if order.PaymentPresentation != nil {
		amount := formatCentsAsYuan(created.Order.AmountCents)
		progress(runtime, "微信支付二维码已生成（订阅订单 "+created.Order.OrderNo+"，"+amount+"，"+created.Order.ExpiresAt+" 前有效）："+order.PaymentPresentation.ImagePath)
		progress(runtime, "请扫码完成支付；支付到账后订阅生效，该创作者全部付费 Skill 解锁")
	}
	return created.Order, order.PaymentPresentation, nil
}

func waitForCreatorSubscriptionPayment(ctx context.Context, runtime *Runtime, creatorHandle, orderNo string, timeout time.Duration) error {
	if timeout <= 0 {
		return output.Validation("CREATOR_SUBSCRIPTION_WAIT_INVALID", "--wait must be positive to wait for the QR payment")
	}
	deadline := runtime.deps.Now().Add(timeout)
	for {
		status, err := runtime.client().GetCreatorSubscriptionOrderStatus(ctx, orderNo)
		if err != nil {
			return err
		}
		switch status.Order.Status {
		case "PAID":
			if err := completeBuyerPurchase(ctx, runtime, "subscription", creatorHandle, orderNo); err != nil {
				return err
			}
			if err := removeCommercePaymentPresentation(runtime, orderNo); err != nil {
				return output.Internal("COMMERCE_PAYMENT_PRESENTATION_CLEANUP_FAILED", "the payment is terminal but its local QR image could not be removed", err)
			}
			return nil
		case "CLOSED", "CANCELLED":
			if err := completeBuyerPurchase(ctx, runtime, "subscription", creatorHandle, orderNo); err != nil {
				return err
			}
			if err := removeCommercePaymentPresentation(runtime, orderNo); err != nil {
				return output.Internal("COMMERCE_PAYMENT_PRESENTATION_CLEANUP_FAILED", "the payment is terminal but its local QR image could not be removed", err)
			}
			return output.Policy("CREATOR_SUBSCRIPTION_ORDER_CLOSED", "the pending subscription order expired or was closed before payment").
				WithDetails(map[string]any{"orderNo": orderNo}).
				WithHint("rerun the same subscribe command to open a fresh order")
		}
		if !runtime.deps.Now().Before(deadline) {
			return output.Confirmation("CREATOR_SUBSCRIPTION_PURCHASE_PENDING", "payment was not observed before the wait deadline").
				WithDetails(map[string]any{"orderNo": orderNo}).
				WithHint("after the user finishes the scan payment, rerun the same subscribe command")
		}
		remaining := deadline.Sub(runtime.deps.Now())
		delay := 2 * time.Second
		if remaining < delay {
			delay = remaining
		}
		if err := runtime.deps.Sleep(ctx, delay); err != nil {
			return output.Network("CREATOR_SUBSCRIPTION_WAIT_INTERRUPTED", "the subscription payment wait was interrupted", err)
		}
	}
}

func newSubscriptionShowCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "show",
		Short: "Show the current fan subscription plan",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantAccountID)
			if err != nil {
				return err
			}
			plan, err := runtime.client().GetCreatorSubscriptionPlan(command.Context(), merchant.ID)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{
				"creatorHandle":         plan.CreatorHandle,
				"displayName":           plan.DisplayName,
				"priceMinor":            plan.PriceMinor,
				"periodDays":            plan.PeriodDays,
				"status":                plan.Status,
				"activeSubscriberCount": plan.ActiveSubscriberCount,
				"updatedAt":             plan.UpdatedAt,
			})
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account id; defaults to the only active merchant")
	return command
}

func newSubscriptionSetCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	var priceMinor int
	command := &cobra.Command{
		Use:   "set",
		Short: "Create or update the fan subscription monthly price",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if priceMinor <= 0 || priceMinor > creatorSubscriptionPriceMinorMax {
				return output.Validation("CREATOR_SUBSCRIPTION_PRICE_INVALID", "--price-minor must be between 1 and 10000000 (CNY cents)")
			}
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantAccountID)
			if err != nil {
				return err
			}
			plan, err := runtime.client().SetCreatorSubscriptionPlan(command.Context(), merchant.ID, priceMinor)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{
				"creatorHandle":         plan.CreatorHandle,
				"displayName":           plan.DisplayName,
				"priceMinor":            plan.PriceMinor,
				"periodDays":            plan.PeriodDays,
				"status":                plan.Status,
				"activeSubscriberCount": plan.ActiveSubscriberCount,
				"updatedAt":             plan.UpdatedAt,
			})
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account id; defaults to the only active merchant")
	command.Flags().IntVar(&priceMinor, "price-minor", 0, "monthly price in CNY cents (1 = ¥0.01)")
	_ = command.MarkFlagRequired("price-minor")
	return command
}

func newSubscriptionOffCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "off",
		Short: "Disable the fan subscription plan; existing subscribers keep their paid period",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantAccountID)
			if err != nil {
				return err
			}
			plan, err := runtime.client().DisableCreatorSubscriptionPlan(command.Context(), merchant.ID)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{
				"creatorHandle":         plan.CreatorHandle,
				"displayName":           plan.DisplayName,
				"priceMinor":            plan.PriceMinor,
				"periodDays":            plan.PeriodDays,
				"status":                plan.Status,
				"activeSubscriberCount": plan.ActiveSubscriberCount,
				"updatedAt":             plan.UpdatedAt,
			})
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account id; defaults to the only active merchant")
	return command
}
