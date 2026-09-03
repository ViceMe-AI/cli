package command

import (
	"bytes"
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

func newMerchantPageCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "page", Short: "Preview, publish, and roll back custom creator and Work pages"}
	command.AddCommand(newMerchantPageInspectCommand(runtime))
	command.AddCommand(newMerchantPagePreviewCommand(runtime))
	command.AddCommand(newMerchantPageStatusCommand(runtime))
	command.AddCommand(newMerchantPageReleaseCommand(runtime, "publish"))
	command.AddCommand(newMerchantPageReleaseCommand(runtime, "activate"))
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
			pkg, err := pagepackage.Inspect(source)
			if err != nil {
				return err
			}
			expectedKind := "CreatorPage"
			if target.Type == "WORK" {
				expectedKind = "WorkPage"
			}
			if pkg.Manifest.Kind != expectedKind {
				return output.Validation("PAGE_MANIFEST_TARGET_MISMATCH", "the page manifest kind does not match --target")
			}
			if target.Type == "CREATOR" {
				for _, capability := range pkg.Manifest.Spec.Capabilities {
					if capability == "work.like" || capability == "comments.read" || capability == "comments.write" || capability == "checkout.open" {
						return output.Validation("PAGE_MANIFEST_CAPABILITY_MISMATCH", "creator pages cannot request Work-only capabilities")
					}
				}
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantAccountID)
			if err != nil {
				return err
			}
			client := runtime.client()
			created, err := client.CreatePageCustomizationDraft(command.Context(), api.CreatePageCustomizationDraftRequest{
				ClientRequestID: runtime.deps.NewID(), ContractVersion: api.PageCustomizationContractVersion,
				CLIVersion: buildinfo.Version, MerchantAccountID: merchant.ID, Target: target, Artifact: pkg.Artifact,
			})
			if err != nil {
				return err
			}
			authorization, err := client.AuthorizePageCustomizationUpload(command.Context(), created.Release.ID, merchant.ID)
			if err != nil {
				return err
			}
			if err := client.PutPresigned(command.Context(), authorization.UploadURL, authorization.Headers, bytes.NewReader(pkg.Bytes), int64(len(pkg.Bytes))); err != nil {
				return err
			}
			release, err := client.CompletePageCustomizationUpload(command.Context(), created.Release.ID, merchant.ID)
			if err != nil {
				return err
			}
			preview, err := client.CreatePageCustomizationPreview(command.Context(), release.ID, merchant.ID, expiresInSeconds)
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
			merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantAccountID)
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
			merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantAccountID)
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
