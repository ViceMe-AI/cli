package api

import (
	"context"
	"net/http"
	"net/url"
)

type CreatorPageSetup struct {
	ApplicationID     string                      `json:"applicationId"`
	ApplicationStatus string                      `json:"applicationStatus"`
	DisplayName       string                      `json:"displayName"`
	Selection         *CreatorOnboardingSelection `json:"selection"`
	SelectedAt        *string                     `json:"selectedAt"`
}

func (c *Client) GetCreatorPageSetup(ctx context.Context, applicationID string) (CreatorPageSetup, error) {
	var response CreatorPageSetup
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/merchant/onboarding/"+url.PathEscape(applicationID)+"/page-setup", nil, &response, "@stored")
	return response, err
}

func (c *Client) GetCreatorPageSetupForMerchant(ctx context.Context, merchantID string) (CreatorPageSetup, error) {
	var response CreatorPageSetup
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/merchant/onboarding/targets/"+url.PathEscape(merchantID)+"/page-setup", nil, &response, "@stored")
	return response, err
}
