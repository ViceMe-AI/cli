package command

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	"github.com/spf13/cobra"
)

type downloadableSkillInstallResult struct {
	ProductID      string                     `json:"productId"`
	Edition        any                        `json:"edition"`
	ReleaseID      string                     `json:"releaseId"`
	ArtifactDigest string                     `json:"artifactDigest"`
	InstalledName  string                     `json:"installedName"`
	Install        skillcontent.InstallReport `json:"install"`
	Trial          *trialInstallSummary       `json:"trial,omitempty"`
	NextAction     string                     `json:"nextAction"`
	Invocation     string                     `json:"invocation"`
}

type downloadableSkillFile struct {
	Data []byte
	Mode os.FileMode
}

var skillUseProductIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func newSkillDetailCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "detail <product-id-or-work-url>", Short: "Show a Skill Work and all of its free or paid editions", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !skillUseProductIDPattern.MatchString(args[0]) {
				_, work, err := resolveSkillUseTarget(command.Context(), runtime, args[0])
				if err != nil {
					return err
				}
				return runtime.business(work)
			}
			result, err := runtime.client().GetSkillDetail(command.Context(), args[0])
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
}

func newSkillAccessCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "access <product-id-or-work-url>", Short: "Resolve free, purchased, or purchase-required access for one Skill edition", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			productID, _, err := resolveSkillUseTarget(command.Context(), runtime, args[0])
			if err != nil {
				return err
			}
			public, err := runtime.client().GetPublicSkillAccess(command.Context(), productID)
			if err != nil {
				return err
			}
			if public.IsFree {
				return runtime.business(public)
			}
			if !runtimeHasAuthentication(runtime) {
				return runtime.business(public)
			}
			owned, err := runtime.client().GetSkillAccess(command.Context(), productID)
			if err != nil {
				return err
			}
			return runtime.business(owned)
		},
	}
}

func newSkillInstallCommand(runtime *Runtime) *cobra.Command {
	var agent string
	var wait time.Duration
	command := &cobra.Command{
		Use: "install <product-id-or-work-url>", Short: "Verify and atomically install one free or purchased Skill edition", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			productID, work, err := resolveSkillUseTarget(command.Context(), runtime, args[0])
			if err != nil {
				return err
			}
			access, err := runtime.client().GetPublicSkillAccess(command.Context(), productID)
			if err != nil {
				return err
			}
			if !access.IsFree {
				if !access.Owned && !access.PurchaseAvailable {
					return output.Policy("SKILL_ACCESS_UNAVAILABLE", "this paid Skill edition is not available for purchase").WithDetails(map[string]any{"productId": productID})
				}
				// 试用优先:付费款开放试用时,匿名装试用版,用满再走购买。
				workSlugForTrial := ""
				if work != nil {
					workSlugForTrial = work.Work.Slug
				}
				if !access.Owned && access.Trial != nil && access.Trial.Available {
					return installTrialSkill(command.Context(), runtime, productID, workSlugForTrial, agent, access)
				}
				if err := runtime.requireSkillUseAuthentication(command.Context()); err != nil {
					return err
				}
				access, err = runtime.client().GetSkillAccess(command.Context(), productID)
				if err != nil {
					return err
				}
				if !access.Owned {
					if !access.PurchaseAvailable {
						return output.Policy("SKILL_ACCESS_UNAVAILABLE", "this paid Skill edition is not available for purchase").WithDetails(map[string]any{"productId": productID})
					}
					if err := runtime.requireBuyerAuthentication(command.Context()); err != nil {
						return err
					}
					orderValue, err := openSkillPurchaseOrder(command.Context(), runtime, productID)
					if err != nil {
						return err
					}
					order := &orderValue
					presentation, err := presentSkillPaymentQR(runtime, order)
					if err != nil {
						return err
					}
					if wait <= 0 && order.Status != "PAID" {
						details := map[string]any{
							"productId": productID, "orderNo": order.OrderNo,
							"amountCents": order.AmountCents, "expiresAt": order.ExpiresAt,
							"paymentPresentation": presentation,
							"edition":             access.Edition, "subscription": access.Subscription,
						}
						hint := "present the payment QR to the user, then rerun the same install command with --wait while the payment is in progress"
						if access.Subscription.Available {
							hint = "present the payment QR to the user, or subscribe to the creator with `viceme subscription subscribe <creator-handle>` to unlock every paid Skill of theirs; rerun with --wait while the payment is in progress"
						}
						return output.Confirmation("SKILL_PURCHASE_REQUIRED", "purchase this edition before installation").WithDetails(details).WithHint(hint)
					}
					paymentWait := wait
					if paymentWait <= 0 {
						paymentWait = time.Second
					}
					if err := waitForSkillOrderPayment(command.Context(), runtime, productID, order.OrderNo, paymentWait); err != nil {
						return err
					}
					access, err = runtime.client().GetSkillAccess(command.Context(), productID)
					if err != nil {
						return err
					}
					if !access.Owned {
						return output.Authentication("SKILL_PURCHASE_PENDING", "payment was not observed before the wait deadline").WithDetails(map[string]any{"productId": productID, "orderNo": order.OrderNo}).WithHint("after the user finishes the scan payment, rerun the same install command; the pending order is recovered instead of duplicated")
					}
				}
			}
			var download api.DownloadURL
			if access.IsFree {
				download, err = runtime.client().GetFreeSkillDownload(command.Context(), productID)
			} else {
				download, err = runtime.client().GetOwnedSkillDownload(command.Context(), productID)
			}
			if err != nil {
				return err
			}
			if download.ReleaseID != access.Release.ID || download.ArtifactDigest != access.Release.ArtifactDigest {
				return output.Policy("SKILL_DOWNLOAD_RECEIPT_MISMATCH", "download authorization does not match the authorized Skill release")
			}
			artifact, err := runtime.client().DownloadArtifact(command.Context(), download.URL)
			if err != nil {
				return err
			}
			digest := fmt.Sprintf("%x", sha256.Sum256(artifact))
			if digest != download.ArtifactDigest {
				return output.Policy("SKILL_ARTIFACT_DIGEST_MISMATCH", "downloaded Skill package does not match the active release")
			}
			files, err := extractDownloadableSkill(artifact)
			if err != nil {
				return err
			}
			workSlug := ""
			if work != nil {
				workSlug = work.Work.Slug
			}
			manifestName, err := downloadableSkillManifestName(files)
			if err != nil {
				return err
			}
			installedName := downloadableSkillName(
				productID,
				manifestName,
				access.Edition.Title,
				workSlug,
			)
			report, err := installDownloadableSkill(installedName, agent, files, runtime.deps.Environment, skillcontent.SkillProvenance{
				ProductID: productID,
				ReleaseID: access.Release.ID,
			})
			if err != nil {
				return err
			}
			if !report.AllSucceeded {
				return output.Internal("SKILL_INSTALL_FAILED", "one or more Skill targets could not be installed", nil).WithDetails(map[string]any{"report": report})
			}
			return runtime.business(downloadableSkillInstallResult{ProductID: productID, Edition: access.Edition, ReleaseID: access.Release.ID, ArtifactDigest: digest, InstalledName: installedName, Install: report, NextAction: "CONTINUE_ORIGINAL_TASK_WITH_INSTALLED_SKILL", Invocation: "$" + installedName})
		},
	}
	command.Flags().StringVar(&agent, "agent", "auto", "installation target: auto, codex, claude, workbuddy, or agents")
	command.Flags().DurationVar(&wait, "wait", 5*time.Minute, "wait up to this duration for the WeChat QR payment of a paid edition; 0 presents the QR without waiting")
	return command
}

func resolveSkillUseTarget(ctx context.Context, runtime *Runtime, target string) (string, *api.PublicWorkProjection, error) {
	target = strings.TrimSpace(target)
	if skillUseProductIDPattern.MatchString(target) {
		return target, nil, nil
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return "", nil, output.Validation("SKILL_TARGET_INVALID", "Skill target must be a Product ID or canonical Work URL")
	}
	if parsed.IsAbs() && parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", nil, output.Validation("SKILL_TARGET_INVALID", "canonical Work URL must use HTTP or HTTPS")
	}
	segments := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".md"), "/"), "/")
	if len(segments) == 3 && (segments[0] == "zh-CN" || segments[0] == "en-US") {
		segments = segments[1:]
	}
	if len(segments) != 2 || segments[0] == "" || segments[1] == "" {
		return "", nil, output.Validation("SKILL_WORK_URL_INVALID", "canonical Work URL must contain /<creator-handle>/<work-slug>")
	}
	work, err := runtime.client().GetPublicWork(ctx, segments[0], segments[1])
	if err != nil {
		return "", nil, err
	}
	products := append([]api.PublicWorkProduct(nil), work.Work.Products...)
	sort.Slice(products, func(left, right int) bool {
		leftOrder, rightOrder := int(^uint(0)>>1), int(^uint(0)>>1)
		if products[left].Edition != nil {
			leftOrder = products[left].Edition.SortOrder
		}
		if products[right].Edition != nil {
			rightOrder = products[right].Edition.SortOrder
		}
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return products[left].ID < products[right].ID
	})
	if values, present := parsed.Query()["product"]; present {
		requested := ""
		if len(values) == 1 {
			requested = values[0]
		}
		if !skillUseProductIDPattern.MatchString(requested) {
			return "", &work, output.Validation("SKILL_EDITION_SELECTOR_INVALID", "the explicit Product selector must contain one valid Product ID")
		}
		for _, product := range products {
			if product.ID == requested {
				if !isDownloadableWorkProduct(product) {
					return "", &work, output.Validation("SKILL_EDITION_NOT_INSTALLABLE", "the requested Product is not a downloadable Skill edition")
				}
				return product.ID, &work, nil
			}
		}
		return "", &work, output.Validation("SKILL_EDITION_NOT_IN_WORK", "the requested Product does not belong to this Work")
	}
	for _, product := range products {
		if product.IsFree && product.InstallKind != nil && *product.InstallKind == "PUBLIC_FREE" && isDownloadableWorkProduct(product) {
			return product.ID, &work, nil
		}
	}
	for _, product := range products {
		if isDownloadableWorkProduct(product) {
			return product.ID, &work, nil
		}
	}
	return "", &work, output.Validation("SKILL_WORK_HAS_NO_EDITIONS", "the Work does not expose an installable Skill edition")
}

func isDownloadableWorkProduct(product api.PublicWorkProduct) bool {
	if product.InstallKind == nil || product.ActiveRelease == nil || product.Edition == nil {
		return false
	}
	switch *product.InstallKind {
	case "PUBLIC_FREE", "OWNED_PAID", "PURCHASE_REQUIRED":
		return true
	default:
		return false
	}
}

func runtimeHasAuthentication(runtime *Runtime) bool {
	if _, source, _ := runtime.overrideCredential(); source != "" {
		return true
	}
	status, err := runtime.manager().CurrentStatus()
	return err == nil && status.Authenticated
}

func (runtime *Runtime) requireSkillUseAuthentication(ctx context.Context) error {
	if !runtimeHasAuthentication(runtime) {
		return output.Authentication("NOT_LOGGED_IN", "sign in before purchasing or reinstalling a paid Skill").WithHint("run 'viceme auth login' for the current profile")
	}
	status, err := runtime.client().AuthStatus(ctx)
	if err != nil {
		return err
	}
	for _, scope := range status.Scopes {
		if scope == "skill-use:read" {
			return nil
		}
	}
	return output.Authorization("SKILL_USE_SCOPE_REQUIRED", "the current login cannot access purchased Skills").WithHint("run 'viceme auth login' again to grant Skill-use access")
}

func extractDownloadableSkill(archive []byte) (map[string]downloadableSkillFile, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, output.Policy("SKILL_ARCHIVE_INVALID", "downloaded Skill package is not a valid ZIP")
	}
	files := make(map[string]downloadableSkillFile)
	var total int64
	for _, entry := range reader.File {
		name := strings.ReplaceAll(entry.Name, "\\", "/")
		if strings.HasSuffix(name, "/") {
			continue
		}
		mode := entry.Mode()
		if name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.HasPrefix(name, "../") || !mode.IsRegular() || mode&(os.ModeSymlink|os.ModeSetuid|os.ModeSetgid|os.ModeSticky|os.ModeDevice|os.ModeNamedPipe|os.ModeSocket) != 0 {
			return nil, output.Policy("SKILL_ARCHIVE_UNSAFE", "downloaded Skill package contains an unsafe path")
		}
		if len(files) >= 1000 || entry.UncompressedSize64 > 10<<20 {
			return nil, output.Policy("SKILL_ARCHIVE_LIMIT_EXCEEDED", "downloaded Skill package exceeds safe extraction limits")
		}
		stream, err := entry.Open()
		if err != nil {
			return nil, output.Policy("SKILL_ARCHIVE_INVALID", "downloaded Skill package could not be read")
		}
		data, readErr := io.ReadAll(io.LimitReader(stream, (10<<20)+1))
		_ = stream.Close()
		if readErr != nil || len(data) > 10<<20 {
			return nil, output.Policy("SKILL_ARCHIVE_LIMIT_EXCEEDED", "downloaded Skill file exceeds safe extraction limits")
		}
		total += int64(len(data))
		if total > 50<<20 {
			return nil, output.Policy("SKILL_ARCHIVE_LIMIT_EXCEEDED", "downloaded Skill package exceeds safe extraction limits")
		}
		installMode := os.FileMode(0o644)
		if mode.Perm()&0o111 != 0 {
			installMode = 0o755
		}
		files[name] = downloadableSkillFile{Data: data, Mode: installMode}
	}
	if _, exists := files["SKILL.md"]; !exists {
		return nil, output.Policy("SKILL_MANIFEST_MISSING", "downloaded Skill package does not contain root SKILL.md")
	}
	return files, nil
}

// downloadableSkillName keeps the author-facing identity: the package's own
// SKILL.md name comes first (the installer requires the directory name to
// match it), then the edition-title slug, the work slug for non-Latin
// titles, and finally a unique platform-scoped fallback.
func downloadableSkillName(
	productID, manifestName, editionTitle, workSlug string,
) string {
	if slug := slugifyInstallName(manifestName); slug != "" {
		return slug
	}
	if slug := slugifyInstallName(editionTitle); slug != "" {
		return slug
	}
	if workSlug != "" {
		return workSlug
	}
	compact := strings.ReplaceAll(productID, "-", "")
	if len(compact) > 8 {
		compact = compact[:8]
	}
	return "viceme-" + compact
}

func slugifyInstallName(title string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case character >= 'a' && character <= 'z',
			character >= '0' && character <= '9':
			builder.WriteRune(character)
		default:
			builder.WriteRune('-')
		}
	}
	slug := builder.String()
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	return strings.Trim(slug, "-")
}

// downloadableSkillManifestName reads the package's own identity without
// rewriting it; the frontmatter is validated but left byte-for-byte intact.
func downloadableSkillManifestName(
	files map[string]downloadableSkillFile,
) (string, error) {
	manifest, exists := files["SKILL.md"]
	if !exists {
		return "", output.Policy("SKILL_MANIFEST_MISSING", "downloaded Skill package does not contain root SKILL.md")
	}
	normalized := strings.ReplaceAll(string(manifest.Data), "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return "", output.Policy("SKILL_MANIFEST_INVALID", "downloaded SKILL.md does not contain YAML frontmatter")
	}
	closing := -1
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			closing = index
			break
		}
	}
	if closing < 0 {
		return "", output.Policy("SKILL_MANIFEST_INVALID", "downloaded SKILL.md frontmatter is not closed")
	}
	metadata := map[string]any{}
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:closing], "\n")), &metadata); err != nil {
		return "", output.Policy("SKILL_MANIFEST_INVALID", "downloaded SKILL.md frontmatter is invalid").WithCause(err)
	}
	name, _ := metadata["name"].(string)
	return strings.TrimSpace(name), nil
}

func installDownloadableSkill(stableName, target string, files map[string]downloadableSkillFile, environment skillcontent.Environment, provenance skillcontent.SkillProvenance) (skillcontent.InstallReport, error) {
	root, err := os.MkdirTemp("", "viceme-downloadable-skill-")
	if err != nil {
		return skillcontent.InstallReport{}, output.Internal("SKILL_STAGE_FAILED", "could not create a private Skill staging directory", err)
	}
	defer os.RemoveAll(root)
	skillRoot := filepath.Join(root, stableName)
	paths := make([]string, 0, len(files))
	for name := range files {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	for _, name := range paths {
		destination := filepath.Join(skillRoot, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return skillcontent.InstallReport{}, output.Internal("SKILL_STAGE_FAILED", "could not create a Skill staging path", err)
		}
		if err := os.WriteFile(destination, files[name].Data, files[name].Mode); err != nil {
			return skillcontent.InstallReport{}, output.Internal("SKILL_STAGE_FAILED", "could not stage a verified Skill file", err)
		}
	}
	if _, exists := files["agents/openai.yaml"]; !exists {
		if err := os.MkdirAll(filepath.Join(skillRoot, "agents"), 0o700); err != nil {
			return skillcontent.InstallReport{}, err
		}
		metadata := fmt.Sprintf("interface:\n  display_name: %q\n  short_description: %q\n  default_prompt: %q\n", stableName, "Use this verified ViceMe Skill edition", "Use $"+stableName+" to continue the current task.")
		if err := os.WriteFile(filepath.Join(skillRoot, "agents", "openai.yaml"), []byte(metadata), 0o600); err != nil {
			return skillcontent.InstallReport{}, err
		}
	}
	packageMetadata := fmt.Sprintf("{\n  \"schema_version\": 1,\n  \"skill_version\": %q,\n  \"minimum_cli_version\": %q,\n  \"cli_compatibility\": %q\n}\n", buildinfo.SkillVersion, buildinfo.MinimumCLIVersion, buildinfo.CLICompatibility)
	if err := os.WriteFile(filepath.Join(skillRoot, "skill-package.json"), []byte(packageMetadata), 0o600); err != nil {
		return skillcontent.InstallReport{}, err
	}
	bundle := skillcontent.New(os.DirFS(root))
	return bundle.InstallWithProvenance(stableName, target, environment, provenance), nil
}
