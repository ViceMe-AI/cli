package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/pagepackage"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/privatepath"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
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

type pageSourceRestoreResult struct {
	Target           string                                  `json:"target"`
	ReleaseID        string                                  `json:"releaseId"`
	ReleaseVersion   int                                     `json:"releaseVersion"`
	Digest           string                                  `json:"digest"`
	ConcurrencyToken *string                                 `json:"concurrencyToken"`
	Template         *api.PageCustomizationTemplateReference `json:"template"`
	FileCount        int                                     `json:"fileCount"`
	ExpandedBytes    uint64                                  `json:"expandedBytes"`
}

func newMerchantPageCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "page", Short: "Preview, publish, and roll back custom creator and Work pages"}
	command.AddCommand(newMerchantPageDescribeCommand(runtime))
	command.AddCommand(newMerchantPageInspectCommand(runtime))
	command.AddCommand(newMerchantPageUploadCommand(runtime))
	command.AddCommand(newMerchantPagePreviewCommand(runtime))
	command.AddCommand(newMerchantPageStatusCommand(runtime))
	command.AddCommand(newMerchantPageSourceCommand(runtime))
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
	var source, editableSource, targetURL, merchantAccountID, templateID, templateVersion string
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
			snapshot, template, err := freezePageSource(target, editableSource, templateID, templateVersion)
			if err != nil {
				return err
			}
			if snapshot != nil {
				defer snapshot.Cleanup()
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			merchant, err := resolvePageCustomizationMerchant(command.Context(), runtime, merchantAccountID, &target)
			if err != nil {
				return err
			}
			release, err := uploadPageCustomization(command.Context(), runtime, pkg, snapshot, template, merchant.ID, target)
			if err != nil {
				return err
			}
			return runtime.business(pageUploadResult{Release: release, SourcePath: pkg.SourcePath, FileCount: pkg.FileCount})
		},
	}
	command.Flags().StringVar(&source, "path", "", "custom page ZIP")
	command.Flags().StringVar(&editableSource, "source", "", "editable custom-page project directory")
	command.Flags().StringVar(&templateID, "template-id", "", "source template ID")
	command.Flags().StringVar(&templateVersion, "template-version", "", "source template version")
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
	var source, editableSource, targetURL, merchantAccountID, templateID, templateVersion string
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
			snapshot, template, err := freezePageSource(target, editableSource, templateID, templateVersion)
			if err != nil {
				return err
			}
			if snapshot != nil {
				defer snapshot.Cleanup()
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantAccountID)
			if err != nil {
				return err
			}
			release, err := uploadPageCustomization(command.Context(), runtime, pkg, snapshot, template, merchant.ID, target)
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
	command.Flags().StringVar(&editableSource, "source", "", "editable custom-page project directory")
	command.Flags().StringVar(&templateID, "template-id", "", "source template ID")
	command.Flags().StringVar(&templateVersion, "template-version", "", "source template version")
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

func newMerchantPageSourceCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "source", Short: "Check or restore the private editable source for a custom page"}
	command.AddCommand(newMerchantPageSourceStatusCommand(runtime))
	command.AddCommand(newMerchantPageSourceRestoreCommand(runtime))
	return command
}

func newMerchantPageSourceStatusCommand(runtime *Runtime) *cobra.Command {
	var targetURL, merchantAccountID string
	command := &cobra.Command{
		Use:   "status",
		Short: "Check whether the active custom page has restorable source",
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
			status, err := runtime.client().GetPageCustomizationSourceStatus(command.Context(), merchant.ID, target)
			if err != nil {
				return err
			}
			if status.Target != target {
				return output.Internal("PAGE_SOURCE_TARGET_MISMATCH", "page source status does not match the requested target", nil)
			}
			return runtime.business(status)
		},
	}
	command.Flags().StringVar(&targetURL, "target", "", "exact ViceMe creator or Work URL")
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID; optional when exactly one eligible page tenant exists")
	_ = command.MarkFlagRequired("target")
	return command
}

func newMerchantPageSourceRestoreCommand(runtime *Runtime) *cobra.Command {
	var targetURL, merchantAccountID, destination string
	command := &cobra.Command{
		Use:   "restore",
		Short: "Restore the active custom page source into a new local directory",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			target, err := parsePageTargetURL(targetURL)
			if err != nil {
				return err
			}
			if strings.TrimSpace(destination) == "" {
				return output.Validation("PAGE_SOURCE_DESTINATION_REQUIRED", "--destination must be a new local directory")
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			merchant, err := resolvePageCustomizationMerchant(command.Context(), runtime, merchantAccountID, &target)
			if err != nil {
				return err
			}
			status, err := runtime.client().GetPageCustomizationSourceStatus(command.Context(), merchant.ID, target)
			if err != nil {
				return err
			}
			if status.Target != target {
				return output.Internal("PAGE_SOURCE_TARGET_MISMATCH", "page source status does not match the requested target", nil)
			}
			if status.Availability != "RESTORABLE" || status.Source == nil || status.ConcurrencyToken == nil {
				return output.Validation("PAGE_SOURCE_NOT_RESTORABLE", "the active custom page does not have a restorable source snapshot").WithDetails(map[string]any{"availability": status.Availability})
			}
			archive, err := runtime.client().DownloadPageCustomizationSource(command.Context(), status.Source.ReleaseID, merchant.ID)
			if err != nil {
				return err
			}
			if int64(len(archive)) != status.Source.SizeBytes {
				return output.Internal("PAGE_SOURCE_SIZE_MISMATCH", "the downloaded custom page source size does not match the verified snapshot", nil)
			}
			digest := sha256.Sum256(archive)
			if !strings.EqualFold(hex.EncodeToString(digest[:]), status.Source.Digest) {
				return output.Internal("PAGE_SOURCE_DIGEST_MISMATCH", "the downloaded custom page source digest does not match the verified snapshot", nil)
			}
			temporaryDirectory, err := privatepath.CreateTempDirectory(os.TempDir(), ".viceme-page-source-restore-*")
			if err != nil {
				return output.Internal("PAGE_SOURCE_TEMP_FAILED", "could not create a private temporary directory for custom page source", err)
			}
			defer os.RemoveAll(temporaryDirectory)
			archivePath := filepath.Join(temporaryDirectory, "source.zip")
			if err := privatefile.Write(archivePath, archive, ".page-source-*.tmp"); err != nil {
				return output.Internal("PAGE_SOURCE_WRITE_FAILED", "could not stage the custom page source", err)
			}
			installed, err := replicacontent.RestoreOwnerSourceArchive(archivePath, destination)
			if err != nil {
				if errors.Is(err, os.ErrExist) {
					return output.Validation("PAGE_SOURCE_DESTINATION_EXISTS", "--destination must not already exist").WithCause(err)
				}
				return output.Validation("PAGE_SOURCE_RESTORE_FAILED", "custom page source could not be safely restored").WithCause(err)
			}
			return runtime.business(pageSourceRestoreResult{
				Target: installed.Target, ReleaseID: status.Source.ReleaseID, ReleaseVersion: status.Source.ReleaseVersion,
				Digest: status.Source.Digest, ConcurrencyToken: status.ConcurrencyToken, Template: status.Source.Template,
				FileCount: installed.FileCount, ExpandedBytes: installed.ExpandedBytes,
			})
		},
	}
	command.Flags().StringVar(&targetURL, "target", "", "exact ViceMe creator or Work URL")
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID; optional when exactly one eligible page tenant exists")
	command.Flags().StringVar(&destination, "destination", "", "new local directory for the restored editable project")
	_ = command.MarkFlagRequired("target")
	_ = command.MarkFlagRequired("destination")
	return command
}

func newMerchantPageReleaseCommand(runtime *Runtime, action string) *cobra.Command {
	var merchantAccountID, expectedActive, expectedConcurrency string
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
			result, err := runtime.client().PublishPageCustomization(command.Context(), args[0], merchant.ID, expected, strings.TrimSpace(expectedConcurrency), action)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID; optional when exactly one active account exists")
	command.Flags().StringVar(&expectedActive, "expected-active", "", "currently active release UUID, or 'none'")
	command.Flags().StringVar(&expectedConcurrency, "expected-concurrency", "", "concurrency token returned by page status")
	_ = command.MarkFlagRequired("expected-active")
	_ = command.MarkFlagRequired("expected-concurrency")
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

func uploadPageCustomization(ctx context.Context, runtime *Runtime, pkg pagepackage.Package, source *replicacontent.FrozenSourceArchive, template *api.PageCustomizationTemplateReference, merchantAccountID string, target api.PageCustomizationTarget) (api.PageCustomizationRelease, error) {
	client := runtime.client()
	request := api.CreatePageCustomizationDraftRequest{
		ClientRequestID: runtime.deps.NewID(), ContractVersion: api.PageCustomizationContractVersion,
		CLIVersion: buildinfo.Version, MerchantAccountID: merchantAccountID, Target: target, Artifact: pkg.Artifact,
	}
	if source != nil {
		request.SourceSnapshot = &api.PageCustomizationSourceSnapshotInput{
			Artifact: api.PageCustomizationArtifact{
				Digest: source.Summary.Digest, SizeBytes: source.Summary.SizeBytes,
				FileName: "custom-page-source.zip", ContentType: "application/zip",
			},
			Template: template,
		}
	}
	created, err := client.CreatePageCustomizationDraft(ctx, request)
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
	if source != nil {
		sourceAuthorization, err := client.AuthorizePageCustomizationSourceUpload(ctx, created.Release.ID, merchantAccountID)
		if err != nil {
			return api.PageCustomizationRelease{}, err
		}
		file, info, err := source.Open()
		if err != nil {
			return api.PageCustomizationRelease{}, output.Internal("PAGE_SOURCE_OPEN_FAILED", "could not open the frozen custom-page source", err)
		}
		defer func() { _ = file.Close() }()
		if err := client.PutPresigned(ctx, sourceAuthorization.UploadURL, sourceAuthorization.Headers, file, info.Size()); err != nil {
			return api.PageCustomizationRelease{}, err
		}
	}
	return client.CompletePageCustomizationUpload(ctx, created.Release.ID, merchantAccountID)
}

func freezePageSource(_ api.PageCustomizationTarget, sourcePath, templateID, templateVersion string) (*replicacontent.FrozenSourceArchive, *api.PageCustomizationTemplateReference, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	templateID = strings.TrimSpace(templateID)
	templateVersion = strings.TrimSpace(templateVersion)
	if sourcePath == "" {
		return nil, nil, output.Validation("PAGE_SOURCE_REQUIRED", "custom-page upload requires --source with the editable project directory")
	}
	sourceInfo, sourceErr := os.Lstat(sourcePath)
	if sourceErr != nil || !sourceInfo.IsDir() || sourceInfo.Mode()&os.ModeSymlink != 0 {
		return nil, nil, output.Validation("PAGE_SOURCE_INVALID", "--source must be a real editable project directory").WithCause(sourceErr)
	}
	if (templateID == "") != (templateVersion == "") {
		return nil, nil, output.Validation("PAGE_SOURCE_TEMPLATE_INVALID", "--template-id and --template-version must be provided together")
	}
	archive, err := replicacontent.FreezeSourceArchive(sourcePath, replicacontent.FreezeSourceOptions{
		Purpose:           "Continue editing this ViceMe custom page from its verified source snapshot.",
		ExpiresAt:         time.Now().Add(30 * time.Minute),
		OwnerOnlySnapshot: true,
	})
	if err != nil {
		return nil, nil, output.Validation("PAGE_SOURCE_INVALID", "custom-page source could not be safely frozen").WithCause(err)
	}
	var template *api.PageCustomizationTemplateReference
	if templateID != "" {
		template = &api.PageCustomizationTemplateReference{ID: templateID, Version: templateVersion}
	}
	return archive, template, nil
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
