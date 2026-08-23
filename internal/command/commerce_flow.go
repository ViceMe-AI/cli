package command

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	commerceFlowSelectProduct      = "SELECT_PRODUCT"
	commerceFlowCollectInput       = "COLLECT_INPUT"
	commerceFlowConfirmPayment     = "CONFIRM_PAYMENT"
	commerceFlowPresentPaymentQR   = "PRESENT_PAYMENT_QR"
	commerceFlowWaitFulfillment    = "WAIT_FULFILLMENT"
	commerceFlowPaymentPending     = "PAYMENT_PENDING"
	commerceFlowPaymentClosed      = "PAYMENT_CLOSED"
	commerceFlowFulfillmentPending = "FULFILLMENT_PENDING"
	commerceFlowCompleted          = "COMPLETED"
)

type commerceFlowStartResult struct {
	NextAction         string                     `json:"nextAction"`
	Products           []api.PurchaseSkillProduct `json:"products,omitempty"`
	Session            *commerceSessionResult     `json:"session,omitempty"`
	Product            *api.CommerceProduct       `json:"product,omitempty"`
	ContractInputGuide map[string]any             `json:"contractInputGuide,omitempty"`
}

type commerceFlowQuoteResult struct {
	NextAction string           `json:"nextAction"`
	Quote      api.ProductQuote `json:"quote"`
}

type commerceFlowConfirmResult struct {
	NextAction string            `json:"nextAction"`
	Order      api.CommerceOrder `json:"order"`
}

type commerceFlowWaitResult struct {
	NextAction string                  `json:"nextAction"`
	Status     api.OrderStatusResponse `json:"status"`
}

func newCommerceFlowCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "flow",
		Short: "Run one deterministic Commerce step per Agent turn",
	}
	command.AddCommand(newCommerceFlowStartCommand(runtime))
	command.AddCommand(newCommerceFlowQuoteCommand(runtime))
	command.AddCommand(newCommerceFlowConfirmCommand(runtime))
	command.AddCommand(newCommerceFlowWaitCommand(runtime))
	return command
}

func newCommerceFlowStartCommand(runtime *Runtime) *cobra.Command {
	var stableName, productID, locale string
	command := &cobra.Command{
		Use:   "start",
		Short: "Discover, bind, and describe the Product in one Agent turn",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if !validStableName(stableName) {
				return output.Validation("COMMERCE_SKILL_NAME_INVALID", "purchase Skill stable name is invalid")
			}
			if locale != "zh-CN" && locale != "en-US" {
				return output.Validation("COMMERCE_LOCALE_INVALID", "--locale must be zh-CN or en-US")
			}
			localContextID := strings.TrimSpace(runtime.commerceContextID)
			if localContextID == "" {
				localContextID = runtime.deps.NewID()
			}
			if !validCommerceContextID(localContextID) {
				return output.Validation("COMMERCE_SESSION_CONTEXT_INVALID", "--session-context must be an opaque localContextId")
			}
			state, recovered, err := runtime.startCommerceSession(command.Context(), localContextID, stableName, productID)
			if err != nil {
				if output.AsError(err).Subtype != "PRODUCT_PURCHASE_SKILL_PRODUCT_SELECTION_REQUIRED" || productID != "" {
					return err
				}
				if deleteErr := runtime.deleteCommerceSessionStartIntent(localContextID, stableName); deleteErr != nil {
					return deleteErr
				}
				descriptor, descriptorErr := runtime.client().GetProductPurchaseSkill(command.Context(), stableName)
				if descriptorErr != nil {
					return descriptorErr
				}
				return runtime.business(commerceFlowStartResult{
					NextAction: commerceFlowSelectProduct,
					Products:   descriptor.Products,
				})
			}
			product, err := runtime.client().GetCommerceProduct(command.Context(), state.ProductID, locale, state.Token)
			if err != nil {
				return err
			}
			session := sessionResult(state, recovered)
			return runtime.business(commerceFlowStartResult{
				NextAction:         commerceFlowCollectInput,
				Session:            &session,
				Product:            &product,
				ContractInputGuide: commerceContractInputGuide(product.SalesSpec.BuyerContract),
			})
		},
	}
	command.Flags().StringVar(&stableName, "skill", "", "purchase Skill stable name")
	command.Flags().StringVar(&productID, "product", "", "eligible Product id when SELECT_PRODUCT was returned")
	command.Flags().StringVar(&locale, "locale", "zh-CN", "localized Product presentation")
	_ = command.MarkFlagRequired("skill")
	return command
}

func commerceContractInputGuide(raw json.RawMessage) map[string]any {
	var contract struct {
		Fields []struct {
			Key  string `json:"key"`
			Kind string `json:"kind"`
		} `json:"fields"`
	}
	if json.Unmarshal(raw, &contract) != nil {
		return nil
	}
	guide := make(map[string]any, len(contract.Fields))
	for _, field := range contract.Fields {
		switch field.Kind {
		case "number":
			guide[field.Key] = map[string]any{"kind": field.Kind, "jsonType": "number"}
		case "boolean":
			guide[field.Key] = map[string]any{"kind": field.Kind, "jsonType": "boolean"}
		case "address":
			guide[field.Key] = map[string]any{
				"kind": field.Kind,
				"valueShape": map[string]string{
					"countryCode": "required ISO 3166-1 alpha-2 string",
					"region":      "required string",
					"city":        "required string",
					"district":    "optional string",
					"postalCode":  "optional string",
					"line1":       "required string",
					"line2":       "optional string",
				},
			}
		case "image", "file":
			guide[field.Key] = map[string]any{
				"kind":  field.Kind,
				"input": "pass each local file with --asset '<field-key>=<local-path>'; do not put the path in JSON",
			}
		default:
			guide[field.Key] = map[string]any{"kind": field.Kind, "jsonType": "string"}
		}
	}
	return guide
}

func newCommerceFlowQuoteCommand(runtime *Runtime) *cobra.Command {
	var stableName, skuID, contractInputJSON string
	var assets []string
	var quantity int
	command := &cobra.Command{
		Use:   "quote",
		Short: "Create the exact Quote from buyer fields in one Agent turn",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if quantity <= 0 {
				return output.Validation("COMMERCE_QUOTE_INPUT_INVALID", "--quantity must be positive")
			}
			contractInput, err := parseInlineJSONObject(contractInputJSON, "COMMERCE_QUOTE_INPUT_INVALID")
			if err != nil {
				return err
			}
			contractInput, err = uploadCommerceFlowAssets(command.Context(), runtime, stableName, contractInput, assets)
			if err != nil {
				return err
			}
			request, err := rawJSONObject(map[string]any{
				"skuId": skuID, "quantity": quantity, "contractInput": contractInput,
			})
			if err != nil {
				return output.Internal("COMMERCE_QUOTE_INPUT_INVALID", "could not encode the Quote input", err)
			}
			quote, err := createCommerceQuoteInput(command.Context(), runtime, stableName, request)
			if err != nil {
				return err
			}
			return runtime.business(commerceFlowQuoteResult{NextAction: commerceFlowConfirmPayment, Quote: quote})
		},
	}
	command.Flags().StringVar(&stableName, "skill", "", "purchase Skill stable name")
	command.Flags().StringVar(&skuID, "sku", "", "active SKU id returned by flow start")
	command.Flags().IntVar(&quantity, "quantity", 1, "purchase quantity")
	command.Flags().StringVar(&contractInputJSON, "contract-input-json", "{}", "inline buyer contract JSON object")
	command.Flags().StringArrayVar(&assets, "asset", nil, "buyer asset as <field-key>=<local-path>; repeat for multiple files")
	_ = command.MarkFlagRequired("skill")
	_ = command.MarkFlagRequired("sku")
	return command
}

func uploadCommerceFlowAssets(ctx context.Context, runtime *Runtime, stableName string, contractInput json.RawMessage, values []string) (json.RawMessage, error) {
	if len(values) == 0 {
		return contractInput, nil
	}
	state, err := runtime.requireCommerceSession(stableName)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(contractInput, &fields); err != nil {
		return nil, output.Validation("COMMERCE_QUOTE_INPUT_INVALID", "contract input is invalid").WithCause(err)
	}
	assetIDs := make(map[string][]string)
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, output.Validation("COMMERCE_FLOW_ASSET_INVALID", "--asset must be <field-key>=<local-path>")
		}
		fieldKey, filename := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if _, exists := fields[fieldKey]; exists {
			return nil, output.Validation("COMMERCE_FLOW_ASSET_CONFLICT", "asset field must not also appear in --contract-input-json")
		}
		asset, err := uploadCommerceAsset(ctx, runtime, state, fieldKey, filename, "")
		if err != nil {
			return nil, err
		}
		assetIDs[fieldKey] = append(assetIDs[fieldKey], asset.AssetID)
	}
	for fieldKey, ids := range assetIDs {
		encoded, _ := json.Marshal(ids)
		fields[fieldKey] = encoded
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, output.Internal("JSON_INPUT_ENCODE_FAILED", "could not encode uploaded contract assets", err)
	}
	return encoded, nil
}

func parseInlineJSONObject(value, code string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) == 0 || len(trimmed) > maxCommandJSONBytes || trimmed[0] != '{' {
		return nil, output.Validation(code, "inline input must be one JSON object")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &object); err != nil || object == nil {
		return nil, output.Validation(code, "inline input must be one valid JSON object").WithCause(err)
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, output.Internal("JSON_INPUT_ENCODE_FAILED", "could not normalize inline JSON input", err)
	}
	return canonical, nil
}

func newCommerceFlowConfirmCommand(runtime *Runtime) *cobra.Command {
	var stableName, quoteID, locale string
	command := &cobra.Command{
		Use:   "confirm",
		Short: "Create the confirmed Order with the fixed conversational payment policy",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if locale != "zh-CN" && locale != "en-US" {
				return output.Validation("COMMERCE_LOCALE_INVALID", "--locale must be zh-CN or en-US")
			}
			state, err := runtime.requireCommerceSession(stableName)
			if err != nil {
				return err
			}
			binding, err := runtime.loadCommerceBinding("quote", state.LocalContextID, quoteID)
			if err != nil {
				return err
			}
			provider, scene, err := commerceFlowPaymentSelection(binding.PaymentOptions)
			if err != nil {
				return err
			}
			fields := map[string]any{"quoteId": quoteID, "paymentProvider": provider, "locale": locale}
			if scene != "" {
				fields["paymentScene"] = scene
			}
			request, err := rawJSONObject(fields)
			if err != nil {
				return output.Internal("COMMERCE_ORDER_INPUT_INVALID", "could not encode the Order input", err)
			}
			created, err := createCommerceOrderInput(command.Context(), runtime, stableName, request)
			if err != nil {
				return err
			}
			nextAction, err := commerceFlowOrderNextAction(created.Order, binding.WaitUntil)
			if err != nil {
				return err
			}
			return runtime.business(commerceFlowConfirmResult{NextAction: nextAction, Order: created.Order})
		},
	}
	command.Flags().StringVar(&stableName, "skill", "", "purchase Skill stable name")
	command.Flags().StringVar(&quoteID, "quote", "", "Quote id returned by flow quote")
	command.Flags().StringVar(&locale, "locale", "zh-CN", "localized Order presentation")
	_ = command.MarkFlagRequired("skill")
	_ = command.MarkFlagRequired("quote")
	return command
}

func commerceFlowPaymentSelection(options []api.CommercePaymentOption) (string, string, error) {
	if slices.ContainsFunc(options, func(option api.CommercePaymentOption) bool {
		return option.Provider == "FREE" && len(option.Scenes) == 0
	}) {
		return "FREE", "", nil
	}
	if slices.ContainsFunc(options, func(option api.CommercePaymentOption) bool {
		return option.Provider == "WECHAT_PAY" && slices.Contains(option.Scenes, "NATIVE")
	}) {
		return "WECHAT_PAY", "NATIVE", nil
	}
	return "", "", output.Validation(
		"COMMERCE_WECHAT_QR_PAYMENT_UNAVAILABLE",
		"WeChat QR payment is unavailable for the current Quote",
	).WithDetails(map[string]any{"paymentOptions": options}).
		WithHint("stop this purchase without choosing another payment method or running viceme auth login")
}

func commerceQuoteWaitUntil(raw json.RawMessage) (string, error) {
	var fulfillment struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &fulfillment); err != nil || len(fulfillment.Capabilities) == 0 {
		return "", output.Internal("COMMERCE_QUOTE_RESPONSE_INVALID", "Quote fulfillment is invalid", err)
	}
	waitUntil := commerceWaitUntilFulfillment
	for _, capability := range fulfillment.Capabilities {
		switch capability {
		case "MANUAL_PROCESSING", "SHIPMENT":
			waitUntil = commerceWaitUntilPayment
		case "PLATFORM_ADAPTER", "DIGITAL_ENTITLEMENT":
		default:
			return "", output.Internal("COMMERCE_QUOTE_RESPONSE_INVALID", "Quote fulfillment capability is invalid", nil)
		}
	}
	return waitUntil, nil
}

func commerceFlowOrderNextAction(order api.CommerceOrder, waitUntil string) (string, error) {
	if waitUntil != commerceWaitUntilPayment && waitUntil != commerceWaitUntilFulfillment {
		return "", output.Internal("COMMERCE_QUOTE_BINDING_INVALID", "Quote fulfillment wait target is missing", nil)
	}
	switch order.Status {
	case "CLOSED":
		return commerceFlowPaymentClosed, nil
	case "PENDING":
		if order.PaymentPresentation != nil {
			return commerceFlowPresentPaymentQR, nil
		}
		return commerceFlowPaymentPending, nil
	case "PAID":
		if waitUntil == commerceWaitUntilPayment {
			return commerceFlowCompleted, nil
		}
		return commerceFlowWaitFulfillment, nil
	default:
		return "", output.Internal("COMMERCE_ORDER_RESPONSE_INVALID", "Order status is invalid", nil)
	}
}

func newCommerceFlowWaitCommand(runtime *Runtime) *cobra.Command {
	var stableName, orderNo, until string
	var timeout, interval time.Duration
	command := &cobra.Command{
		Use:   "wait",
		Short: "Poll one bounded payment or fulfillment window in one Agent turn",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			status, err := waitCommerceOrder(command.Context(), runtime, stableName, orderNo, until, timeout, interval)
			if err != nil {
				return err
			}
			nextAction, err := commerceFlowStatusNextAction(status, until)
			if err != nil {
				return err
			}
			return runtime.business(commerceFlowWaitResult{NextAction: nextAction, Status: status})
		},
	}
	command.Flags().StringVar(&stableName, "skill", "", "purchase Skill stable name")
	command.Flags().StringVar(&orderNo, "order", "", "order number")
	command.Flags().StringVar(&until, "until", "", "terminal target: payment or fulfillment")
	command.Flags().DurationVar(&timeout, "timeout", 8*time.Second, "bounded wait for this Agent turn")
	command.Flags().DurationVar(&interval, "interval", time.Second, "poll interval")
	_ = command.MarkFlagRequired("skill")
	_ = command.MarkFlagRequired("order")
	_ = command.MarkFlagRequired("until")
	return command
}

func commerceFlowStatusNextAction(status api.OrderStatusResponse, until string) (string, error) {
	var payment struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(status.Payment, &payment); err != nil {
		return "", commerceFlowStatusInvalid(err)
	}
	switch payment.Status {
	case "PENDING":
		return commerceFlowPaymentPending, nil
	case "CLOSED":
		return commerceFlowPaymentClosed, nil
	case "PAID":
	default:
		return "", commerceFlowStatusInvalid(nil)
	}
	if until == commerceWaitUntilPayment {
		return commerceFlowCompleted, nil
	}
	if until != commerceWaitUntilFulfillment {
		return "", output.Validation("COMMERCE_WAIT_TARGET_INVALID", "--until must be payment or fulfillment")
	}
	if string(status.Fulfillment) == "null" {
		return commerceFlowCompleted, nil
	}
	var fulfillment struct {
		Status string `json:"status"`
	}
	if len(status.Fulfillment) == 0 {
		return "", commerceFlowStatusInvalid(nil)
	}
	if err := json.Unmarshal(status.Fulfillment, &fulfillment); err != nil {
		return "", commerceFlowStatusInvalid(err)
	}
	switch fulfillment.Status {
	case "SUCCEEDED", "FAILED", "CANCELLED":
		return commerceFlowCompleted, nil
	case "AWAITING_PAYMENT", "PENDING", "PROCESSING", "RECONCILING":
		return commerceFlowFulfillmentPending, nil
	default:
		return "", commerceFlowStatusInvalid(nil)
	}
}

func commerceFlowStatusInvalid(cause error) error {
	return output.Internal("COMMERCE_ORDER_STATUS_RESPONSE_INVALID", "order status response is invalid", cause)
}
