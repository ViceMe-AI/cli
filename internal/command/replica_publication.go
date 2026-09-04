package command

import (
	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

type replicaPublicationResume struct {
	Command string `json:"command"`
}

type replicaPublicationPresentation struct {
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
	return &cobra.Command{
		Use:   "status <publication-id>",
		Short: "Get authoritative Website Replica Publication status",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := requireReplicaPublicationCN(runtime); err != nil {
				return err
			}
			publicationID := args[0]
			if !replicaUUIDPattern.MatchString(publicationID) {
				return output.Validation("REPLICA_PUBLICATION_ID_INVALID", "Website Replica Publication ID must be a UUID")
			}
			store := replicaPublicationStore(runtime)
			if err := store.CleanupExpiredArtifacts(); err != nil {
				return err
			}
			publication, err := runtime.client().GetWebsiteReplicaPublication(command.Context(), publicationID)
			if err != nil {
				return err
			}
			pending, found, err := store.LoadPublication(publicationID)
			if err != nil {
				return err
			}
			if found {
				presentation, err := synchronizeReplicaPublication(runtime, store, pending, publication)
				if err != nil {
					return err
				}
				return runtime.business(presentation)
			}
			return runtime.business(presentReplicaPublication(publication))
		},
	}
}

func presentReplicaPublication(publication api.WebsiteReplicaPublication) replicaPublicationPresentation {
	phase := publication.Status
	message := "Website Replica Publication has not been submitted."
	switch publication.Status {
	case "PROCESSING":
		phase = "SUBMITTED_NOT_PUBLISHED"
		message = "Website Replica Publication was submitted and is not published yet."
	case "PUBLISHED":
		message = "Website Replica publication complete."
	case "PUBLISHED_DEGRADED":
		message = "Website Replica publication complete with degraded hosting: source is published, hosting failed, and the native Work page is active."
	case "FAILED":
		message = "Website Replica Publication failed and was not published."
	case "CANCELLED":
		message = "Website Replica Publication was cancelled and was not published."
	}
	return replicaPublicationPresentation{
		PublicationID:  publication.ID,
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
