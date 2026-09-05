package command

import (
	"context"
	"errors"
	"io/fs"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/replicapublication"
	"github.com/spf13/cobra"
)

func newReplicaResumeCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "resume <publication-id>",
		Short: "Resume missing Website Replica upload or processing steps",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := resumeWebsiteReplicaPublication(command.Context(), runtime, strings.TrimSpace(args[0]))
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	addReplicaStorageFlag(command, runtime)
	return command
}

func newReplicaCancelCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "cancel <publication-id>", Short: "Cancel a Website Replica Publication before activation", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		result, err := controlReplicaPublication(command.Context(), runtime, args[0], true)
		if err != nil {
			return err
		}
		return runtime.business(result)
	}}
	addReplicaStorageFlag(command, runtime)
	return command
}

func resumeWebsiteReplicaPublication(ctx context.Context, runtime *Runtime, publicationID string) (_ replicaPublicationPresentation, returnErr error) {
	if err := requireReplicaPublicationCN(runtime); err != nil {
		return replicaPublicationPresentation{}, err
	}
	if !replicaUUIDPattern.MatchString(publicationID) {
		return replicaPublicationPresentation{}, output.Validation("REPLICA_PUBLICATION_ID_INVALID", "Website Replica Publication ID must be a UUID")
	}
	store, err := selectedReplicaPublicationStore(runtime)
	if err != nil {
		return replicaPublicationPresentation{}, err
	}
	pending, found, unlock, err := loadReplicaRecovery(runtime, store, publicationID)
	if err != nil {
		return replicaPublicationPresentation{}, err
	}
	defer func() { returnErr = errors.Join(returnErr, unlock()) }()
	progress(runtime, "Reading authoritative Website Replica Publication status")
	publication, err := runtime.client().GetWebsiteReplicaPublication(ctx, publicationID)
	if err != nil {
		return replicaPublicationPresentation{}, err
	}
	if !found {
		if publication.Status == "DRAFT" {
			return replicaPublicationPresentation{}, output.Validation(
				"REPLICA_PUBLICATION_RECOVERY_NOT_FOUND",
				"this draft needs its original frozen source, but no matching local recovery state exists",
			).WithDetails(map[string]any{
				"publicationId": publication.ID, "statusUrl": publication.StatusURL,
			}).WithHint("return to the original project and CLI profile, or cancel this draft before rebuilding it")
		}
		if publication.Status == "FAILED" && publication.Failure != nil && publication.Failure.Retryable && hasReplicaPublicationAction(publication, "RETRY") {
			progress(runtime, "Retrying Website Replica Publication processing")
			publication, err = runtime.client().RetryWebsiteReplicaPublication(ctx, publicationID)
			if err != nil {
				return replicaPublicationPresentation{}, err
			}
		}
		return presentStoredReplicaPublication(runtime, publication), nil
	}
	return driveReplicaPublication(ctx, runtime, store, pending, publication, true)
}

func driveReplicaPublication(ctx context.Context, runtime *Runtime, store replicapublication.Store, pending replicapublication.Pending, publication api.WebsiteReplicaPublication, allowRetry bool) (replicaPublicationPresentation, error) {
	if err := preflightReplicaRecovery(runtime, store, pending); err != nil {
		return replicaPublicationPresentation{}, withReplicaPublicationRecovery(runtime, err, publication)
	}
	for step := 0; step < 5; step++ {
		if err := validateReplicaPublicationRecovery(pending, publication); err != nil {
			return replicaPublicationPresentation{}, err
		}
		if publication.Status == "DRAFT" {
			if err := validateLocalReplicaPublicationConfirmation(pending); err != nil {
				return replicaPublicationPresentation{}, err
			}
		}
		if publication.Status != "DRAFT" {
			if publication.Status == "FAILED" && allowRetry && publication.Failure != nil && publication.Failure.Retryable && hasReplicaPublicationAction(publication, "RETRY") {
				progress(runtime, "Retrying Website Replica Publication processing")
				retried, err := runtime.client().RetryWebsiteReplicaPublication(ctx, publication.ID)
				if err != nil {
					return replicaPublicationPresentation{}, withReplicaPublicationRecovery(runtime, err, publication)
				}
				publication = retried
				allowRetry = false
				continue
			}
			return synchronizeReplicaPublication(runtime, store, pending, publication)
		}

		pending.Publication = publicationReference(publication)
		if err := store.Save(&pending); err != nil {
			return replicaPublicationPresentation{}, err
		}
		if publication.Page != nil {
			switch publication.Page.Status {
			case "WAITING_UPLOAD":
				if !runtime.deps.Now().Before(pending.ArtifactExpiresAt) {
					cleanupErr := store.DeleteArtifact(pending)
					return replicaPublicationPresentation{}, output.Validation(
						"REPLICA_PUBLICATION_ARTIFACT_EXPIRED",
						"the recoverable upload window expired and the frozen artifacts were removed",
					).WithDetails(replicaRecoveryDetails(runtime, publication)).WithHint(
						"cancel this draft, then run replica publish again to preview, freeze, and confirm a new Publication",
					).WithCause(cleanupErr)
				}
				if !hasReplicaPublicationAction(publication, "AUTHORIZE_PAGE_UPLOAD") {
					return presentStoredReplicaPublication(runtime, publication), nil
				}
				authorization, err := runtime.client().AuthorizeWebsiteReplicaPublicationPageUpload(ctx, publication.ID)
				if err != nil {
					return replicaPublicationPresentation{}, withReplicaPublicationRecovery(runtime, err, publication)
				}
				file, err := store.OpenPageArtifact(pending)
				if err != nil {
					return replicaPublicationPresentation{}, withReplicaPublicationRecovery(runtime, err, publication)
				}
				progress(runtime, "Uploading frozen Website Replica page")
				uploadErr := runtime.client().PutUpload(ctx, api.UploadAuthorization{
					Method: authorization.Upload.Method, URL: authorization.Upload.URL,
					ExpiresAt: authorization.Upload.ExpiresAt, Headers: authorization.Upload.Headers,
				}, file, pending.Request.Page.SizeBytes)
				closeErr := file.Close()
				if errors.Is(closeErr, fs.ErrClosed) {
					closeErr = nil
				}
				if uploadErr != nil || closeErr != nil {
					return replicaPublicationPresentation{}, withReplicaPublicationRecovery(runtime, errors.Join(uploadErr, closeErr), publication)
				}
				progress(runtime, "Verifying uploaded Website Replica page")
				completed, err := runtime.client().CompleteWebsiteReplicaPublicationPageUpload(ctx, publication.ID)
				if err != nil {
					return replicaPublicationPresentation{}, withReplicaPublicationRecovery(runtime, err, publication)
				}
				publication = completed
				if publication.Page == nil || (publication.Page.Status != "VERIFIED" && publication.Page.Status != "FAILED") {
					return synchronizeReplicaPublication(runtime, store, pending, publication)
				}
			case "UPLOADED", "VALIDATING":
				if !hasReplicaPublicationAction(publication, "COMPLETE_PAGE_UPLOAD") {
					return presentStoredReplicaPublication(runtime, publication), nil
				}
				progress(runtime, "Resuming Website Replica page verification")
				completed, err := runtime.client().CompleteWebsiteReplicaPublicationPageUpload(ctx, publication.ID)
				if err != nil {
					return replicaPublicationPresentation{}, withReplicaPublicationRecovery(runtime, err, publication)
				}
				publication = completed
				if publication.Page == nil || (publication.Page.Status != "VERIFIED" && publication.Page.Status != "FAILED") {
					return synchronizeReplicaPublication(runtime, store, pending, publication)
				}
			case "VERIFIED":
			case "ACTIVATED":
				return replicaPublicationPresentation{}, invalidReplicaResponse("draft Website Replica Publication exposed an activated page")
			case "FAILED":
			default:
				return replicaPublicationPresentation{}, invalidReplicaResponse("Website Replica Publication exposed an unknown page state")
			}
		}
		switch publication.Source.Status {
		case "WAITING_UPLOAD":
			if !runtime.deps.Now().Before(pending.ArtifactExpiresAt) {
				cleanupErr := store.DeleteArtifact(pending)
				return replicaPublicationPresentation{}, output.Validation(
					"REPLICA_PUBLICATION_ARTIFACT_EXPIRED",
					"the recoverable upload window expired and the frozen source was removed",
				).WithDetails(replicaRecoveryDetails(runtime, publication)).WithHint(
					"cancel this draft, then run replica publish again to preview, freeze, and confirm a new Publication",
				).WithCause(cleanupErr)
			}
			if !hasReplicaPublicationAction(publication, "AUTHORIZE_SOURCE_UPLOAD") {
				return presentStoredReplicaPublication(runtime, publication), nil
			}
			authorization, err := runtime.client().AuthorizeWebsiteReplicaPublicationSourceUpload(ctx, publication.ID)
			if err != nil {
				return replicaPublicationPresentation{}, withReplicaPublicationRecovery(runtime, err, publication)
			}
			file, err := store.OpenArtifact(pending)
			if err != nil {
				return replicaPublicationPresentation{}, withReplicaPublicationRecovery(runtime, err, publication)
			}
			progress(runtime, "Uploading frozen Website Replica source")
			uploadErr := runtime.client().PutUpload(ctx, api.UploadAuthorization{
				Method: authorization.Upload.Method, URL: authorization.Upload.URL,
				ExpiresAt: authorization.Upload.ExpiresAt, Headers: authorization.Upload.Headers,
			}, file, pending.SourceArchive.SizeBytes)
			closeErr := file.Close()
			if errors.Is(closeErr, fs.ErrClosed) {
				closeErr = nil
			}
			if uploadErr != nil || closeErr != nil {
				return replicaPublicationPresentation{}, withReplicaPublicationRecovery(runtime, errors.Join(uploadErr, closeErr), publication)
			}
			progress(runtime, "Verifying uploaded Website Replica source")
			completed, err := runtime.client().CompleteWebsiteReplicaPublicationSourceUpload(ctx, publication.ID)
			if err != nil {
				return replicaPublicationPresentation{}, withReplicaPublicationRecovery(runtime, err, publication)
			}
			publication = completed
			if publication.Source.Status != "VERIFIED" {
				return synchronizeReplicaPublication(runtime, store, pending, publication)
			}
		case "UPLOADED", "VALIDATING":
			if !hasReplicaPublicationAction(publication, "COMPLETE_SOURCE_UPLOAD") {
				return presentStoredReplicaPublication(runtime, publication), nil
			}
			progress(runtime, "Resuming Website Replica source verification")
			completed, err := runtime.client().CompleteWebsiteReplicaPublicationSourceUpload(ctx, publication.ID)
			if err != nil {
				return replicaPublicationPresentation{}, withReplicaPublicationRecovery(runtime, err, publication)
			}
			publication = completed
			if publication.Source.Status != "VERIFIED" {
				return synchronizeReplicaPublication(runtime, store, pending, publication)
			}
		case "VERIFIED":
		case "ACTIVATED":
			return replicaPublicationPresentation{}, invalidReplicaResponse("draft Website Replica Publication exposed an activated source")
		case "FAILED":
			return synchronizeReplicaPublication(runtime, store, pending, publication)
		default:
			return replicaPublicationPresentation{}, invalidReplicaResponse("Website Replica Publication exposed an unknown source state")
		}
		if publication.Status == "DRAFT" && publication.Source.Status == "VERIFIED" &&
			(publication.Page == nil || publication.Page.Status == "VERIFIED" || publication.Page.Status == "FAILED") {
			if !hasReplicaPublicationAction(publication, "SUBMIT") {
				return presentStoredReplicaPublication(runtime, publication), nil
			}
			progress(runtime, "Submitting Website Replica Publication for asynchronous processing")
			submitted, err := runtime.client().SubmitWebsiteReplicaPublication(ctx, publication.ID)
			if err != nil {
				return replicaPublicationPresentation{}, withReplicaPublicationRecovery(runtime, err, publication)
			}
			publication = submitted
			continue
		}
	}
	return replicaPublicationPresentation{}, output.Internal("REPLICA_PUBLICATION_STATE_STALLED", "Website Replica Publication did not reach a stable recoverable state", nil)
}

func synchronizeReplicaPublication(runtime *Runtime, store replicapublication.Store, pending replicapublication.Pending, publication api.WebsiteReplicaPublication) (_ replicaPublicationPresentation, returnErr error) {
	defer func() {
		if returnErr != nil {
			returnErr = withReplicaPublicationRecovery(runtime, returnErr, publication)
		}
	}()
	if err := validateReplicaPublicationRecovery(pending, publication); err != nil {
		return replicaPublicationPresentation{}, err
	}
	pending.Publication = publicationReference(publication)
	if publication.Status == "PROCESSING" || publication.Status == "PUBLISHED" || publication.Status == "PUBLISHED_DEGRADED" ||
		(publication.Status == "FAILED" && publication.SubmittedAt != nil) {
		pending.TakenOver = true
	}
	if err := store.Save(&pending); err != nil {
		if pending.TakenOver {
			err = errors.Join(err, store.DeleteArtifact(pending))
		}
		return replicaPublicationPresentation{}, err
	}

	bindingStore := replicaBindingStore(runtime)
	shouldBind := pending.TakenOver || publication.Status == "PUBLISHED" || publication.Status == "PUBLISHED_DEGRADED"
	if publication.Status == "CANCELLED" && !shouldBind {
		_, found, err := bindingStore.Load(pending.ProjectPath)
		if err != nil {
			return replicaPublicationPresentation{}, err
		}
		shouldBind = found
	}
	var synchronizationErr error
	if shouldBind {
		synchronizationErr = bindingStore.Save(pending.ProjectPath, pending, publication)
	}

	if pending.TakenOver || publication.Status == "CANCELLED" {
		synchronizationErr = errors.Join(synchronizationErr, store.DeleteArtifact(pending))
	}
	if synchronizationErr != nil {
		return replicaPublicationPresentation{}, synchronizationErr
	}
	finishedLocally := publication.Status == "PUBLISHED" || publication.Status == "PUBLISHED_DEGRADED" || publication.Status == "CANCELLED" ||
		(publication.Status == "FAILED" && !hasReplicaPublicationAction(publication, "RETRY"))
	if finishedLocally {
		if err := store.Delete(pending); err != nil {
			return replicaPublicationPresentation{}, err
		}
	}
	return presentStoredReplicaPublication(runtime, publication), nil
}

func publicationReference(publication api.WebsiteReplicaPublication) *replicapublication.PublicationReference {
	return &replicapublication.PublicationReference{ID: publication.ID, Status: publication.Status, StatusURL: publication.StatusURL}
}

func hasReplicaPublicationAction(publication api.WebsiteReplicaPublication, action string) bool {
	for _, candidate := range publication.AllowedActions {
		if candidate == action {
			return true
		}
	}
	return false
}

func validateReplicaPublicationRecovery(pending replicapublication.Pending, publication api.WebsiteReplicaPublication) error {
	if pending.Publication == nil || publication.ID != pending.Publication.ID || publication.ClientRequestID != pending.ClientRequestID ||
		publication.Market != pending.Market || publication.Source.Digest != pending.SourceArchive.Digest ||
		publication.Source.SizeBytes != pending.SourceArchive.SizeBytes {
		return invalidReplicaResponse("Website Replica Publication recovery does not match the frozen local request")
	}
	if (pending.Request.Page == nil) != (publication.Page == nil) ||
		(pending.Request.Page != nil && (publication.Page.Digest != pending.Request.Page.Digest || publication.Page.SizeBytes != pending.Request.Page.SizeBytes)) {
		return invalidReplicaResponse("Website Replica Publication recovery does not match the frozen local page")
	}
	if pending.Confirmation != nil {
		review := pending.Confirmation.Review
		if publication.MerchantAccountID != review.MerchantAccountID ||
			(publication.Result != nil && publication.Result.WorkURL != review.WorkURL) {
			return invalidReplicaResponse("Website Replica Publication recovery does not match the confirmed target")
		}
	}
	if pending.Target != nil {
		target := pending.Target
		if publication.MerchantAccountID != target.MerchantAccountID || publication.WorkID != target.WorkID ||
			publication.ReplicaID != target.ReplicaID || (publication.Result != nil && publication.Result.WorkURL != target.WorkURL) ||
			(target.ProductID != nil && publication.Result != nil && publication.Result.Product.ID != *target.ProductID) {
			return invalidReplicaResponse("Website Replica Publication recovery does not match the resolved target")
		}
	}
	if publication.Result != nil {
		expectedCurrency := "CNY"
		if pending.Market == "GLOBAL" {
			expectedCurrency = "USD"
		}
		if publication.Result.Product.Title != pending.Request.Title ||
			publication.Result.Product.Currency != expectedCurrency || publication.Result.Product.PriceCents != pending.Request.PriceCents {
			return invalidReplicaResponse("Website Replica Publication result does not match the confirmed product metadata")
		}
	}
	return nil
}

func withReplicaPublicationRecovery(runtime *Runtime, err error, publication api.WebsiteReplicaPublication) error {
	if err == nil {
		return nil
	}
	cliErr := output.AsError(err)
	cliErr.PublicationID = publication.ID
	cliErr.ConsoleURL = publication.StatusURL
	mergeReplicaRecoveryDetails(cliErr, map[string]any{
		"publicationId": publication.ID,
		"statusUrl":     publication.StatusURL,
		"resume":        map[string]string{"command": replicaStorageCommand(runtime, "viceme replica resume "+publication.ID)},
	})
	return cliErr
}

func replicaRecoveryDetails(runtime *Runtime, publication api.WebsiteReplicaPublication) map[string]any {
	return map[string]any{
		"publicationId": publication.ID,
		"statusUrl":     publication.StatusURL,
		"resume":        map[string]string{"command": replicaStorageCommand(runtime, "viceme replica resume "+publication.ID)},
	}
}
