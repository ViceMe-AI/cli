package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/spf13/cobra"
)

type trialPurchaseState struct {
	ClientRequestID string `json:"clientRequestId"`
	OrderNo         string `json:"orderNo,omitempty"`
	Presented       bool   `json:"presented,omitempty"`
	Closed          bool   `json:"closed,omitempty"`
}

func sharedGuidanceURL(runtime *Runtime, path string) string {
	return strings.TrimSuffix(config.AgentInstallDocURL(runtime.region), "/start/agent-install.md") + "/skills/" + path
}

// Caller holds the shared O_EXCL product lock. Unknown script fields survive.
func saveScriptTrialState(runtime *Runtime, productID string, state scriptTrialState) error {
	fields := map[string]json.RawMessage{}
	if raw, err := os.ReadFile(scriptTrialCredentialPath(runtime, productID)); err == nil {
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	var known map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &known); err != nil {
		return err
	}
	for key, value := range known {
		fields[key] = value
	}
	encoded, err = json.Marshal(fields)
	if err != nil {
		return err
	}
	return privatefile.Write(scriptTrialCredentialPath(runtime, productID), encoded, ".trial-state-*.tmp")
}

func newSkillTrialPurchaseCommand(runtime *Runtime) *cobra.Command {
	var wait time.Duration
	var agent string
	command := &cobra.Command{Use: "trial-purchase <product-id>", Short: "Purchase and restore a trial using this installation's credential", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			productID, _, err := resolveSkillUseTarget(command.Context(), runtime, args[0])
			if err != nil {
				return err
			}
			return runTrialPurchase(command.Context(), runtime, productID, wait, agent)
		},
	}
	command.Flags().DurationVar(&wait, "wait", 0, "bounded payment wait after the QR was presented; 0 presents immediately")
	command.Flags().StringVar(&agent, "agent", "auto", "installation target")
	return command
}

func newSkillTrialStatusCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{Use: "trial-status <product-id>", Short: "Read remaining trial uses without consuming one", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			productID, _, err := resolveSkillUseTarget(command.Context(), runtime, args[0])
			if err != nil {
				return err
			}
			credential, ok, err := trialPurchaseCredential(runtime, productID)
			if err != nil {
				return err
			}
			if !ok {
				return output.Policy("SKILL_TRIAL_GRANT_MISSING", "no local trial credential")
			}
			grant, err := runtime.client().CreateSkillTrialGrant(command.Context(), productID, credential.InstallID)
			if err != nil {
				return err
			}
			if grant.InstallID != credential.InstallID || grant.LimitUses <= 0 || grant.RemainingUses < 0 || grant.RemainingUses > grant.LimitUses {
				return output.Policy("SKILL_TRIAL_RESPONSE_INVALID", "invalid quota response")
			}
			return runtime.business(map[string]any{"productId": productID, "limitUses": grant.LimitUses, "remainingUses": grant.RemainingUses, "nextAction": "SHOW_REMAINING_USES"})
		},
	}
}

func trialPurchaseCredential(runtime *Runtime, productID string) (skillTrialCredential, bool, error) {
	credential, ok, err := loadSkillTrialCredential(runtime, productID)
	if err != nil || ok {
		return credential, ok, err
	}
	return adoptScriptTrialCredential(runtime, productID)
}

func runTrialPurchase(ctx context.Context, runtime *Runtime, productID string, wait time.Duration, agent string) error {
	if wait < 0 || wait > 10*time.Minute {
		return output.Validation("SKILL_PURCHASE_WAIT_INVALID", "--wait must be between 0 and 10m")
	}
	credential, ok, err := trialPurchaseCredential(runtime, productID)
	if err != nil {
		return err
	}
	if !ok {
		return output.Policy("SKILL_TRIAL_GRANT_MISSING", "no local trial credential for this purchase")
	}
	var order api.TrialPurchase
	presented := false
	err = withScriptTrialLock(runtime, productID, func() error {
		state, exists := readScriptTrialState(runtime, productID)
		if exists && (state.InstallID != credential.InstallID || state.Secret != credential.Secret || state.Market != string(runtime.region) || state.ProductID != productID) {
			return output.Policy("SKILL_TRIAL_IDENTITY_MISMATCH", "local trial credentials do not match this purchase; preserve both records")
		}
		if !exists {
			state = scriptTrialState{InstallID: credential.InstallID, Secret: credential.Secret, ProductID: productID, Market: string(runtime.region)}
		}
		if state.Purchase == nil || state.Purchase.Closed {
			state.Purchase = &trialPurchaseState{ClientRequestID: runtime.deps.NewID()}
		}
		if err := saveScriptTrialState(runtime, productID, state); err != nil {
			return err
		}
		presented = state.Purchase.Presented
		var err error
		order, err = runtime.client().TrialPurchase(ctx, productID, credential.InstallID, credential.Secret, state.Purchase.ClientRequestID, localeForRuntimeMarket(runtime), state.Purchase.OrderNo)
		if err != nil {
			return err
		}
		if expiry, parseErr := time.Parse(time.RFC3339, order.ExpiresAt); state.Purchase.OrderNo != "" && order.Status == "PENDING" && (len(order.PaymentAction) == 0 || string(order.PaymentAction) == "null") && parseErr == nil && expiry.After(runtime.deps.Now()) {
			order, err = runtime.client().TrialPurchase(ctx, productID, credential.InstallID, credential.Secret, state.Purchase.ClientRequestID, localeForRuntimeMarket(runtime), "")
			if err != nil {
				return err
			}
			if order.OrderNo != state.Purchase.OrderNo {
				return output.Policy("SKILL_PURCHASE_RESPONSE_INVALID", "payment recovery returned a different order")
			}
		}
		state.Purchase.OrderNo = order.OrderNo
		state.Purchase.Closed = order.Status == "CLOSED"
		return saveScriptTrialState(runtime, productID, state)
	})
	if err != nil {
		return err
	}
	deadline := runtime.deps.Now().Add(wait)
	for presented && order.Status == "PENDING" && runtime.deps.Now().Before(deadline) {
		delay := min(3*time.Second, deadline.Sub(runtime.deps.Now()))
		if err := runtime.deps.Sleep(ctx, delay); err != nil {
			return err
		}
		order, err = runtime.client().TrialPurchase(ctx, productID, credential.InstallID, credential.Secret, "", "", order.OrderNo)
		if err != nil {
			return err
		}
	}
	if order.Status == "PAID" {
		receipt, err := runtime.client().TrialOwnedSkillDownload(ctx, productID, credential.InstallID, credential.Secret)
		if err != nil {
			return err
		}
		installed, err := installSkillFromReceipt(runtime, ctx, productID, "", agent, receipt.Access, receipt.Download)
		if err != nil {
			return err
		}
		if err := removeCommercePaymentPresentation(runtime, order.OrderNo); err != nil {
			return err
		}
		return runtime.business(skillTrialUseResult{ProductID: productID, Allowed: true, Owned: true, OrderNo: order.OrderNo, Install: &installed, NextAction: "CONTINUE_TASK", Invocation: installed.Invocation})
	}
	if order.Status == "CLOSED" {
		if err := setTrialPurchasePresentation(runtime, productID, order.OrderNo, false, true); err != nil {
			return err
		}
		return output.Policy("SKILL_PURCHASE_ORDER_CLOSED", "the payment order was closed; rerun trial-purchase to request another order")
	}
	commerce := api.CommerceOrder{OrderNo: order.OrderNo, Status: order.Status, Currency: order.Currency, AmountCents: order.AmountCents, ExpiresAt: order.ExpiresAt, PaymentProvider: "WECHAT_PAY", PaymentAction: order.PaymentAction}
	commerce.Item, _ = json.Marshal(map[string]string{"productTitle": order.Title})
	if err := prepareCommercePaymentPresentation(runtime, &commerce); err != nil {
		return err
	}
	if commerce.PaymentPresentation != nil {
		if err := setTrialPurchasePresentation(runtime, productID, order.OrderNo, true, false); err != nil {
			return err
		}
	}
	return output.Confirmation("SKILL_PURCHASE_REQUIRED", "scan to pay; payment will restore the formal edition without login").WithDetails(map[string]any{
		"productId": productID, "orderNo": order.OrderNo, "amountCents": order.AmountCents, "expiresAt": order.ExpiresAt,
		"paymentPresentation": commerce.PaymentPresentation,
	}).WithHint(fmt.Sprintf("read and render paymentPresentation.widgetPath with the host widget tool before waiting; then run viceme skill trial-purchase %s --wait 60s; expiry never proves an order is closed", productID))
}

func setTrialPurchasePresentation(runtime *Runtime, productID, orderNo string, presented, closed bool) error {
	return withScriptTrialLock(runtime, productID, func() error {
		state, ok := readScriptTrialState(runtime, productID)
		if !ok || state.Purchase == nil || state.Purchase.OrderNo != orderNo {
			return fmt.Errorf("trial purchase recovery state changed")
		}
		state.Purchase.Presented, state.Purchase.Closed = presented, closed
		return saveScriptTrialState(runtime, productID, state)
	})
}
