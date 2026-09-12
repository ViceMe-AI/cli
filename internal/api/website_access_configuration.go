package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// WebsiteAccessConfigurationRequest is a delta. The server merges feature keys
// transactionally and owns Work creation idempotency and creator authorization.
type WebsiteAccessConfigurationRequest struct {
	RequestID         string                      `json:"requestId"`
	ProjectBindingID  string                      `json:"projectBindingId"`
	Market            string                      `json:"market"`
	MerchantAccountID string                      `json:"merchantAccountId,omitempty"`
	WorkID            string                      `json:"workId,omitempty"`
	Work              WebsiteAccessWorkInput      `json:"work"`
	AccessFeatures    []WebsiteAccessFeatureInput `json:"accessFeatures"`
}

type WebsiteAccessWorkInput struct {
	Title           string `json:"title,omitempty"`
	Slug            string `json:"slug,omitempty"`
	Summary         string `json:"summary,omitempty"`
	BodyMarkdown    string `json:"bodyMarkdown,omitempty"`
	CanonicalOrigin string `json:"canonicalOrigin,omitempty"`
}

type WebsiteAccessPricingIntent struct {
	Currency    string `json:"currency"`
	AmountMinor int64  `json:"amountMinor"`
}

type WebsiteAccessFeatureInput struct {
	FeatureKey    string                      `json:"featureKey"`
	Title         string                      `json:"title"`
	PolicyType    string                      `json:"policyType"`
	Availability  string                      `json:"availability"`
	PricingIntent *WebsiteAccessPricingIntent `json:"pricingIntent"`
	Status        string                      `json:"status"`
}

type WebsiteAccessConfigurationResponse struct {
	NextAction        string            `json:"nextAction"`
	CompletedSteps    []string          `json:"completedSteps"`
	WorkID            string            `json:"workId,omitempty"`
	MerchantAccountID string            `json:"merchantAccountId,omitempty"`
	Access            *WorkSdkAccess    `json:"access,omitempty"`
	Availability      string            `json:"availability,omitempty"`
	MissingFields     []string          `json:"missingFields,omitempty"`
	Merchants         []MerchantAccount `json:"merchants,omitempty"`
}

func (c *Client) ConfigureWebsiteAccess(ctx context.Context, input WebsiteAccessConfigurationRequest) (WebsiteAccessConfigurationResponse, error) {
	var result WebsiteAccessConfigurationResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/website-access/configurations", input, &result, "@stored")
	return result, err
}

func (result *WebsiteAccessConfigurationResponse) validateAPIResponse() error {
	switch result.NextAction {
	case "AUTHENTICATE_CREATOR", "APPLY_CREATOR", "SELECT_MERCHANT", "WAIT_CREATOR_REVIEW", "SUPPLY_CREATOR_INFO", "COLLECT_INPUT", "UPGRADE_CLI", "COMPLETE_CREATOR_QUALIFICATION", "PROVIDE_INPUT":
	case "INTEGRATE_HOST", "PENDING_CHANNEL", "COMPLETE":
		if !uuidPattern.MatchString(result.WorkID) || !uuidPattern.MatchString(result.MerchantAccountID) || result.Access == nil {
			return errors.New("website access configuration has no verified target")
		}
	default:
		return errors.New("website access configuration has an unknown next action")
	}
	if result.Availability != "" && result.Availability != "ACTIVE" && result.Availability != "PENDING_CHANNEL" && result.Availability != "DISABLED" {
		return errors.New("website access configuration has an invalid availability")
	}
	if result.CompletedSteps == nil {
		return errors.New("website access configuration is missing completed steps")
	}
	if result.Access != nil {
		if result.Access.WorkID != result.WorkID {
			return errors.New("website access configuration target does not match access")
		}
		return result.Access.validateAPIResponse()
	}
	return nil
}

func ValidWebsiteAccessFeatureInput(feature WebsiteAccessFeatureInput) bool {
	if !accessWorkFeatureKeyPattern.MatchString(feature.FeatureKey) || strings.TrimSpace(feature.Title) == "" || utf16CodeUnits(feature.Title) > 120 || (feature.Status != "ACTIVE" && feature.Status != "DISABLED") {
		return false
	}
	switch feature.Availability {
	case "ACTIVE", "PENDING_CHANNEL", "DISABLED":
	default:
		return false
	}
	if feature.PolicyType == "WORK_ENTITLEMENT" {
		return validWebsiteAccessPricing(feature.PricingIntent)
	}
	return (feature.PolicyType == "PUBLIC" || feature.PolicyType == "FOLLOW_OWNER") && feature.PricingIntent == nil && feature.Availability != "PENDING_CHANNEL"
}

func validWebsiteAccessPricing(price *WebsiteAccessPricingIntent) bool {
	return price != nil && (price.Currency == "CNY" || price.Currency == "USD") && price.AmountMinor > 0 && price.AmountMinor <= 2147483647
}

func websiteAccessPricingEqual(left, right *WebsiteAccessPricingIntent) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
