package api

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/ViceMe-AI/cli/internal/output"
)

// DeliveryMode is independent of entitlement: a purchased cloud Skill still
// requires server authorization for every new task.
func DeliveryMode(value string) string {
	if value == "" {
		return "SOURCE"
	}
	return value
}

type SkillCloudPackageManifest struct {
	Version      int      `json:"version"`
	Purpose      string   `json:"purpose"`
	PrivateFiles []string `json:"privateFiles"`
	PublicFiles  []string `json:"publicFiles"`
}

type SkillCloudSubmit struct {
	ReleaseID  string            `json:"releaseId,omitempty"`
	ProductID  string            `json:"productId"`
	RequestKey string            `json:"requestKey"`
	SessionID  string            `json:"sessionId,omitempty"`
	Prompt     string            `json:"prompt"`
	Facts      map[string]string `json:"facts"`
}

type SkillCloudTrial struct {
	RemainingUses int `json:"remainingUses"`
	LimitUses     int `json:"limitUses"`
}

type SkillCloudResult struct {
	RequestID    string           `json:"requestId"`
	SessionID    string           `json:"sessionId"`
	ReleaseID    string           `json:"releaseId"`
	Version      int              `json:"version"`
	Status       string           `json:"status"`
	Instructions *string          `json:"instructions"`
	Message      *string          `json:"message"`
	Outcome      *string          `json:"outcome"`
	ErrorCode    *string          `json:"errorCode"`
	Retryable    bool             `json:"retryable"`
	ExpiresAt    string           `json:"expiresAt"`
	Trial        *SkillCloudTrial `json:"trial,omitempty"`
}

func (c *Client) SubmitSkillCloud(ctx context.Context, input SkillCloudSubmit, installID, secret string) (SkillCloudResult, error) {
	endpoint, credential := "/v1/cli/skill-cloud/requests", "@stored"
	var body any = input
	if installID != "" {
		endpoint, credential = "/v1/skill-cloud/trial/requests", ""
		body = struct {
			SkillCloudSubmit
			InstallID string `json:"installId"`
			Secret    string `json:"secret"`
		}{input, installID, secret}
	}
	return c.skillCloudCall(ctx, http.MethodPost, endpoint, body, credential, "")
}

func (c *Client) ReadSkillCloud(ctx context.Context, productID, requestID, installID, secret string, resume bool) (SkillCloudResult, error) {
	endpoint, credential, method := "/v1/cli/skill-cloud/requests/"+url.PathEscape(requestID), "@stored", http.MethodGet
	var body any
	if resume {
		endpoint += "/resume"
		method = http.MethodPost
		body = struct{}{}
	}
	if installID != "" {
		action := "read"
		if resume {
			action = "resume"
		}
		endpoint, credential, method = "/v1/skill-cloud/trial/requests/"+url.PathEscape(requestID)+"/"+action, "", http.MethodPost
		body = map[string]string{"productId": productID, "installId": installID, "secret": secret}
	}
	return c.skillCloudCall(ctx, method, endpoint, body, credential, requestID)
}

var cloudUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (c *Client) skillCloudCall(ctx context.Context, method, endpoint string, body any, credential, requestID string) (SkillCloudResult, error) {
	var result SkillCloudResult
	if err := c.doJSON(ctx, method, endpoint, body, &result, credential); err != nil {
		return result, err
	}
	expiry, expiryErr := time.Parse(time.RFC3339, result.ExpiresAt)
	valid := cloudUUIDPattern.MatchString(result.RequestID) && cloudUUIDPattern.MatchString(result.SessionID) && cloudUUIDPattern.MatchString(result.ReleaseID) && result.Version > 0 && expiryErr == nil && expiry.After(time.Now()) && (requestID == "" || result.RequestID == requestID)
	valid = valid && (result.Status == "QUEUED" || result.Status == "RUNNING" || result.Status == "SUCCEEDED" || result.Status == "FAILED")
	if result.Status == "SUCCEEDED" {
		valid = valid && result.Outcome != nil
		if result.Outcome != nil {
			switch *result.Outcome {
			case "ready":
				valid = valid && result.Instructions != nil && len(*result.Instructions) > 0 && utf8.RuneCountInString(*result.Instructions) <= 16000 && result.Message == nil
			case "needs_input", "refused":
				valid = valid && result.Instructions == nil && result.Message != nil && utf8.RuneCountInString(*result.Message) <= 16000
			default:
				valid = false
			}
		}
	} else {
		valid = valid && result.Outcome == nil && result.Instructions == nil
	}
	if result.Trial != nil {
		valid = valid && result.Trial.LimitUses > 0 && result.Trial.RemainingUses >= 0 && result.Trial.RemainingUses <= result.Trial.LimitUses
	}
	if !valid {
		return SkillCloudResult{}, output.Policy("SKILL_CLOUD_RESPONSE_INVALID", "cloud task response is invalid; preserve the request and retry")
	}
	return result, nil
}
