package command

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/pagepackage"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
	"github.com/ViceMe-AI/cli/internal/replicapreview"
	"github.com/ViceMe-AI/cli/internal/replicapublication"
	"github.com/spf13/cobra"
)

type replicaPublishOptions struct {
	ProjectPath              string
	PreviewURL               string
	PreviewReviewed          bool
	WorkID                   string
	Slug                     string
	MerchantAccountID        string
	CanonicalOrigin          string
	Title                    string
	Summary                  string
	PriceCents               int
	ConfirmationVersion      string
	ConfirmUnverifiedPreview bool
	ReplicaOnly              bool
	AutoApplyCreator         bool
}

type replicaPublicationFinalReview struct {
	api.WebsiteReplicaPublicationReview
	SourceArchive                 replicacontent.SourceArchiveSummary     `json:"sourceArchive"`
	Exclusions                    []replicacontent.SourceArchiveExclusion `json:"exclusions"`
	PageArtifact                  any                                     `json:"pageArtifact"`
	Hosting                       string                                  `json:"hosting"`
	AutomaticDegradation          bool                                    `json:"automaticDegradation"`
	ImmutableVersions             bool                                    `json:"immutableVersions"`
	ExistingBuyerVersionsRetained bool                                    `json:"existingBuyerVersionsRetained"`
	AutomaticCreatorApplication   bool                                    `json:"automaticCreatorApplication"`
	Preview                       replicapublication.Preview              `json:"preview"`
	ConfirmationTTLSeconds        int                                     `json:"confirmationTtlSeconds"`
	ConfirmationExpiresAt         string                                  `json:"confirmationExpiresAt"`
}

func newReplicaPublishCommand(runtime *Runtime) *cobra.Command {
	options := replicaPublishOptions{}
	command := &cobra.Command{
		Use:   "publish",
		Short: "Preview, confirm, upload, and submit a Website Replica Publication",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := publishWebsiteReplica(command.Context(), runtime, options)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&options.ProjectPath, "path", "", "Website Replica project directory or existing ZIP path")
	command.Flags().StringVar(&options.PreviewURL, "preview-url", "", "actual HTTP(S) loopback page selected and started by your agent")
	command.Flags().BoolVar(&options.PreviewReviewed, "preview-reviewed", false, "attest that your agent observed the actual local page")
	command.Flags().StringVar(&options.WorkID, "work-id", "", "existing Website Work UUID (omit when creating a new Work)")
	command.Flags().StringVar(&options.Slug, "slug", "", "new public Work slug")
	command.Flags().StringVar(&options.MerchantAccountID, "merchant-id", "", "ACTIVE OWNER Merchant UUID")
	command.Flags().StringVar(&options.CanonicalOrigin, "canonical-origin", "", "optional public HTTPS website origin")
	command.Flags().StringVar(&options.Title, "title", "", "public Website Replica title")
	command.Flags().StringVar(&options.Summary, "summary", "", "public Website Replica summary")
	command.Flags().IntVar(&options.PriceCents, "price-cents", -1, "one-time price in the market currency's minor unit")
	command.Flags().StringVar(&options.ConfirmationVersion, "confirm", "", "exact final-review confirmation version")
	command.Flags().BoolVar(&options.ConfirmUnverifiedPreview, "confirm-unverified-replica-only", false, "allow Replica-only publication when local preview cannot be verified")
	command.Flags().BoolVar(&options.ReplicaOnly, "replica-only", false, "publish source only even when an existing static output can be hosted")
	command.Flags().BoolVar(&options.AutoApplyCreator, "auto-apply-creator", false, "authorize one idempotent creator application if publication requires it")
	addReplicaStorageFlag(command, runtime)
	_ = command.MarkFlagRequired("path")
	_ = command.MarkFlagRequired("title")
	return command
}

func publishWebsiteReplica(ctx context.Context, runtime *Runtime, options replicaPublishOptions) (_ replicaPublicationPresentation, returnErr error) {
	options = normalizeReplicaPublishOptions(options)
	if err := validateReplicaPublishOptions(options); err != nil {
		return replicaPublicationPresentation{}, err
	}
	market := replicaPublicationMarket(runtime)
	if err := requireReplicaPublicationCN(runtime); err != nil {
		return replicaPublicationPresentation{}, err
	}
	derivedFingerprint, projectPath, err := replicapublication.ProjectFingerprint(runtime.apiBaseURL, market, options.ProjectPath)
	if err != nil {
		return replicaPublicationPresentation{}, err
	}
	options.ProjectPath = projectPath
	bindingStore := replicaBindingStore(runtime)
	binding, bindingFound, err := bindingStore.Load(projectPath)
	if err != nil {
		return replicaPublicationPresentation{}, err
	}
	if bindingFound && binding.ProjectFingerprint != derivedFingerprint {
		return replicaPublicationPresentation{}, output.Validation("REPLICA_BINDING_PROJECT_MISMATCH", "local Website Replica binding belongs to another source path")
	}
	if options.PriceCents < 0 && bindingFound && binding.Product != nil {
		options.PriceCents = binding.Product.PriceCents
	}
	if options.PriceCents < 0 {
		return replicaPublicationPresentation{}, output.Validation(
			"REPLICA_PRICE_REQUIRED",
			"--price-cents is required for a first Website Replica publication",
		)
	}
	projectFingerprint := derivedFingerprint
	if bindingFound {
		projectFingerprint = binding.ProjectFingerprint
	}
	// An input-only preview must not create project recovery directories. A
	// durable request still follows its normal locked recovery path below.
	if !bindingFound && options.ConfirmationVersion == "" && !options.ConfirmUnverifiedPreview && (options.PreviewURL == "" || !options.PreviewReviewed) {
		localStore, err := projectReplicaPublicationStore(runtime, projectPath)
		if err != nil {
			return replicaPublicationPresentation{}, err
		}
		_, localFound, err := localStore.LoadProject(projectFingerprint)
		if err != nil {
			return replicaPublicationPresentation{}, err
		}
		_, globalFound, err := replicaPublicationStore(runtime).LoadProject(projectFingerprint)
		if err != nil {
			return replicaPublicationPresentation{}, err
		}
		if !localFound && !globalFound {
			if err := replicacontent.ValidateSourceWorktree(projectPath); err != nil {
				return replicaPublicationPresentation{}, replicaSourceArchiveError(err)
			}
			session, _, previewErr := startReplicaPublicationPreview(ctx, runtime, options)
			if session != nil {
				previewErr = finishReplicaPreview(session, previewErr)
			}
			return replicaPublicationPresentation{}, previewErr
		}
	}
	store, unlock, err := prepareReplicaPublishStore(runtime, projectPath, projectFingerprint)
	if err != nil {
		return replicaPublicationPresentation{}, err
	}
	defer func() { returnErr = errors.Join(returnErr, unlock()) }()
	if err := store.CleanupExpiredArtifacts(); err != nil {
		return replicaPublicationPresentation{}, err
	}

	if bindingFound && (binding.Publication.Status == "DRAFT" || binding.Publication.Status == "PROCESSING") {
		publication, getErr := runtime.client().GetWebsiteReplicaPublication(ctx, binding.Publication.ID)
		if getErr != nil {
			return replicaPublicationPresentation{}, getErr
		}
		pending, found, loadErr := store.LoadPublication(publication.ID)
		if loadErr != nil {
			return replicaPublicationPresentation{}, loadErr
		}
		if found {
			return synchronizeReplicaPublication(runtime, store, pending, publication)
		}
		return presentStoredReplicaPublication(runtime, publication), nil
	}

	pending, found, err := store.LoadProject(projectFingerprint)
	if err != nil {
		return replicaPublicationPresentation{}, err
	}
	if found && pending.Publication != nil {
		publication, getErr := runtime.client().GetWebsiteReplicaPublication(ctx, pending.Publication.ID)
		if getErr != nil {
			return replicaPublicationPresentation{}, getErr
		}
		return synchronizeReplicaPublication(runtime, store, pending, publication)
	}

	if options.ConfirmationVersion != "" {
		if !found {
			return replicaPublicationPresentation{}, output.Confirmation(
				"REPLICA_PUBLICATION_CONFIRMATION_NOT_FOUND",
				"the frozen final review is unavailable or belongs to another project",
			).WithHint("run the same publish command without --confirm to generate a fresh final review")
		}
		if !runtime.deps.Now().Before(pending.ArtifactExpiresAt) {
			cleanupErr := errors.Join(store.DeleteArtifact(pending), store.Delete(pending))
			return replicaPublicationPresentation{}, output.Confirmation(
				"REPLICA_PUBLICATION_CONFIRMATION_EXPIRED",
				"the thirty-minute Website Replica final review expired; the frozen source was removed",
			).WithHint("run the same publish command without --confirm to preview and freeze a fresh source archive").WithCause(cleanupErr)
		}
		if err := validateConfirmedReplicaRequest(options, pending, binding, bindingFound); err != nil {
			return replicaPublicationPresentation{}, err
		}
		artifact, err := store.OpenArtifact(pending)
		if err != nil {
			cliErr := output.AsError(err)
			if cliErr.Subtype == "REPLICA_PUBLICATION_ARTIFACT_CHANGED" {
				cleanupErr := errors.Join(store.DeleteArtifact(pending), store.Delete(pending))
				cliErr.Cause = errors.Join(cliErr.Cause, cleanupErr)
				cliErr.WithHint("run the same publish command without --confirm to freeze a fresh source archive and request a new final review")
			}
			return replicaPublicationPresentation{}, cliErr
		}
		if err := artifact.Close(); err != nil {
			return replicaPublicationPresentation{}, output.Internal("REPLICA_PUBLICATION_ARTIFACT_READ_FAILED", "could not close the verified frozen Website Replica source", err)
		}
		if pending.Request.Page != nil {
			pageArtifact, err := store.OpenPageArtifact(pending)
			if err != nil {
				return replicaPublicationPresentation{}, err
			}
			if err := pageArtifact.Close(); err != nil {
				return replicaPublicationPresentation{}, output.Internal("REPLICA_PUBLICATION_PAGE_ARTIFACT_READ_FAILED", "could not close the verified frozen Website Replica page", err)
			}
		}
		confirmedAt := runtime.deps.Now().UTC()
		pending.ConfirmedAt = &confirmedAt
		if err := store.Save(&pending); err != nil {
			return replicaPublicationPresentation{}, err
		}
		return createAndDriveReplicaPublication(ctx, runtime, store, pending)
	}

	if found && !runtime.deps.Now().Before(pending.ArtifactExpiresAt) {
		if err := store.DeleteArtifact(pending); err != nil {
			return replicaPublicationPresentation{}, err
		}
		pending.Confirmation = nil
		pending.ConfirmedAt = nil
	}

	progress(runtime, "Checking Website Replica source locally")
	if err := replicacontent.ValidateSourceWorktree(projectPath); err != nil {
		return replicaPublicationPresentation{}, replicaSourceArchiveError(err)
	}

	previewSession, preview, err := startReplicaPublicationPreview(ctx, runtime, options)
	if previewSession != nil {
		defer func() {
			if closeErr := finishReplicaPreview(previewSession, nil); closeErr != nil {
				returnErr = errors.Join(returnErr, closeErr)
			}
		}()
	}
	if err != nil {
		var inputError *output.Error
		if errors.As(err, &inputError) {
			return replicaPublicationPresentation{}, inputError
		}
		return replicaPublicationPresentation{}, replicaPreviewBoundaryError(err).WithHint(
			"fix the reported preview problem and retry; only after the user accepts the unverified boundaries, rerun publish without --preview-reviewed and with --confirm-unverified-replica-only",
		)
	}
	var hostedPage pagepackage.Package
	if preview.Verified && !options.ReplicaOnly {
		progress(runtime, "Checking for an existing static output")
		hostedPage, _, err = pagepackage.BuildWebsiteWorkPage(projectPath, options.Title)
		if err != nil {
			return replicaPublicationPresentation{}, err
		}
	}
	clientRequestID := ""
	creatorApplicationRequestID := ""
	createdAt := time.Time{}
	if found {
		clientRequestID = pending.ClientRequestID
		creatorApplicationRequestID = pending.CreatorApplicationRequestID
		createdAt = pending.CreatedAt
	} else {
		clientRequestID = runtime.deps.NewID()
		if !replicaUUIDPattern.MatchString(clientRequestID) {
			return replicaPublicationPresentation{}, output.Internal("REPLICA_CLIENT_REQUEST_ID_INVALID", "could not create a valid Website Replica request identity", nil)
		}
	}
	target, merchantID, err := replicaPublicationTarget(options, binding, bindingFound)
	if err != nil {
		return replicaPublicationPresentation{}, err
	}

	progress(runtime, "Freezing Website Replica source")
	frozen, err := replicacontent.FreezeSourceArchive(projectPath, replicacontent.FreezeSourceOptions{
		Purpose: options.Summary,
	})
	if err != nil {
		return replicaPublicationPresentation{}, replicaSourceArchiveError(err)
	}
	defer func() { returnErr = errors.Join(returnErr, frozen.Cleanup()) }()
	expiresAt := runtime.deps.Now().UTC().Add(api.WebsiteReplicaPublicationConfirmationTTL * time.Second)
	frozen.ExpiresAt = expiresAt
	if err := store.SaveArtifact(clientRequestID, frozen); err != nil {
		return replicaPublicationPresentation{}, err
	}
	request := api.CreateWebsiteReplicaPublicationRequest{
		ProtocolVersion:    api.WebsiteReplicaPublicationProtocolVersion,
		ClientRequestID:    clientRequestID,
		Market:             market,
		MerchantAccountID:  merchantID,
		ProjectFingerprint: projectFingerprint,
		Target:             target,
		Title:              options.Title,
		Summary:            options.Summary,
		PriceCents:         options.PriceCents,
		Source: api.WebsiteReplicaPublicationSourceArtifact{
			FileName: "source.zip", ContentType: "application/zip",
			SizeBytes: frozen.Summary.SizeBytes, Digest: frozen.Summary.Digest,
		},
		Confirmation: nil,
	}
	if len(hostedPage.Bytes) > 0 {
		request.AllowAutomaticDegradation = true
		request.Page = &api.WebsiteReplicaPublicationSourceArtifact{
			FileName: hostedPage.Artifact.FileName, ContentType: hostedPage.Artifact.ContentType,
			SizeBytes: hostedPage.Artifact.SizeBytes, Digest: hostedPage.Artifact.Digest,
		}
		if err := store.SavePageArtifact(clientRequestID, hostedPage.Bytes, *request.Page); err != nil {
			cleanupErr := store.DeleteArtifact(replicapublication.Pending{ClientRequestID: clientRequestID})
			return replicaPublicationPresentation{}, errors.Join(err, cleanupErr)
		}
	}
	if options.CanonicalOrigin != "" {
		origin := options.CanonicalOrigin
		request.CanonicalOrigin = &origin
	}
	pending = replicapublication.Pending{
		EndpointOrigin: runtime.apiBaseURL, Market: market, ProjectPath: projectPath,
		ProjectFingerprint: projectFingerprint, ClientRequestID: clientRequestID,
		Request: request, SourceArchive: frozen.Summary, ArtifactExpiresAt: expiresAt,
		Preview: preview, AutoApplyCreator: options.AutoApplyCreator,
		Hosting: func() string {
			if request.Page != nil {
				return "HOSTED"
			}
			return "REPLICA_ONLY"
		}(),
		CreatorApplicationRequestID: creatorApplicationRequestID, CreatedAt: createdAt,
	}
	if err := store.Save(&pending); err != nil {
		// Keep an existing request recoverable after a permission change. A
		// failed atomic state replacement must not delete the previous state.
		if !found {
			return replicaPublicationPresentation{}, errors.Join(err, store.DeleteArtifact(pending))
		}
		return replicaPublicationPresentation{}, err
	}
	return createAndDriveReplicaPublication(ctx, runtime, store, pending)
}

func createAndDriveReplicaPublication(ctx context.Context, runtime *Runtime, store replicapublication.Store, pending replicapublication.Pending) (replicaPublicationPresentation, error) {
	client, err := optionalReplicaPublicationClient(runtime)
	if err != nil {
		return replicaPublicationPresentation{}, err
	}
	request := pending.Request
	if pending.ConfirmedAt != nil {
		if pending.Confirmation == nil {
			return replicaPublicationPresentation{}, output.Internal("REPLICA_PUBLICATION_CONFIRMATION_INVALID", "local Website Replica confirmation is incomplete", nil)
		}
		request.Confirmation = &api.WebsiteReplicaPublicationConfirmation{
			Version: pending.Confirmation.Version, Review: pending.Confirmation.Review,
			IssuedAt: pending.Confirmation.IssuedAt, ExpiresAt: pending.Confirmation.ExpiresAt,
			ConfirmedAt: pending.ConfirmedAt.UTC().Format(time.RFC3339Nano),
		}
	}
	progress(runtime, "Resolving Website Replica publication target")
	response, err := client.CreateWebsiteReplicaPublication(ctx, request)
	if err != nil {
		return replicaPublicationPresentation{}, err
	}
	if response.Outcome == "ACTION_REQUIRED" {
		if response.ClientRequestID != pending.ClientRequestID || response.Market != pending.Market {
			return replicaPublicationPresentation{}, invalidReplicaResponse("Website Replica next action does not match the local publication request")
		}
		return handleReplicaPublicationNextAction(ctx, runtime, store, pending, response.NextAction)
	}
	if err := validateLocalReplicaPublicationConfirmation(pending); err != nil {
		return replicaPublicationPresentation{}, err
	}
	if response.Publication == nil || response.Target == nil || response.Publication.ClientRequestID != pending.ClientRequestID ||
		response.Publication.Market != pending.Market || response.Publication.Source.Digest != pending.SourceArchive.Digest ||
		response.Publication.Source.SizeBytes != pending.SourceArchive.SizeBytes || !replicaPublicationPageMatchesRequest(*response.Publication, pending.Request) ||
		!replicaResolvedTargetMatchesRequest(*response.Target, pending.Confirmation, pending.Request) {
		return replicaPublicationPresentation{}, invalidReplicaResponse("Website Replica Publication does not match the frozen request")
	}
	pending.Publication = &replicapublication.PublicationReference{
		ID: response.Publication.ID, Status: response.Publication.Status, StatusURL: response.Publication.StatusURL,
	}
	pending.Target = response.Target
	if err := store.Save(&pending); err != nil {
		return replicaPublicationPresentation{}, err
	}
	return driveReplicaPublication(ctx, runtime, store, pending, *response.Publication, false)
}

func handleReplicaPublicationNextAction(ctx context.Context, runtime *Runtime, store replicapublication.Store, pending replicapublication.Pending, action api.WebsiteReplicaPublicationNextAction) (replicaPublicationPresentation, error) {
	resumeCommand := replicaStorageCommand(runtime, replicaPublishResumeCommand(pending))
	details := map[string]any{"nextAction": action, "clientRequestId": pending.ClientRequestID, "resumeCommand": resumeCommand}
	switch action.Kind {
	case "CONFIRM_PUBLICATION":
		if action.Confirmation == nil || !replicaConfirmationMatchesRequest(*action.Confirmation, pending.Request) {
			return replicaPublicationPresentation{}, invalidReplicaResponse("Website Replica final review does not match the frozen request")
		}
		expiresAt, valid := parseReplicaStateDatetime(action.Confirmation.ExpiresAt)
		if !valid {
			return replicaPublicationPresentation{}, invalidReplicaResponse("Website Replica final review expiration is invalid")
		}
		pending.Confirmation = action.Confirmation
		pending.ConfirmedAt = nil
		pending.ArtifactExpiresAt = expiresAt
		if err := store.Save(&pending); err != nil {
			return replicaPublicationPresentation{}, err
		}
		details["confirmationVersion"] = action.Confirmation.Version
		details["expiresAt"] = action.Confirmation.ExpiresAt
		details["review"] = finalReplicaPublicationReview(pending, *action.Confirmation)
		details["confirmCommand"] = resumeCommand + " --confirm " + action.Confirmation.Version
		return replicaPublicationPresentation{}, output.Confirmation(
			"REPLICA_PUBLICATION_CONFIRMATION_REQUIRED",
			"review the complete Website Replica publication and confirm it once before any source upload",
		).WithDetails(details).WithHint("rerun the reported confirmCommand before expiresAt only if every review field is accepted")
	case "AUTHENTICATE_CREATOR":
		return replicaPublicationPresentation{}, output.Authentication(
			"REPLICA_PUBLICATION_AUTHENTICATION_REQUIRED",
			"sign in before ViceMe can resolve an OWNER publication target; no source was uploaded",
		).WithDetails(details).WithHint("run 'viceme auth login', then rerun the same publish command")
	case "APPLY_CREATOR":
		if err := validateLocalReplicaPublicationConfirmation(pending); err != nil {
			return replicaPublicationPresentation{}, err
		}
		if !pending.AutoApplyCreator {
			details["automaticApplicationAuthorized"] = false
			details["authorizeCommand"] = resumeCommand + " --auto-apply-creator"
			return replicaPublicationPresentation{}, output.Confirmation(
				"REPLICA_CREATOR_APPLICATION_AUTHORIZATION_REQUIRED",
				"creator access is required and automatic application has not been authorized; no source was uploaded",
			).WithDetails(details).WithHint("rerun authorizeCommand only if ViceMe may submit one creator application, then resume this same publication request")
		}
		if pending.CreatorApplicationRequestID == "" {
			pending.CreatorApplicationRequestID = runtime.deps.NewID()
			if !replicaUUIDPattern.MatchString(pending.CreatorApplicationRequestID) {
				return replicaPublicationPresentation{}, output.Internal("REPLICA_CREATOR_APPLICATION_REQUEST_ID_INVALID", "could not create a valid creator application request identity", nil)
			}
			if err := store.Save(&pending); err != nil {
				return replicaPublicationPresentation{}, err
			}
		}
		progress(runtime, "Submitting the authorized creator application")
		application, err := runtime.client().CreateMerchantApplication(ctx, pending.CreatorApplicationRequestID, nil, nil)
		if err != nil {
			return replicaPublicationPresentation{}, err
		}
		details["automaticApplicationAuthorized"] = true
		details["application"] = application
		return replicaPublicationPresentation{}, output.Authorization(
			"REPLICA_CREATOR_APPLICATION_PENDING",
			"the authorized creator application was submitted; publication stopped without uploading source",
		).WithDetails(details).WithHint("complete or await creator review, then rerun the same publish command; do not start a second publication")
	case "WAIT_CREATOR_REVIEW":
		return replicaPublicationPresentation{}, output.Authorization(
			"REPLICA_CREATOR_REVIEW_PENDING",
			"creator review is pending; publication stopped without uploading source",
		).WithDetails(details)
	case "SUPPLY_CREATOR_INFO":
		return replicaPublicationPresentation{}, output.Authorization(
			"REPLICA_CREATOR_INFO_REQUIRED",
			"creator review needs more information; publication stopped without uploading source",
		).WithDetails(details)
	case "CREATOR_APPLICATION_REJECTED":
		return replicaPublicationPresentation{}, output.Policy(
			"REPLICA_CREATOR_APPLICATION_REJECTED",
			"creator application was rejected; publication stopped without uploading source",
		).WithDetails(details)
	case "SELECT_MERCHANT":
		return replicaPublicationPresentation{}, output.Confirmation(
			"REPLICA_PUBLICATION_MERCHANT_REQUIRED",
			"select the exact ACTIVE OWNER Merchant before final review; no source was uploaded",
		).WithDetails(details).WithHint("rerun the same publish command with --merchant-id from nextAction.merchants")
	case "CHOOSE_WORK_SLUG":
		return replicaPublicationPresentation{}, output.Confirmation(
			"REPLICA_PUBLICATION_SLUG_REQUIRED",
			"the requested Work URL is unavailable and must be reconfirmed; no source was uploaded",
		).WithDetails(details).WithHint("rerun the same publish command with --slug from nextAction.candidates to generate a new final review")
	case "UPGRADE_CLI":
		return replicaPublicationPresentation{}, output.Policy(
			"REPLICA_PUBLICATION_CLI_UPGRADE_REQUIRED",
			"this CLI cannot use the current Website Replica Publication protocol; no source was uploaded",
		).WithDetails(details)
	case "CHECK_STATUS":
		publication, err := runtime.client().GetWebsiteReplicaPublication(ctx, action.PublicationID)
		if err != nil {
			return replicaPublicationPresentation{}, err
		}
		if publication.StatusURL != action.StatusURL {
			return replicaPublicationPresentation{}, invalidReplicaResponse("Website Replica status action does not match the authoritative Publication")
		}
		if publication.ClientRequestID != pending.ClientRequestID {
			return presentStoredReplicaPublication(runtime, publication), nil
		}
		if err := validateLocalReplicaPublicationConfirmation(pending); err != nil {
			return replicaPublicationPresentation{}, err
		}
		pending.Publication = publicationReference(publication)
		return driveReplicaPublication(ctx, runtime, store, pending, publication, false)
	case "AUTHORIZE_SOURCE_UPLOAD", "AUTHORIZE_PAGE_UPLOAD":
		if err := validateLocalReplicaPublicationConfirmation(pending); err != nil {
			return replicaPublicationPresentation{}, err
		}
		publication, err := runtime.client().GetWebsiteReplicaPublication(ctx, action.PublicationID)
		if err != nil {
			return replicaPublicationPresentation{}, err
		}
		if publication.ClientRequestID != pending.ClientRequestID || publication.Source.Digest != pending.SourceArchive.Digest ||
			publication.Source.SizeBytes != pending.SourceArchive.SizeBytes || !replicaPublicationPageMatchesRequest(publication, pending.Request) {
			return replicaPublicationPresentation{}, invalidReplicaResponse("Website Replica upload action does not match the frozen local request")
		}
		pending.Publication = publicationReference(publication)
		return driveReplicaPublication(ctx, runtime, store, pending, publication, false)
	default:
		return replicaPublicationPresentation{}, invalidReplicaResponse("Website Replica Publication returned an unsupported next action")
	}
}

func replicaPublicationPageMatchesRequest(publication api.WebsiteReplicaPublication, request api.CreateWebsiteReplicaPublicationRequest) bool {
	if publication.AllowAutomaticDegradation != request.AllowAutomaticDegradation {
		return false
	}
	if request.Page == nil || publication.Page == nil {
		return request.Page == nil && publication.Page == nil
	}
	return publication.Page.FileName == request.Page.FileName && publication.Page.ContentType == request.Page.ContentType &&
		publication.Page.SizeBytes == request.Page.SizeBytes && publication.Page.Digest == request.Page.Digest
}

func validateLocalReplicaPublicationConfirmation(pending replicapublication.Pending) error {
	if pending.Confirmation == nil || pending.ConfirmedAt == nil ||
		!replicaConfirmationMatchesRequest(*pending.Confirmation, pending.Request) {
		return invalidReplicaResponse("Website Replica Publication crossed the local final-review boundary")
	}
	issuedAt, issuedValid := parseReplicaStateDatetime(pending.Confirmation.IssuedAt)
	expiresAt, expiresValid := parseReplicaStateDatetime(pending.Confirmation.ExpiresAt)
	if !issuedValid || !expiresValid || pending.ConfirmedAt.Before(issuedAt) || pending.ConfirmedAt.After(expiresAt) {
		return invalidReplicaResponse("Website Replica Publication local confirmation is invalid")
	}
	return nil
}

func startReplicaPublicationPreview(ctx context.Context, runtime *Runtime, options replicaPublishOptions) (replicapreview.Running, replicapublication.Preview, error) {
	if options.ConfirmUnverifiedPreview {
		return nil, replicapublication.Preview{Verified: false}, nil
	}
	if options.PreviewURL == "" {
		return nil, replicapublication.Preview{}, replicaPreviewBoundaryError(&replicapreview.StartError{
			Code: "REPLICA_PREVIEW_URL_REQUIRED", Stage: replicapreview.StageInspect,
			Message: "provide the actual loopback page URL selected and started by your agent",
		})
	}
	progress(runtime, "Starting local Website Replica preview")
	session, err := runtime.deps.StartReplicaPreview(ctx, replicapreview.Options{
		ExistingURL: options.PreviewURL, ErrOut: runtime.deps.ErrOut,
		Report: func(event replicapreview.Event) {
			_, _ = fmt.Fprintf(runtime.deps.ErrOut, "%s...\n", event.Message)
		},
	})
	if err != nil {
		return nil, replicapublication.Preview{}, err
	}
	result := session.Result()
	if err := runtime.deps.OpenURL(ctx, result.TargetURL); err != nil {
		return session, replicapublication.Preview{}, fmt.Errorf("open the local page: %w", err)
	}
	if !options.PreviewReviewed {
		return session, replicapublication.Preview{}, output.Validation("REPLICA_PREVIEW_REVIEW_REQUIRED", "the local service responded; your agent must observe the actual page before publication").WithDetails(map[string]any{
			"nextAction": "REVIEW_LOCAL_PREVIEW", "previewVerified": false,
			"browserVerificationRequired": true, "remoteUpload": false, "publicationCreated": false,
		}).WithHint("inspect the opened local page, then rerun with the same --preview-url and --preview-reviewed; connectivity alone is not visual verification")
	}
	_, _ = fmt.Fprintln(runtime.deps.ErrOut, "Local page opened; the final review is the only authorization to upload source.")
	return session, replicapublication.Preview{
		Verified: true, ReviewedBy: "AGENT", TargetURL: result.TargetURL, Reused: result.Reused, StartedByCLI: result.StartedByCLI,
	}, nil
}

func optionalReplicaPublicationClient(runtime *Runtime) (*api.Client, error) {
	if runtime.buyerClient != nil {
		return runtime.buyerClient, nil
	}
	if token, _, _ := runtime.overrideCredential(); token != "" {
		return runtime.client(), nil
	}
	status, err := runtime.manager().CurrentStatus()
	if err != nil {
		return nil, err
	}
	if status.Authenticated {
		return runtime.client(), nil
	}
	return api.NewClient(runtime.apiBaseURL, runtime.deps.HTTPClient, nil, "viceme/"+buildinfo.Version), nil
}

func replicaPublicationTarget(options replicaPublishOptions, binding replicapublication.Binding, bindingFound bool) (api.WebsiteReplicaPublicationTarget, string, error) {
	merchantID := options.MerchantAccountID
	if bindingFound && binding.Work != nil {
		if options.WorkID != "" || options.Slug != "" {
			return api.WebsiteReplicaPublicationTarget{}, "", output.Validation("REPLICA_PUBLICATION_TARGET_CONFLICT", "--work-id and --slug cannot replace an existing managed .viceme binding")
		}
		if merchantID != "" && merchantID != binding.Merchant.ID {
			return api.WebsiteReplicaPublicationTarget{}, "", output.Validation("REPLICA_PUBLICATION_MERCHANT_CONFLICT", "--merchant-id does not match the managed .viceme binding")
		}
		return api.WebsiteReplicaPublicationTarget{
			Kind: "MANAGED_BINDING", WorkID: binding.Work.ID,
			ReplicaID: binding.Replica.ID, ProductID: binding.Product.ID,
		}, binding.Merchant.ID, nil
	}
	if options.WorkID != "" {
		if options.Slug != "" {
			return api.WebsiteReplicaPublicationTarget{}, "", output.Validation("REPLICA_PUBLICATION_TARGET_CONFLICT", "use exactly one of --work-id or --slug")
		}
		return api.WebsiteReplicaPublicationTarget{Kind: "EXISTING_WORK", WorkID: options.WorkID}, merchantID, nil
	}
	if options.Slug == "" {
		return api.WebsiteReplicaPublicationTarget{}, "", output.Validation("REPLICA_PUBLICATION_SLUG_REQUIRED", "--slug is required when the project has no managed .viceme binding")
	}
	return api.WebsiteReplicaPublicationTarget{Kind: "NEW_WORK", Slug: options.Slug}, merchantID, nil
}

func finalReplicaPublicationReview(pending replicapublication.Pending, confirmation api.WebsiteReplicaPublicationConfirmationChallenge) replicaPublicationFinalReview {
	return replicaPublicationFinalReview{
		WebsiteReplicaPublicationReview: confirmation.Review,
		SourceArchive:                   pending.SourceArchive, Exclusions: pending.SourceArchive.ExcludedPaths,
		PageArtifact: pending.Request.Page, Hosting: pending.Hosting, AutomaticDegradation: pending.Request.AllowAutomaticDegradation,
		ImmutableVersions: true, ExistingBuyerVersionsRetained: true,
		AutomaticCreatorApplication: pending.AutoApplyCreator, Preview: pending.Preview,
		ConfirmationTTLSeconds: api.WebsiteReplicaPublicationConfirmationTTL,
		ConfirmationExpiresAt:  confirmation.ExpiresAt,
	}
}

func replicaConfirmationMatchesRequest(confirmation api.WebsiteReplicaPublicationConfirmationChallenge, request api.CreateWebsiteReplicaPublicationRequest) bool {
	review := confirmation.Review
	if review.ProjectFingerprint != request.ProjectFingerprint || review.Title != request.Title || review.Summary != request.Summary ||
		review.AllowAutomaticDegradation != request.AllowAutomaticDegradation || review.PriceCents != request.PriceCents || !reflect.DeepEqual(review.Source, request.Source) || !reflect.DeepEqual(review.Page, request.Page) {
		return false
	}
	if request.MerchantAccountID != "" && review.MerchantAccountID != request.MerchantAccountID {
		return false
	}
	if request.CanonicalOrigin == nil {
		return true
	}
	requestOrigin, requestValid := canonicalReplicaOrigin(*request.CanonicalOrigin)
	if review.CanonicalOrigin == nil || !requestValid {
		return false
	}
	reviewOrigin, reviewValid := canonicalReplicaOrigin(*review.CanonicalOrigin)
	return reviewValid && reviewOrigin == requestOrigin
}

func replicaResolvedTargetMatchesRequest(target api.WebsiteReplicaPublicationResolvedTarget, confirmation *api.WebsiteReplicaPublicationConfirmationChallenge, request api.CreateWebsiteReplicaPublicationRequest) bool {
	if confirmation == nil {
		return false
	}
	review := confirmation.Review
	if target.Resolution != review.Resolution || target.MerchantAccountID != review.MerchantAccountID || target.WorkURL != review.WorkURL {
		return false
	}
	switch request.Target.Kind {
	case "MANAGED_BINDING":
		return target.WorkID == request.Target.WorkID && target.ReplicaID == request.Target.ReplicaID &&
			target.ProductID != nil && *target.ProductID == request.Target.ProductID
	case "EXISTING_WORK":
		return target.WorkID == request.Target.WorkID
	case "NEW_WORK":
		if target.Resolution != "CREATE" {
			return true
		}
		parsed, err := url.Parse(target.WorkURL)
		if err != nil {
			return false
		}
		path := strings.Trim(parsed.Path, "/")
		segments := strings.Split(path, "/")
		return len(segments) >= 2 && segments[len(segments)-1] == request.Target.Slug
	default:
		return false
	}
}

func validateConfirmedReplicaRequest(options replicaPublishOptions, pending replicapublication.Pending, binding replicapublication.Binding, bindingFound bool) error {
	if pending.Confirmation == nil || pending.Confirmation.Version != options.ConfirmationVersion {
		return output.Confirmation("REPLICA_PUBLICATION_CONFIRMATION_CHANGED", "the supplied confirmation version does not match the current frozen final review").
			WithHint("run the same publish command without --confirm to generate a fresh final review")
	}
	if options.AutoApplyCreator != pending.AutoApplyCreator {
		return output.Confirmation("REPLICA_PUBLICATION_CONFIRMATION_CHANGED", "automatic creator-application authorization changed after the final review; no source was uploaded").
			WithHint("rerun the changed publish command without --confirm to generate a fresh final review")
	}
	if pending.Preview.ReviewedBy == "AGENT" &&
		(!options.PreviewReviewed || options.PreviewURL != pending.Preview.TargetURL || options.ConfirmUnverifiedPreview) {
		return output.Confirmation("REPLICA_PUBLICATION_CONFIRMATION_CHANGED", "the agent-reviewed preview changed after the final review; no source was uploaded").
			WithHint("rerun without --confirm to review the new preview and generate a fresh final review")
	}
	if options.ReplicaOnly && pending.Hosting != "REPLICA_ONLY" {
		return output.Confirmation("REPLICA_PUBLICATION_CONFIRMATION_CHANGED", "hosting selection changed after the final review; no artifact was uploaded").
			WithHint("rerun the changed publish command without --confirm to freeze it and generate a new final review")
	}
	target, merchantID, err := replicaPublicationTarget(options, binding, bindingFound)
	if err != nil {
		return err
	}
	expected := pending.Request
	expected.Target = target
	expected.MerchantAccountID = merchantID
	expected.Title = options.Title
	expected.Summary = options.Summary
	expected.PriceCents = options.PriceCents
	expected.CanonicalOrigin = nil
	if options.CanonicalOrigin != "" {
		origin := options.CanonicalOrigin
		expected.CanonicalOrigin = &origin
	}
	if !reflect.DeepEqual(expected, pending.Request) {
		return output.Confirmation("REPLICA_PUBLICATION_CONFIRMATION_CHANGED", "slug, Merchant, price, metadata, or target changed after the final review; no source was uploaded").
			WithHint("rerun the changed publish command without --confirm to freeze it and generate a new final review")
	}
	return nil
}

func normalizeReplicaPublishOptions(options replicaPublishOptions) replicaPublishOptions {
	options.ProjectPath = strings.TrimSpace(options.ProjectPath)
	options.PreviewURL = strings.TrimSpace(options.PreviewURL)
	options.WorkID = strings.TrimSpace(options.WorkID)
	options.Slug = strings.TrimSpace(options.Slug)
	options.MerchantAccountID = strings.TrimSpace(options.MerchantAccountID)
	options.CanonicalOrigin = strings.TrimSpace(options.CanonicalOrigin)
	if origin, valid := canonicalReplicaOrigin(options.CanonicalOrigin); valid {
		options.CanonicalOrigin = origin
	}
	options.Title = strings.TrimSpace(options.Title)
	options.Summary = strings.TrimSpace(options.Summary)
	options.ConfirmationVersion = strings.TrimSpace(options.ConfirmationVersion)
	return options
}

func validateReplicaPublishOptions(options replicaPublishOptions) error {
	if options.ProjectPath == "" {
		return output.Validation("REPLICA_PROJECT_PATH_REQUIRED", "--path is required")
	}
	if utf16CodeUnits(options.Title) < 1 || utf16CodeUnits(options.Title) > 200 || utf16CodeUnits(options.Summary) > 500 {
		return output.Validation("REPLICA_METADATA_INVALID", "--title must be 1-200 characters and --summary at most 500 characters")
	}
	if options.PriceCents < -1 || options.PriceCents > 10_000_000 {
		return output.Validation("REPLICA_PRICE_INVALID", "--price-cents must be between 0 and 10000000 when supplied")
	}
	if options.WorkID != "" && !replicaUUIDPattern.MatchString(options.WorkID) {
		return output.Validation("REPLICA_WORK_ID_INVALID", "--work-id must be a UUID")
	}
	if options.MerchantAccountID != "" && !replicaUUIDPattern.MatchString(options.MerchantAccountID) {
		return output.Validation("REPLICA_MERCHANT_ID_INVALID", "--merchant-id must be a UUID")
	}
	if options.Slug != "" && !validReplicaWorkSlug(options.Slug) {
		return output.Validation("REPLICA_WORK_SLUG_INVALID", "--slug must be 2-64 lowercase letters, digits, and single hyphens")
	}
	if options.CanonicalOrigin != "" {
		if _, valid := canonicalReplicaOrigin(options.CanonicalOrigin); !valid {
			return output.Validation("REPLICA_CANONICAL_ORIGIN_INVALID", "--canonical-origin must be a public HTTPS URL")
		}
	}
	if options.PreviewURL != "" {
		parsed, err := url.Parse(options.PreviewURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return output.Validation("REPLICA_PREVIEW_URL_INVALID", "--preview-url must be a valid local preview URL")
		}
	}
	if options.PreviewReviewed && (options.PreviewURL == "" || options.ConfirmUnverifiedPreview) {
		return output.Validation("REPLICA_PREVIEW_OPTIONS_INVALID", "--preview-reviewed requires --preview-url and cannot be combined with --confirm-unverified-replica-only")
	}
	if options.ConfirmationVersion != "" && !replicaConfirmationVersionPattern.MatchString(options.ConfirmationVersion) {
		return output.Validation("REPLICA_PUBLICATION_CONFIRMATION_INVALID", "--confirm must be the exact wrv1 confirmation version")
	}
	return nil
}

func replicaPublishResumeCommand(pending replicapublication.Pending) string {
	request := pending.Request
	parts := []string{"viceme replica publish", "--path", shellQuote(pending.ProjectPath), "--title", shellQuote(request.Title), "--summary", shellQuote(request.Summary), "--price-cents", fmt.Sprintf("%d", request.PriceCents)}
	switch request.Target.Kind {
	case "NEW_WORK":
		parts = append(parts, "--slug", shellQuote(request.Target.Slug))
	case "EXISTING_WORK":
		parts = append(parts, "--work-id", request.Target.WorkID)
	}
	if request.MerchantAccountID != "" {
		parts = append(parts, "--merchant-id", request.MerchantAccountID)
	}
	if request.CanonicalOrigin != nil {
		parts = append(parts, "--canonical-origin", shellQuote(*request.CanonicalOrigin))
	}
	if pending.Preview.Verified && pending.Preview.Reused && pending.Preview.TargetURL != "" {
		parts = append(parts, "--preview-url", shellQuote(pending.Preview.TargetURL))
		if pending.Preview.ReviewedBy == "AGENT" {
			parts = append(parts, "--preview-reviewed")
		}
	}
	if !pending.Preview.Verified {
		parts = append(parts, "--confirm-unverified-replica-only")
	}
	if pending.Hosting == "REPLICA_ONLY" {
		parts = append(parts, "--replica-only")
	}
	if pending.AutoApplyCreator {
		parts = append(parts, "--auto-apply-creator")
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\r\n'\"\\$`;&|<>()[]{}*?!") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func replicaPublicationMarket(runtime *Runtime) string {
	if runtime.profile.MarketRegion == "global" {
		return "GLOBAL"
	}
	return "CN"
}

func requireReplicaPublicationCN(runtime *Runtime) error {
	if replicaPublicationMarket(runtime) != "CN" {
		return output.Policy("REPLICA_PUBLICATION_MARKET_UNSUPPORTED", "Website Replica publication is currently available only in the CN market")
	}
	return nil
}

func replicaPublicationStore(runtime *Runtime) replicapublication.Store {
	market := replicaPublicationMarket(runtime)
	return replicapublication.Store{
		Directory: replicapublication.ScopedDirectory(
			filepath.Join(runtime.configBase, "replica-publications"), runtime.apiBaseURL, market,
		),
		EndpointOrigin: runtime.apiBaseURL, Market: market, Now: runtime.deps.Now,
	}
}

func replicaBindingStore(runtime *Runtime) replicapublication.BindingStore {
	return replicapublication.BindingStore{
		EndpointOrigin: runtime.apiBaseURL, Market: replicaPublicationMarket(runtime), Now: runtime.deps.Now,
	}
}

func utf16CodeUnits(value string) int { return len(utf16.Encode([]rune(value))) }

func validReplicaWorkSlug(value string) bool {
	if len(value) < 2 || len(value) > 64 || !replicaWorkSlugPattern.MatchString(value) {
		return false
	}
	switch value {
	case "works", "skills", "manage", "posts", "about":
		return false
	default:
		return true
	}
}

func canonicalReplicaOrigin(value string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", false
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "", false
	}
	port := parsed.Port()
	if port == "443" {
		port = ""
	}
	host := hostname
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	return "https://" + host, true
}

var (
	replicaWorkSlugPattern            = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	replicaConfirmationVersionPattern = regexp.MustCompile(`^wrv1-[a-f0-9]{64}$`)
)
