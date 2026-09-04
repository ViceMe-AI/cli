package command

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/commerceartifact"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/pathidentity"
	"github.com/ViceMe-AI/cli/internal/privatepath"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
	"github.com/spf13/cobra"
)

const (
	replicaLicenseTermsVersion = "website-replica-license/v1"
	replicaPaymentWaitTimeout  = 3 * time.Minute
	replicaPaymentPollInterval = time.Minute
	replicaPaymentImagePrefix  = ".viceme-replica-payment-"
)

type replicaInstallResult struct {
	ReplicaID      string `json:"replicaId"`
	VersionID      string `json:"versionId"`
	Version        int    `json:"version"`
	OrderNo        string `json:"orderNo"`
	Target         string `json:"target"`
	ArtifactDigest string `json:"artifactDigest"`
	LicensePath    string `json:"licensePath"`
	FileCount      int    `json:"fileCount"`
	ExpandedBytes  uint64 `json:"expandedBytes"`
}

func newReplicaInstallCommand(runtime *Runtime) *cobra.Command {
	var target, locale string
	var timeout, interval time.Duration
	var confirm, paymentPresented bool
	var acceptedPriceCents int
	command := &cobra.Command{
		Use:   "install <replica-code>",
		Short: "Purchase and atomically install one Website Replica source package",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := installReplica(command.Context(), runtime, args[0], target, locale, timeout, interval, confirm, paymentPresented, acceptedPriceCents)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&target, "target", "", "new destination directory for the Replica source")
	command.Flags().StringVar(&locale, "locale", "zh-CN", "localized checkout presentation: zh-CN or en-US")
	command.Flags().DurationVar(&timeout, "timeout", replicaPaymentWaitTimeout, "maximum time to wait for payment")
	command.Flags().DurationVar(&interval, "interval", replicaPaymentPollInterval, "payment status poll interval")
	command.Flags().BoolVar(&confirm, "confirm", false, "create the order for the previously presented quote")
	command.Flags().BoolVar(&paymentPresented, "payment-presented", false, "confirm the host successfully opened the returned checkout page")
	command.Flags().IntVar(&acceptedPriceCents, "accept-price-cents", -1, "price shown when the user chose to continue; creates the order without another confirmation")
	return command
}

func installReplica(ctx context.Context, runtime *Runtime, code, target, locale string, timeout, interval time.Duration, confirm, paymentPresented bool, acceptedPriceCents int) (replicaInstallResult, error) {
	if acceptedPriceCents > 10_000_000 {
		return replicaInstallResult{}, output.Validation("REPLICA_PRICE_INVALID", "--accept-price-cents must be between 0 and 10000000")
	}
	if acceptedPriceCents >= 0 && confirm {
		return replicaInstallResult{}, output.Validation("REPLICA_CONFIRMATION_CONFLICT", "--accept-price-cents creates or resumes the order without --confirm")
	}
	if acceptedPriceCents < 0 && paymentPresented {
		return replicaInstallResult{}, output.Validation("REPLICA_PAYMENT_PRESENTATION_INVALID", "--payment-presented is only valid for anonymous checkout")
	}
	shortCode, err := parseReplicaCode(code)
	if err != nil {
		return replicaInstallResult{}, err
	}
	if strings.TrimSpace(target) == "" {
		workspace, err := os.Getwd()
		if err != nil {
			return replicaInstallResult{}, output.Validation("REPLICA_TARGET_INVALID", "could not resolve the current workspace").WithCause(err)
		}
		target = defaultReplicaTarget(workspace, shortCode)
	}
	absTarget, err := validateReplicaTarget(target)
	if err != nil {
		return replicaInstallResult{}, err
	}
	if locale != "zh-CN" && locale != "en-US" {
		return replicaInstallResult{}, output.Validation("REPLICA_LOCALE_INVALID", "--locale must be zh-CN or en-US")
	}
	if timeout <= 0 || interval < 250*time.Millisecond {
		return replicaInstallResult{}, output.Validation("REPLICA_WAIT_INVALID", "--timeout must be positive and --interval at least 250ms")
	}
	if !replicacontent.AtomicInstallSupported() {
		return replicaInstallResult{}, output.Policy("REPLICA_PLATFORM_UNSUPPORTED", "Website Replica installation requires atomic no-replace directory activation on this platform")
	}
	store, err := newReplicaPurchaseStore(runtime, shortCode, absTarget)
	if err != nil {
		return replicaInstallResult{}, err
	}
	var result replicaInstallResult
	err = store.withLock(func() error {
		var installErr error
		if acceptedPriceCents >= 0 {
			result, installErr = installReplicaAnonymousLocked(ctx, runtime, store, code, shortCode, absTarget, locale, timeout, interval, paymentPresented, acceptedPriceCents)
		} else {
			result, installErr = installReplicaLocked(ctx, runtime, store, code, shortCode, absTarget, locale, timeout, interval, confirm)
		}
		return installErr
	})
	return result, err
}

func defaultReplicaTarget(workspace, shortCode string) string {
	return filepath.Join(workspace, strings.ToLower(shortCode))
}

func installReplicaAnonymousLocked(
	ctx context.Context,
	runtime *Runtime,
	store replicaPurchaseStore,
	code string,
	shortCode string,
	absTarget string,
	locale string,
	timeout time.Duration,
	interval time.Duration,
	paymentPresented bool,
	acceptedPriceCents int,
) (replicaInstallResult, error) {
	completion, completed, err := store.loadCompletion()
	if err != nil {
		return replicaInstallResult{}, err
	}
	if completed {
		if _, err := validateReplicaCompletion(ctx, runtime, completion); err != nil {
			return replicaInstallResult{}, err
		}
		return completion.Result, nil
	}
	if result, installed, err := installRecordedPaidReplica(ctx, runtime, store, absTarget, false); err != nil || installed {
		return result, err
	}
	state, exists, err := store.load()
	if err != nil {
		return replicaInstallResult{}, err
	}
	client := runtime.client()
	if exists && state.OrderNo != "" && !paymentPresented {
		status, err := client.RecoverWebsiteReplicaOrderStatus(ctx, api.RecoverWebsiteReplicaDownloadRequest{
			OrderNo: state.OrderNo, RecoverySecret: state.DownloadRecoverySecret,
		})
		if err != nil {
			return replicaInstallResult{}, err
		}
		switch status.Payment.Status {
		case "PAID":
		case "PENDING":
			closed, err := client.CancelWebsiteReplicaOrderAttempt(ctx, api.RecoverWebsiteReplicaDownloadRequest{
				OrderNo: state.OrderNo, RecoverySecret: state.DownloadRecoverySecret,
			})
			if err != nil {
				return replicaInstallResult{}, err
			}
			if closed.Payment.Status != "CLOSED" {
				return replicaInstallResult{}, invalidReplicaResponse("Website Replica cancellation was not definitive")
			}
			if err := store.retire(state); err != nil {
				return replicaInstallResult{}, err
			}
			return installReplicaAnonymousLocked(ctx, runtime, store, code, shortCode, absTarget, locale, timeout, interval, paymentPresented, acceptedPriceCents)
		case "CLOSED":
			if err := store.retire(state); err != nil {
				return replicaInstallResult{}, err
			}
			return installReplicaAnonymousLocked(ctx, runtime, store, code, shortCode, absTarget, locale, timeout, interval, paymentPresented, acceptedPriceCents)
		default:
			return replicaInstallResult{}, invalidReplicaResponse("Website Replica recovery status is invalid")
		}
	}
	if exists && state.SessionID == "" && state.QuoteID != "" {
		return replicaInstallResult{}, replicaPurchaseConflict(state, "finish or retire the legacy Website Replica purchase before anonymous checkout")
	}
	if exists && state.Target != absTarget {
		return replicaInstallResult{}, replicaPurchaseConflict(state, "this Website Replica already has an unfinished purchase for another target")
	}
	if !exists {
		if err := requireMissingReplicaTarget(absTarget); err != nil {
			return replicaInstallResult{}, err
		}
		requestID, err := runtime.newReplicaRequestID()
		if err != nil {
			return replicaInstallResult{}, err
		}
		state = store.create(requestID)
		state.SessionReplaySecret, err = newReplicaSessionSecret()
		if err != nil {
			return replicaInstallResult{}, err
		}
		state.DownloadRecoverySecret, err = newReplicaSessionSecret()
		if err != nil {
			return replicaInstallResult{}, err
		}
		if err := store.reserve(&state); err != nil {
			return replicaInstallResult{}, err
		}
		if err := store.save(&state); err != nil {
			_ = store.retire(state)
			return replicaInstallResult{}, err
		}
	}
	if err := store.verifyReservation(state); err != nil {
		return replicaInstallResult{}, err
	}
	if err := requireMissingReplicaTarget(absTarget); err != nil {
		return replicaInstallResult{}, replicaPurchaseConflict(state, "the Website Replica target appeared before installation completed").WithCause(err)
	}

	if state.SessionID == "" {
		resolved, err := client.ResolveWebsiteReplicaPublic(ctx, code)
		if err != nil {
			return replicaInstallResult{}, err
		}
		if resolved.ShortCode != shortCode || resolved.Title != resolved.Product.Title {
			return replicaInstallResult{}, invalidReplicaResponse("Website Replica resolution does not match the supplied code")
		}
		if resolved.Product.PriceCents != acceptedPriceCents {
			return replicaInstallResult{}, output.Confirmation(
				"REPLICA_PRICE_CHANGED",
				"the Website Replica price changed after the user chose to continue",
			).WithDetails(map[string]any{
				"nextAction": "OPEN_WORK_PREVIEW", "workUrl": resolved.ViceMeWorkURL,
				"currency": resolved.Product.Currency, "totalAmountCents": resolved.Product.PriceCents,
			}).WithHint("open the Work preview and ask the user to continue again at the new price")
		}
		if standaloneReplicaAttemptMayExist(runtime, shortCode) {
			if err := retireStandaloneUnpaidAttempt(ctx, runtime, resolved); err != nil {
				return replicaInstallResult{}, err
			}
		}
		state.ReplicaID = resolved.ReplicaID
		state.ProductID = resolved.Product.ID
		state.SKUID = resolved.Product.SKUID
		state.ProductTitle = resolved.Product.Title
		state.Currency = resolved.Product.Currency
		state.PriceCents = resolved.Product.PriceCents
		if err := store.save(&state); err != nil {
			return replicaInstallResult{}, err
		}
		session, err := client.CreateWebsiteReplicaSession(ctx, api.CreateWebsiteReplicaSessionRequest{
			Instruction: code, ClientRequestID: state.QuoteRequestID, ReplaySecret: state.SessionReplaySecret,
		})
		if err != nil {
			return replicaInstallResult{}, err
		}
		if !replicaResolutionMatchesState(session.Replica, state) {
			return replicaInstallResult{}, invalidReplicaResponse("Website Replica session does not match the accepted product")
		}
		state.SessionID = session.SessionID
		state.SessionToken = session.Token
		state.SessionExpiresAt = session.ExpiresAt
		if err := store.save(&state); err != nil {
			return replicaInstallResult{}, err
		}
	}
	if state.PriceCents != acceptedPriceCents {
		return replicaInstallResult{}, output.Confirmation("REPLICA_PRICE_CHANGED", "the accepted Website Replica price does not match the recoverable purchase").WithDetails(map[string]any{
			"currency": state.Currency, "totalAmountCents": state.PriceCents,
		})
	}
	if state.OrderNo != "" {
		result, recovered, err := tryInstallRecoveredReplica(ctx, runtime, store, state, client, absTarget)
		if err != nil || recovered {
			return result, err
		}
	}
	if state.OrderNo == "" {
		if state.CheckoutQuoteRequestID == "" {
			state.CheckoutQuoteRequestID, err = runtime.newReplicaRequestID()
			if err != nil {
				return replicaInstallResult{}, err
			}
			state.OrderRequestID, err = runtime.newReplicaRequestID()
			if err != nil {
				return replicaInstallResult{}, err
			}
			state.Locale = locale
			if err := store.save(&state); err != nil {
				return replicaInstallResult{}, err
			}
		}
		checkout, err := client.CheckoutWebsiteReplica(ctx, state.SessionID, state.SessionToken, api.CheckoutWebsiteReplicaRequest{
			AcceptedPriceCents: acceptedPriceCents, QuoteClientRequestID: state.CheckoutQuoteRequestID,
			OrderClientRequestID: state.OrderRequestID, DownloadRecoverySecret: state.DownloadRecoverySecret, Locale: state.Locale,
		})
		if err != nil {
			return replicaInstallResult{}, err
		}
		state.OrderNo = checkout.OrderNo
		state.OrderExpiresAt = checkout.ExpiresAt
		state.CheckoutURL = checkout.CheckoutURL
		if checkout.Status == "PAID" {
			_ = removeReplicaPaymentPresentation(runtime, state)
			if err := store.save(&state); err != nil {
				return replicaInstallResult{}, err
			}
			return installReplicaRecoveredDownload(ctx, runtime, store, state, client, checkout.OrderNo, absTarget)
		}
		if checkout.Status != "PENDING" {
			return replicaInstallResult{}, invalidReplicaResponse("Website Replica checkout returned an invalid payment state")
		}
		if strings.TrimSpace(state.CheckoutURL) == "" {
			return replicaInstallResult{}, invalidReplicaResponse("Website Replica checkout did not return its hosted payment page")
		}
		if err := store.save(&state); err != nil {
			return replicaInstallResult{}, err
		}
	}
	if strings.TrimSpace(state.CheckoutURL) == "" {
		return replicaInstallResult{}, output.Policy("REPLICA_PURCHASE_STATE_INVALID", "Website Replica checkout page is unavailable")
	}
	_ = removeReplicaPaymentPresentation(runtime, state)
	if !paymentPresented {
		return replicaInstallResult{}, replicaPaymentPageConfirmation(state)
	}
	if state.PaymentPresentedAt == "" {
		state.PaymentPresentedAt = runtime.deps.Now().UTC().Format(time.RFC3339Nano)
		if err := store.save(&state); err != nil {
			return replicaInstallResult{}, err
		}
	}
	if err := waitForReplicaSessionPayment(ctx, runtime, client, state, timeout, interval); err != nil {
		if output.AsError(err).Subtype == "REPLICA_PAYMENT_TERMINAL" {
			_ = store.retire(state)
		}
		return replicaInstallResult{}, err
	}
	return installReplicaRecoveredDownload(ctx, runtime, store, state, client, state.OrderNo, absTarget)
}

func newReplicaSessionSecret() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", output.Internal("REPLICA_SESSION_SECRET_FAILED", "could not create Website Replica session recovery secret", err)
	}
	return base64.RawURLEncoding.EncodeToString(secret), nil
}

func replicaPaymentPageConfirmation(state replicaPurchaseState) error {
	return output.Confirmation("REPLICA_PAYMENT_REQUIRED", "open the ViceMe Website Replica checkout page").WithDetails(map[string]any{
		"nextAction": "OPEN_PAYMENT_PAGE", "checkoutUrl": state.CheckoutURL, "orderNo": state.OrderNo,
		"currency": state.Currency, "totalAmountCents": state.PriceCents, "expiresAt": state.OrderExpiresAt,
	}).WithHint("open checkoutUrl with the available URL presentation tool, then rerun the same command with --payment-presented --timeout 3m --interval 1m; do not ask for another confirmation")
}

func installReplicaLocked(
	ctx context.Context,
	runtime *Runtime,
	store replicaPurchaseStore,
	code string,
	shortCode string,
	absTarget string,
	locale string,
	timeout time.Duration,
	interval time.Duration,
	confirm bool,
) (replicaInstallResult, error) {
	if err := runtime.requireWebsiteReplicaAuthentication(ctx, "website-replica:read", "website-replica:purchase"); err != nil {
		return replicaInstallResult{}, err
	}
	completion, completed, err := store.loadCompletion()
	if err != nil {
		return replicaInstallResult{}, err
	}
	if completed {
		claims, err := validateReplicaCompletion(ctx, runtime, completion)
		if err != nil {
			return replicaInstallResult{}, err
		}
		if err := completeReplicaInstallation(ctx, runtime.client(), completion.Result, claims); err != nil {
			return replicaInstallResult{}, err
		}
		state, active, err := store.load()
		if err != nil {
			return replicaInstallResult{}, err
		}
		if active {
			if state.Target != completion.Result.Target || state.ReplicaID != completion.Result.ReplicaID || state.OrderNo != completion.Result.OrderNo {
				return replicaInstallResult{}, output.Policy("REPLICA_COMPLETION_STATE_INVALID", "Website Replica completion receipt does not match its active purchase")
			}
			if err := store.retire(state); err != nil {
				return replicaInstallResult{}, err
			}
		} else if err := store.removeOwnedOrphanReservation(replicaTargetReservationPath(completion.Result.Target)); err != nil {
			return replicaInstallResult{}, err
		}
		return completion.Result, nil
	}
	if result, installed, err := installRecordedPaidReplica(ctx, runtime, store, absTarget, true); err != nil || installed {
		return result, err
	}
	state, exists, err := store.load()
	if err != nil {
		return replicaInstallResult{}, err
	}
	client := runtime.client()
	if exists && state.OrderNo != "" && !confirm {
		status, err := client.GetWebsiteReplicaOrderStatus(ctx, state.OrderNo)
		if err != nil {
			return replicaInstallResult{}, err
		}
		switch status.Payment.Status {
		case "PAID":
		case "PENDING":
			closed, err := client.CancelWebsiteReplicaOrder(ctx, state.OrderNo)
			if err != nil {
				return replicaInstallResult{}, err
			}
			if closed.Payment.Status != "CLOSED" {
				return replicaInstallResult{}, invalidReplicaResponse("Website Replica cancellation was not definitive")
			}
			if err := store.retire(state); err != nil {
				return replicaInstallResult{}, err
			}
			return installReplicaLocked(ctx, runtime, store, code, shortCode, absTarget, locale, timeout, interval, confirm)
		case "CLOSED":
			if err := store.retire(state); err != nil {
				return replicaInstallResult{}, err
			}
			return installReplicaLocked(ctx, runtime, store, code, shortCode, absTarget, locale, timeout, interval, confirm)
		default:
			return replicaInstallResult{}, invalidReplicaResponse("Website Replica order status is invalid")
		}
	}
	quotePresentedBeforeInvocation := exists && state.QuoteID != ""
	if exists && state.Target != absTarget {
		return replicaInstallResult{}, replicaPurchaseConflict(
			state,
			"this Website Replica already has an unfinished purchase for another target",
		)
	}
	if !exists {
		if err := requireMissingReplicaTarget(absTarget); err != nil {
			return replicaInstallResult{}, err
		}
		quoteRequestID, err := runtime.newReplicaRequestID()
		if err != nil {
			return replicaInstallResult{}, err
		}
		state = store.create(quoteRequestID)
		if err := store.reserve(&state); err != nil {
			return replicaInstallResult{}, err
		}
		if err := store.save(&state); err != nil {
			_ = store.retire(state)
			return replicaInstallResult{}, err
		}
	} else if state.OrderNo == "" {
		if err := requireMissingReplicaTarget(absTarget); err != nil {
			return replicaInstallResult{}, replicaPurchaseConflict(state, "the Website Replica target appeared before payment was created").WithCause(err)
		}
	}
	if state.QuoteID == "" {
		resolved, err := client.ResolveWebsiteReplica(ctx, code)
		if err != nil {
			return replicaInstallResult{}, err
		}
		if resolved.ShortCode != shortCode || resolved.Title != resolved.Product.Title {
			return replicaInstallResult{}, invalidReplicaResponse("Website Replica resolution does not match the supplied code")
		}
		if state.ReplicaID == "" {
			state.ReplicaID = resolved.ReplicaID
			state.ProductID = resolved.Product.ID
			state.SKUID = resolved.Product.SKUID
			state.ProductTitle = resolved.Product.Title
			state.Currency = resolved.Product.Currency
			state.PriceCents = resolved.Product.PriceCents
			if err := store.save(&state); err != nil {
				return replicaInstallResult{}, err
			}
		} else if !replicaResolutionMatchesState(resolved, state) {
			return replicaInstallResult{}, replicaPurchaseConflict(state, "the Website Replica identity changed while recovering this purchase")
		}
		quote, err := client.CreateWebsiteReplicaQuote(ctx, api.CreateWebsiteReplicaQuoteRequest{
			Instruction: code, ClientRequestID: state.QuoteRequestID,
		})
		if err != nil {
			return replicaInstallResult{}, err
		}
		if !replicaQuoteMatchesState(quote, state) {
			return replicaInstallResult{}, invalidReplicaResponse("Website Replica quote does not match the resolved product")
		}
		state.QuoteID = quote.ID
		state.QuoteExpiresAt = quote.ExpiresAt
		if err := store.save(&state); err != nil {
			return replicaInstallResult{}, err
		}
	}
	if state.OrderRequestID == "" {
		if expiresAt, valid := parseReplicaStateDatetime(state.QuoteExpiresAt); valid && !runtime.deps.Now().Before(expiresAt) {
			return refreshReplicaQuote(ctx, runtime, store, state, code, shortCode, absTarget, locale, timeout, interval, confirm)
		}
		if !confirm || !quotePresentedBeforeInvocation {
			return replicaInstallResult{}, replicaQuoteConfirmation(state)
		}
		if standaloneReplicaAttemptMayExist(runtime, shortCode) {
			resolved, err := client.ResolveWebsiteReplicaPublic(ctx, code)
			if err != nil {
				return replicaInstallResult{}, err
			}
			if err := retireStandaloneUnpaidAttempt(ctx, runtime, resolved); err != nil {
				return replicaInstallResult{}, err
			}
		}
		state.OrderRequestID, err = runtime.newReplicaRequestID()
		if err != nil {
			return replicaInstallResult{}, err
		}
		state.Locale = locale
		if err := store.save(&state); err != nil {
			return replicaInstallResult{}, err
		}
	}
	order := api.WebsiteReplicaOrder{
		OrderNo: state.OrderNo, ExpiresAt: state.OrderExpiresAt,
	}
	if state.OrderNo != "" {
		status, err := client.GetWebsiteReplicaOrderStatus(ctx, state.OrderNo)
		if err != nil {
			return replicaInstallResult{}, err
		}
		order.Status = status.Payment.Status
		if order.Status == "CLOSED" {
			if err := store.retire(state); err != nil {
				return replicaInstallResult{}, err
			}
			return replicaInstallResult{}, output.Policy("REPLICA_PAYMENT_TERMINAL", "Website Replica payment did not complete")
		}
	}
	if state.OrderNo == "" || order.Status == "PENDING" {
		if err := store.verifyReservation(state); err != nil {
			return replicaInstallResult{}, err
		}
		if err := requireMissingReplicaTarget(absTarget); err != nil {
			return replicaInstallResult{}, replicaPurchaseConflict(state, "the Website Replica target appeared before payment completed").WithCause(err)
		}
		order, err = client.CreateWebsiteReplicaOrder(ctx, api.CreateWebsiteReplicaOrderRequest{
			QuoteID: state.QuoteID, ClientRequestID: state.OrderRequestID, Locale: state.Locale,
		})
		if err != nil {
			if output.AsError(err).Subtype == "PRODUCT_ALREADY_OWNED" {
				return installOwnedReplica(ctx, runtime, store, state, client, shortCode, absTarget)
			}
			if output.AsError(err).Subtype == "QUOTE_EXPIRED" {
				return refreshReplicaQuote(ctx, runtime, store, state, code, shortCode, absTarget, locale, timeout, interval, confirm)
			}
			return replicaInstallResult{}, err
		}
		if state.OrderNo != "" && state.OrderNo != order.OrderNo {
			return replicaInstallResult{}, replicaPurchaseConflict(state, "the idempotent Website Replica order identity changed")
		}
		if order.Status == "PENDING" {
			if err := validateReplicaPaymentAction(order.PaymentAction); err != nil {
				return replicaInstallResult{}, err
			}
		}
		state.OrderNo = order.OrderNo
		state.OrderExpiresAt = order.ExpiresAt
		if err := store.save(&state); err != nil {
			return replicaInstallResult{}, err
		}
	}
	if order.Status == "PENDING" && state.PaymentPresentedAt == "" {
		presentation, err := prepareReplicaPaymentPresentation(runtime, order)
		if err != nil {
			return replicaInstallResult{}, err
		}
		state.PaymentPresentedAt = runtime.deps.Now().UTC().Format(time.RFC3339Nano)
		if err := store.save(&state); err != nil {
			return replicaInstallResult{}, err
		}
		return replicaInstallResult{}, replicaPaymentConfirmation(state, presentation)
	}
	if err := waitForReplicaPayment(ctx, runtime, client, order, timeout, interval); err != nil {
		if output.AsError(err).Subtype == "REPLICA_PAYMENT_TERMINAL" {
			_ = store.retire(state)
		}
		return replicaInstallResult{}, err
	}
	return installReplicaDownload(ctx, runtime, store, state, client, shortCode, order.OrderNo, absTarget)
}

func refreshReplicaQuote(
	ctx context.Context,
	runtime *Runtime,
	store replicaPurchaseStore,
	state replicaPurchaseState,
	code string,
	shortCode string,
	target string,
	locale string,
	timeout time.Duration,
	interval time.Duration,
	confirm bool,
) (replicaInstallResult, error) {
	if err := store.retire(state); err != nil {
		return replicaInstallResult{}, err
	}
	return installReplicaLocked(ctx, runtime, store, code, shortCode, target, locale, timeout, interval, confirm)
}

func replicaQuoteConfirmation(state replicaPurchaseState) error {
	return output.Confirmation(
		"REPLICA_PURCHASE_CONFIRMATION_REQUIRED",
		"confirm the Website Replica quote before creating an order",
	).WithDetails(map[string]any{
		"replicaId":        state.ReplicaID,
		"replicaCode":      "VICEME-REPLICA:" + state.ShortCode,
		"productId":        state.ProductID,
		"title":            state.ProductTitle,
		"currency":         state.Currency,
		"totalAmountCents": state.PriceCents,
		"quoteId":          state.QuoteID,
		"expiresAt":        state.QuoteExpiresAt,
		"target":           state.Target,
	}).WithHint("show the exact product, price, and quote expiry to the user; only after explicit confirmation rerun the same install command with --confirm")
}

func replicaPaymentConfirmation(state replicaPurchaseState, presentation *api.CommercePaymentPresentation) error {
	return output.Confirmation(
		"REPLICA_PAYMENT_REQUIRED",
		"render the Website Replica payment QR before waiting for payment",
	).WithDetails(map[string]any{
		"nextAction":          "PRESENT_PAYMENT_QR",
		"orderNo":             state.OrderNo,
		"currency":            state.Currency,
		"totalAmountCents":    state.PriceCents,
		"expiresAt":           state.OrderExpiresAt,
		"paymentPresentation": presentation,
	}).WithHint("render only paymentPresentation.imagePath as a Markdown image, show the order number, then rerun the same confirmed install command with a bounded --timeout")
}

func installOwnedReplica(
	ctx context.Context,
	runtime *Runtime,
	store replicaPurchaseStore,
	state replicaPurchaseState,
	client *api.Client,
	shortCode string,
	target string,
) (replicaInstallResult, error) {
	return installReplicaDownload(ctx, runtime, store, state, client, shortCode, "", target)
}

func installReplicaDownload(
	ctx context.Context,
	runtime *Runtime,
	store replicaPurchaseStore,
	state replicaPurchaseState,
	client *api.Client,
	shortCode string,
	expectedOrderNo string,
	target string,
) (replicaInstallResult, error) {
	if err := store.verifyReservation(state); err != nil {
		return replicaInstallResult{}, err
	}
	download, err := client.GetWebsiteReplicaDownload(ctx, shortCode)
	if err != nil {
		return replicaInstallResult{}, err
	}
	return installReplicaDownloaded(ctx, runtime, store, state, client, download, expectedOrderNo, target)
}

func tryInstallRecoveredReplica(
	ctx context.Context,
	runtime *Runtime,
	store replicaPurchaseStore,
	state replicaPurchaseState,
	client *api.Client,
	target string,
) (replicaInstallResult, bool, error) {
	result, err := installReplicaRecoveredDownload(ctx, runtime, store, state, client, state.OrderNo, target)
	if err != nil && output.AsError(err).Subtype == "WEBSITE_REPLICA_NOT_FOUND" {
		return replicaInstallResult{}, false, nil
	}
	return result, err == nil, err
}

func installReplicaRecoveredDownload(
	ctx context.Context,
	runtime *Runtime,
	store replicaPurchaseStore,
	state replicaPurchaseState,
	client *api.Client,
	expectedOrderNo string,
	target string,
) (replicaInstallResult, error) {
	if err := store.verifyReservation(state); err != nil {
		return replicaInstallResult{}, err
	}
	download, err := client.RecoverWebsiteReplicaDownload(ctx, api.RecoverWebsiteReplicaDownloadRequest{
		OrderNo: expectedOrderNo, RecoverySecret: state.DownloadRecoverySecret,
	})
	if err != nil {
		return replicaInstallResult{}, err
	}
	return installReplicaDownloaded(ctx, runtime, store, state, client, download, expectedOrderNo, target)
}

func installReplicaDownloaded(
	ctx context.Context,
	runtime *Runtime,
	store replicaPurchaseStore,
	state replicaPurchaseState,
	client *api.Client,
	download api.WebsiteReplicaDownload,
	expectedOrderNo string,
	target string,
) (replicaInstallResult, error) {
	if err := validateReplicaDownloadBinding(download, state.ReplicaID); err != nil {
		return replicaInstallResult{}, err
	}
	claims, err := verifiedReplicaLicenseClaims(ctx, runtime, download, expectedOrderNo)
	if err != nil {
		return replicaInstallResult{}, err
	}
	installed, err := downloadAndInstallReplica(ctx, client, store, state, download, claims, target)
	if err != nil {
		return replicaInstallResult{}, err
	}
	result := replicaInstallResult{
		ReplicaID: download.ReplicaID, VersionID: download.VersionID, Version: download.Version,
		OrderNo: claims.OrderNo, Target: installed.Target, ArtifactDigest: download.ArtifactDigest,
		LicensePath: installed.LicensePath, FileCount: installed.FileCount, ExpandedBytes: installed.ExpandedBytes,
	}
	if err := store.saveCompletion(result); err != nil {
		return replicaInstallResult{}, err
	}
	if state.SessionID == "" {
		if err := completeReplicaInstallation(ctx, client, result, claims); err != nil {
			return replicaInstallResult{}, err
		}
	}
	if err := store.retire(state); err != nil {
		return replicaInstallResult{}, err
	}
	return result, nil
}

func installRecordedPaidReplica(
	ctx context.Context,
	runtime *Runtime,
	store replicaPurchaseStore,
	target string,
	reportInstallation bool,
) (replicaInstallResult, bool, error) {
	var result replicaInstallResult
	var exists bool
	err := store.withPaidLock(func() error {
		paid, found, err := store.loadPaidLocked()
		if err != nil || !found {
			return err
		}
		exists = true
		if err := requireMissingReplicaTarget(target); err != nil {
			return err
		}
		download := api.WebsiteReplicaDownload{
			ReplicaID: paid.ReplicaID, VersionID: paid.VersionID, Version: paid.Version,
			FileName: "source.zip", SizeBytes: paid.SizeBytes, ArtifactDigest: paid.ArtifactDigest, License: paid.License,
		}
		claims, err := verifiedReplicaLicenseClaims(ctx, runtime, download, paid.OrderNo)
		if err != nil {
			return err
		}
		if err := verifyRecordedReplicaArchive(store.paidArchiveFilename, paid.SizeBytes, paid.ArtifactDigest); err != nil {
			return err
		}
		installed, err := replicacontent.InstallArchiveAnchored(store.paidArchiveFilename, target, store.targetParentID, replicacontent.LicenseRecord{
			ReplicaID: paid.ReplicaID, VersionID: paid.VersionID, Version: paid.Version,
			ArtifactDigest: paid.ArtifactDigest, License: paid.License,
		})
		if err != nil {
			return output.Internal("REPLICA_INSTALL_FAILED", "could not atomically install the recorded paid Website Replica", err)
		}
		result = replicaInstallResult{
			ReplicaID: paid.ReplicaID, VersionID: paid.VersionID, Version: paid.Version, OrderNo: paid.OrderNo,
			Target: installed.Target, ArtifactDigest: paid.ArtifactDigest, LicensePath: installed.LicensePath,
			FileCount: installed.FileCount, ExpandedBytes: installed.ExpandedBytes,
		}
		if err := store.saveCompletion(result); err != nil {
			return err
		}
		if reportInstallation {
			if err := completeReplicaInstallation(ctx, runtime.client(), result, claims); err != nil {
				return err
			}
		}
		if state, active, err := store.load(); err != nil {
			return err
		} else if active {
			return store.retire(state)
		}
		return nil
	})
	return result, exists, err
}

func validateReplicaCompletion(ctx context.Context, runtime *Runtime, completion replicaCompletionState) (api.WebsiteReplicaLicenseClaims, error) {
	result := completion.Result
	tree, err := replicacontent.InspectInstalledTree(result.Target)
	if err != nil || tree.FileCount != result.FileCount || tree.ExpandedBytes != result.ExpandedBytes || tree.Digest != completion.TreeDigest {
		return api.WebsiteReplicaLicenseClaims{}, output.Policy("REPLICA_COMPLETION_TARGET_INVALID", "Website Replica completion target no longer matches its receipt").WithCause(err)
	}
	record, err := replicacontent.ReadInstalledLicenseRecord(result.Target)
	if err != nil {
		return api.WebsiteReplicaLicenseClaims{}, output.Policy("REPLICA_COMPLETION_TARGET_INVALID", "Website Replica completion target no longer matches its receipt").WithCause(err)
	}
	if record.ReplicaID != result.ReplicaID || record.VersionID != result.VersionID || record.Version != result.Version ||
		record.ArtifactDigest != result.ArtifactDigest {
		return api.WebsiteReplicaLicenseClaims{}, output.Policy("REPLICA_COMPLETION_TARGET_INVALID", "Website Replica completion target no longer matches its receipt")
	}
	return verifiedReplicaLicenseClaims(ctx, runtime, api.WebsiteReplicaDownload{
		ReplicaID:      result.ReplicaID,
		VersionID:      result.VersionID,
		Version:        result.Version,
		ArtifactDigest: result.ArtifactDigest,
		License:        record.License,
	}, result.OrderNo)
}

func completeReplicaInstallation(
	ctx context.Context,
	client *api.Client,
	result replicaInstallResult,
	claims api.WebsiteReplicaLicenseClaims,
) error {
	receipt, err := client.CompleteWebsiteReplicaInstallation(ctx, api.CompleteWebsiteReplicaInstallationRequest{
		EntitlementID: claims.EntitlementID,
		VersionID:     result.VersionID,
	})
	if err != nil {
		return err
	}
	if receipt.ReplicaID != result.ReplicaID || receipt.VersionID != result.VersionID || receipt.Version != result.Version {
		return invalidReplicaResponse("Website Replica installation receipt does not match the installed artifact")
	}
	return nil
}

func replicaResolutionMatchesState(resolved api.WebsiteReplicaResolution, state replicaPurchaseState) bool {
	return resolved.ReplicaID == state.ReplicaID && resolved.Product.ID == state.ProductID &&
		resolved.Product.SKUID == state.SKUID && resolved.Product.Title == state.ProductTitle &&
		resolved.Product.Currency == state.Currency && resolved.Product.PriceCents == state.PriceCents
}

func replicaQuoteMatchesState(quote api.WebsiteReplicaQuote, state replicaPurchaseState) bool {
	return quote.Product.ID == state.ProductID && quote.Product.Title == state.ProductTitle &&
		quote.SKU.ID == state.SKUID && quote.SKU.Code == "default" &&
		len(quote.SKU.SelectedOptions) == 0 && quote.Currency == state.Currency &&
		quote.UnitAmountCents == state.PriceCents
}

func validateReplicaDownloadBinding(download api.WebsiteReplicaDownload, replicaID string) error {
	if download.ReplicaID != replicaID || download.SizeBytes < 1 || download.SizeBytes > replicacontent.MaxArchiveBytes ||
		!validReplicaDownloadFileName(download.FileName) {
		return invalidReplicaResponse("Website Replica download does not match the purchased artifact")
	}
	return nil
}

func validReplicaDownloadFileName(value string) bool {
	extension := filepath.Ext(value)
	return value != "" && value != "." && value != ".." && len(value) > len(extension) &&
		!strings.ContainsAny(value, "/\\\x00") && strings.EqualFold(extension, ".zip")
}

func invalidReplicaResponse(reason string) error {
	return output.Internal(
		"RESPONSE_INVALID",
		"ViceMe API returned an incomplete or invalid response",
		errors.New(reason),
	)
}

func verifyReplicaLicense(ctx context.Context, runtime *Runtime, download api.WebsiteReplicaDownload, orderNo string) error {
	_, err := verifiedReplicaLicenseClaims(ctx, runtime, download, orderNo)
	return err
}

func verifiedReplicaLicenseClaims(ctx context.Context, runtime *Runtime, download api.WebsiteReplicaDownload, orderNo string) (api.WebsiteReplicaLicenseClaims, error) {
	if len(download.License) == 0 || len(download.License) > 64<<10 {
		return api.WebsiteReplicaLicenseClaims{}, output.Policy("REPLICA_LICENSE_INVALID", "Website Replica license is missing or too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(download.License))
	decoder.DisallowUnknownFields()
	var license api.WebsiteReplicaLicense
	if err := decoder.Decode(&license); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return api.WebsiteReplicaLicenseClaims{}, output.Policy("REPLICA_LICENSE_INVALID", "Website Replica license has an invalid schema")
	}
	claims := license.Claims
	if license.Algorithm != "Ed25519" || license.SigningKeyID == "" || license.SigningPublicKey == "" || license.Signature == "" ||
		claims.SchemaVersion != replicaLicenseTermsVersion || claims.LicenseTermsVersion != replicaLicenseTermsVersion ||
		!replicaUUIDPattern.MatchString(claims.EntitlementID) || claims.ReplicaID != download.ReplicaID ||
		claims.VersionID != download.VersionID || claims.Version != download.Version || (orderNo != "" && claims.OrderNo != orderNo) ||
		claims.ArtifactDigest != download.ArtifactDigest {
		return api.WebsiteReplicaLicenseClaims{}, output.Policy("REPLICA_LICENSE_IDENTITY_MISMATCH", "Website Replica license does not match the purchased artifact")
	}
	trustedPublicKey, err := runtime.resolveCommerceTrustKey(ctx, license.SigningKeyID)
	if err != nil {
		return api.WebsiteReplicaLicenseClaims{}, err
	}
	if trustedPublicKey != license.SigningPublicKey {
		return api.WebsiteReplicaLicenseClaims{}, output.Policy("REPLICA_LICENSE_SIGNING_KEY_UNTRUSTED", "Website Replica license embedded an untrusted signing key")
	}
	if err := commerceartifact.VerifyDocument(claims, trustedPublicKey, license.Signature); err != nil {
		return api.WebsiteReplicaLicenseClaims{}, output.Policy("REPLICA_LICENSE_SIGNATURE_INVALID", "Website Replica license signature is invalid").WithCause(err)
	}
	return claims, nil
}

func parseReplicaCode(code string) (string, error) {
	const prefix = "VICEME-REPLICA:"
	if !strings.HasPrefix(code, prefix) {
		return "", output.Validation("REPLICA_CODE_INVALID", "Replica code must match VICEME-REPLICA:VMR-[A-Z0-9]{20}")
	}
	shortCode := strings.TrimPrefix(code, prefix)
	if !replicaShortCodePattern.MatchString(shortCode) || code != prefix+shortCode {
		return "", output.Validation("REPLICA_CODE_INVALID", "Replica code must match VICEME-REPLICA:VMR-[A-Z0-9]{20}")
	}
	return shortCode, nil
}

func validateReplicaTarget(target string) (string, error) {
	if strings.TrimSpace(target) == "" {
		return "", output.Validation("REPLICA_TARGET_INVALID", "Website Replica target must be a new directory")
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", output.Validation("REPLICA_TARGET_INVALID", "could not resolve --target").WithCause(err)
	}
	absTarget = filepath.Clean(absTarget)
	parent, err := filepath.EvalSymlinks(filepath.Dir(absTarget))
	if err != nil {
		return "", output.Validation("REPLICA_TARGET_PARENT_INVALID", "--target parent must already exist").WithCause(err)
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return "", output.Validation("REPLICA_TARGET_PARENT_INVALID", "could not resolve --target parent").WithCause(err)
	}
	parent = filepath.Clean(parent)
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", output.Validation("REPLICA_TARGET_PARENT_INVALID", "--target parent must be a real existing directory").WithCause(err)
	}
	return filepath.Join(parent, filepath.Base(absTarget)), nil
}

func requireMissingReplicaTarget(absTarget string) error {
	if _, err := os.Lstat(absTarget); err == nil {
		return output.Policy("REPLICA_TARGET_EXISTS", "refusing to overwrite the existing Replica target").WithDetails(map[string]any{"target": absTarget})
	} else if !errors.Is(err, fs.ErrNotExist) {
		return output.Validation("REPLICA_TARGET_INVALID", "could not inspect --target").WithCause(err)
	}
	return nil
}

func (runtime *Runtime) newReplicaRequestID() (string, error) {
	requestID := runtime.deps.NewID()
	if !replicaUUIDPattern.MatchString(requestID) {
		return "", output.Internal("REPLICA_CLIENT_REQUEST_ID_INVALID", "could not create a valid Replica request identity", nil)
	}
	return requestID, nil
}

func prepareReplicaPaymentPresentation(runtime *Runtime, order api.WebsiteReplicaOrder) (*api.CommercePaymentPresentation, error) {
	action := order.PaymentAction
	if err := validateReplicaPaymentAction(action); err != nil {
		return nil, err
	}
	presentation, err := newCommercePaymentPresentation(runtime, order.OrderNo, order.ExpiresAt, action.Content)
	if err != nil {
		return nil, output.Internal("REPLICA_PAYMENT_PRESENTATION_FAILED", "Website Replica payment QR image could not be prepared", err)
	}
	return presentation, nil
}

func validateReplicaPaymentAction(action *api.WebsiteReplicaPaymentAction) error {
	if action == nil {
		return invalidReplicaResponse("Website Replica order is missing its payment action")
	}
	if action.Type != "QR_CODE" || strings.TrimSpace(action.Content) == "" {
		return invalidReplicaResponse("Website Replica order did not return a WeChat QR_CODE payment action")
	}
	return nil
}

func waitForReplicaPayment(ctx context.Context, runtime *Runtime, client *api.Client, order api.WebsiteReplicaOrder, timeout, interval time.Duration) error {
	switch order.Status {
	case "PAID":
		_ = removeCommercePaymentPresentation(runtime, order.OrderNo)
		return nil
	case "PENDING":
	case "CLOSED", "FAILED", "CANCELLED":
		return output.Policy("REPLICA_PAYMENT_TERMINAL", "Website Replica payment did not complete")
	default:
		return output.Internal("REPLICA_ORDER_RESPONSE_INVALID", "Website Replica order status is invalid", nil)
	}
	deadline := runtime.deps.Now().Add(timeout)
	for {
		status, err := client.GetWebsiteReplicaOrderStatus(ctx, order.OrderNo)
		if err != nil {
			return err
		}
		switch status.Payment.Status {
		case "PAID":
			_ = removeCommercePaymentPresentation(runtime, order.OrderNo)
			return nil
		case "PENDING":
		case "CLOSED", "FAILED", "CANCELLED":
			_ = removeCommercePaymentPresentation(runtime, order.OrderNo)
			return output.Policy("REPLICA_PAYMENT_TERMINAL", "Website Replica payment did not complete").WithDetails(map[string]any{"orderNo": order.OrderNo, "status": status.Payment.Status})
		}
		if !runtime.deps.Now().Before(deadline) {
			pending := output.Network("REPLICA_PAYMENT_TIMEOUT", "Website Replica payment was not observed before the wait deadline", context.DeadlineExceeded)
			pending.WithDetails(map[string]any{
				"nextAction": "PAYMENT_PENDING",
				"orderNo":    order.OrderNo,
				"expiresAt":  order.ExpiresAt,
			})
			return pending
		}
		delay := interval
		if remaining := deadline.Sub(runtime.deps.Now()); remaining < delay {
			delay = remaining
		}
		if err := runtime.deps.Sleep(ctx, delay); err != nil {
			return output.Network("REPLICA_PAYMENT_INTERRUPTED", "Website Replica payment wait was interrupted", err)
		}
	}
}

func waitForReplicaSessionPayment(ctx context.Context, runtime *Runtime, client *api.Client, state replicaPurchaseState, timeout, interval time.Duration) error {
	deadline := runtime.deps.Now().Add(timeout)
	for {
		delay := interval
		if remaining := deadline.Sub(runtime.deps.Now()); remaining < delay {
			delay = remaining
		}
		if err := runtime.deps.Sleep(ctx, delay); err != nil {
			return output.Network("REPLICA_PAYMENT_INTERRUPTED", "Website Replica payment wait was interrupted", err)
		}
		status, err := client.GetWebsiteReplicaSessionOrderStatus(ctx, state.SessionID, state.SessionToken, state.OrderNo)
		if err != nil {
			return err
		}
		switch status.Payment.Status {
		case "PAID":
			_ = removeReplicaPaymentPresentation(runtime, state)
			return nil
		case "PENDING":
		case "CLOSED", "FAILED", "CANCELLED":
			_ = removeReplicaPaymentPresentation(runtime, state)
			return output.Policy("REPLICA_PAYMENT_TERMINAL", "Website Replica payment did not complete").WithDetails(map[string]any{"orderNo": state.OrderNo, "status": status.Payment.Status})
		default:
			return invalidReplicaResponse("Website Replica order status is invalid")
		}
		if !runtime.deps.Now().Before(deadline) {
			return output.Network("REPLICA_PAYMENT_TIMEOUT", "Website Replica payment was not observed before the wait deadline", context.DeadlineExceeded).WithDetails(map[string]any{
				"nextAction": "PAYMENT_PENDING", "orderNo": state.OrderNo, "expiresAt": state.OrderExpiresAt,
			})
		}
	}
}

func replicaPaymentPresentationPath(state replicaPurchaseState) string {
	return filepath.Join(filepath.Dir(state.Target), replicaPaymentImagePrefix+commercePaymentPresentationFilename(state.OrderNo))
}

func removeReplicaPaymentPresentation(runtime *Runtime, state replicaPurchaseState) error {
	workspaceErr := os.Remove(replicaPaymentPresentationPath(state))
	if errors.Is(workspaceErr, os.ErrNotExist) {
		workspaceErr = nil
	}
	return errors.Join(workspaceErr, removeCommercePaymentPresentation(runtime, state.OrderNo))
}

func verifyRecordedReplicaArchive(filename string, sizeBytes int64, artifactDigest string) error {
	if err := privatepath.RequirePrivateFile(filename); err != nil {
		return output.Policy("REPLICA_PAID_ARCHIVE_INVALID", "recorded paid Website Replica source is unavailable").WithCause(err)
	}
	before, err := os.Lstat(filename)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() != sizeBytes {
		return output.Policy("REPLICA_PAID_ARCHIVE_INVALID", "recorded paid Website Replica source is invalid").WithCause(err)
	}
	file, err := os.Open(filename)
	if err != nil {
		return output.Policy("REPLICA_PAID_ARCHIVE_INVALID", "recorded paid Website Replica source is unavailable").WithCause(err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(hash, io.LimitReader(file, sizeBytes+1))
	after, statErr := file.Stat()
	closeErr := file.Close()
	expected, digestErr := hex.DecodeString(artifactDigest)
	if copyErr != nil || statErr != nil || closeErr != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) ||
		written != sizeBytes || digestErr != nil || len(expected) != sha256.Size || subtle.ConstantTimeCompare(hash.Sum(nil), expected) != 1 {
		return output.Policy("REPLICA_PAID_ARCHIVE_INVALID", "recorded paid Website Replica source no longer matches its receipt").WithCause(errors.Join(copyErr, statErr, closeErr, digestErr))
	}
	return nil
}

func downloadAndInstallReplica(
	ctx context.Context,
	client *api.Client,
	store replicaPurchaseStore,
	state replicaPurchaseState,
	download api.WebsiteReplicaDownload,
	claims api.WebsiteReplicaLicenseClaims,
	target string,
) (replicacontent.InstallResult, error) {
	if download.SizeBytes < 1 || download.SizeBytes > replicacontent.MaxArchiveBytes || !validReplicaDownloadFileName(download.FileName) {
		return replicacontent.InstallResult{}, output.Internal("REPLICA_DOWNLOAD_RESPONSE_INVALID", "Website Replica download metadata is invalid", nil)
	}
	parent := filepath.Dir(target)
	parentInfo, err := os.Lstat(parent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return replicacontent.InstallResult{}, output.Internal("REPLICA_DOWNLOAD_STAGE_FAILED", "the Replica target parent changed before download", err)
	}
	if err := requireReplicaTargetParentIdentity(parent, state.TargetParentID); err != nil {
		return replicacontent.InstallResult{}, err
	}
	temporary, err := privatepath.CreateTempFile(parent, ".viceme-replica-download-*.zip")
	if err != nil {
		return replicacontent.InstallResult{}, output.Internal("REPLICA_DOWNLOAD_STAGE_FAILED", "could not create a private Replica download file", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	hash := sha256.New()
	written, downloadErr := client.DownloadPresigned(ctx, download.DownloadURL, io.MultiWriter(temporary, hash), download.SizeBytes)
	closeErr := temporary.Close()
	if downloadErr != nil {
		return replicacontent.InstallResult{}, downloadErr
	}
	if closeErr != nil {
		return replicacontent.InstallResult{}, output.Internal("REPLICA_DOWNLOAD_STAGE_FAILED", "could not close the Replica download file", closeErr)
	}
	if written != download.SizeBytes {
		return replicacontent.InstallResult{}, output.Policy("REPLICA_DOWNLOAD_SIZE_MISMATCH", "downloaded Website Replica length does not match its authorization")
	}
	actualDigest := hash.Sum(nil)
	expectedDigest, err := hex.DecodeString(download.ArtifactDigest)
	if err != nil || len(expectedDigest) != sha256.Size || subtle.ConstantTimeCompare(actualDigest, expectedDigest) != 1 {
		return replicacontent.InstallResult{}, output.Policy("REPLICA_DOWNLOAD_DIGEST_MISMATCH", "downloaded Website Replica digest does not match its authorization")
	}
	if state.PriceCents > 0 {
		if err := store.savePaid(temporaryName, download, claims.OrderNo); err != nil {
			return replicacontent.InstallResult{}, err
		}
	}
	if err := requireReplicaTargetParentIdentity(parent, state.TargetParentID); err != nil {
		return replicacontent.InstallResult{}, err
	}
	result, err := replicacontent.InstallArchiveAnchored(temporaryName, target, state.TargetParentID, replicacontent.LicenseRecord{
		ReplicaID: download.ReplicaID, VersionID: download.VersionID, Version: download.Version,
		ArtifactDigest: download.ArtifactDigest, License: download.License,
	})
	if err != nil {
		return replicacontent.InstallResult{}, output.Policy("REPLICA_ARCHIVE_UNSAFE", "downloaded Website Replica could not be installed safely").WithCause(err)
	}
	if err := requireReplicaTargetParentIdentity(parent, state.TargetParentID); err != nil {
		return replicacontent.InstallResult{}, err
	}
	return result, nil
}

func requireReplicaTargetParentIdentity(parent, expected string) error {
	current, err := pathidentity.Directory(parent)
	if err != nil || current != expected {
		return output.Policy("REPLICA_TARGET_PARENT_CHANGED", "the Website Replica target parent changed after it was reserved").WithCause(err)
	}
	return nil
}
