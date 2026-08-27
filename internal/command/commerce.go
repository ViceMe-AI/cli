package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

type commerceSessionResult struct {
	LocalContextID string    `json:"localContextId"`
	StableName     string    `json:"stableName"`
	ProductID      string    `json:"productId"`
	SessionID      string    `json:"sessionId"`
	PrincipalID    string    `json:"principalId"`
	PrincipalKind  string    `json:"principalKind"`
	ExpiresAt      time.Time `json:"expiresAt"`
	Recovered      bool      `json:"recovered"`
}

func newCommerceCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "commerce", Short: "Run signed ViceMe product purchase Skills"}
	command.PersistentFlags().StringVar(&runtime.commerceContextID, "session-context", "", "opaque localContextId returned by commerce session start")
	command.AddCommand(newCommerceSkillCommand(runtime))
	command.AddCommand(newCommerceFlowCommand(runtime))
	command.AddCommand(newCommerceSessionCommand(runtime))
	command.AddCommand(newCommerceProductCommand(runtime))
	command.AddCommand(newCommerceAssetCommand(runtime))
	command.AddCommand(newCommerceQuoteCommand(runtime))
	command.AddCommand(newCommerceOrderCommand(runtime))
	return command
}

func commerceCommandRequested(command *cobra.Command) bool {
	for current := command; current != nil; current = current.Parent() {
		if current.Name() == "commerce" {
			return true
		}
	}
	return false
}

func newCommerceSkillCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "skill", Short: "Inspect or install a generated product purchase Skill"}
	command.AddCommand(&cobra.Command{
		Use:     "get <stable-name>",
		Aliases: []string{"show"},
		Short:   "Get the authoritative signed purchase Skill descriptor",
		Args:    cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := runtime.client().GetProductPurchaseSkill(command.Context(), args[0])
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	})
	command.AddCommand(newCommerceSkillInstallCommand(runtime))
	return command
}

func newCommerceSessionCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "session", Short: "Manage a local purchase-Skill Commerce Session"}
	command.AddCommand(newCommerceSessionStartCommand(runtime))
	return command
}

func newCommerceSessionStartCommand(runtime *Runtime) *cobra.Command {
	var stableName string
	command := &cobra.Command{
		Use:   "start",
		Short: "Create or recover the session bound to one purchase Skill",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			localContextID := strings.TrimSpace(runtime.commerceContextID)
			if localContextID == "" {
				localContextID = runtime.deps.NewID()
			}
			if !validCommerceContextID(localContextID) {
				return output.Validation("COMMERCE_SESSION_CONTEXT_INVALID", "--session-context must be the opaque localContextId returned by commerce session start")
			}
			state, recovered, err := runtime.startCommerceSession(command.Context(), localContextID, stableName)
			if err != nil {
				return err
			}
			return runtime.business(sessionResult(state, recovered))
		},
	}
	command.Flags().StringVar(&stableName, "skill", "", "purchase Skill stable name")
	_ = command.MarkFlagRequired("skill")
	return command
}

func (runtime *Runtime) startCommerceSession(ctx context.Context, localContextID, stableName string) (commerceSessionState, bool, error) {
	if !validStableName(stableName) {
		return commerceSessionState{}, false, output.Validation("COMMERCE_SKILL_NAME_INVALID", "purchase Skill stable name is invalid")
	}
	unlock, err := runtime.lockCommerceSession(ctx, localContextID, stableName)
	if err != nil {
		return commerceSessionState{}, false, err
	}
	defer unlock()
	state, err := runtime.loadCommerceSession(localContextID, stableName)
	if err == nil && state.ExpiresAt.After(runtime.deps.Now().Add(5*time.Second)) {
		return state, true, nil
	}
	if err == nil {
		if deleteErr := runtime.deleteCommerceSessionCredentials(localContextID, stableName); deleteErr != nil {
			return commerceSessionState{}, false, deleteErr
		}
		return commerceSessionState{}, false, output.Policy("COMMERCE_SESSION_CONTEXT_EXPIRED", "the Commerce Session for this localContextId has expired").
			WithHint("start a new purchase without --session-context; cross-session order queries remain unsupported")
	}
	if err != nil && !errors.Is(err, errCommerceSessionMissing) {
		return commerceSessionState{}, false, err
	}
	intent, err := runtime.loadOrCreateCommerceSessionStartIntent(localContextID, stableName)
	if err != nil {
		return commerceSessionState{}, false, err
	}
	created, err := runtime.client().CreateCommerceSession(
		ctx,
		stableName,
		intent.ClientRequestID,
		intent.ReplaySecret,
	)
	if err != nil {
		return commerceSessionState{}, false, err
	}
	if created.ProductID == nil || *created.ProductID == "" {
		return commerceSessionState{}, false, output.Internal("COMMERCE_SESSION_RESPONSE_INVALID", "Commerce Session Product selection is missing", nil)
	}
	expiresAt, err := time.Parse(time.RFC3339, created.ExpiresAt)
	if err != nil {
		return commerceSessionState{}, false, output.Internal("COMMERCE_SESSION_RESPONSE_INVALID", "Commerce Session expiry is invalid", err)
	}
	state = commerceSessionState{
		LocalContextID: localContextID, StableName: stableName, ProductID: *created.ProductID, SessionID: created.SessionID,
		PrincipalID: created.PrincipalID, PrincipalKind: created.PrincipalKind,
		Token: created.Token, ExpiresAt: expiresAt,
	}
	if err := runtime.saveCommerceSession(state); err != nil {
		return commerceSessionState{}, false, err
	}
	return state, created.Recovered, nil
}

func (runtime *Runtime) requireCommerceSession(stableName string) (commerceSessionState, error) {
	if !validStableName(stableName) {
		return commerceSessionState{}, output.Validation("COMMERCE_SKILL_NAME_INVALID", "purchase Skill stable name is invalid")
	}
	localContextID := strings.TrimSpace(runtime.commerceContextID)
	if !validCommerceContextID(localContextID) {
		return commerceSessionState{}, output.Validation("COMMERCE_SESSION_CONTEXT_REQUIRED", "--session-context must be the localContextId returned by commerce session start").
			WithHint("keep localContextId only in the current Agent task and pass it to every purchase command")
	}
	state, err := runtime.loadCommerceSession(localContextID, stableName)
	if err != nil {
		return commerceSessionState{}, err
	}
	if !state.ExpiresAt.After(runtime.deps.Now()) {
		if deleteErr := runtime.deleteCommerceSessionCredentials(localContextID, stableName); deleteErr != nil {
			return commerceSessionState{}, deleteErr
		}
		return commerceSessionState{}, output.Policy("COMMERCE_SESSION_RECOVERY_UNAVAILABLE", "the original Commerce Session is no longer recoverable").
			WithHint("cross-session order queries are intentionally unsupported")
	}
	return state, nil
}

func sessionResult(state commerceSessionState, recovered bool) commerceSessionResult {
	return commerceSessionResult{
		LocalContextID: state.LocalContextID, StableName: state.StableName, ProductID: state.ProductID,
		SessionID: state.SessionID, PrincipalID: state.PrincipalID,
		PrincipalKind: state.PrincipalKind, ExpiresAt: state.ExpiresAt,
		Recovered: recovered,
	}
}

func validCommerceContextID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, character := range strings.ToLower(value) {
		switch index {
		case 8, 13, 18, 23:
			if character != '-' {
				return false
			}
		default:
			if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
				return false
			}
		}
	}
	return true
}

func validStableName(value string) bool {
	if len(value) < len("viceme-a") || len(value) > 160 || !strings.HasPrefix(value, "viceme-") {
		return false
	}
	for index, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			continue
		}
		if character != '-' || index == 0 || index == len(value)-1 || value[index-1] == '-' {
			return false
		}
	}
	return true
}

func newCommerceProductCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "product", Short: "Read the exact Product bound to a purchase Skill"}
	command.AddCommand(newCommerceProductDescribeCommand(runtime))
	return command
}

func newCommerceProductDescribeCommand(runtime *Runtime) *cobra.Command {
	var stableName string
	command := &cobra.Command{
		Use:   "describe",
		Short: "Describe price, SKU, buyer contract, and fulfillment requirements",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			state, err := runtime.requireCommerceSession(stableName)
			if err != nil {
				return err
			}
			product, err := runtime.client().GetCommerceProduct(command.Context(), state.ProductID, state.Token)
			if err != nil {
				return err
			}
			return runtime.business(product)
		},
	}
	command.Flags().StringVar(&stableName, "skill", "", "purchase Skill stable name")
	_ = command.MarkFlagRequired("skill")
	return command
}

func newCommerceAssetCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "asset", Short: "Upload buyer-provided contract assets"}
	command.AddCommand(newCommerceAssetUploadCommand(runtime))
	return command
}

func newCommerceAssetUploadCommand(runtime *Runtime) *cobra.Command {
	var stableName, fieldKey, filename, contentType string
	command := &cobra.Command{
		Use:   "upload",
		Short: "Upload one immutable contract asset in the current purchase session",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			state, err := runtime.requireCommerceSession(stableName)
			if err != nil {
				return err
			}
			asset, err := uploadCommerceAsset(command.Context(), runtime, state, fieldKey, filename, contentType)
			if err != nil {
				return err
			}
			return runtime.business(asset)
		},
	}
	command.Flags().StringVar(&stableName, "skill", "", "purchase Skill stable name")
	command.Flags().StringVar(&fieldKey, "field", "", "buyer contract asset field key")
	command.Flags().StringVar(&filename, "path", "", "local asset file")
	command.Flags().StringVar(&contentType, "content-type", "", "MIME type; inferred from extension when omitted")
	_ = command.MarkFlagRequired("skill")
	_ = command.MarkFlagRequired("field")
	_ = command.MarkFlagRequired("path")
	return command
}

func uploadCommerceAsset(ctx context.Context, runtime *Runtime, state commerceSessionState, fieldKey, filename, contentType string) (api.ContractAsset, error) {
	file, err := os.Open(filename)
	if err != nil {
		return api.ContractAsset{}, output.Validation("CONTRACT_ASSET_READ_FAILED", "could not open the contract asset").WithCause(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 100*1024*1024 {
		return api.ContractAsset{}, output.Validation("CONTRACT_ASSET_INVALID", "contract asset must be a non-empty regular file no larger than 100 MiB")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return api.ContractAsset{}, output.Validation("CONTRACT_ASSET_READ_FAILED", "could not hash the contract asset").WithCause(err)
	}
	if contentType == "" {
		contentType = mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	request, err := rawJSONObject(map[string]any{
		"fieldKey": fieldKey, "fileName": filepath.Base(filename), "contentType": contentType,
		"sizeBytes": info.Size(), "digest": hex.EncodeToString(hash.Sum(nil)),
	})
	if err != nil {
		return api.ContractAsset{}, output.Internal("CONTRACT_ASSET_REQUEST_INVALID", "could not encode the contract asset request", err)
	}
	authorization, err := runtime.client().CreateContractAsset(ctx, request, state.Token)
	if err != nil {
		return api.ContractAsset{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return api.ContractAsset{}, output.Validation("CONTRACT_ASSET_READ_FAILED", "could not rewind the contract asset").WithCause(err)
	}
	if err := runtime.client().PutPresigned(ctx, authorization.Upload.URL, authorization.Upload.Headers, file, info.Size()); err != nil {
		return api.ContractAsset{}, err
	}
	return runtime.client().CompleteContractAsset(ctx, authorization.AssetID, state.Token)
}

func newCommerceQuoteCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "quote", Short: "Create an authoritative Product quote"}
	command.AddCommand(newCommerceQuoteCreateCommand(runtime))
	return command
}

func newCommerceQuoteCreateCommand(runtime *Runtime) *cobra.Command {
	var stableName, inputFile string
	command := &cobra.Command{
		Use:   "create",
		Short: "Validate buyer input and calculate the final payable total",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			quote, err := createCommerceQuote(command.Context(), runtime, stableName, inputFile)
			if err != nil {
				return err
			}
			return runtime.business(quote)
		},
	}
	command.Flags().StringVar(&stableName, "skill", "", "purchase Skill stable name")
	command.Flags().StringVar(&inputFile, "input", "", "strict quote JSON without clientRequestId")
	_ = command.MarkFlagRequired("skill")
	_ = command.MarkFlagRequired("input")
	return command
}

func createCommerceQuote(ctx context.Context, runtime *Runtime, stableName, inputFile string) (api.ProductQuote, error) {
	input, err := readJSONObject(inputFile, "COMMERCE_QUOTE_INPUT_INVALID")
	if err != nil {
		return api.ProductQuote{}, err
	}
	return createCommerceQuoteInput(ctx, runtime, stableName, input)
}

func createCommerceQuoteInput(ctx context.Context, runtime *Runtime, stableName string, input json.RawMessage) (api.ProductQuote, error) {
	state, err := runtime.requireCommerceSession(stableName)
	if err != nil {
		return api.ProductQuote{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil {
		return api.ProductQuote{}, output.Validation("COMMERCE_QUOTE_INPUT_INVALID", "quote input is invalid").WithCause(err)
	}
	unlock, err := runtime.lockCommerceIntent(ctx, "quote", state.LocalContextID, stableName, fields)
	if err != nil {
		return api.ProductQuote{}, err
	}
	defer unlock()
	requestID, intentKey, err := runtime.intentFor("quote", state.LocalContextID, stableName, fields)
	if err != nil {
		return api.ProductQuote{}, err
	}
	encodedID, _ := json.Marshal(requestID)
	fields["clientRequestId"] = encodedID
	request, _ := json.Marshal(fields)
	quote, err := runtime.client().CreateProductQuote(ctx, request, state.Token)
	if err != nil {
		return api.ProductQuote{}, err
	}
	expiresAt, err := time.Parse(time.RFC3339, quote.ExpiresAt)
	if err != nil {
		return api.ProductQuote{}, output.Internal("COMMERCE_QUOTE_RESPONSE_INVALID", "quote expiry is invalid", err)
	}
	waitUntil, err := commerceQuoteWaitUntil(quote.Fulfillment)
	if err != nil {
		return api.ProductQuote{}, err
	}
	if err := runtime.saveCommerceBinding("quote", quote.ID, commerceResourceBinding{
		LocalContextID: state.LocalContextID,
		StableName:     stableName,
		SessionID:      state.SessionID,
		ExpiresAt:      expiresAt,
		PaymentOptions: quote.PaymentOptions,
		WaitUntil:      waitUntil,
	}); err != nil {
		return api.ProductQuote{}, err
	}
	if err := runtime.completeIntent(intentKey, requestID, quote.ID, &expiresAt); err != nil {
		return api.ProductQuote{}, err
	}
	return quote, nil
}

func newCommerceOrderCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "order", Short: "Create and recover an order in its original Commerce Session"}
	command.AddCommand(newCommerceOrderCreateCommand(runtime))
	command.AddCommand(newCommerceOrderStatusCommand(runtime))
	command.AddCommand(newCommerceOrderWaitCommand(runtime))
	return command
}

func newCommerceOrderCreateCommand(runtime *Runtime) *cobra.Command {
	var stableName, inputFile string
	command := &cobra.Command{
		Use:   "create",
		Short: "Create the order and return the provider payment action",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			created, err := createCommerceOrder(command.Context(), runtime, stableName, inputFile)
			if err != nil {
				return err
			}
			return runtime.business(created)
		},
	}
	command.Flags().StringVar(&stableName, "skill", "", "purchase Skill stable name")
	command.Flags().StringVar(&inputFile, "input", "", "strict order JSON without clientRequestId")
	_ = command.MarkFlagRequired("skill")
	_ = command.MarkFlagRequired("input")
	return command
}

func createCommerceOrder(ctx context.Context, runtime *Runtime, stableName, inputFile string) (api.CreateOrderResponse, error) {
	input, err := readJSONObject(inputFile, "COMMERCE_ORDER_INPUT_INVALID")
	if err != nil {
		return api.CreateOrderResponse{}, err
	}
	return createCommerceOrderInput(ctx, runtime, stableName, input)
}

func createCommerceOrderInput(ctx context.Context, runtime *Runtime, stableName string, input json.RawMessage) (api.CreateOrderResponse, error) {
	state, err := runtime.requireCommerceSession(stableName)
	if err != nil {
		return api.CreateOrderResponse{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil {
		return api.CreateOrderResponse{}, output.Validation("COMMERCE_ORDER_INPUT_INVALID", "order input is invalid").WithCause(err)
	}
	var quoteID string
	if err := json.Unmarshal(fields["quoteId"], &quoteID); err != nil || quoteID == "" {
		return api.CreateOrderResponse{}, output.Validation("COMMERCE_ORDER_INPUT_INVALID", "quoteId is required")
	}
	quoteBinding, err := runtime.loadCommerceBinding("quote", state.LocalContextID, quoteID)
	if err != nil {
		return api.CreateOrderResponse{}, err
	}
	if quoteBinding.StableName != stableName || quoteBinding.SessionID != state.SessionID {
		return api.CreateOrderResponse{}, output.Policy("COMMERCE_QUOTE_SESSION_MISMATCH", "quote belongs to a different Commerce Session")
	}
	if err := validateCommerceOrderPaymentSelection(fields, quoteBinding.PaymentOptions); err != nil {
		return api.CreateOrderResponse{}, err
	}
	unlock, err := runtime.lockCommerceIntent(ctx, "order", state.LocalContextID, stableName, fields)
	if err != nil {
		return api.CreateOrderResponse{}, err
	}
	defer unlock()
	requestID, intentKey, err := runtime.intentFor("order", state.LocalContextID, stableName, fields)
	if err != nil {
		return api.CreateOrderResponse{}, err
	}
	encodedID, _ := json.Marshal(requestID)
	fields["clientRequestId"] = encodedID
	request, _ := json.Marshal(fields)
	created, err := runtime.client().CreateCommerceOrder(ctx, request, state.Token)
	if err != nil {
		if recovery, ok := api.RecoveryReferenceFromError(err); ok && recovery.ResourceType == "ORDER" {
			if saveErr := runtime.saveCommerceBinding("order", recovery.ResourceID, commerceResourceBinding{
				LocalContextID: state.LocalContextID,
				StableName:     stableName,
				SessionID:      state.SessionID,
				ExpiresAt:      state.ExpiresAt,
			}); saveErr != nil {
				return api.CreateOrderResponse{}, output.Internal(
					"COMMERCE_ORDER_RECOVERY_SAVE_FAILED",
					"the order was created but its same-session recovery binding could not be saved",
					saveErr,
				).WithDetails(map[string]any{"orderNo": recovery.ResourceID})
			}
			if completeErr := runtime.completeIntent(intentKey, requestID, recovery.ResourceID, &state.ExpiresAt); completeErr != nil {
				return api.CreateOrderResponse{}, output.Internal(
					"COMMERCE_ORDER_RECOVERY_SAVE_FAILED",
					"the order was created but its idempotency recovery intent could not be completed",
					completeErr,
				).WithDetails(map[string]any{"orderNo": recovery.ResourceID})
			}
		}
		return api.CreateOrderResponse{}, err
	}
	if _, err := time.Parse(time.RFC3339, created.Order.ExpiresAt); err != nil {
		return api.CreateOrderResponse{}, output.Internal("COMMERCE_ORDER_RESPONSE_INVALID", "order expiry is invalid", err)
	}
	if err := runtime.saveCommerceBinding("order", created.Order.OrderNo, commerceResourceBinding{LocalContextID: state.LocalContextID, StableName: stableName, SessionID: state.SessionID, ExpiresAt: state.ExpiresAt}); err != nil {
		return api.CreateOrderResponse{}, err
	}
	if err := runtime.completeIntent(intentKey, requestID, created.Order.OrderNo, &state.ExpiresAt); err != nil {
		return api.CreateOrderResponse{}, err
	}
	if err := prepareCommercePaymentPresentation(runtime, &created.Order); err != nil {
		return api.CreateOrderResponse{}, output.Internal(
			"COMMERCE_PAYMENT_PRESENTATION_FAILED",
			"the order was created but its local payment QR image could not be prepared",
			err,
		).WithDetails(map[string]any{"orderNo": created.Order.OrderNo}).
			WithHint("rerun the same order create command in this Commerce Session; do not expose the payment URI or use an external QR service")
	}
	return created, nil
}

func validateCommerceOrderPaymentSelection(fields map[string]json.RawMessage, options []api.CommercePaymentOption) error {
	allowedFields := map[string]bool{
		"quoteId": true, "paymentProvider": true, "paymentScene": true,
	}
	for field := range fields {
		if !allowedFields[field] {
			return output.Validation("COMMERCE_ORDER_INPUT_INVALID", "order input contains an unsupported field").
				WithHint("use only quoteId, paymentProvider, and optional paymentScene; the CLI owns clientRequestId")
		}
	}
	var provider string
	if err := json.Unmarshal(fields["paymentProvider"], &provider); err != nil || provider == "" {
		return commerceOrderPaymentOptionInvalid(options)
	}
	var scene string
	rawScene, hasScene := fields["paymentScene"]
	if hasScene {
		if err := json.Unmarshal(rawScene, &scene); err != nil || scene == "" {
			return commerceOrderPaymentOptionInvalid(options)
		}
	}
	for _, option := range options {
		if option.Provider == "FREE" && provider == "FREE" && len(option.Scenes) == 0 && !hasScene {
			return nil
		}
	}
	wechatNativeAvailable := false
	for _, option := range options {
		if option.Provider == "WECHAT_PAY" && slices.Contains(option.Scenes, "NATIVE") {
			wechatNativeAvailable = true
			break
		}
	}
	if !wechatNativeAvailable {
		return output.Validation(
			"COMMERCE_WECHAT_QR_PAYMENT_UNAVAILABLE",
			"WeChat QR payment is unavailable for the current Quote",
		).WithDetails(map[string]any{"paymentOptions": options}).
			WithHint("stop this purchase without choosing another payment method or running viceme auth login")
	}
	if provider != "WECHAT_PAY" || !hasScene || scene != "NATIVE" {
		return output.Validation(
			"COMMERCE_WECHAT_QR_PAYMENT_REQUIRED",
			"paid Commerce Skill orders require WECHAT_PAY with the NATIVE scene",
		).WithDetails(map[string]any{"paymentOptions": options}).
			WithHint("use WECHAT_PAY and NATIVE from the current Quote; do not choose Alipay, balance, H5, or run viceme auth login")
	}
	return nil
}

func commerceOrderPaymentOptionInvalid(options []api.CommercePaymentOption) error {
	return output.Validation(
		"COMMERCE_ORDER_PAYMENT_OPTION_INVALID",
		"paymentProvider and paymentScene must match one option returned by the current Quote",
	).WithDetails(map[string]any{"paymentOptions": options}).
		WithHint("copy one exact provider/scene pair from Quote.paymentOptions; do not run viceme auth login")
}

func newCommerceOrderStatusCommand(runtime *Runtime) *cobra.Command {
	var stableName, orderNo string
	command := &cobra.Command{
		Use:   "status",
		Short: "Get payment and fulfillment state from the original session",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			status, err := loadCommerceOrderStatus(command.Context(), runtime, stableName, orderNo)
			if err != nil {
				return err
			}
			return runtime.business(status)
		},
	}
	command.Flags().StringVar(&stableName, "skill", "", "purchase Skill stable name")
	command.Flags().StringVar(&orderNo, "order", "", "order number")
	_ = command.MarkFlagRequired("skill")
	_ = command.MarkFlagRequired("order")
	return command
}

func newCommerceOrderWaitCommand(runtime *Runtime) *cobra.Command {
	var stableName, orderNo, until string
	var timeout, interval time.Duration
	command := &cobra.Command{
		Use:   "wait",
		Short: "Wait for an explicit payment or fulfillment target without changing sessions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			status, err := waitCommerceOrder(command.Context(), runtime, stableName, orderNo, until, timeout, interval)
			if err != nil {
				return err
			}
			return runtime.business(status)
		},
	}
	command.Flags().StringVar(&stableName, "skill", "", "purchase Skill stable name")
	command.Flags().StringVar(&orderNo, "order", "", "order number")
	command.Flags().StringVar(&until, "until", "", "terminal target: payment or fulfillment")
	command.Flags().DurationVar(&timeout, "timeout", 20*time.Minute, "maximum wait duration")
	command.Flags().DurationVar(&interval, "interval", 1500*time.Millisecond, "poll interval")
	_ = command.MarkFlagRequired("skill")
	_ = command.MarkFlagRequired("order")
	_ = command.MarkFlagRequired("until")
	return command
}

func waitCommerceOrder(ctx context.Context, runtime *Runtime, stableName, orderNo, until string, timeout, interval time.Duration) (api.OrderStatusResponse, error) {
	if timeout <= 0 || interval < 250*time.Millisecond {
		return api.OrderStatusResponse{}, output.Validation("COMMERCE_WAIT_INVALID", "--timeout must be positive and --interval at least 250ms")
	}
	if until != commerceWaitUntilPayment && until != commerceWaitUntilFulfillment {
		return api.OrderStatusResponse{}, output.Validation(
			"COMMERCE_WAIT_TARGET_INVALID",
			"--until must be payment or fulfillment",
		).WithDetails(map[string]any{"until": until})
	}
	deadline := runtime.deps.Now().Add(timeout)
	for {
		status, err := loadCommerceOrderStatus(ctx, runtime, stableName, orderNo)
		if err != nil {
			return api.OrderStatusResponse{}, err
		}
		if commerceWaitTargetReached(status, until) || !runtime.deps.Now().Before(deadline) {
			return status, nil
		}
		remaining := deadline.Sub(runtime.deps.Now())
		delay := interval
		if remaining < delay {
			delay = remaining
		}
		if err := runtime.deps.Sleep(ctx, delay); err != nil {
			return api.OrderStatusResponse{}, output.Network("COMMERCE_WAIT_INTERRUPTED", "order wait was interrupted", err)
		}
	}
}

func loadCommerceOrderStatus(ctx context.Context, runtime *Runtime, stableName, orderNo string) (api.OrderStatusResponse, error) {
	state, err := runtime.requireCommerceSession(stableName)
	if err != nil {
		return api.OrderStatusResponse{}, err
	}
	binding, err := runtime.loadCommerceBinding("order", state.LocalContextID, orderNo)
	if err != nil {
		return api.OrderStatusResponse{}, err
	}
	if binding.StableName != stableName {
		return api.OrderStatusResponse{}, output.Policy("COMMERCE_ORDER_SESSION_MISMATCH", "order belongs to a different purchase Skill session")
	}
	if state.SessionID != binding.SessionID || !state.ExpiresAt.After(runtime.deps.Now()) {
		return api.OrderStatusResponse{}, output.Policy("COMMERCE_SESSION_RECOVERY_UNAVAILABLE", "the original Commerce Session is no longer recoverable").
			WithHint("cross-session order queries are intentionally unsupported")
	}
	status, err := runtime.client().GetCommerceOrderStatus(ctx, orderNo, state.Token)
	if err != nil {
		return api.OrderStatusResponse{}, err
	}
	if commercePaymentTerminal(status) {
		if err := removeCommercePaymentPresentation(runtime, orderNo); err != nil {
			return api.OrderStatusResponse{}, output.Internal(
				"COMMERCE_PAYMENT_PRESENTATION_CLEANUP_FAILED",
				"the payment is terminal but its local QR image could not be removed",
				err,
			).WithDetails(map[string]any{"orderNo": orderNo})
		}
	}
	return status, nil
}

func commercePaymentTerminal(status api.OrderStatusResponse) bool {
	var payment struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(status.Payment, &payment) != nil {
		return false
	}
	return payment.Status == "PAID" || payment.Status == "CLOSED"
}

const (
	commerceWaitUntilPayment     = "payment"
	commerceWaitUntilFulfillment = "fulfillment"
)

func commerceWaitTargetReached(status api.OrderStatusResponse, until string) bool {
	var payment struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(status.Payment, &payment) != nil {
		return false
	}
	if payment.Status == "CLOSED" {
		return true
	}
	if payment.Status != "PAID" {
		return false
	}
	if until == commerceWaitUntilPayment {
		return true
	}
	if until != commerceWaitUntilFulfillment {
		return false
	}
	if len(status.Fulfillment) == 0 || string(status.Fulfillment) == "null" {
		return true
	}
	var fulfillment struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(status.Fulfillment, &fulfillment) != nil {
		return false
	}
	switch fulfillment.Status {
	case "SUCCEEDED", "FAILED", "CANCELLED":
		return true
	default:
		return false
	}
}
