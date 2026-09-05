package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newReplicaSalesCommand(runtime *Runtime, operation string) *cobra.Command {
	var replicaID, requestID, confirmation, snapshot string
	var price int
	command := &cobra.Command{Use: operation, Short: "Inspect or manage the current Website Replica sales state", Args: cobra.NoArgs}
	command.Flags().StringVar(&replicaID, "replica", "", "Website Replica UUID")
	_ = command.MarkFlagRequired("replica")
	if operation != "sales" {
		command.Flags().StringVar(&requestID, "request-id", "", "idempotent request UUID from the review")
		command.Flags().StringVar(&snapshot, "request", "", "exact encoded request snapshot from the review")
		command.Flags().StringVar(&confirmation, "confirm", "", "exact confirmation digest from the review")
	}
	if operation == "price" {
		command.Flags().IntVar(&price, "price-cents", 0, "new CNY price in cents (0 is free)")
		_ = command.MarkFlagRequired("price-cents")
	}
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := requireReplicaPublicationCN(runtime); err != nil {
			return err
		}
		if !replicaUUIDPattern.MatchString(replicaID) {
			return output.Validation("REPLICA_ID_INVALID", "--replica must be a UUID")
		}
		if price < 0 || price > 10_000_000 {
			return output.Validation("REPLICA_PRICE_INVALID", "--price-cents must be between 0 and 10000000")
		}
		if requestID != "" && !replicaUUIDPattern.MatchString(requestID) {
			return output.Validation("REPLICA_CLIENT_REQUEST_ID_INVALID", "--request-id must be a UUID")
		}
		if confirmation != "" && (len(confirmation) != 64 || requestID == "") {
			return output.Validation("REPLICA_SALES_CONFIRMATION_INVALID", "use the exact confirmation command from the review")
		}
		if err := runtime.requireWebsiteReplicaAuthentication(cmd.Context(), "website-replica:write"); err != nil {
			return err
		}
		if confirmation != "" {
			raw, err := base64.RawURLEncoding.DecodeString(snapshot)
			var reviewed replicaSalesConfirmation
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if err != nil || decoder.Decode(&reviewed) != nil || decoder.Decode(new(any)) != io.EOF {
				return output.Validation("REPLICA_SALES_CONFIRMATION_INVALID", "use the exact confirmation command from the review")
			}
			canonical, _ := json.Marshal(reviewed)
			sum := sha256.Sum256(canonical)
			input := reviewed.Request
			validPrice := (operation == "price" && input.PriceCents != nil && *input.PriceCents == price) || (operation != "price" && input.PriceCents == nil)
			if hex.EncodeToString(sum[:]) != confirmation || reviewed.Authority != runtime.client().BaseURL || reviewed.Operation != operation || reviewed.ReplicaID != replicaID || input.ClientRequestID != requestID || !validPrice || input.ExpectedRevision <= 0 || !replicaUUIDPattern.MatchString(input.ExpectedProductID) || !replicaUUIDPattern.MatchString(input.ExpectedSalesSpecVersionID) || !replicaUUIDPattern.MatchString(input.ExpectedReplicaVersionID) {
				return output.Validation("REPLICA_SALES_CONFIRMATION_INVALID", "confirmation does not match the reviewed operation")
			}
			result, err := runtime.client().MutateWebsiteReplicaSales(cmd.Context(), replicaID, operation, input)
			if err != nil {
				return err
			}
			return runtime.business(result)
		}
		if snapshot != "" {
			return output.Validation("REPLICA_SALES_CONFIRMATION_INVALID", "--request requires --confirm")
		}
		state, err := runtime.client().GetWebsiteReplicaSales(cmd.Context(), replicaID)
		if err != nil {
			return err
		}
		if operation == "sales" {
			return runtime.business(state)
		}
		if !state.OperationsEnabled {
			return output.Policy("REPLICA_SALES_READ_ONLY", "the OWNER may inspect history but cannot change sales")
		}
		if state.Product.Currency != "CNY" {
			return output.Policy("REPLICA_SALES_MARKET_UNSUPPORTED", "sales management currently requires the CN CNY product")
		}
		if requestID == "" {
			requestID = runtime.deps.NewID()
		}
		if !replicaUUIDPattern.MatchString(requestID) {
			return output.Internal("REPLICA_CLIENT_REQUEST_ID_INVALID", "could not create a request identity", nil)
		}
		input := api.WebsiteReplicaSalesRequest{ClientRequestID: requestID, ExpectedProductID: state.Product.ID, ExpectedRevision: state.Product.Revision, ExpectedSalesSpecVersionID: state.Product.SalesSpecVersionID, ExpectedReplicaVersionID: state.ReplicaVersion.ID}
		if operation == "price" {
			input.PriceCents = &price
		}
		review := struct {
			Authority string                         `json:"authority"`
			Operation string                         `json:"operation"`
			State     api.WebsiteReplicaSalesState   `json:"state"`
			Request   api.WebsiteReplicaSalesRequest `json:"request"`
			Impact    string                         `json:"impact"`
		}{runtime.client().BaseURL, operation, state, input, replicaSalesImpact(operation)}
		encoded, err := json.Marshal(replicaSalesConfirmation{Authority: review.Authority, Operation: operation, ReplicaID: replicaID, Request: input})
		if err != nil {
			return output.Internal("REPLICA_SALES_REVIEW_FAILED", "could not encode the sales review", nil)
		}
		digestBytes := sha256.Sum256(encoded)
		digest := hex.EncodeToString(digestBytes[:])
		parts := []string{"viceme replica", operation, "--replica", replicaID, "--request-id", requestID, "--confirm", digest, "--request", base64.RawURLEncoding.EncodeToString(encoded)}
		if operation == "price" {
			parts = append(parts, "--price-cents", fmt.Sprint(price))
		}
		details := map[string]any{"nextAction": "CONFIRM_REPLICA_SALES", "review": review, "confirmDigest": digest, "confirmCommand": strings.Join(parts, " ")}
		return output.Confirmation("REPLICA_SALES_CONFIRMATION_REQUIRED", "review the current version, price and operation impact before confirming").WithDetails(details)
	}
	return command
}

func replicaSalesImpact(operation string) string {
	switch operation {
	case "price":
		return "Create a new sales price without uploading source. Existing unexpired quotes retain their snapshot price; purchased versions and download rights remain unchanged."
	case "delist":
		return "Stop new quotes and orders and remove the make-copy entry. The hosted Work remains available; existing purchases keep their download rights."
	default:
		return "Resume selling the displayed active source version at the displayed current price. Do not create a new source version or alter existing purchases."
	}
}

// The confirmation carries the immutable CAS request so a lost response can be
// retried with the same idempotency identity and exact input.
type replicaSalesConfirmation struct {
	Authority string                         `json:"authority"`
	Operation string                         `json:"operation"`
	ReplicaID string                         `json:"replicaId"`
	Request   api.WebsiteReplicaSalesRequest `json:"request"`
}
