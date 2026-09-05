package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

type WebsiteReplicaSalesState struct {
	ReplicaID         string                     `json:"replicaId"`
	WorkID            string                     `json:"workId"`
	SaleStatus        string                     `json:"saleStatus"`
	OperationsEnabled bool                       `json:"operationsEnabled"`
	ReplicaVersion    WebsiteReplicaSalesVersion `json:"replicaVersion"`
	Product           WebsiteReplicaSalesProduct `json:"product"`
}
type WebsiteReplicaSalesVersion struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Title   string `json:"title"`
	Digest  string `json:"digest"`
}
type WebsiteReplicaSalesProduct struct {
	ID                 string `json:"id"`
	Revision           int    `json:"revision"`
	Status             string `json:"status"`
	SalesSpecVersionID string `json:"salesSpecVersionId"`
	SalesSpecVersion   int    `json:"salesSpecVersion"`
	SKUID              string `json:"skuId"`
	Currency           string `json:"currency"`
	PriceCents         int    `json:"priceCents"`
}
type WebsiteReplicaSalesRequest struct {
	ClientRequestID            string `json:"clientRequestId"`
	ExpectedProductID          string `json:"expectedProductId"`
	ExpectedRevision           int    `json:"expectedRevision"`
	ExpectedSalesSpecVersionID string `json:"expectedSalesSpecVersionId"`
	ExpectedReplicaVersionID   string `json:"expectedReplicaVersionId"`
	PriceCents                 *int   `json:"priceCents,omitempty"`
}
type WebsiteReplicaSalesMutation struct {
	MutationID string                   `json:"mutationId"`
	Kind       string                   `json:"kind"`
	Changed    bool                     `json:"changed"`
	State      WebsiteReplicaSalesState `json:"state"`
	CreatedAt  string                   `json:"createdAt"`
}

func (*WebsiteReplicaSalesState) strictAPIResponse()    {}
func (*WebsiteReplicaSalesMutation) strictAPIResponse() {}
func (s *WebsiteReplicaSalesState) validateAPIResponse() error {
	p, v := s.Product, s.ReplicaVersion
	if !zodUUIDPattern.MatchString(s.ReplicaID) || !zodUUIDPattern.MatchString(s.WorkID) || !zodUUIDPattern.MatchString(v.ID) || !validPositiveSafeInteger(v.Version) || v.Title == "" || utf16CodeUnits(v.Title) > 200 || !sha256HexPattern.MatchString(v.Digest) || !zodUUIDPattern.MatchString(p.ID) || !validPositiveSafeInteger(p.Revision) || !zodUUIDPattern.MatchString(p.SalesSpecVersionID) || !validPositiveSafeInteger(p.SalesSpecVersion) || !zodUUIDPattern.MatchString(p.SKUID) || !validNonnegativeSafeInteger(p.PriceCents) || !validReplicaCurrency(p.Currency) {
		return errors.New("invalid Replica sales state")
	}
	if (s.SaleStatus != "ACTIVE" && s.SaleStatus != "DELISTED") || (p.Status != "ACTIVE" && p.Status != "SUSPENDED") || (s.SaleStatus == "ACTIVE") != (p.Status == "ACTIVE") {
		return errors.New("inconsistent Replica sale status")
	}
	return nil
}
func (s *WebsiteReplicaSalesMutation) validateAPIResponse() error {
	if !zodUUIDPattern.MatchString(s.MutationID) || !validTimestamp(s.CreatedAt) || (s.Kind != "PRICE_CHANGED" && s.Kind != "DELISTED" && s.Kind != "RELISTED") {
		return errors.New("invalid Replica sales mutation")
	}
	return s.State.validateAPIResponse()
}
func (c *Client) GetWebsiteReplicaSales(ctx context.Context, replicaID string) (WebsiteReplicaSalesState, error) {
	var response WebsiteReplicaSalesState
	err := c.doJSON(ctx, http.MethodGet, "/v1/website-replicas/"+url.PathEscape(replicaID)+"/sales", nil, &response, "@stored")
	if err == nil && response.ReplicaID != replicaID {
		err = invalidAPIResponse(errors.New("sales response target mismatch"))
	}
	return response, err
}
func (c *Client) MutateWebsiteReplicaSales(ctx context.Context, replicaID, operation string, input WebsiteReplicaSalesRequest) (WebsiteReplicaSalesMutation, error) {
	var response WebsiteReplicaSalesMutation
	kinds := map[string]string{"price": "PRICE_CHANGED", "delist": "DELISTED", "relist": "RELISTED"}
	kind, ok := kinds[operation]
	if !ok {
		return response, errors.New("unsupported Replica sales operation")
	}
	err := c.doJSON(ctx, http.MethodPost, "/v1/website-replicas/"+url.PathEscape(replicaID)+"/sales/"+operation, input, &response, "@stored")
	if err == nil && (response.State.ReplicaID != replicaID || response.State.Product.ID != input.ExpectedProductID || response.State.ReplicaVersion.ID != input.ExpectedReplicaVersionID || response.Kind != kind || (operation == "price" && (input.PriceCents == nil || response.State.Product.PriceCents != *input.PriceCents)) || (operation == "delist" && response.State.SaleStatus != "DELISTED") || (operation == "relist" && response.State.SaleStatus != "ACTIVE")) {
		err = invalidAPIResponse(errors.New("sales mutation response target mismatch"))
	}
	return response, err
}
