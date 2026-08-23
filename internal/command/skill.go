package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/output"
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
	PublicationID string              `json:"publicationId"`
	RequiresPrice bool                `json:"requiresPrice"`
	Presentation  previewPresentation `json:"presentation"`
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
	var resume string
	var priceMinor int
	var merchantAccountID string
	var forceNew bool
	command := &cobra.Command{
		Use: "publish", Short: "Upload a Skill and prepare its listing for explicit review", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if source != "" && resume != "" {
				return output.Validation("PUBLICATION_FLAGS_CONFLICT", "--path and --resume cannot be used together")
			}
			if source == "" && resume == "" {
				return output.Validation("SKILL_PATH_REQUIRED", "provide --path or --resume")
			}
			priceConfirmed := command.Flags().Changed("price-minor")
			if priceConfirmed && (priceMinor < 0 || priceMinor > 10_000_000) {
				return output.Validation("SKILL_PRICE_INVALID", "priceMinor must be between 0 and 10000000")
			}
			store := publication.PendingStore{Directory: filepath.Join(runtime.configBase, "publications"), Now: runtime.deps.Now}
			if resume != "" {
				pending, err := store.Load(resume)
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
					if err := store.Save(pending); err != nil {
						return err
					}
				}
				// An explicit resume continues Draft enrichment even while the
				// price is still unset. Price gates final confirmation, not media
				// upload or listing analysis.
				return continueSkillPublication(command.Context(), runtime, store, pending, pkg, nil, false)
			}
			pkg, err := publication.Build(source)
			if err != nil {
				return err
			}
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantAccountID)
			if err != nil {
				return err
			}
			prepared, _, err := prepareSkillListing(command.Context(), runtime, pkg, forceNew, "")
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
				}
				if priceConfirmed {
					pending.PriceMinor = &priceMinor
				}
				if err := store.Save(pending); err != nil {
					return err
				}
				return continueSkillPublication(command.Context(), runtime, store, pending, pkg, nil, pending.PriceMinor == nil)
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
				created = api.CreateSkillPublicationResponse{PublicationID: current.ID, ListingID: current.ListingID, MerchantAccountID: current.MerchantAccountID, DraftRevision: current.DraftRevision, Status: current.Status}
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
			}
			if priceConfirmed {
				pending.PriceMinor = &priceMinor
			}
			if err := store.Save(pending); err != nil {
				return err
			}
			return continueSkillPublication(command.Context(), runtime, store, pending, pkg, created.PackageUpload, pending.PriceMinor == nil)
		},
	}
	command.Flags().StringVar(&source, "path", "", "Skill directory or ZIP")
	command.Flags().StringVar(&resume, "resume", "", "resume an interrupted publication by ID")
	command.Flags().IntVar(&priceMinor, "price-minor", 0, "set the CNY price in fen while continuing the private draft")
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "Merchant account ID; required only when multiple active accounts exist")
	command.Flags().BoolVar(&forceNew, "new-listing", false, "explicitly create a separate Listing even when content matches")
	return command
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

func continueSkillPublication(ctx context.Context, runtime *Runtime, store publication.PendingStore, pending publication.Pending, pkg publication.Package, initialUpload *api.UploadAuthorization, packageOnly bool) error {
	client := runtime.client()
	current, err := client.GetSkillPublication(ctx, pending.PublicationID)
	if err != nil {
		return err
	}
	if current.MerchantAccountID != pending.MerchantAccountID {
		return output.Authorization("PUBLICATION_MERCHANT_CHANGED", "the server publication no longer matches local Merchant recovery state").WithHint("inspect the publication on the current profile before continuing")
	}
	if current.Status == "PUBLISHED" || current.Status == "CANCELLED" {
		if err := retirePublicationRecovery(store, pending, current.Status); err != nil {
			return err
		}
		return presentPublication(ctx, runtime, current)
	}
	if pending.PriceMinor != nil && (current.Draft.PriceMinor == nil || *current.Draft.PriceMinor != *pending.PriceMinor) {
		current, err = client.UpdateListingPrice(ctx, pending.PublicationID, *pending.PriceMinor)
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
		return presentPublication(ctx, runtime, current)
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
	return presentPublication(ctx, runtime, current)
}

func presentPublication(ctx context.Context, runtime *Runtime, current api.SkillPublication) error {
	presentation, err := previewPresentationForPublication(ctx, runtime, current)
	if err != nil {
		return err
	}
	return runtime.business(publicationPresentationResult{
		SkillPublication: current,
		PublicationID:    current.ID,
		RequiresPrice:    current.Draft.PriceMinor == nil,
		Presentation:     presentation,
	})
}

func previewPresentationForPublication(ctx context.Context, runtime *Runtime, current api.SkillPublication) (previewPresentation, error) {
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
