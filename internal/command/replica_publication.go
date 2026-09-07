package command

import (
	"context"
	"errors"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

type replicaPublicationResume struct {
	Command string `json:"command"`
}

type replicaPublicationPresentation struct {
	Hosting        api.WebsiteReplicaHostingProjection   `json:"hosting"`
	Rollback       api.WebsiteReplicaRollbackProjection  `json:"rollback"`
	PriceCents     int                                   `json:"priceCents"`
	PublicationID  string                                `json:"publicationId"`
	Status         string                                `json:"status"`
	Phase          string                                `json:"phase"`
	Message        string                                `json:"message"`
	StatusURL      string                                `json:"statusUrl"`
	Source         api.WebsiteReplicaPublicationSource   `json:"source"`
	Page           *api.WebsiteReplicaPublicationSource  `json:"page"`
	Failure        *api.WebsiteReplicaPublicationFailure `json:"failure"`
	Result         *api.WebsiteReplicaPublicationResult  `json:"result"`
	AllowedActions []string                              `json:"allowedActions"`
	Resume         replicaPublicationResume              `json:"resume"`
}

func newReplicaStatusCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "status <publication-id>", Short: "Get authoritative Website Replica Publication status", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		result, err := controlReplicaPublication(command.Context(), runtime, args[0], false)
		if err != nil {
			return err
		}
		return runtime.business(result)
	}}
	addReplicaStorageFlag(command, runtime)
	return command
}

// Release recovery locks before the caller emits its single JSON envelope.
func controlReplicaPublication(ctx context.Context, runtime *Runtime, publicationID string, cancel bool) (_ replicaPublicationPresentation, returnErr error) {
	if err := requireReplicaPublicationCN(runtime); err != nil {
		return replicaPublicationPresentation{}, err
	}
	publicationID = strings.TrimSpace(publicationID)
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
	var publication api.WebsiteReplicaPublication
	if cancel {
		progress(runtime, "Cancelling Website Replica Publication")
		publication, err = runtime.client().CancelWebsiteReplicaPublication(ctx, publicationID)
	} else {
		publication, err = runtime.client().GetWebsiteReplicaPublication(ctx, publicationID)
	}
	if err != nil {
		return replicaPublicationPresentation{}, err
	}
	if found {
		return synchronizeReplicaPublication(runtime, store, pending, publication)
	}
	return presentStoredReplicaPublication(runtime, publication), nil
}

func presentReplicaPublication(publication api.WebsiteReplicaPublication) replicaPublicationPresentation {
	phase := publication.Status
	message := "Website Replica Publication has not been submitted."
	switch publication.Status {
	case "PROCESSING":
		phase = "SUBMITTED_NOT_PUBLISHED"
		message = "Website Replica Publication was submitted and is not published yet."
	case "PUBLISHED":
		message = "Website Replica source publication complete; no hosted HTML page is active."
		if publication.Hosting.Status == "ACTIVE" {
			message = "Website Replica source and hosted HTML publication complete."
		}
	case "PUBLISHED_DEGRADED":
		message = "Website Replica publication complete with degraded hosting: source is published, hosting failed, and the native Work page is active."
		if publication.Hosting.Status == "ACTIVE" {
			message = "Website Replica source remains published; hosting has been repaired. The original degraded publication audit is retained."
		}
	case "FAILED":
		message = "Website Replica Publication failed and was not published."
	case "CANCELLED":
		message = "Website Replica Publication was cancelled and was not published."
	}
	return replicaPublicationPresentation{
		PublicationID: publication.ID,
		Hosting:       publication.Hosting, Rollback: publication.Rollback, PriceCents: publication.PriceCents,
		Status:         publication.Status,
		Phase:          phase,
		Message:        message,
		StatusURL:      publication.StatusURL,
		Source:         publication.Source,
		Page:           publication.Page,
		Failure:        publication.Failure,
		Result:         publication.Result,
		AllowedActions: publication.AllowedActions,
		Resume:         replicaPublicationResume{Command: "viceme replica resume " + publication.ID},
	}
}
