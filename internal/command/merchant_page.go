package command

import (
	"bytes"
	"context"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/pagepackage"
	"github.com/spf13/cobra"
)

var (
	pageCreatorHandlePattern   = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	pageWorkSlugPattern        = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	pageUUIDPattern            = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	pageReservedCreatorHandles = map[string]struct{}{
		"api": {}, "admin": {}, "auth": {}, "agent": {}, "works": {}, "orders": {},
		"messages": {}, "history": {}, "settings": {}, "login": {}, "checkout": {},
		"hosted-checkout": {}, "payment-result": {}, "cli": {}, "creator": {},
		"merchant-onboarding": {}, "wishlist": {}, "me": {}, "run": {}, "share": {},
		"public-view": {}, "terms": {}, "privacy": {},
	}
	pageReservedWorkSlugs = map[string]struct{}{
		"works": {}, "skills": {}, "manage": {}, "posts": {}, "about": {},
	}
)

type pagePreviewResult struct {
	Release    api.PageCustomizationRelease `json:"release"`
	Preview    api.PageCustomizationPreview `json:"preview"`
	SourcePath string                       `json:"sourcePath"`
	FileCount  int                          `json:"fileCount"`
}

type pageUploadResult struct {
	Release    api.PageCustomizationRelease `json:"release"`
	SourcePath string                       `json:"sourcePath"`
	FileCount  int                          `json:"fileCount"`
}

func newMerchantPageCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "page", Short: "Preview, publish, and roll back custom creator and Work pages"}
	command.AddCommand(newMerchantPageDescribeCommand(runtime))
	command.AddCommand(newMerchantPageInspectCommand(runtime))
	command.AddCommand(newMerchantPageUploadCommand(runtime))
	command.AddCommand(newMerchantPagePreviewCommand(runtime))
	command.AddCommand(newMerchantPageStatusCommand(runtime))
	command.AddCommand(newMerchantPageReleaseCommand(runtime, "publish"))
	command.AddCommand(newMerchantPageReleaseCommand(runtime, "activate"))
	return command
}

func newMerchantPageDescribeCommand(runtime *Runtime) *cobra.Command {
	var targetURL, merchantAccountID string
	command := &cobra.Command{
		Use:   "describe",
		Short: "Describe the platform data and actions available to one page target",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			target, err := parsePageTargetURL(targetURL)
			if err != nil {
				return err
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			merchant, err := resolvePageCustomizationMerchant(command.Context(), runtime, merchantAccountID, &target)
			if err != nil {
				return err
			}
			result, err := runtime.client().DescribePageCustomizationTarget(command.Context(), merchant.ID, target)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&targetURL, "target", "", "exact ViceMe creator or Work URL")
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID; optional when exactly one active account exists")
	_ = command.MarkFlagRequired("target")
	return command
}

func newMerchantPageUploadCommand(runtime *Runtime) *cobra.Command {
	var source, targetURL, merchantAccountID string
	command := &cobra.Command{
		Use:   "upload",
		Short: "Upload and validate a page without creating an online preview",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			target, err := parsePageTargetURL(targetURL)
			if err != nil {
				return err
			}
			pkg, err := inspectPagePackageForTarget(source, target)
			if err != nil {
				return err
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			merchant, err := resolvePageCustomizationMerchant(command.Context(), runtime, merchantAccountID, &target)
			if err != nil {
				return err
			}
			release, err := uploadPageCustomization(command.Context(), runtime, pkg, merchant.ID, target)
			if err != nil {
				return err
			}
			return runtime.business(pageUploadResult{Release: release, SourcePath: pkg.SourcePath, FileCount: pkg.FileCount})
		},
	}
	command.Flags().StringVar(&source, "path", "", "custom page ZIP")
	command.Flags().StringVar(&targetURL, "target", "", "exact ViceMe creator or Work URL")
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID; optional when exactly one eligible page tenant exists")
	_ = command.MarkFlagRequired("path")
	_ = command.MarkFlagRequired("target")
	return command
}

func newMerchantPageInspectCommand(runtime *Runtime) *cobra.Command {
	var source string
	command := &cobra.Command{
		Use:   "inspect",
		Short: "Validate a custom page ZIP without side effects",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			pkg, err := pagepackage.Inspect(source)
			if err != nil {
				return err
			}
			return runtime.business(pkg)
		},
	}
	command.Flags().StringVar(&source, "path", "", "custom page ZIP")
	_ = command.MarkFlagRequired("path")
	return command
}

func newMerchantPagePreviewCommand(runtime *Runtime) *cobra.Command {
	var source, targetURL, merchantAccountID string
	var expiresInSeconds int
	command := &cobra.Command{
		Use:   "preview",
		Short: "Upload a validated page draft and create a real-route preview",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if expiresInSeconds < 60 || expiresInSeconds > 3600 {
				return output.Validation("PAGE_PREVIEW_TTL_INVALID", "--expires-in must be between 60 and 3600 seconds")
			}
			target, err := parsePageTargetURL(targetURL)
			if err != nil {
				return err
			}
			pkg, err := inspectPagePackageForTarget(source, target)
			if err != nil {
				return err
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantAccountID)
			if err != nil {
				return err
			}
			release, err := uploadPageCustomization(command.Context(), runtime, pkg, merchant.ID, target)
			if err != nil {
				return err
			}
			preview, err := runtime.client().CreatePageCustomizationPreview(command.Context(), release.ID, merchant.ID, expiresInSeconds)
			if err != nil {
				return err
			}
			return runtime.business(pagePreviewResult{Release: release, Preview: preview, SourcePath: pkg.SourcePath, FileCount: pkg.FileCount})
		},
	}
	command.Flags().StringVar(&source, "path", "", "custom page ZIP")
	command.Flags().StringVar(&targetURL, "target", "", "exact ViceMe creator or Work URL")
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID; optional when exactly one active account exists")
	command.Flags().IntVar(&expiresInSeconds, "expires-in", 900, "preview lifetime in seconds")
	_ = command.MarkFlagRequired("path")
	_ = command.MarkFlagRequired("target")
	return command
}

func newMerchantPageStatusCommand(runtime *Runtime) *cobra.Command {
	var targetURL, merchantAccountID string
	command := &cobra.Command{
		Use:   "status",
		Short: "List the active and recent releases for one exact page target",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			target, err := parsePageTargetURL(targetURL)
			if err != nil {
				return err
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			merchant, err := resolvePageCustomizationMerchant(command.Context(), runtime, merchantAccountID, &target)
			if err != nil {
				return err
			}
			result, err := runtime.client().GetPageCustomizationState(command.Context(), merchant.ID, target)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&targetURL, "target", "", "exact ViceMe creator or Work URL")
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID; optional when exactly one active account exists")
	_ = command.MarkFlagRequired("target")
	return command
}

func newMerchantPageReleaseCommand(runtime *Runtime, action string) *cobra.Command {
	var merchantAccountID, expectedActive string
	short := "Publish an exact validated custom page release"
	if action == "activate" {
		short = "Roll back by activating an exact historical page release"
	}
	command := &cobra.Command{
		Use:   action + " <release-id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !pageUUIDPattern.MatchString(strings.ToLower(args[0])) {
				return output.Validation("PAGE_RELEASE_ID_INVALID", "release ID must be a UUID")
			}
			expected, err := parseExpectedActiveRelease(expectedActive)
			if err != nil {
				return err
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			merchant, err := resolvePageCustomizationMerchant(command.Context(), runtime, merchantAccountID, nil)
			if err != nil {
				return err
			}
			result, err := runtime.client().PublishPageCustomization(command.Context(), args[0], merchant.ID, expected, action)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID; optional when exactly one active account exists")
	command.Flags().StringVar(&expectedActive, "expected-active", "", "currently active release UUID, or 'none'")
	_ = command.MarkFlagRequired("expected-active")
	return command
}

func inspectPagePackageForTarget(source string, target api.PageCustomizationTarget) (pagepackage.Package, error) {
	pkg, err := pagepackage.Inspect(source)
	if err != nil {
		return pagepackage.Package{}, err
	}
	expectedKind := "CreatorPage"
	if target.Type == "WORK" {
		expectedKind = "WorkPage"
	}
	if pkg.Manifest.Kind != expectedKind {
		return pagepackage.Package{}, output.Validation("PAGE_MANIFEST_TARGET_MISMATCH", "the page manifest kind does not match --target")
	}
	if target.Type == "CREATOR" {
		for _, capability := range pkg.Manifest.Spec.Capabilities {
			if capability == "work.like" || capability == "comments.read" || capability == "comments.write" || capability == "checkout.open" {
				return pagepackage.Package{}, output.Validation("PAGE_MANIFEST_CAPABILITY_MISMATCH", "creator pages cannot request Work-only capabilities")
			}
		}
	}
	return pkg, nil
}

func uploadPageCustomization(ctx context.Context, runtime *Runtime, pkg pagepackage.Package, merchantAccountID string, target api.PageCustomizationTarget) (api.PageCustomizationRelease, error) {
	client := runtime.client()
	created, err := client.CreatePageCustomizationDraft(ctx, api.CreatePageCustomizationDraftRequest{
		ClientRequestID: runtime.deps.NewID(), ContractVersion: api.PageCustomizationContractVersion,
		CLIVersion: buildinfo.Version, MerchantAccountID: merchantAccountID, Target: target, Artifact: pkg.Artifact,
	})
	if err != nil {
		return api.PageCustomizationRelease{}, err
	}
	authorization, err := client.AuthorizePageCustomizationUpload(ctx, created.Release.ID, merchantAccountID)
	if err != nil {
		return api.PageCustomizationRelease{}, err
	}
	if err := client.PutPresigned(ctx, authorization.UploadURL, authorization.Headers, bytes.NewReader(pkg.Bytes), int64(len(pkg.Bytes))); err != nil {
		return api.PageCustomizationRelease{}, err
	}
	return client.CompletePageCustomizationUpload(ctx, created.Release.ID, merchantAccountID)
}

func resolvePageCustomizationMerchant(ctx context.Context, runtime *Runtime, requestedID string, target *api.PageCustomizationTarget) (api.MerchantAccount, error) {
	accounts, err := runtime.client().ListMerchantAccounts(ctx)
	if err != nil {
		return api.MerchantAccount{}, err
	}
	requestedID = strings.TrimSpace(requestedID)
	active := make([]api.MerchantAccount, 0, len(accounts.Items))
	var selected *api.MerchantAccount
	for index := range accounts.Items {
		account := &accounts.Items[index]
		if account.Status == "ACTIVE" {
			active = append(active, *account)
		}
		if requestedID != "" && account.ID == requestedID {
			selected = account
		}
	}
	if selected != nil && selected.Status == "ACTIVE" {
		return *selected, nil
	}
	if requestedID == "" && len(active) == 1 {
		return active[0], nil
	}
	if requestedID != "" && selected == nil {
		return api.MerchantAccount{}, output.Authorization("MERCHANT_REQUIRED", "the selected page tenant is not owned by the current login").WithDetails(map[string]any{"merchantAccountId": requestedID})
	}

	current, err := runtime.client().GetMerchantOnboarding(ctx)
	if err != nil {
		return api.MerchantAccount{}, err
	}
	pending := current.Merchant
	validPending := pending != nil && pending.Status == "SUSPENDED" &&
		current.Onboarding != nil && current.Onboarding.Kind == "APPLICATION" &&
		current.Onboarding.MerchantAccountID != nil && *current.Onboarding.MerchantAccountID == pending.ID &&
		(current.Onboarding.Status == "SUBMITTED" || current.Onboarding.Status == "UNDER_REVIEW" || current.Onboarding.Status == "NEEDS_MORE_EVIDENCE") &&
		current.CreatorIdentity != nil && current.CreatorIdentity.Status == "DRAFT" && pending.CreatorAccountID != nil &&
		current.NextAction == "WAIT_FOR_REVIEW"
	if validPending && target != nil {
		validPending = target.Type == "CREATOR" && target.CreatorHandle == current.CreatorIdentity.Handle
	}
	if validPending && (requestedID == "" || pending.ID == requestedID) {
		return *pending, nil
	}
	if requestedID != "" {
		return api.MerchantAccount{}, output.Authorization("PAGE_CUSTOMIZATION_MERCHANT_REQUIRED", "the selected Merchant cannot customize this page while inactive").WithDetails(map[string]any{"merchantAccountId": requestedID})
	}
	if len(active) > 1 {
		return api.MerchantAccount{}, output.Validation("MERCHANT_SELECTION_REQUIRED", "multiple active Merchant accounts are available; select one explicitly").WithDetails(map[string]any{"merchants": active})
	}
	return api.MerchantAccount{}, output.Authorization("PAGE_CUSTOMIZATION_MERCHANT_REQUIRED", "an active Merchant or pending creator page tenant is required")
}

func parsePageTargetURL(value string) (api.PageCustomizationTarget, error) {
	targetURL, err := url.ParseRequestURI(strings.TrimSpace(value))
	if err != nil || targetURL.Scheme == "" || targetURL.Host == "" || targetURL.User != nil || targetURL.RawQuery != "" || targetURL.Fragment != "" || (targetURL.Scheme != "https" && !(targetURL.Scheme == "http" && isLoopbackHost(targetURL.Hostname()))) {
		return api.PageCustomizationTarget{}, output.Validation("PAGE_TARGET_INVALID", "--target must be an HTTPS ViceMe creator or Work URL without query or fragment; loopback HTTP is allowed for development")
	}
	segments := strings.Split(strings.Trim(targetURL.EscapedPath(), "/"), "/")
	for index, segment := range segments {
		decoded, decodeErr := url.PathUnescape(segment)
		if decodeErr != nil || decoded != segment {
			return api.PageCustomizationTarget{}, output.Validation("PAGE_TARGET_INVALID", "--target path must use canonical unescaped route segments")
		}
		segments[index] = decoded
	}
	_, creatorReserved := pageReservedCreatorHandles[segments[0]]
	if len(segments) < 1 || len(segments) > 2 || len(segments[0]) < 2 || len(segments[0]) > 32 || !pageCreatorHandlePattern.MatchString(segments[0]) || creatorReserved {
		return api.PageCustomizationTarget{}, output.Validation("PAGE_TARGET_INVALID", "--target must identify one canonical creator or Work route")
	}
	target := api.PageCustomizationTarget{Type: "CREATOR", CreatorHandle: segments[0]}
	if len(segments) == 2 {
		_, workReserved := pageReservedWorkSlugs[segments[1]]
		if len(segments[1]) < 2 || len(segments[1]) > 64 || !pageWorkSlugPattern.MatchString(segments[1]) || workReserved {
			return api.PageCustomizationTarget{}, output.Validation("PAGE_TARGET_INVALID", "--target contains an invalid Work slug")
		}
		target.Type = "WORK"
		target.WorkSlug = segments[1]
	}
	return target, nil
}

func parseExpectedActiveRelease(value string) (*string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "none" {
		return nil, nil
	}
	if !pageUUIDPattern.MatchString(normalized) {
		return nil, output.Validation("PAGE_EXPECTED_ACTIVE_INVALID", "--expected-active must be a release UUID or 'none'")
	}
	return &normalized, nil
}

func isLoopbackHost(host string) bool {
	address := net.ParseIP(host)
	return strings.EqualFold(host, "localhost") || (address != nil && address.IsLoopback())
}
