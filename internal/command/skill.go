package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/publication"
	"github.com/spf13/cobra"
)

type inspectResult struct {
	publication.Package
	PriceConfirmed bool `json:"priceConfirmed"`
}

type listingPrepareResult struct {
	api.PrepareSkillListingResponse
	SourceType             string              `json:"sourceType"`
	SourcePath             string              `json:"sourcePath"`
	CanonicalPackageDigest string              `json:"canonicalPackageDigest"`
	RequiresPrice          bool                `json:"requiresPrice"`
	Presentation           previewPresentation `json:"presentation"`
}

type listingGetResult struct {
	api.SkillListingPreview
	Presentation previewPresentation `json:"presentation"`
}

type publicationPresentationResult struct {
	api.SkillPublication
	PublicationID string `json:"publicationId"`
	// Resolution identifies the Work, not whether an edition is added or updated.
	// The manifest edition key and published editions identify that operation.
	Resolution    string              `json:"resolution"`
	RequiresPrice bool                `json:"requiresPrice"`
	Presentation  previewPresentation `json:"presentation"`
	// Warnings carry non-fatal outcomes such as a local recovery cleanup that
	// failed after the server already reached a terminal state.
	Warnings []string `json:"warnings,omitempty"`
}

type previewPresentation struct {
	Intent           string  `json:"intent"`
	OpenURL          *string `json:"openUrl,omitempty"`
	OpenURLExpiresAt *string `json:"openUrlExpiresAt,omitempty"`
	FallbackURL      string  `json:"fallbackUrl"`
	Mode             string  `json:"mode"`
}

func newSkillCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "skill", Short: "Inspect and publish a local Skill"}
	command.AddCommand(newSkillInspectCommand(runtime))
	command.AddCommand(newSkillListingCommand(runtime))
	command.AddCommand(newSkillPublishCommand(runtime))
	command.AddCommand(newSkillDetailCommand(runtime))
	command.AddCommand(newSkillAccessCommand(runtime))
	command.AddCommand(newSkillInstallCommand(runtime))
	command.AddCommand(newSkillUsePrecheckCommand(runtime))
	return command
}

func newSkillListingCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "listing", Short: "Prepare and recover a stable Skill listing"}
	command.AddCommand(newSkillListingPrepareCommand(runtime))
	command.AddCommand(newSkillListingGetCommand(runtime))
	command.AddCommand(newSkillListingBindCommand(runtime))
	return command
}

func newSkillListingPrepareCommand(runtime *Runtime) *cobra.Command {
	var source string
	var forceNew bool
	command := &cobra.Command{
		Use: "prepare", Short: "Create or recover the private owner preview for a Skill source", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			pkg, err := publication.Build(source)
			if err != nil {
				return err
			}
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			prepared, _, err := prepareSkillListing(command.Context(), runtime, pkg, forceNew, "")
			if err != nil {
				return err
			}
			return runtime.business(prepared)
		},
	}
	command.Flags().StringVar(&source, "path", "", "Skill directory or ZIP")
	command.Flags().BoolVar(&forceNew, "new-listing", false, "explicitly create a separate Listing even when content matches")
	_ = command.MarkFlagRequired("path")
	return command
}

func newSkillListingGetCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "get <listing-id>", Short: "Get the authoritative private preview state", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			result, err := runtime.client().GetSkillListingPreview(command.Context(), args[0])
			if err != nil {
				return err
			}
			presentation := createPreviewPresentation(command.Context(), runtime, result.ListingID, result.Preview.FallbackURL)
			return runtime.business(listingGetResult{SkillListingPreview: result, Presentation: presentation})
		},
	}
}

func newSkillListingBindCommand(runtime *Runtime) *cobra.Command {
	var source string
	command := &cobra.Command{
		Use: "bind <listing-id>", Short: "Explicitly bind a ZIP or workspace to an owned Listing", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			pkg, err := publication.Build(source)
			if err != nil {
				return err
			}
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			prepared, _, err := prepareSkillListing(command.Context(), runtime, pkg, true, args[0])
			if err != nil {
				return err
			}
			return runtime.business(prepared)
		},
	}
	command.Flags().StringVar(&source, "path", "", "Skill directory or ZIP")
	_ = command.MarkFlagRequired("path")
	return command
}

func newSkillInspectCommand(runtime *Runtime) *cobra.Command {
	var source string
	command := &cobra.Command{
		Use: "inspect", Short: "Validate a local Skill directory or ZIP without side effects", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			result, err := publication.Build(source)
			if err != nil {
				return err
			}
			return runtime.business(inspectResult{Package: result, PriceConfirmed: false})
		},
	}
	command.Flags().StringVar(&source, "path", "", "Skill directory or ZIP")
	_ = command.MarkFlagRequired("path")
	return command
}

func newSkillPublishCommand(runtime *Runtime) *cobra.Command {
	var source string
	var githubRepository string
	var githubRef string
	var githubPath string
	var xiaohongshuSkillID string
	var xiaohongshuSearch string
	var resume string
	var priceMinor int
	var trialUseLimit int
	var merchantAccountID string
	var editionKey string
	var editionTitle string
	var editionOrder int
	var editionHighlights []string
	var forceNew bool
	var listingID string
	command := &cobra.Command{
		Use: "publish", Short: "Upload a Skill and prepare its listing for explicit review", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			sourceCount := 0
			for _, candidate := range []string{source, githubRepository, xiaohongshuSkillID, xiaohongshuSearch} {
				if strings.TrimSpace(candidate) != "" {
					sourceCount++
				}
			}
			if resume != "" && (sourceCount != 0 || strings.TrimSpace(listingID) != "") {
				return output.Validation("PUBLICATION_FLAGS_CONFLICT", "--resume cannot be combined with a source or --listing")
			}
			if resume == "" && sourceCount != 1 {
				return output.Validation("SKILL_SOURCE_REQUIRED", "provide exactly one of --path, --github, or --xiaohongshu-skill-id")
			}
			if forceNew && strings.TrimSpace(listingID) != "" {
				return output.Validation("PUBLICATION_FLAGS_CONFLICT", "--new-listing cannot be combined with --listing")
			}
			if resume == "" && (!command.Flags().Changed("edition-key") || !command.Flags().Changed("edition-order")) {
				return output.Validation("SKILL_EDITION_SELECTION_REQUIRED", "publishing requires explicit --edition-key and --edition-order; reuse the selected Skill's key/order to update it, or use an unused key/order to add a separate Skill").WithHint("read the existing publication's editions and confirm whether the original free Skill must remain before publishing")
			}
			if resume != "" && (forceNew || command.Flags().Changed("edition-key") || command.Flags().Changed("edition-title") || command.Flags().Changed("edition-order") || command.Flags().Changed("edition-highlight")) {
				return output.Validation("PUBLICATION_FLAGS_CONFLICT", "--resume continues the same Skill; it cannot change the edition or create a new Listing")
			}
			priceConfirmed := command.Flags().Changed("price-minor")
			if priceConfirmed && (priceMinor < 0 || priceMinor > 10_000_000) {
				return output.Validation("SKILL_PRICE_INVALID", "priceMinor must be between 0 and 10000000")
			}
			// 试用次数:0=关闭,1~100 有效;免费款不允许开试用(确认层兜底)。
			trialConfirmed := command.Flags().Changed("trial-use-limit")
			if trialConfirmed && (trialUseLimit < 0 || trialUseLimit > 100) {
				return output.Validation("SKILL_TRIAL_USE_LIMIT_INVALID", "trialUseLimit must be between 0 (off) and 100")
			}
			if trialConfirmed && trialUseLimit > 0 && priceConfirmed && priceMinor <= 0 {
				return output.Validation("SKILL_TRIAL_USE_LIMIT_REQUIRES_PRICE", "a trial use limit requires a positive --price-minor").WithHint("pass a positive --price-minor together with --trial-use-limit")
			}
			trialLimit := (*int)(nil)
			if trialConfirmed {
				if trialUseLimit > 0 {
					value := trialUseLimit
					trialLimit = &value
				}
			}
			store := publication.PendingStore{Directory: filepath.Join(runtime.configBase, "publications"), Now: runtime.deps.Now}
			if resume != "" {
				pending, err := store.Load(resume)
				if err != nil {
					return err
				}
				var pkg publication.Package
				if pending.Source.Type == "GITHUB" || pending.Source.Type == "XIAOHONGSHU" {
					pkg, err = publication.BuildRemoteArchive(pending.SourcePath)
				} else {
					pkg, err = publication.Build(pending.SourcePath)
				}
				if err != nil {
					return err
				}
				pkg, err = publication.Customize(pkg, pending.Source, pending.Edition)
				if err != nil {
					return err
				}
				if pkg.Artifact.Digest != pending.ArtifactDigest {
					return output.Validation("PUBLICATION_SOURCE_CHANGED", "local Skill source changed after the publication started").WithHint("restore the original source or start a new publication")
				}
				if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
					return err
				}
				requestedMerchantID := pending.MerchantAccountID
				if merchantAccountID != "" {
					if merchantAccountID != pending.MerchantAccountID {
						return output.Validation("PUBLICATION_MERCHANT_CHANGED", "--merchant does not match the Merchant saved for this publication").WithHint("resume with the original Merchant account, or start a new publication")
					}
					requestedMerchantID = merchantAccountID
				}
				if _, err := resolveSkillPublicationMerchant(command.Context(), runtime, requestedMerchantID); err != nil {
					return err
				}
				if priceConfirmed {
					pending.PriceMinor = &priceMinor
					if trialConfirmed {
						pending.TrialUseLimit = trialLimit
					}
					if err := store.Save(pending); err != nil {
						return err
					}
				} else if trialConfirmed {
					pending.TrialUseLimit = trialLimit
					if err := store.Save(pending); err != nil {
						return err
					}
				}
				// An explicit resume continues Draft enrichment even while the
				// price is still unset. Price gates final confirmation, not media
				// upload or listing analysis.
				return continueSkillPublication(command.Context(), runtime, store, pending, pkg, nil, false, "UPDATE")
			}
			if source != "" {
				if _, err := publication.Build(source); err != nil {
					return err
				}
			}
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantAccountID)
			if err != nil {
				return err
			}
			pkg, manifestSource, edition, err := resolveSkillPublicationPackage(command.Context(), runtime, merchant.ID, source, githubRepository, githubRef, githubPath, xiaohongshuSkillID, xiaohongshuSearch, editionKey, editionTitle, editionOrder, editionHighlights)
			if err != nil {
				return err
			}
			prepared, _, err := prepareSkillListing(command.Context(), runtime, pkg, forceNew, strings.TrimSpace(listingID))
			if err != nil {
				return err
			}
			fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(
				runtime.profile.ID+"\x00"+prepared.ListingID+"\x00"+pkg.Artifact.Digest+"\x00"+pkg.Digest+"\x00"+
					merchant.ID+"\x00"+buildinfo.Version,
			)))
			intent, err := store.LoadOrCreateIntent(fingerprint, runtime.deps.NewID)
			if err != nil {
				return err
			}
			if intent.PublicationID != "" {
				pending := publication.Pending{
					PublicationID: intent.PublicationID, ClientRequestID: intent.ClientRequestID,
					MerchantAccountID: merchant.ID,
					Fingerprint:       fingerprint,
					SourcePath:        pkg.SourcePath, ArtifactDigest: pkg.Artifact.Digest,
					Source: manifestSource, Edition: edition,
				}
				if priceConfirmed {
					pending.PriceMinor = &priceMinor
				}
				if trialConfirmed {
					pending.TrialUseLimit = trialLimit
				}
				if err := store.Save(pending); err != nil {
					return err
				}
				return continueSkillPublication(command.Context(), runtime, store, pending, pkg, nil, pending.PriceMinor == nil, "UPDATE")
			}
			created, err := runtime.client().CreateSkillPublication(command.Context(), api.CreateSkillPublicationRequest{
				ClientRequestID: intent.ClientRequestID, ContractVersion: api.SkillPublicationContractVersion, CLIVersion: buildinfo.Version,
				Manifest: pkg.Manifest, ManifestDigest: pkg.Digest, Artifact: pkg.Artifact, ListingID: prepared.ListingID,
				MerchantAccountID: merchant.ID,
			})
			if err != nil {
				if output.AsError(err).Subtype != "SKILL_PUBLICATION_ALREADY_ACTIVE" {
					return err
				}
				preview, previewErr := runtime.client().GetSkillListingPreview(command.Context(), prepared.ListingID)
				if previewErr != nil || preview.Publication == nil {
					return err
				}
				current, currentErr := runtime.client().GetSkillPublication(command.Context(), preview.Publication.ID)
				if currentErr != nil {
					return currentErr
				}
				if current.MerchantAccountID != merchant.ID {
					return output.Authorization("PUBLICATION_MERCHANT_CHANGED", "the active publication belongs to another Merchant")
				}
				created = api.CreateSkillPublicationResponse{PublicationID: current.ID, ListingID: current.ListingID, MerchantAccountID: current.MerchantAccountID, DraftRevision: current.DraftRevision, Status: current.Status, Resolution: "UPDATE"}
			}
			if created.MerchantAccountID != merchant.ID {
				return output.Authorization("PUBLICATION_MERCHANT_CHANGED", "the publication response does not match the selected Merchant")
			}
			intent.PublicationID = created.PublicationID
			if err := store.SaveIntent(intent); err != nil {
				return err
			}
			pending := publication.Pending{
				PublicationID: created.PublicationID, ClientRequestID: intent.ClientRequestID,
				MerchantAccountID: merchant.ID,
				Fingerprint:       fingerprint,
				SourcePath:        pkg.SourcePath, ArtifactDigest: pkg.Artifact.Digest,
				Source: manifestSource, Edition: edition,
			}
			if priceConfirmed {
				pending.PriceMinor = &priceMinor
			}
			if trialConfirmed {
				pending.TrialUseLimit = trialLimit
			}
			if err := store.Save(pending); err != nil {
				return err
			}
			return continueSkillPublication(command.Context(), runtime, store, pending, pkg, created.PackageUpload, pending.PriceMinor == nil, created.Resolution)
		},
	}
	command.Flags().StringVar(&source, "path", "", "Skill directory or ZIP")
	command.Flags().StringVar(&githubRepository, "github", "", "personal GitHub repository as owner/name")
	command.Flags().StringVar(&githubRef, "github-ref", "HEAD", "Git ref to publish")
	command.Flags().StringVar(&githubPath, "github-path", "", "repository-relative directory containing SKILL.md")
	command.Flags().StringVar(&xiaohongshuSkillID, "xiaohongshu-skill-id", "", "Skill ID from the verified Xiaohongshu channel")
	command.Flags().StringVar(&xiaohongshuSearch, "xiaohongshu-search", "", "search verified Xiaohongshu Skills by ID or name")
	command.Flags().StringVar(&resume, "resume", "", "resume an interrupted publication by ID")
	command.Flags().IntVar(&priceMinor, "price-minor", 0, "set the CNY price in fen while continuing the private draft")
	command.Flags().IntVar(&trialUseLimit, "trial-use-limit", 0, "free trial uses before payment (1-100); 0 disables the trial")
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "Merchant account ID; required only when multiple active accounts exist")
	command.Flags().StringVar(&editionKey, "edition-key", "", "required stable lowercase edition key (except --resume)")
	command.Flags().StringVar(&editionTitle, "edition-title", "", "buyer-visible edition title; defaults to the Skill title")
	command.Flags().IntVar(&editionOrder, "edition-order", 0, "explicit edition display order")
	command.Flags().StringSliceVar(&editionHighlights, "edition-highlight", nil, "buyer-visible edition highlight; repeat or comma-separate")
	command.Flags().StringVar(&listingID, "listing", "", "existing Skill Listing ID for another edition of the same Work")
	command.Flags().BoolVar(&forceNew, "new-listing", false, "explicitly create a separate Listing even when content matches")
	return command
}

var publicationEditionKeyPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var githubRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func resolveSkillPublicationPackage(ctx context.Context, runtime *Runtime, merchantAccountID, localPath, githubRepository, githubRef, githubPath, xiaohongshuSkillID, xiaohongshuSearch, editionKey, editionTitle string, editionOrder int, editionHighlights []string) (publication.Package, api.SkillPublicationSource, api.SkillPublicationEdition, error) {
	editionKey = strings.TrimSpace(editionKey)
	if !publicationEditionKeyPattern.MatchString(editionKey) || len(editionKey) > 64 {
		return publication.Package{}, api.SkillPublicationSource{}, api.SkillPublicationEdition{}, output.Validation("SKILL_EDITION_KEY_INVALID", "--edition-key must be a lowercase kebab-case key up to 64 characters")
	}
	if editionOrder < 0 || editionOrder > 10_000 {
		return publication.Package{}, api.SkillPublicationSource{}, api.SkillPublicationEdition{}, output.Validation("SKILL_EDITION_ORDER_INVALID", "--edition-order must be between 0 and 10000")
	}

	source := api.SkillPublicationSource{}
	pathToBuild := localPath
	remotePackageDigest := ""
	if githubRepository != "" {
		repository := normalizeGithubRepository(githubRepository)
		if !githubRepositoryPattern.MatchString(repository) {
			return publication.Package{}, source, api.SkillPublicationEdition{}, output.Validation("GITHUB_REPOSITORY_INVALID", "--github must be owner/name or a github.com/owner/name URL")
		}
		githubRef = strings.TrimSpace(githubRef)
		if githubRef == "" {
			return publication.Package{}, source, api.SkillPublicationEdition{}, output.Validation("GITHUB_REF_INVALID", "--github-ref cannot be empty")
		}
		archive, err := runtime.client().DownloadGithubSkillSource(ctx, merchantAccountID, repository, githubRef, normalizeGithubPath(githubPath))
		if err != nil {
			return publication.Package{}, source, api.SkillPublicationEdition{}, err
		}
		pathToBuild, err = persistPublicationSource(runtime.configBase, archive.Bytes)
		if err != nil {
			return publication.Package{}, source, api.SkillPublicationEdition{}, err
		}
		if archive.ResolvedCommit == "" || archive.OwnerSubjectID == "" || archive.Repository == "" || archive.SourceReceiptID == "" || archive.PackageDigest == "" {
			return publication.Package{}, source, api.SkillPublicationEdition{}, output.Internal("GITHUB_SOURCE_RECEIPT_INVALID", "GitHub source response did not contain an immutable repository receipt", nil)
		}
		private := archive.Private
		remotePackageDigest = archive.PackageDigest
		var selectedPath *string
		if archive.Path != "" {
			selectedPath = &archive.Path
		}
		source = api.SkillPublicationSource{Type: "GITHUB", Entry: "SKILL.md", Repository: archive.Repository, Ref: archive.ResolvedCommit, Private: &private, OwnerSubjectID: archive.OwnerSubjectID, Path: selectedPath, SourceReceiptID: archive.SourceReceiptID}
	} else if xiaohongshuSkillID != "" || xiaohongshuSearch != "" {
		skillID := strings.TrimSpace(xiaohongshuSkillID)
		if skillID == "" {
			matches, err := runtime.client().SearchXiaohongshuSkills(ctx, merchantAccountID, strings.TrimSpace(xiaohongshuSearch))
			if err != nil {
				return publication.Package{}, source, api.SkillPublicationEdition{}, err
			}
			if len(matches.Items) == 0 {
				return publication.Package{}, source, api.SkillPublicationEdition{}, output.Validation("XIAOHONGSHU_SKILL_NOT_FOUND", "no verified Xiaohongshu Skill matches the search")
			}
			if len(matches.Items) > 1 {
				return publication.Package{}, source, api.SkillPublicationEdition{}, output.Confirmation("XIAOHONGSHU_SKILL_SELECTION_REQUIRED", "multiple Xiaohongshu Skills match; rerun with --xiaohongshu-skill-id").WithDetails(map[string]any{"candidates": matches.Items})
			}
			skillID = matches.Items[0].SkillID
		}
		archive, err := runtime.client().DownloadXiaohongshuSkillSource(ctx, merchantAccountID, skillID)
		if err != nil {
			return publication.Package{}, source, api.SkillPublicationEdition{}, err
		}
		pathToBuild, err = persistPublicationSource(runtime.configBase, archive.Bytes)
		if err != nil {
			return publication.Package{}, source, api.SkillPublicationEdition{}, err
		}
		if archive.SkillID != skillID || archive.ArtifactVersion == "" || archive.ArtifactDigest == "" || archive.SourceReceiptID == "" || archive.PackageDigest == "" {
			return publication.Package{}, source, api.SkillPublicationEdition{}, output.Internal("XIAOHONGSHU_SOURCE_RECEIPT_INVALID", "Xiaohongshu source response did not contain an immutable artifact receipt", nil)
		}
		source = api.SkillPublicationSource{Type: "XIAOHONGSHU", Entry: "SKILL.md", SkillID: skillID, ArtifactVersion: archive.ArtifactVersion, ArtifactDigest: archive.ArtifactDigest, SourceReceiptID: archive.SourceReceiptID}
		remotePackageDigest = archive.PackageDigest
	}
	var pkg publication.Package
	var err error
	if githubRepository != "" || xiaohongshuSkillID != "" || xiaohongshuSearch != "" {
		pkg, err = publication.BuildRemoteArchive(pathToBuild)
	} else {
		pkg, err = publication.Build(pathToBuild)
	}
	if err != nil {
		return publication.Package{}, source, api.SkillPublicationEdition{}, err
	}
	if remotePackageDigest != "" && pkg.Artifact.Digest != remotePackageDigest {
		return publication.Package{}, source, api.SkillPublicationEdition{}, output.Internal("SKILL_SOURCE_RECEIPT_INVALID", "remote Skill package digest does not match the API receipt", nil)
	}
	if localPath != "" {
		source = pkg.Manifest.Spec.Source
	} else if source.Type == "GITHUB" {
		selected := ""
		if source.Path != nil {
			selected = *source.Path
		}
		pkg.BindingIdentity = "remote:github:" + source.OwnerSubjectID + "/" + source.Repository + "/" + selected
	} else if source.Type == "XIAOHONGSHU" {
		pkg.BindingIdentity = "remote:xiaohongshu:" + source.SkillID
	}

	editionTitle = strings.TrimSpace(editionTitle)
	if editionTitle == "" {
		editionTitle = pkg.Manifest.Metadata.Title
	}
	if len([]rune(editionTitle)) > 64 {
		return publication.Package{}, source, api.SkillPublicationEdition{}, output.Validation("SKILL_EDITION_TITLE_INVALID", "edition title must be at most 64 characters")
	}
	highlights := make([]string, 0, len(editionHighlights))
	for _, highlight := range editionHighlights {
		highlight = strings.TrimSpace(highlight)
		if highlight == "" || len([]rune(highlight)) > 200 {
			return publication.Package{}, source, api.SkillPublicationEdition{}, output.Validation("SKILL_EDITION_HIGHLIGHT_INVALID", "each edition highlight must be 1 to 200 characters")
		}
		highlights = append(highlights, highlight)
	}
	if len(highlights) == 0 {
		highlights = []string{defaultEditionHighlight(pkg.Manifest.Metadata.Summary)}
	}
	if len(highlights) > 8 {
		return publication.Package{}, source, api.SkillPublicationEdition{}, output.Validation("SKILL_EDITION_HIGHLIGHT_INVALID", "an edition supports at most eight highlights")
	}
	edition := api.SkillPublicationEdition{Key: editionKey, Title: editionTitle, SortOrder: editionOrder, Highlights: highlights}
	pkg, err = publication.Customize(pkg, source, edition)
	return pkg, source, edition, err
}

func normalizeGithubRepository(value string) string {
	value = strings.TrimSpace(strings.TrimSuffix(value, "/"))
	value = strings.TrimSuffix(value, ".git")
	for _, prefix := range []string{"https://github.com/", "http://github.com/", "git@github.com:"} {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return value
}

// normalizeGithubPath treats the repository root spellings ("", ".", "./")
// as the root directory so the download request omits the path instead of
// sending a literal "." the server cannot resolve.
func normalizeGithubPath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "/")
	if value == "." {
		return ""
	}
	return value
}

// defaultEditionHighlight derives the fallback highlight from the manifest
// summary, cutting at a sentence or word boundary so auto-derived copy never
// exceeds the 200-character edition highlight limit.
func defaultEditionHighlight(summary string) string {
	runes := []rune(strings.TrimSpace(summary))
	if len(runes) <= 200 {
		return string(runes)
	}
	cut := runes[:200]
	for index := len(cut) - 1; index >= 0; index-- {
		if strings.ContainsRune("。！？；，、,.!?;: ", cut[index]) {
			return strings.TrimSpace(string(cut[:index+1]))
		}
	}
	return string(cut)
}

func persistPublicationSource(configBase string, archive []byte) (string, error) {
	if len(archive) == 0 || len(archive) > publication.MaxPackageBytes {
		return "", output.Validation("SKILL_PACKAGE_SIZE_INVALID", "remote Skill archive has an invalid size")
	}
	directory := filepath.Join(configBase, "publication-sources")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", output.Internal("PUBLICATION_SOURCE_SAVE_FAILED", "could not create the private remote-source cache", err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(archive))
	filename := filepath.Join(directory, digest+".zip")
	if existing, err := os.ReadFile(filename); err == nil {
		if fmt.Sprintf("%x", sha256.Sum256(existing)) == digest {
			return filename, nil
		}
		return "", output.Policy("PUBLICATION_SOURCE_CACHE_CONFLICT", "remote-source cache content does not match its digest")
	}
	if err := privatefile.Write(filename, archive, ".source-*.zip"); err != nil {
		return "", output.Internal("PUBLICATION_SOURCE_SAVE_FAILED", "could not write the remote Skill source", err)
	}
	return filename, nil
}

func prepareSkillListing(ctx context.Context, runtime *Runtime, pkg publication.Package, forceNew bool, targetListingID string) (listingPrepareResult, publication.ResolvedSourceIdentity, error) {
	sourceType, sourcePath, err := publication.SourceType(pkg.SourcePath)
	if err != nil {
		return listingPrepareResult{}, publication.ResolvedSourceIdentity{}, err
	}
	bindingSourcePath := sourcePath
	if pkg.BindingIdentity != "" {
		bindingSourcePath = pkg.BindingIdentity
	}
	origin, err := api.NormalizeAPIOrigin(runtime.apiBaseURL)
	if err != nil {
		return listingPrepareResult{}, publication.ResolvedSourceIdentity{}, output.Internal("SKILL_BINDING_SCOPE_INVALID", "could not normalize the current API endpoint", err)
	}
	store := publication.BindingStore{Directory: filepath.Join(runtime.configBase, "skill-bindings"), EndpointOrigin: origin, Now: runtime.deps.Now}
	resolution := ""
	if targetListingID != "" {
		resolution = "BIND_EXISTING:" + targetListingID
	} else if forceNew {
		resolution = "CREATE_NEW"
	}
	identity, err := store.ResolveOrCreate(bindingSourcePath, sourceType, pkg.Artifact.Digest, resolution, runtime.deps.NewID)
	if err != nil {
		return listingPrepareResult{}, identity, err
	}
	var receipt *string
	if identity.Binding != nil {
		receipt = &identity.Binding.BindingReceipt
	}
	request := api.PrepareSkillListingRequest{
		ClientRequestID: identity.ClientRequestID,
		Source: api.PrepareSkillListingSource{
			Type: sourceType, ClientWorkID: identity.ClientWorkID, BindingReceipt: receipt,
			PackageDigest: pkg.Artifact.Digest, DisplayName: pkg.Manifest.Metadata.Title,
		},
	}
	if targetListingID != "" {
		request.Resolution = &api.SkillListingResolution{Mode: "BIND_EXISTING", ListingID: targetListingID}
	} else if forceNew {
		request.Resolution = &api.SkillListingResolution{Mode: "CREATE_NEW"}
	}
	response, err := runtime.client().PrepareSkillListing(ctx, request)
	if err != nil {
		cliErr := output.AsError(err)
		if cliErr.Subtype == "SKILL_LISTING_SOURCE_AMBIGUOUS" {
			candidates, candidateErr := runtime.client().ListSkillListingCandidates(ctx, api.SkillListingCandidatesRequest{
				PackageDigest: pkg.Artifact.Digest,
			})
			if candidateErr == nil {
				enriched := output.Validation(
					"SKILL_LISTING_SOURCE_AMBIGUOUS",
					"multiple owned Skill listings match this package; choose the intended Listing explicitly",
				).WithDetails(map[string]any{"candidates": candidates.Candidates}).WithHint(
					"run 'viceme skill listing bind <listing-id> --path <source>' with one candidate, or retry with --new-listing for a separate work",
				).WithCause(err)
				enriched.RequestID = cliErr.RequestID
				return listingPrepareResult{}, identity, enriched
			}
		}
		return listingPrepareResult{}, identity, err
	}
	binding := publication.SkillBinding{
		APIVersion: publication.BindingAPIVersion, Kind: "SkillListing", ListingID: response.ListingID,
		ClientWorkID: identity.ClientWorkID, Market: response.Market, EndpointOrigin: origin,
		BindingReceipt: response.BindingReceipt, LastPackageDigest: pkg.Artifact.Digest,
	}
	if err := store.Save(bindingSourcePath, sourceType, binding); err != nil {
		return listingPrepareResult{}, identity, err
	}
	identity.Binding = &binding
	presentation := createPreviewPresentation(ctx, runtime, response.ListingID, response.OwnerPreviewURL)
	return listingPrepareResult{PrepareSkillListingResponse: response, SourceType: sourceType, SourcePath: bindingSourcePath, CanonicalPackageDigest: pkg.Artifact.Digest, Presentation: presentation}, identity, nil
}

func createPreviewPresentation(ctx context.Context, runtime *Runtime, listingID string, fallbackURL string) previewPresentation {
	presentation := previewPresentation{
		Intent:      "OPEN_OWNER_PREVIEW",
		FallbackURL: fallbackURL,
		Mode:        "FALLBACK_URL",
	}
	launch, err := runtime.client().CreateSkillPreviewLaunch(ctx, listingID)
	if err != nil {
		return presentation
	}
	presentation.OpenURL = &launch.LaunchURL
	presentation.OpenURLExpiresAt = &launch.ExpiresAt
	presentation.Mode = "ONE_TIME_LAUNCH"
	return presentation
}

func (runtime *Runtime) requireSkillPublicationAuthentication(ctx context.Context) error {
	if _, source, _ := runtime.overrideCredential(); source == "" {
		status, err := runtime.manager().CurrentStatus()
		if err != nil {
			return err
		}
		if !status.Authenticated {
			return output.Authentication("NOT_LOGGED_IN", "sign in before starting a Skill publication").
				WithHint("run 'viceme auth login' for the current profile; do not switch profiles to reuse another account").
				WithDetails(map[string]any{"profile": runtime.profile.Name, "apiBaseUrl": runtime.apiBaseURL})
		}
	}
	status, err := runtime.client().AuthStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Authenticated {
		return output.Authentication("NOT_LOGGED_IN", "sign in before starting a Skill publication").
			WithHint("run 'viceme auth login' for the current profile; do not switch profiles to reuse another account").
			WithDetails(map[string]any{"profile": runtime.profile.Name, "apiBaseUrl": runtime.apiBaseURL})
	}
	requiredScopes := []string{"skill-publication:read", "skill-publication:write"}
	availableScopes := make(map[string]struct{}, len(status.Scopes))
	for _, scope := range status.Scopes {
		availableScopes[scope] = struct{}{}
	}
	missingScopes := make([]string, 0, len(requiredScopes))
	for _, scope := range requiredScopes {
		if _, ok := availableScopes[scope]; !ok {
			missingScopes = append(missingScopes, scope)
		}
	}
	if len(missingScopes) != 0 {
		return output.Authorization("PUBLICATION_SCOPE_REQUIRED", "the current login is not authorized to publish Skills").
			WithHint("run 'viceme auth login' again for the current profile to grant publication access").
			WithDetails(map[string]any{"profile": runtime.profile.Name, "missingScopes": missingScopes})
	}
	return nil
}

func resolveSkillPublicationMerchant(ctx context.Context, runtime *Runtime, requestedID string) (api.MerchantAccount, error) {
	accounts, err := runtime.client().ListMerchantAccounts(ctx)
	if err != nil {
		return api.MerchantAccount{}, err
	}
	requestedID = strings.TrimSpace(requestedID)
	active := make([]api.MerchantAccount, 0, len(accounts.Items))
	for _, account := range accounts.Items {
		if account.Status == "ACTIVE" {
			active = append(active, account)
		}
		if requestedID != "" && account.ID == requestedID {
			if account.Status != "ACTIVE" {
				return api.MerchantAccount{}, output.Authorization("MERCHANT_SUSPENDED", "the selected Merchant is not active").WithDetails(map[string]any{"merchantAccountId": requestedID})
			}
			return account, nil
		}
	}
	if requestedID != "" {
		return api.MerchantAccount{}, output.Authorization("MERCHANT_REQUIRED", "the selected Merchant is not owned by the current login").WithDetails(map[string]any{"merchantAccountId": requestedID})
	}
	if len(active) == 1 {
		return active[0], nil
	}
	if len(active) == 0 {
		return api.MerchantAccount{}, output.Authorization("MERCHANT_REQUIRED", "an active Merchant owned by the current login is required before publishing").WithHint("ask a ViceMe Admin to create or activate your Merchant account")
	}
	return api.MerchantAccount{}, output.Validation("MERCHANT_SELECTION_REQUIRED", "multiple active Merchant accounts are available; select one explicitly").WithDetails(map[string]any{"merchants": active}).WithHint("run 'viceme merchant accounts', then retry with '--merchant <merchant-account-id>'")
}

func continueSkillPublication(ctx context.Context, runtime *Runtime, store publication.PendingStore, pending publication.Pending, pkg publication.Package, initialUpload *api.UploadAuthorization, packageOnly bool, createdResolution string) error {
	client := runtime.client()
	current, err := client.GetSkillPublication(ctx, pending.PublicationID)
	if err != nil {
		return err
	}
	if current.MerchantAccountID != pending.MerchantAccountID {
		return output.Authorization("PUBLICATION_MERCHANT_CHANGED", "the server publication no longer matches local Merchant recovery state").WithHint("inspect the publication on the current profile before continuing")
	}
	if current.Status == "PUBLISHED" || current.Status == "CANCELLED" {
		warnings := retirePublicationRecovery(runtime, store, pending, current.Status)
		return presentPublicationWithWarnings(ctx, runtime, current, "UPDATE", warnings)
	}
	if pending.PriceMinor != nil && (current.Draft.PriceMinor == nil || *current.Draft.PriceMinor != *pending.PriceMinor) {
		current, err = client.UpdateListingPrice(ctx, pending.PublicationID, *pending.PriceMinor)
		if err != nil {
			return err
		}
	}
	// 试用次数与价格同一条售卖条款链:本地恢复态有值且服务端草稿不一致时补丁。
	if pending.TrialUseLimit != nil && (current.Draft.TrialUseLimit == nil || *current.Draft.TrialUseLimit != *pending.TrialUseLimit) {
		current, err = client.UpdateListingTrialUseLimit(ctx, pending.PublicationID, pending.TrialUseLimit)
		if err != nil {
			return err
		}
	}
	if !verifiedUpload(current.Uploads, "PACKAGE", pkg.Artifact.Digest, "") {
		authorization := initialUpload
		if authorization == nil {
			value, err := client.AuthorizeUpload(ctx, pending.PublicationID, api.UploadAuthorizationRequest{
				Kind: "PACKAGE", Digest: pkg.Artifact.Digest, SizeBytes: pkg.Artifact.SizeBytes,
				FileName: pkg.Artifact.FileName, ContentType: pkg.Artifact.ContentType, SortOrder: 0,
			})
			if err != nil {
				return err
			}
			authorization = &value
		}
		progress(runtime, "Uploading deterministic Skill package")
		if err := client.PutUpload(ctx, *authorization, bytes.NewReader(pkg.Bytes), int64(len(pkg.Bytes))); err != nil {
			return err
		}
		current, err = client.CompleteUpload(ctx, pending.PublicationID, authorization.UploadID)
		if err != nil {
			return err
		}
	}
	if packageOnly {
		return presentPublication(ctx, runtime, current, "UPDATE")
	}
	for index, candidate := range pkg.Candidates {
		if verifiedUpload(current.Uploads, "MEDIA", candidate.Digest, candidate.RelativePath) {
			continue
		}
		authorization, err := client.AuthorizeUpload(ctx, pending.PublicationID, api.UploadAuthorizationRequest{
			Kind: "MEDIA", Digest: candidate.Digest, SizeBytes: candidate.SizeBytes,
			FileName: candidate.FileName, ContentType: candidate.ContentType,
			RelativePath: candidate.RelativePath, SortOrder: index,
		})
		if err != nil {
			return err
		}
		progress(runtime, fmt.Sprintf("Uploading listing candidate %d/%d", index+1, len(pkg.Candidates)))
		if err := client.PutUpload(ctx, authorization, bytes.NewReader(candidate.Bytes), candidate.SizeBytes); err != nil {
			return err
		}
		current, err = client.CompleteUpload(ctx, pending.PublicationID, authorization.UploadID)
		if err != nil {
			return err
		}
	}
	if current.Status == "PUBLISHED" {
		warnings := retirePublicationRecovery(runtime, store, pending, current.Status)
		return presentPublicationWithWarnings(ctx, runtime, current, createdResolution, warnings)
	}
	return presentPublication(ctx, runtime, current, createdResolution)
}

func presentPublication(ctx context.Context, runtime *Runtime, current api.SkillPublication, createdResolution string) error {
	return presentPublicationWithWarnings(ctx, runtime, current, createdResolution, nil)
}

func presentPublicationWithWarnings(ctx context.Context, runtime *Runtime, current api.SkillPublication, createdResolution string, warnings []string) error {
	presentation, err := previewPresentationForPublication(ctx, runtime, current)
	if err != nil {
		return err
	}
	return runtime.business(publicationPresentationResult{
		SkillPublication: current,
		PublicationID:    current.ID,
		Resolution:       createdResolution,
		RequiresPrice:    current.Draft.PriceMinor == nil,
		Presentation:     presentation,
		Warnings:         warnings,
	})
}

func previewPresentationForPublication(ctx context.Context, runtime *Runtime, current api.SkillPublication) (previewPresentation, error) {
	preview, err := runtime.client().GetSkillListingPreview(ctx, current.ListingID)
	if err != nil {
		return previewPresentation{}, err
	}
	return createPreviewPresentation(ctx, runtime, current.ListingID, preview.Preview.FallbackURL), nil
}

// retirePublicationRecovery discards the local recovery intent after the
// server reached a terminal state. The publication outcome is already
// authoritative at that point, so a cleanup failure must not flip a
// successful publish into a command error: the failure is surfaced as a
// warning on stderr and in the command result, and the retained recovery
// state makes the next publish or resume retry the same idempotent cleanup.
func retirePublicationRecovery(runtime *Runtime, store publication.PendingStore, pending publication.Pending, status string) []string {
	var warnings []string
	if err := store.RetireIntent(pending.Fingerprint, pending.PublicationID, pending.ClientRequestID); err != nil {
		return append(warnings, publicationRecoveryWarning(runtime,
			"PUBLICATION_RECOVERY_RETIRE_FAILED",
			"publication reached a terminal state but its local intent could not be retired",
			err, pending.PublicationID, status))
	}
	if err := store.Delete(pending.PublicationID); err != nil {
		warnings = append(warnings, publicationRecoveryWarning(runtime,
			"PUBLICATION_RECOVERY_CLEANUP_FAILED",
			"publication reached a terminal state but its local recovery file could not be removed",
			err, pending.PublicationID, status))
	}
	return warnings
}

func publicationRecoveryWarning(runtime *Runtime, code, message string, err error, publicationID, status string) string {
	warning := fmt.Sprintf("%s: %s: %v (publicationId %s, status %s); retry the same command after repairing access to the ViceMe publication recovery directory",
		code, message, err, publicationID, status)
	_, _ = fmt.Fprintln(runtime.deps.ErrOut, "warning: "+warning)
	return warning
}

func verifiedUpload(uploads []api.SkillPublicationUpload, kind, digest, relativePath string) bool {
	for _, upload := range uploads {
		if upload.Kind == kind && upload.Status == "VERIFIED" && upload.Digest == digest {
			if relativePath == "" || (upload.RelativePath != nil && *upload.RelativePath == relativePath) {
				return true
			}
		}
	}
	return false
}

func progress(runtime *Runtime, message string) {
	_, _ = fmt.Fprintln(runtime.deps.ErrOut, message+"...")
}
