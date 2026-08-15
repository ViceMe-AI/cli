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
	PriceConfirmed bool `json:"priceConfirmed"`
}

type listingPrepareResult struct {
	api.PrepareSkillListingResponse
	SourceType             string `json:"sourceType"`
	SourcePath             string `json:"sourcePath"`
	CanonicalPackageDigest string `json:"canonicalPackageDigest"`
	RequiresPrice          bool   `json:"requiresPrice"`
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
			pkg, err := publication.Build(source, 0)
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
			return runtime.business(result)
		},
	}
}

func newSkillListingBindCommand(runtime *Runtime) *cobra.Command {
	var source string
	command := &cobra.Command{
		Use: "bind <listing-id>", Short: "Explicitly bind a ZIP or workspace to an owned Listing", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			pkg, err := publication.Build(source, 0)
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
			result, err := publication.Build(source, 0)
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
	var productID string
	var creatorDisplayName string
	var dryRun bool
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
			if dryRun && resume != "" {
				return output.Validation("PUBLICATION_FLAGS_CONFLICT", "--dry-run cannot be combined with --resume")
			}
			store := publication.PendingStore{Directory: filepath.Join(runtime.configBase, "publications"), Now: runtime.deps.Now}
			if resume != "" {
				pending, err := store.Load(resume)
				if err != nil {
					return err
				}
				pkg, err := publication.Build(pending.SourcePath, pending.PriceMinor)
				if err != nil {
					return err
				}
				if pkg.Artifact.Digest != pending.ArtifactDigest {
					return output.Validation("PUBLICATION_SOURCE_CHANGED", "local Skill source changed after the publication started").WithHint("restore the original source or start a new publication")
				}
				if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
					return err
				}
				return continueSkillPublication(command.Context(), runtime, store, pending, pkg, nil)
			}
			priceConfirmed := command.Flags().Changed("price-minor")
			pkg, err := publication.Build(source, priceMinor)
			if err != nil {
				return err
			}
			if dryRun {
				return runtime.business(inspectResult{Package: pkg, PriceConfirmed: priceConfirmed})
			}
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			prepared, _, err := prepareSkillListing(command.Context(), runtime, pkg, forceNew, "")
			if err != nil {
				return err
			}
			if !priceConfirmed {
				prepared.RequiresPrice = true
				return runtime.business(prepared)
			}
			fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(
				runtime.profile.ID+"\x00"+prepared.ListingID+"\x00"+pkg.Artifact.Digest+"\x00"+pkg.Digest+"\x00"+
					fmt.Sprintf("%d", priceMinor)+"\x00"+productID+"\x00"+creatorDisplayName+"\x00"+buildinfo.Version,
			)))
			intent, err := store.LoadOrCreateIntent(fingerprint, runtime.deps.NewID)
			if err != nil {
				return err
			}
			if intent.PublicationID != "" {
				pending := publication.Pending{
					PublicationID: intent.PublicationID, ClientRequestID: intent.ClientRequestID,
					Fingerprint: fingerprint,
					SourcePath:  pkg.SourcePath, PriceMinor: priceMinor, ArtifactDigest: pkg.Artifact.Digest,
				}
				if err := store.Save(pending); err != nil {
					return err
				}
				return continueSkillPublication(command.Context(), runtime, store, pending, pkg, nil)
			}
			created, err := runtime.client().CreateSkillPublication(command.Context(), api.CreateSkillPublicationRequest{
				ClientRequestID: intent.ClientRequestID, ContractVersion: "2026-08-11", CLIVersion: buildinfo.Version,
				Manifest: pkg.Manifest, ManifestDigest: pkg.Digest, Artifact: pkg.Artifact, ListingID: prepared.ListingID,
				ProductID: productID, CreatorDisplayName: creatorDisplayName,
			})
			if err != nil {
				if output.AsError(err).Subtype != "SKILL_PUBLICATION_ALREADY_ACTIVE" {
					return err
				}
				preview, previewErr := runtime.client().GetSkillListingPreview(command.Context(), prepared.ListingID)
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
				SourcePath:  pkg.SourcePath, PriceMinor: priceMinor, ArtifactDigest: pkg.Artifact.Digest,
			}
			if err := store.Save(pending); err != nil {
				return err
			}
			return continueSkillPublication(command.Context(), runtime, store, pending, pkg, created.PackageUpload)
		},
	}
	command.Flags().StringVar(&source, "path", "", "Skill directory or ZIP")
	command.Flags().StringVar(&resume, "resume", "", "resume an interrupted publication by ID")
	command.Flags().IntVar(&priceMinor, "price-minor", 0, "confirmed CNY price in fen")
	command.Flags().StringVar(&productID, "product-id", "", "update an owned product instead of creating one")
	command.Flags().StringVar(&creatorDisplayName, "creator-display-name", "", "creator display name used when the account has none")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "validate and show the deterministic plan without network writes")
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
	market := "CN"
	if runtime.profile.Region == "global" {
		market = "GLOBAL"
	}
	store := publication.BindingStore{Directory: filepath.Join(runtime.configBase, "skill-bindings"), EndpointOrigin: origin, Market: market, Now: runtime.deps.Now}
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
		Market:          market,
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
				Market: market, PackageDigest: pkg.Artifact.Digest,
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
		ClientWorkID: identity.ClientWorkID, Market: market, EndpointOrigin: origin,
		BindingReceipt: response.BindingReceipt, LastPackageDigest: pkg.Artifact.Digest,
	}
	if err := store.Save(sourcePath, sourceType, binding); err != nil {
		return listingPrepareResult{}, identity, err
	}
	identity.Binding = &binding
	return listingPrepareResult{PrepareSkillListingResponse: response, SourceType: sourceType, SourcePath: sourcePath, CanonicalPackageDigest: pkg.Artifact.Digest}, identity, nil
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

func continueSkillPublication(ctx context.Context, runtime *Runtime, store publication.PendingStore, pending publication.Pending, pkg publication.Package, initialUpload *api.UploadAuthorization) error {
	client := runtime.client()
	current, err := client.GetSkillPublication(ctx, pending.PublicationID)
	if err != nil {
		return err
	}
	if current.Status == "PUBLISHED" || current.Status == "CANCELLED" {
		if err := retirePublicationRecovery(store, pending, current.Status); err != nil {
			return err
		}
		return runtime.business(current)
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
	if len(pkg.Candidates) > 0 && current.Analysis == nil && (current.Status == "DRAFT" || current.Status == "FAILED") {
		progress(runtime, "Requesting non-authoritative cover and gallery suggestions")
		current, err = client.AnalyzeListing(ctx, pending.PublicationID)
		if err != nil {
			return err
		}
	}
	if current.Status == "PUBLISHED" {
		if err := retirePublicationRecovery(store, pending, current.Status); err != nil {
			return err
		}
	}
	return runtime.business(current)
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
