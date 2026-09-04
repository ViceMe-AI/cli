package command

import (
	"context"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

type replicaRollbackResult struct {
	PublicationID   string                                  `json:"publicationId"`
	SourceVersion   int                                     `json:"sourceVersion"`
	SourceVersionID string                                  `json:"sourceVersionId"`
	WorkRevisionID  string                                  `json:"workRevisionId"`
	PageMode        string                                  `json:"pageMode"`
	PageRelease     *api.WebsiteReplicaPageReleaseReference `json:"pageRelease"`
	ProductID       string                                  `json:"productId"`
	SKUID           string                                  `json:"skuId"`
	PriceCents      int                                     `json:"priceCents"`
	Currency        string                                  `json:"currency"`
	PriceUnchanged  bool                                    `json:"priceUnchanged"`
}

func newReplicaRollbackCommand(runtime *Runtime) *cobra.Command {
	var publicationID, pairID string
	command := &cobra.Command{
		Use:   "rollback",
		Short: "Roll back to an authoritative Website Replica source/page pair",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := rollbackReplica(command.Context(), runtime, publicationID, pairID)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&publicationID, "publication", "", "Website Replica Publication UUID")
	command.Flags().StringVar(&pairID, "pair", "", "authoritative rollback pair UUID")
	_ = command.MarkFlagRequired("publication")
	_ = command.MarkFlagRequired("pair")
	return command
}

func rollbackReplica(ctx context.Context, runtime *Runtime, publicationID, pairID string) (replicaRollbackResult, error) {
	publicationID = strings.TrimSpace(publicationID)
	pairID = strings.TrimSpace(pairID)
	if !replicaUUIDPattern.MatchString(publicationID) {
		return replicaRollbackResult{}, output.Validation("REPLICA_PUBLICATION_ID_INVALID", "--publication must be a UUID")
	}
	if !replicaUUIDPattern.MatchString(pairID) {
		return replicaRollbackResult{}, output.Validation("REPLICA_VERSION_PAIR_ID_INVALID", "--pair must be a UUID")
	}
	if err := runtime.requireWebsiteReplicaAuthentication(ctx, "website-replica:read", "website-replica:write"); err != nil {
		return replicaRollbackResult{}, err
	}
	state, err := runtime.client().GetWebsiteReplicaPublicationRollbackState(ctx, publicationID)
	if err != nil {
		return replicaRollbackResult{}, err
	}
	if state.Rollback.ActivePair == nil {
		return replicaRollbackResult{}, output.Policy("REPLICA_ROLLBACK_STATE_UNAVAILABLE", "Website Replica rollback state is unavailable")
	}
	var target *api.WebsiteReplicaVersionPair
	for index := range state.Rollback.AvailablePairs {
		if state.Rollback.AvailablePairs[index].ID == pairID {
			target = &state.Rollback.AvailablePairs[index]
			break
		}
	}
	if target == nil {
		return replicaRollbackResult{}, output.Policy("REPLICA_ROLLBACK_TARGET_UNAVAILABLE", "--pair is not an authoritative rollback target in the current Publication status")
	}
	clientRequestID := runtime.deps.NewID()
	if !replicaUUIDPattern.MatchString(clientRequestID) {
		return replicaRollbackResult{}, output.Internal("REPLICA_CLIENT_REQUEST_ID_INVALID", "could not create a valid Replica request identity", nil)
	}
	rolledBack, err := runtime.client().RollbackWebsiteReplicaPublication(ctx, publicationID, api.WebsiteReplicaRollbackRequest{
		ClientRequestID: clientRequestID, TargetPairID: target.ID, ExpectedActivePairID: state.Rollback.ActivePair.ID,
	})
	if err != nil {
		return replicaRollbackResult{}, err
	}
	pageMode := "NATIVE_WORK"
	if rolledBack.ActivePair.PageRelease != nil {
		pageMode = "HOSTED"
	}
	return replicaRollbackResult{
		PublicationID:   rolledBack.PublicationID,
		SourceVersion:   rolledBack.ActivePair.ReplicaVersion.Version,
		SourceVersionID: rolledBack.ActivePair.ReplicaVersion.ID,
		WorkRevisionID:  rolledBack.ActivePair.WorkRevisionID,
		PageMode:        pageMode,
		PageRelease:     rolledBack.ActivePair.PageRelease,
		ProductID:       rolledBack.Product.ID,
		SKUID:           rolledBack.Product.SKUID,
		PriceCents:      rolledBack.Product.PriceCents,
		Currency:        rolledBack.Product.Currency,
		PriceUnchanged:  rolledBack.PriceUnchanged,
	}, nil
}
