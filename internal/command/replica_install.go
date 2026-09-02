package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
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

const replicaLicenseTermsVersion = "website-replica-license/v1"

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
	command := &cobra.Command{
		Use:   "install <replica-code>",
		Short: "Purchase and atomically install one Website Replica source package",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := installReplica(command.Context(), runtime, args[0], target, locale, timeout, interval)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&target, "target", "", "new destination directory for the Replica source")
	command.Flags().StringVar(&locale, "locale", "zh-CN", "localized checkout presentation: zh-CN or en-US")
	command.Flags().DurationVar(&timeout, "timeout", 20*time.Minute, "maximum time to wait for payment")
	command.Flags().DurationVar(&interval, "interval", 1500*time.Millisecond, "payment status poll interval")
	_ = command.MarkFlagRequired("target")
	return command
}

func installReplica(ctx context.Context, runtime *Runtime, code, target, locale string, timeout, interval time.Duration) (replicaInstallResult, error) {
	shortCode, err := parseReplicaCode(code)
	if err != nil {
		return replicaInstallResult{}, err
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
		result, installErr = installReplicaLocked(ctx, runtime, store, code, shortCode, absTarget, locale, timeout, interval)
		return installErr
	})
	return result, err
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
) (replicaInstallResult, error) {
	if err := runtime.requireWebsiteReplicaAuthentication(ctx, "website-replica:read", "website-replica:purchase"); err != nil {
		return replicaInstallResult{}, err
	}
	completion, completed, err := store.loadCompletion()
	if err != nil {
		return replicaInstallResult{}, err
	}
	if completed {
		if err := validateReplicaCompletion(ctx, runtime, completion); err != nil {
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
	state, exists, err := store.load()
	if err != nil {
		return replicaInstallResult{}, err
	}
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
	client := runtime.client()
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
		if err := store.save(&state); err != nil {
			return replicaInstallResult{}, err
		}
	}
	if state.OrderRequestID == "" {
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
				_ = store.retire(state)
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
	if order.Status == "PENDING" {
		if err := presentReplicaPayment(ctx, runtime, order); err != nil {
			return replicaInstallResult{}, err
		}
	}
	if err := waitForReplicaPayment(ctx, runtime, client, order, timeout, interval); err != nil {
		if output.AsError(err).Subtype == "REPLICA_PAYMENT_TERMINAL" {
			_ = store.retire(state)
		}
		return replicaInstallResult{}, err
	}
	return installReplicaDownload(ctx, runtime, store, state, client, shortCode, order.OrderNo, absTarget)
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
	if err := validateReplicaDownloadBinding(download, state.ReplicaID); err != nil {
		return replicaInstallResult{}, err
	}
	claims, err := verifiedReplicaLicenseClaims(ctx, runtime, download, expectedOrderNo)
	if err != nil {
		return replicaInstallResult{}, err
	}
	installed, err := downloadAndInstallReplica(ctx, client, download, target, state.TargetParentID)
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
	if err := store.retire(state); err != nil {
		return replicaInstallResult{}, err
	}
	return result, nil
}

func validateReplicaCompletion(ctx context.Context, runtime *Runtime, completion replicaCompletionState) error {
	result := completion.Result
	tree, err := replicacontent.InspectInstalledTree(result.Target)
	if err != nil || tree.FileCount != result.FileCount || tree.ExpandedBytes != result.ExpandedBytes || tree.Digest != completion.TreeDigest {
		return output.Policy("REPLICA_COMPLETION_TARGET_INVALID", "Website Replica completion target no longer matches its receipt").WithCause(err)
	}
	record, err := replicacontent.ReadInstalledLicenseRecord(result.Target)
	if err != nil {
		return output.Policy("REPLICA_COMPLETION_TARGET_INVALID", "Website Replica completion target no longer matches its receipt").WithCause(err)
	}
	if record.ReplicaID != result.ReplicaID || record.VersionID != result.VersionID || record.Version != result.Version ||
		record.ArtifactDigest != result.ArtifactDigest {
		return output.Policy("REPLICA_COMPLETION_TARGET_INVALID", "Website Replica completion target no longer matches its receipt")
	}
	return verifyReplicaLicense(ctx, runtime, api.WebsiteReplicaDownload{
		ReplicaID:      result.ReplicaID,
		VersionID:      result.VersionID,
		Version:        result.Version,
		ArtifactDigest: result.ArtifactDigest,
		License:        record.License,
	}, result.OrderNo)
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
		return "", output.Validation("REPLICA_TARGET_INVALID", "--target must be an explicit new directory")
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

func presentReplicaPayment(ctx context.Context, runtime *Runtime, order api.WebsiteReplicaOrder) error {
	action := order.PaymentAction
	if err := validateReplicaPaymentAction(action); err != nil {
		return err
	}
	var target string
	switch action.Type {
	case "REDIRECT":
		target = action.URL
		_, _ = fmt.Fprintln(runtime.deps.ErrOut, "Opening the temporary payment page. Waiting for payment...")
	case "QR_CODE":
		imagePath, err := createCommercePaymentQRImage(runtime, order.OrderNo, action.Content)
		if err != nil {
			return output.Internal("REPLICA_PAYMENT_ACTION_INVALID", "Website Replica order returned an invalid QR_CODE payment action", err)
		}
		target = imagePath
		_, _ = fmt.Fprintf(runtime.deps.ErrOut, "Open this private local QR image to complete payment:\n\n  %s\n\nWaiting for payment...\n", target)
	default:
		return invalidReplicaResponse("Website Replica order returned an unsupported payment action")
	}
	if err := runtime.deps.OpenURL(ctx, target); err != nil {
		if action.Type == "REDIRECT" {
			return output.Internal("REPLICA_PAYMENT_PRESENTATION_FAILED", "the temporary payment page could not be opened safely", nil).
				WithHint("rerun the same install command to reopen the original order")
		}
		_, _ = fmt.Fprintln(runtime.deps.ErrOut, "The payment presentation could not be opened automatically; use the temporary location shown above.")
	}
	return nil
}

func validateReplicaPaymentAction(action *api.WebsiteReplicaPaymentAction) error {
	if action == nil {
		return invalidReplicaResponse("Website Replica order is missing its payment action")
	}
	switch action.Type {
	case "REDIRECT":
		if !validReplicaPaymentURL(action.URL) || action.Content != "" {
			return invalidReplicaResponse("Website Replica order returned an invalid REDIRECT payment action")
		}
	case "QR_CODE":
		parsed, err := url.Parse(action.Content)
		if action.URL != "" || len(action.Content) == 0 || len(action.Content) > 4096 || err != nil || !strings.EqualFold(parsed.Scheme, "weixin") {
			return invalidReplicaResponse("Website Replica order returned an invalid QR_CODE payment action")
		}
	default:
		return invalidReplicaResponse("Website Replica order returned an unsupported payment action")
	}
	return nil
}

func validReplicaPaymentURL(value string) bool {
	if value == "" || len(value) > 8192 {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.IsAbs() && parsed.Hostname() != "" && parsed.User == nil &&
		(strings.EqualFold(parsed.Scheme, "https") || (strings.EqualFold(parsed.Scheme, "http") && isLoopbackOrigin(parsed.Scheme+"://"+parsed.Host)))
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
			pending.WithDetails(map[string]any{"orderNo": order.OrderNo})
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

func downloadAndInstallReplica(ctx context.Context, client *api.Client, download api.WebsiteReplicaDownload, target, targetParentID string) (replicacontent.InstallResult, error) {
	if download.SizeBytes < 1 || download.SizeBytes > replicacontent.MaxArchiveBytes || !validReplicaDownloadFileName(download.FileName) {
		return replicacontent.InstallResult{}, output.Internal("REPLICA_DOWNLOAD_RESPONSE_INVALID", "Website Replica download metadata is invalid", nil)
	}
	parent := filepath.Dir(target)
	parentInfo, err := os.Lstat(parent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return replicacontent.InstallResult{}, output.Internal("REPLICA_DOWNLOAD_STAGE_FAILED", "the Replica target parent changed before download", err)
	}
	if err := requireReplicaTargetParentIdentity(parent, targetParentID); err != nil {
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
	if err := requireReplicaTargetParentIdentity(parent, targetParentID); err != nil {
		return replicacontent.InstallResult{}, err
	}
	result, err := replicacontent.InstallArchiveAnchored(temporaryName, target, targetParentID, replicacontent.LicenseRecord{
		ReplicaID: download.ReplicaID, VersionID: download.VersionID, Version: download.Version,
		ArtifactDigest: download.ArtifactDigest, License: download.License,
	})
	if err != nil {
		return replicacontent.InstallResult{}, output.Policy("REPLICA_ARCHIVE_UNSAFE", "downloaded Website Replica could not be installed safely").WithCause(err)
	}
	if err := requireReplicaTargetParentIdentity(parent, targetParentID); err != nil {
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
