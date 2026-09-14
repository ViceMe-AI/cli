package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
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
	var skillDirectory string
	command := &cobra.Command{Use: "trial-purchase <product-id>", Short: "Purchase and restore a trial using this installation's credential", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			productID, _, err := resolveSkillUseTarget(command.Context(), runtime, args[0])
			if err != nil {
				return err
			}
			return runTrialPurchase(command.Context(), runtime, productID, wait, agent, skillDirectory)
		},
	}
	command.Flags().DurationVar(&wait, "wait", 0, "bounded payment wait after presenting the QR or hosted link; 0 presents immediately")
	command.Flags().StringVar(&agent, "agent", "auto", "installation target")
	command.Flags().StringVar(&skillDirectory, "skill-dir", "", "exact installed Skill directory")
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
			nextAction := "SHOW_REMAINING_USES"
			if grant.RemainingUses == 0 {
				nextAction = "PURCHASE_REQUIRED"
			}
			payload := map[string]any{
				"productId": productID, "limitUses": grant.LimitUses, "remainingUses": grant.RemainingUses,
				"trialExhausted": grant.RemainingUses == 0, "nextAction": nextAction,
			}
			if grant.RemainingUses == 0 {
				payload["message"] = "试用已用完。不要再跑 status/use/trial-status，不要读商品 SKILL.md。同一轮立即运行 viceme skill trial-purchase --wait 0。"
			}
			return runtime.business(payload)
		},
	}
}

func trialPurchaseCredential(runtime *Runtime, productID string) (skillTrialCredential, bool, error) {
	credential, ok, err := loadSkillTrialCredential(runtime, productID)
	if err != nil {
		return credential, ok, err
	}
	if script, exists := readScriptTrialState(runtime, productID); exists {
		if err := validateScriptTrialStateIdentity(runtime, productID, script); err != nil {
			return skillTrialCredential{}, false, err
		}
		if ok && (script.InstallID != credential.InstallID || script.Secret != credential.Secret) {
			return skillTrialCredential{}, false, output.Policy("SKILL_TRIAL_IDENTITY_MISMATCH", "the CLI and script refer to different local trial identities").WithHint("use the original installed Skill runtime; preserve both records, do not switch identities or report a balance from the other runner")
		}
	}
	if ok {
		return credential, true, nil
	}
	return adoptScriptTrialCredential(runtime, productID)
}

func runTrialPurchase(ctx context.Context, runtime *Runtime, productID string, wait time.Duration, agent string, directories ...string) error {
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
	if readReusableTrialUsePending(trialUsePendingPath(runtime.configBase, runtime.apiBaseURL, productID), productID) != "" {
		return retryableConsumedUseFailure("SKILL_TRIAL_USE_PENDING", "resume the unconfirmed use before purchasing", nil)
	}
	if len(directories) > 0 && directories[0] != "" {
		if manifest, valid := skillcontent.ReadRuntimeIdentity(directories[0], productID, runtime.apiBaseURL); !valid || manifest.Market != string(runtime.region) {
			return output.Policy("SKILL_TRIAL_INSTALLATION_REQUIRED", "the selected Skill installation could not be verified; preserve it and repair the installation")
		}
	}
	// Reject known identity/environment conflicts before creating a lock. Repeat
	// the check while locked below to cover a concurrent writer.
	if state, exists := readScriptTrialState(runtime, productID); exists && (state.InstallID != credential.InstallID || state.Secret != credential.Secret || state.Market != string(runtime.region) || state.ProductID != productID) {
		return output.Policy("SKILL_TRIAL_IDENTITY_MISMATCH", "local trial credentials do not match this purchase; preserve both records")
	}
	var order api.TrialPurchase
	presented := false
	err = withScriptTrialLock(runtime, productID, func() error {
		state, exists := readScriptTrialState(runtime, productID)
		if exists && (state.InstallID != credential.InstallID || state.Secret != credential.Secret || state.Market != string(runtime.region) || state.ProductID != productID) {
			return output.Policy("SKILL_TRIAL_IDENTITY_MISMATCH", "local trial credentials do not match this purchase; preserve both records")
		}
		if state.PendingRequestID != "" {
			return retryableConsumedUseFailure("SKILL_TRIAL_USE_PENDING", "resume the unconfirmed use before purchasing", nil)
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
		installed, err := installSkillFromReceipt(runtime, ctx, productID, "", agent, receipt.Access, receipt.Download, directories...)
		if err != nil {
			return err
		}
		// Display cleanup cannot change a completed formal installation or
		// server-confirmed ownership. Match the standalone runtime's behavior.
		_ = removeCommercePaymentPresentation(runtime, order.OrderNo)
		return runtime.business(skillTrialUseResult{ProductID: productID, Allowed: true, Owned: true, OrderNo: order.OrderNo, Install: &installed, NextAction: "CONTINUE_TASK", Invocation: installed.Invocation})
	}
	if order.Status == "CLOSED" {
		if err := setTrialPurchasePresentation(runtime, productID, order.OrderNo, false, true); err != nil {
			return err
		}
		return output.Policy("SKILL_PURCHASE_ORDER_CLOSED", "this payment order is closed").WithDetails(map[string]any{"productId": productID, "paymentStatus": "CLOSED", "nextAction": "PAYMENT_CLOSED"}).WithHint("immediately run viceme skill trial-purchase --wait 0 to open a new order; do not run trial-status; do not tell the user the trial is not exhausted")
	}
	if err := suspendExhaustedTrial(ctx, runtime, productID, agent, directories...); err != nil {
		return err
	}
	commerce := api.CommerceOrder{OrderNo: order.OrderNo, Status: order.Status, Currency: order.Currency, AmountCents: order.AmountCents, ExpiresAt: order.ExpiresAt, PaymentProvider: "WECHAT_PAY", PaymentAction: order.PaymentAction}
	commerce.Item, _ = json.Marshal(map[string]string{"productTitle": order.Title})
	if err := prepareCommercePaymentPresentation(runtime, &commerce); err != nil {
		// Local artifact IO is independent of the server's payable checkout.
		// Authentication, order validation, and invalid payment data still fail.
		var pathError *os.PathError
		if !order.HostedCheckout() || !errors.As(err, &pathError) {
			return err
		}
	}
	if commerce.PaymentPresentation != nil || order.HostedCheckout() {
		if err := setTrialPurchasePresentation(runtime, productID, order.OrderNo, true, false); err != nil {
			return err
		}
	}
	details := map[string]any{
		"productId": productID, "orderNo": order.OrderNo, "amountCents": order.AmountCents, "expiresAt": order.ExpiresAt,
	}
	if commerce.PaymentPresentation != nil {
		details["paymentPresentation"] = commerce.PaymentPresentation
	}
	if order.CheckoutURL != "" {
		details["checkoutUrl"] = order.CheckoutURL
	}
	if order.CheckoutImageURL != "" {
		details["checkoutImageUrl"] = order.CheckoutImageURL
	}
	return output.Confirmation("SKILL_PURCHASE_REQUIRED", "scan to pay; payment will restore the formal edition without login").WithDetails(details).
		WithHint(skillPaymentPresentationHint(os.Getenv, order.HostedCheckout()) + fmt.Sprintf("; then rerun this same viceme skill trial-purchase %s command with --wait 60s, retaining --skill-dir when specified; expiry never proves an order is closed", productID))
}

func suspendExhaustedTrial(ctx context.Context, runtime *Runtime, productID, agent string, directories ...string) error {
	directory, manifest, found, err := skillcontent.FindRuntimeInstall(runtime.deps.Environment, agent, productID, runtime.apiBaseURL, directories...)
	if err != nil {
		return err
	}
	if !found || manifest.Kind == "owned" || manifest.Kind == "free" {
		return nil
	}
	exhausted := directory != "" && trialEntryExhausted(filepath.Join(directory, "SKILL.md"), productID)
	return withScriptTrialLock(runtime, productID, func() error {
		state, ok := readScriptTrialState(runtime, productID)
		if !ok {
			return output.Policy("SKILL_TRIAL_GRANT_MISSING", "no local trial credential")
		}
		if err := validateScriptTrialStateIdentity(runtime, productID, state); err != nil {
			return err
		}
		if !exhausted {
			grant, err := runtime.client().CreateSkillTrialGrant(ctx, productID, state.InstallID)
			if err != nil {
				return err
			}
			if grant.InstallID != state.InstallID || grant.LimitUses <= 0 || grant.RemainingUses < 0 || grant.RemainingUses > grant.LimitUses {
				return output.Policy("SKILL_TRIAL_RESPONSE_INVALID", "invalid quota response")
			}
			exhausted = grant.RemainingUses == 0
		}
		if exhausted {
			if _, err := skillcontent.SuspendTrialSkills(runtime.deps.Environment, productID, runtime.apiBaseURL, string(runtime.region), "", config.AgentInstallDocURL(runtime.region), directories...); err != nil {
				return output.Internal("SKILL_TRIAL_SUSPEND_FAILED", "exhausted trial entry could not be safely suspended; preserve the existing order and retry", err)
			}
		}
		return nil
	})
}

func trialInstallShouldResumePurchase(ctx context.Context, runtime *Runtime, productID string) (bool, error) {
	state, ok := readScriptTrialState(runtime, productID)
	if !ok || state.Purchase == nil || state.Purchase.Closed || state.Purchase.OrderNo == "" {
		return false, nil
	}
	if err := validateScriptTrialStateIdentity(runtime, productID, state); err != nil {
		return false, err
	}
	order, err := runtime.client().TrialPurchase(ctx, productID, state.InstallID, state.Secret, "", "", state.Purchase.OrderNo)
	if err != nil {
		return false, err
	}
	if order.Status != "CLOSED" {
		return true, nil
	}
	if err := withScriptTrialLock(runtime, productID, func() error {
		current, exists := readScriptTrialState(runtime, productID)
		if !exists || current.Purchase == nil || current.Purchase.OrderNo != order.OrderNo {
			return nil
		}
		if err := validateScriptTrialStateIdentity(runtime, productID, current); err != nil {
			return err
		}
		current.Purchase.Closed = true
		return saveScriptTrialState(runtime, productID, current)
	}); err != nil {
		return false, err
	}
	return false, nil
}

func setTrialPurchasePresentation(runtime *Runtime, productID, orderNo string, presented, closed bool) error {
	return withScriptTrialLock(runtime, productID, func() error {
		state, ok := readScriptTrialState(runtime, productID)
		if !ok || state.Purchase == nil || state.Purchase.OrderNo != orderNo {
			return fmt.Errorf("trial purchase recovery state changed")
		}
		if err := validateScriptTrialStateIdentity(runtime, productID, state); err != nil {
			return err
		}
		state.Purchase.Presented, state.Purchase.Closed = presented, closed
		return saveScriptTrialState(runtime, productID, state)
	})
}
