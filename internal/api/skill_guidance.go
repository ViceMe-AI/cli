package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/ViceMe-AI/cli/internal/output"
)

const SkillGuidanceMaxRequestBytes = 16 * 1024 * 1024

// DeliveryMode is independent of entitlement: a purchased protected Skill still
// requires server authorization for every new task.
func DeliveryMode(value string) string {
	if value == "" {
		return "SOURCE"
	}
	return value
}

func (c *Client) GetGuidanceSkillDownload(ctx context.Context, productID string) (DownloadURL, error) {
	var result DownloadURL
	err := c.doJSON(ctx, http.MethodGet, "/v1/downloads/guidance/"+url.PathEscape(productID), nil, &result, "")
	return result, err
}

type SkillGuidanceSubmit struct {
	ReleaseID  string            `json:"releaseId,omitempty"`
	ProductID  string            `json:"productId"`
	RequestKey string            `json:"requestKey"`
	Prompt     string            `json:"prompt"`
	Facts      map[string]string `json:"facts"`
}

type SkillGuidanceTrial struct {
	RemainingUses int `json:"remainingUses"`
	LimitUses     int `json:"limitUses"`
}

type SkillGuidanceResult struct {
	RequestID    string              `json:"requestId"`
	ReleaseID    string              `json:"releaseId"`
	Version      int                 `json:"version"`
	Status       string              `json:"status"`
	Instructions *string             `json:"instructions"`
	Message      *string             `json:"message"`
	Outcome      *string             `json:"outcome"`
	ErrorCode    *string             `json:"errorCode"`
	Retryable    bool                `json:"retryable"`
	ExpiresAt    string              `json:"expiresAt"`
	Trial        *SkillGuidanceTrial `json:"trial,omitempty"`
}

func (c *Client) SubmitSkillGuidance(ctx context.Context, input SkillGuidanceSubmit, installID, secret string, kinds ...string) (SkillGuidanceResult, error) {
	endpoint, credential := "/v1/cli/skill-guidance/requests", "@stored"
	var body any = input
	if installID != "" {
		endpoint, credential = "/v1/skill-guidance/"+guidanceCredentialKind(kinds)+"/requests", ""
		body = struct {
			SkillGuidanceSubmit
			InstallID string `json:"installId"`
			Secret    string `json:"secret"`
		}{input, installID, secret}
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(body); err != nil {
		return SkillGuidanceResult{}, output.Internal("REQUEST_ENCODE_FAILED", "failed to encode guidance input", err)
	}
	if encoded.Len() > SkillGuidanceMaxRequestBytes {
		return SkillGuidanceResult{}, output.Validation("SKILL_GUIDANCE_INPUT_TOO_LARGE", "guidance HTTP body exceeds the 16 MiB transport limit").WithHint("Condense the complete task and submit a new requestKey")
	}
	return c.skillGuidanceCall(ctx, http.MethodPost, endpoint, encodedJSONRequest(encoded.Bytes()), credential, "")
}

func (c *Client) ReadSkillGuidance(ctx context.Context, productID, requestID, installID, secret string, resume bool, kinds ...string) (SkillGuidanceResult, error) {
	endpoint, credential, method := "/v1/cli/skill-guidance/requests/"+url.PathEscape(requestID), "@stored", http.MethodGet
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
		endpoint, credential, method = "/v1/skill-guidance/"+guidanceCredentialKind(kinds)+"/requests/"+url.PathEscape(requestID)+"/"+action, "", http.MethodPost
		body = map[string]string{"productId": productID, "installId": installID, "secret": secret}
	}
	return c.skillGuidanceCall(ctx, method, endpoint, body, credential, requestID)
}

var guidanceUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (c *Client) skillGuidanceCall(ctx context.Context, method, endpoint string, body any, credential, requestID string) (SkillGuidanceResult, error) {
	var result SkillGuidanceResult
	if err := c.doJSON(ctx, method, endpoint, body, &result, credential); err != nil {
		return result, err
	}
	expiry, expiryErr := time.Parse(time.RFC3339, result.ExpiresAt)
	valid := guidanceUUIDPattern.MatchString(result.RequestID) && guidanceUUIDPattern.MatchString(result.ReleaseID) && result.Version > 0 && expiryErr == nil && expiry.After(time.Now()) && (requestID == "" || result.RequestID == requestID)
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
		return SkillGuidanceResult{}, output.Policy("SKILL_GUIDANCE_RESPONSE_INVALID", "guidance task response is invalid; preserve the request and retry")
	}
	return result, nil
}

func guidanceCredentialKind(kinds []string) string {
	if len(kinds) > 0 && kinds[0] == "purchase" {
		return "purchase"
	}
	return "trial"
}
