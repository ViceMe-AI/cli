package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
)

type TrialPurchase struct {
	ProductID     string          `json:"productId"`
	OrderNo       string          `json:"orderNo"`
	Title         string          `json:"title"`
	Status        string          `json:"status"`
	AmountCents   int             `json:"amountCents"`
	Currency      string          `json:"currency"`
	ExpiresAt     string          `json:"expiresAt"`
	PaymentAction json.RawMessage `json:"paymentAction"`
}

type TrialOwnedDownload struct {
	Access   SkillAccess `json:"access"`
	Download DownloadURL `json:"download"`
}

func (c *Client) TrialPurchase(ctx context.Context, productID, installID, secret, requestID, locale, orderNo string) (TrialPurchase, error) {
	endpoint := "/v1/skills/" + url.PathEscape(productID) + "/trial-purchase"
	body := map[string]string{"installId": installID, "secret": secret}
	if orderNo != "" {
		endpoint += "/status"
		body["orderNo"] = orderNo
	} else {
		body["clientRequestId"], body["locale"] = requestID, locale
	}
	var response struct {
		TrialPurchase
		Amount *int `json:"amountCents"`
	}
	if err := c.doJSON(ctx, http.MethodPost, endpoint, body, &response, ""); err != nil {
		return TrialPurchase{}, err
	}
	_, expiryErr := time.Parse(time.RFC3339, response.ExpiresAt)
	if response.ProductID != productID || len(response.OrderNo) < 6 || len(response.OrderNo) > 40 ||
		(orderNo != "" && response.OrderNo != orderNo) || response.Amount == nil || *response.Amount < 0 ||
		response.Currency != "CNY" || expiryErr != nil ||
		(response.Status != "PENDING" && response.Status != "PAID" && response.Status != "CLOSED") {
		return TrialPurchase{}, output.Policy("SKILL_PURCHASE_RESPONSE_INVALID", "the trial purchase response is incomplete or mismatched; preserve the purchase state and retry")
	}
	response.AmountCents = *response.Amount
	return response.TrialPurchase, nil
}

func (c *Client) TrialOwnedSkillDownload(ctx context.Context, productID, installID, secret string) (TrialOwnedDownload, error) {
	var response TrialOwnedDownload
	err := c.doJSON(ctx, http.MethodPost, "/v1/skills/"+url.PathEscape(productID)+"/trial-purchase/download",
		map[string]string{"installId": installID, "secret": secret}, &response, "")
	if err != nil {
		return response, err
	}
	if !response.Access.Owned || response.Access.InstallKind != "OWNED_PAID" || response.Access.ProductID != productID ||
		response.Access.Release.ID == "" || response.Access.Release.ID != response.Download.ReleaseID ||
		response.Access.Release.ArtifactDigest == "" || response.Access.Release.ArtifactDigest != response.Download.ArtifactDigest {
		return TrialOwnedDownload{}, output.Authorization("SKILL_NOT_OWNED", "no matching active paid Skill release was authorized")
	}
	return response, nil
}
