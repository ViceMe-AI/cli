package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/publication"
	"github.com/spf13/cobra"
)

type inspectResult struct {
	publication.Package
}

type listingPrepareResult struct {
	api.PrepareSkillListingResponse
	SourceType             string              `json:"sourceType"`
	SourcePath             string              `json:"sourcePath"`
	CanonicalPackageDigest string              `json:"canonicalPackageDigest"`
	Presentation           previewPresentation `json:"presentation"`
}

type listingGetResult struct {
	api.SkillListingPreview
	Presentation previewPresentation `json:"presentation"`
}

type publicationPresentationResult struct {
	api.SkillPublication
	PublicationID string              `json:"publicationId"`
	Presentation  previewPresentation `json:"presentation"`
	Workflow      publicationWorkflow `json:"workflow"`
}

type previewPresentation struct {
	Intent           string  `json:"intent"`
	OpenURL          *string `json:"openUrl,omitempty"`
	OpenURLExpiresAt *string `json:"openUrlExpiresAt,omitempty"`
	FallbackURL      string  `json:"fallbackUrl"`
	ResultPaneURL    string  `json:"resultPaneUrl"`
	Mode             string  `json:"mode"`
}

func newSkillCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "skill", Short: "Inspect and publish a local Skill"}
	command.AddCommand(newSkillInspectCommand(runtime))
	command.AddCommand(newSkillListingCommand(runtime))
	command.AddCommand(newSkillPublishCommand(runtime))
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
			if err := runtime.requirePublicationAuthentication(command.Context()); err != nil {
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
			if err := runtime.requirePublicationAuthentication(command.Context()); err != nil {
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
			if err := runtime.requirePublicationAuthentication(command.Context()); err != nil {
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
			return runtime.business(inspectResult{Package: result})
		},
	}
	command.Flags().StringVar(&source, "path", "", "Skill directory or ZIP")
	_ = command.MarkFlagRequired("path")
	return command
}

func newSkillPublishCommand(runtime *Runtime) *cobra.Command {
	var source string
	var resume string
	var accessMode string
	var creatorMonthlyPriceCents int
	var creatorDisplayName string
	var forceNew bool
	var existingAction string
	command := &cobra.Command{
		Use: "publish", Short: "Upload a Skill and prepare its listing for explicit review", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runSkillPublish(command.Context(), runtime, skillPublishRequest{
				Source:                   source,
				Resume:                   resume,
				AccessMode:               accessMode,
				AccessModeSet:            command.Flags().Changed("access-mode"),
				CreatorMonthlyPriceCents: creatorMonthlyPriceCents,
				CreatorMonthlyPriceSet:   command.Flags().Changed("creator-monthly-price-cents"),
				CreatorDisplayName:       creatorDisplayName,
				ForceNew:                 forceNew,
				ExistingSkillAction:      existingAction,
			})
		},
	}
	command.Flags().StringVar(&source, "path", "", "Skill directory or ZIP")
	command.Flags().StringVar(&resume, "resume", "", "resume an interrupted publication by ID")
	command.Flags().StringVar(&accessMode, "access-mode", "FREE", "Skill access mode: FREE or CREATOR_SUBSCRIPTION")
	command.Flags().IntVar(&creatorMonthlyPriceCents, "creator-monthly-price-cents", 0, "set the creator's shared monthly subscription price in fen when required")
	command.Flags().StringVar(&creatorDisplayName, "creator-display-name", "", "creator display name used when the account has none")
	command.Flags().BoolVar(&forceNew, "new-listing", false, "explicitly create a separate Listing even when content matches")
	command.Flags().StringVar(&existingAction, "existing-skill-action", "", "action for an existing Skill: UPGRADE, UPDATE_FREE, or UPDATE_UPGRADED")
	return command
}

type skillPublishRequest struct {
	Source                   string
	Resume                   string
	AccessMode               string
	AccessModeSet            bool
	CreatorMonthlyPriceCents int
	CreatorMonthlyPriceSet   bool
	CreatorDisplayName       string
	ForceNew                 bool
	ExistingSkillAction      string
}

func newTopLevelSkillPublishCommand(runtime *Runtime) *cobra.Command {
	var accessMode string
	var creatorMonthlyPriceCents int
	var existingAction string
	command := &cobra.Command{
		Use:   "publish <path>",
		Short: "Upload a Skill and prepare its listing for explicit review",
		Args: func(_ *cobra.Command, args []string) error {
			switch len(args) {
			case 0:
				return output.Validation("SKILL_PATH_REQUIRED", "provide a Skill directory or ZIP path")
			case 1:
				return nil
			default:
				return output.Validation("SKILL_PATH_ARGUMENTS_INVALID", "publish accepts exactly one Skill directory or ZIP path")
			}
		},
		RunE: func(command *cobra.Command, args []string) error {
			return runSkillPublish(command.Context(), runtime, skillPublishRequest{
				Source: args[0], AccessMode: accessMode, AccessModeSet: command.Flags().Changed("access-mode"),
				CreatorMonthlyPriceCents: creatorMonthlyPriceCents, CreatorMonthlyPriceSet: command.Flags().Changed("creator-monthly-price-cents"),
				ExistingSkillAction: existingAction,
			})
		},
	}
	command.Flags().StringVar(&accessMode, "access-mode", "FREE", "Skill access mode: FREE or CREATOR_SUBSCRIPTION")
	command.Flags().IntVar(&creatorMonthlyPriceCents, "creator-monthly-price-cents", 0, "set the creator's shared monthly subscription price in fen when required")
	command.Flags().StringVar(&existingAction, "existing-skill-action", "", "action for an existing Skill: UPGRADE, UPDATE_FREE, or UPDATE_UPGRADED")
	return command
}

func runSkillPublish(ctx context.Context, runtime *Runtime, request skillPublishRequest) error {
	if request.Source != "" && request.Resume != "" {
		return output.Validation("PUBLICATION_FLAGS_CONFLICT", "--path and --resume cannot be used together")
	}
	if request.Source == "" && request.Resume == "" {
		return output.Validation("SKILL_PATH_REQUIRED", "provide --path or --resume")
	}
	if request.AccessMode == "" {
		request.AccessMode = "FREE"
	}
	if request.AccessMode != "FREE" && request.AccessMode != "CREATOR_SUBSCRIPTION" {
		return output.Validation("SKILL_ACCESS_MODE_INVALID", "access mode must be FREE or CREATOR_SUBSCRIPTION")
	}
	if request.CreatorMonthlyPriceSet && (request.CreatorMonthlyPriceCents <= 0 || request.CreatorMonthlyPriceCents > 100_000_000) {
		return output.Validation("CREATOR_MONTHLY_PRICE_INVALID", "creatorMonthlyPriceCents must be between 1 and 100000000")
	}
	if request.CreatorMonthlyPriceSet && request.Resume == "" && request.AccessMode != "CREATOR_SUBSCRIPTION" {
		return output.Validation("CREATOR_MONTHLY_PRICE_NOT_APPLICABLE", "creator monthly price is only valid for CREATOR_SUBSCRIPTION Skills")
	}
	if request.ExistingSkillAction != "" && request.ExistingSkillAction != "UPGRADE" && request.ExistingSkillAction != "UPDATE_FREE" && request.ExistingSkillAction != "UPDATE_UPGRADED" {
		return output.Validation("EXISTING_SKILL_ACTION_INVALID", "existing skill action must be UPGRADE, UPDATE_FREE, or UPDATE_UPGRADED")
	}
	store := publication.PendingStore{Directory: filepath.Join(runtime.configBase, "publications"), Now: runtime.deps.Now}
	if request.Resume != "" {
		pending, err := store.Load(request.Resume)
		if err != nil {
			return err
		}
		pkg, err := publication.Build(pending.SourcePath)
		if err != nil {
			return err
		}
		if pkg.Artifact.Digest != pending.ArtifactDigest {
			return output.Validation("PUBLICATION_SOURCE_CHANGED", "local Skill source changed after the publication started").WithHint("restore the original source or start a new publication")
		}
		if err := runtime.requirePublicationAuthentication(ctx); err != nil {
			return err
		}
		if pending.AccessMode == "" {
			pending.AccessMode = request.AccessMode
		}
		if request.AccessModeSet && pending.AccessMode != request.AccessMode {
			return output.Validation("PUBLICATION_ACCESS_MODE_CONFLICT", "the resumed publication access mode cannot be changed")
		}
		if request.CreatorMonthlyPriceSet {
			pending.CreatorMonthlyPriceCents = &request.CreatorMonthlyPriceCents
		}
		// An explicit resume continues Draft enrichment even while the
		// price is still unset. Price gates final confirmation, not media
		// upload or listing analysis.
		return continueSkillPublication(ctx, runtime, store, pending, pkg, nil, false)
	}
	pkg, err := publication.Build(request.Source)
	if err != nil {
		return err
	}
	if err := runtime.requirePublicationAuthentication(ctx); err != nil {
		return err
	}
	prepared, _, err := prepareSkillListing(ctx, runtime, pkg, request.ForceNew, "")
	if err != nil {
		return err
	}
	preview, previewErr := runtime.client().GetSkillListingPreview(ctx, prepared.ListingID)
	if !request.ForceNew && previewErr == nil && preview.Publication != nil && preview.Publication.Status == "PUBLISHED" {
		if request.ExistingSkillAction == "" {
			return output.Validation("EXISTING_SKILL_ACTION_REQUIRED", "this source is already bound to an existing Skill; choose how to publish the new release").WithDetails(map[string]any{
				"listingId": prepared.ListingID, "publicationId": preview.Publication.ID,
				"currentAccessMode": preview.Publication.AccessMode,
				"options":           []string{"UPGRADE", "UPDATE_FREE", "UPDATE_UPGRADED"},
			}).WithHint("retry with --existing-skill-action UPGRADE, UPDATE_FREE, or UPDATE_UPGRADED")
		}
		switch request.ExistingSkillAction {
		case "UPGRADE", "UPDATE_UPGRADED":
			request.AccessMode = "CREATOR_SUBSCRIPTION"
		case "UPDATE_FREE":
			request.AccessMode = "FREE"
		}
	}
	pkg.Manifest.Spec.Sale.AccessMode = request.AccessMode
	if request.AccessMode == "FREE" {
		pkg.Manifest.Spec.Sale.Entitlement = "PUBLIC_COPY"
	} else {
		pkg.Manifest.Spec.Sale.Entitlement = "CREATOR_SUBSCRIPTION"
	}
	pkg.Digest, err = publication.CanonicalDigest(pkg.Manifest)
	if err != nil {
		return output.Internal("MANIFEST_DIGEST_FAILED", "failed to canonicalize publication manifest", err)
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(
		runtime.profile.ID+"\x00"+prepared.ListingID+"\x00"+pkg.Artifact.Digest+"\x00"+pkg.Digest+"\x00"+
			request.CreatorDisplayName+"\x00"+buildinfo.Version,
	)))
	intent, err := store.LoadOrCreateIntent(fingerprint, runtime.deps.NewID)
	if err != nil {
		return err
	}
	if intent.PublicationID != "" {
		pending := publication.Pending{
			PublicationID: intent.PublicationID, ClientRequestID: intent.ClientRequestID,
			Fingerprint: fingerprint,
			SourcePath:  pkg.SourcePath, ArtifactDigest: pkg.Artifact.Digest, AccessMode: request.AccessMode,
		}
		if request.CreatorMonthlyPriceSet {
			pending.CreatorMonthlyPriceCents = &request.CreatorMonthlyPriceCents
		}
		if err := store.Save(pending); err != nil {
			return err
		}
		return continueSkillPublication(ctx, runtime, store, pending, pkg, nil, true)
	}
	created, err := runtime.client().CreateSkillPublication(ctx, api.CreateSkillPublicationRequest{
		ClientRequestID: intent.ClientRequestID, ContractVersion: api.SkillPublicationContractVersion, CLIVersion: buildinfo.Version,
		Manifest: pkg.Manifest, ManifestDigest: pkg.Digest, Artifact: pkg.Artifact, ListingID: prepared.ListingID,
		CreatorDisplayName: request.CreatorDisplayName,
	})
	if err != nil {
		if output.AsError(err).Subtype != "SKILL_PUBLICATION_ALREADY_ACTIVE" {
			return err
		}
		preview, previewErr := runtime.client().GetSkillListingPreview(ctx, prepared.ListingID)
		if previewErr != nil || preview.Publication == nil {
			return err
		}
		created = api.CreateSkillPublicationResponse{PublicationID: preview.Publication.ID, ListingID: prepared.ListingID, DraftRevision: preview.DraftRevision, Status: preview.Publication.Status}
	}
	intent.PublicationID = created.PublicationID
	if err := store.SaveIntent(intent); err != nil {
		return err
	}
	pending := publication.Pending{
		PublicationID: created.PublicationID, ClientRequestID: intent.ClientRequestID,
		Fingerprint: fingerprint,
		SourcePath:  pkg.SourcePath, ArtifactDigest: pkg.Artifact.Digest, AccessMode: request.AccessMode,
	}
	if request.CreatorMonthlyPriceSet {
		pending.CreatorMonthlyPriceCents = &request.CreatorMonthlyPriceCents
	}
	if err := store.Save(pending); err != nil {
		return err
	}
	return continueSkillPublication(ctx, runtime, store, pending, pkg, created.PackageUpload, true)
}

func prepareSkillListing(ctx context.Context, runtime *Runtime, pkg publication.Package, forceNew bool, targetListingID string) (listingPrepareResult, publication.ResolvedSourceIdentity, error) {
	sourceType, sourcePath, err := publication.SourceType(pkg.SourcePath)
	if err != nil {
		return listingPrepareResult{}, publication.ResolvedSourceIdentity{}, err
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
	identity, err := store.ResolveOrCreate(sourcePath, sourceType, pkg.Artifact.Digest, resolution, runtime.deps.NewID)
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
	if err := store.Save(sourcePath, sourceType, binding); err != nil {
		return listingPrepareResult{}, identity, err
	}
	identity.Binding = &binding
	presentation := createPreviewPresentation(ctx, runtime, response.ListingID, response.OwnerPreviewURL)
	return listingPrepareResult{PrepareSkillListingResponse: response, SourceType: sourceType, SourcePath: sourcePath, CanonicalPackageDigest: pkg.Artifact.Digest, Presentation: presentation}, identity, nil
}

func createPreviewPresentation(ctx context.Context, runtime *Runtime, listingID string, fallbackURL string) previewPresentation {
	presentation := previewPresentation{
		Intent:        "OPEN_OWNER_PREVIEW",
		FallbackURL:   fallbackURL,
		ResultPaneURL: fallbackURL,
		Mode:          "FALLBACK_URL",
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

func (runtime *Runtime) requirePublicationAuthentication(ctx context.Context) error {
	if _, source, _ := runtime.overrideCredential(); source == "" {
		status, err := runtime.manager().CurrentStatus()
		if err != nil {
			return err
		}
		if !status.Authenticated {
			return output.Authentication("NOT_LOGGED_IN", "sign in before starting a publication").
				WithHint("run 'viceme auth login' for the current profile; do not switch profiles to reuse another account").
				WithDetails(map[string]any{"profile": runtime.profile.Name, "apiBaseUrl": runtime.apiBaseURL})
		}
	}
	status, err := runtime.client().AuthStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Authenticated {
		return output.Authentication("NOT_LOGGED_IN", "sign in before starting a publication").
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
		return output.Authorization("PUBLICATION_SCOPE_REQUIRED", "the current login is not authorized to publish").
			WithHint("run 'viceme auth login' again for the current profile to grant publication access").
			WithDetails(map[string]any{"profile": runtime.profile.Name, "missingScopes": missingScopes})
	}
	return nil
}

func continueSkillPublication(ctx context.Context, runtime *Runtime, store publication.PendingStore, pending publication.Pending, pkg publication.Package, initialUpload *api.UploadAuthorization, packageOnly bool) error {
	client := runtime.client()
	current, err := client.GetSkillPublication(ctx, pending.PublicationID)
	if err != nil {
		return err
	}
	if current.Status == "PUBLISHED" || current.Status == "CANCELLED" {
		if err := retirePublicationRecovery(store, pending, current.Status); err != nil {
			return err
		}
		return presentPublication(ctx, runtime, current)
	}
	if pending.CreatorMonthlyPriceCents != nil {
		if current.AccessMode != "CREATOR_SUBSCRIPTION" || current.CreatorAccountID == nil {
			return output.Validation("CREATOR_MONTHLY_PRICE_NOT_APPLICABLE", "creator monthly price requires a creator-subscription publication with a resolved creator account")
		}
		if !current.RequiresCreatorMonthlyPrice || current.CreatorMonthlyPriceCents != nil {
			details := map[string]any{"publicationId": current.ID, "creatorAccountId": current.CreatorAccountID}
			if current.CreatorMonthlyPriceCents != nil {
				details["creatorMonthlyPriceCents"] = *current.CreatorMonthlyPriceCents
			}
			return output.Validation("CREATOR_MONTHLY_PRICE_NOT_APPLICABLE", "the creator already has an authoritative monthly price").WithDetails(details)
		}
		_, err = client.SetCreatorMonthlyPrice(ctx, api.CreateCreatorSubscriptionPlanRequest{CreatorAccountID: *current.CreatorAccountID, MonthlyPriceCents: *pending.CreatorMonthlyPriceCents})
		if err != nil {
			return err
		}
		current, err = client.GetSkillPublication(ctx, pending.PublicationID)
		if err != nil {
			return err
		}
		pending.CreatorMonthlyPriceCents = nil
		if err := store.Save(pending); err != nil {
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
		return presentPublicationWithPhase(ctx, runtime, current, workflowPhaseContinueDraft)
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
		if err := retirePublicationRecovery(store, pending, current.Status); err != nil {
			return err
		}
	}
	return presentPublicationWithPhase(ctx, runtime, current, workflowPhaseEnrichDraft)
}

func presentPublication(ctx context.Context, runtime *Runtime, current api.SkillPublication) error {
	return presentPublicationWithPhase(ctx, runtime, current, "")
}

func presentPublicationWithPhase(ctx context.Context, runtime *Runtime, current api.SkillPublication, phaseOverride string) error {
	presentation, err := previewPresentationForPublication(ctx, runtime, current)
	if err != nil {
		return err
	}
	return runtime.business(publicationPresentationResult{
		SkillPublication: current,
		PublicationID:    current.ID,
		Presentation:     presentation,
		Workflow:         workflowForPublication(current, phaseOverride),
	})
}

func previewPresentationForPublication(ctx context.Context, runtime *Runtime, current api.SkillPublication) (previewPresentation, error) {
	if current.Status == "PUBLISHED" && current.Product != nil && current.Product.DetailURL != "" {
		openURL := current.Product.DetailURL
		return previewPresentation{
			Intent:        "OPEN_PUBLISHED_SKILL",
			OpenURL:       &openURL,
			FallbackURL:   openURL,
			ResultPaneURL: openURL,
			Mode:          "STABLE_URL",
		}, nil
	}
	preview, err := runtime.client().GetSkillListingPreview(ctx, current.ListingID)
	if err != nil {
		return previewPresentation{}, err
	}
	return createPreviewPresentation(ctx, runtime, current.ListingID, preview.Preview.FallbackURL), nil
}

func retirePublicationRecovery(store publication.PendingStore, pending publication.Pending, status string) error {
	details := map[string]any{"publicationId": pending.PublicationID, "status": status}
	if err := store.RetireIntent(pending.Fingerprint, pending.PublicationID, pending.ClientRequestID); err != nil {
		return output.Internal("PUBLICATION_RECOVERY_RETIRE_FAILED", "publication reached a terminal state but its local intent could not be retired", err).
			WithDetails(details).
			WithHint("retry the same command after repairing access to the ViceMe publication recovery directory")
	}
	if err := store.Delete(pending.PublicationID); err != nil {
		return output.Internal("PUBLICATION_RECOVERY_CLEANUP_FAILED", "publication reached a terminal state but its local recovery file could not be removed", err).
			WithDetails(details).
			WithHint("retry the same command after repairing access to the ViceMe publication recovery directory")
	}
	return nil
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
