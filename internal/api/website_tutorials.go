package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/ViceMe-AI/cli/internal/output"
)

type WebsiteTutorialStep struct {
	Title       string  `json:"title"`
	Explanation string  `json:"explanation"`
	Prompt      string  `json:"prompt"`
	Model       *string `json:"model,omitempty"`
}
type WebsiteTutorialContent struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Title         string                `json:"title"`
	Summary       string                `json:"summary"`
	Prerequisites []string              `json:"prerequisites"`
	Steps         []WebsiteTutorialStep `json:"steps"`
}
type WebsiteTutorialView struct {
	WorkID           string                    `json:"workId"`
	Version          int                       `json:"version"`
	Title            string                    `json:"title"`
	Summary          string                    `json:"summary"`
	Prerequisites    []string                  `json:"prerequisites"`
	PriceCents       int                       `json:"priceCents"`
	ProductID        *string                   `json:"productId"`
	Unlocked         bool                      `json:"unlocked"`
	PreviewStepCount int                       `json:"previewStepCount"`
	Steps            []WebsiteTutorialViewStep `json:"steps"`
}
type WebsiteTutorialViewStep struct {
	Title   string               `json:"title"`
	Content *WebsiteTutorialStep `json:"content"`
}
type PublishWebsiteTutorialRequest struct {
	MerchantAccountID string                 `json:"merchantAccountId"`
	ClientRequestID   string                 `json:"clientRequestId"`
	ExpectedVersion   int                    `json:"expectedVersion"`
	Content           WebsiteTutorialContent `json:"content"`
	PreviewStepCount  int                    `json:"previewStepCount"`
	PriceCents        int                    `json:"priceCents"`
}

// Zod string lengths count UTF-16 code units, including surrogate pairs.
func tutorialTextValid(value string, min, max int, trim bool) bool {
	if trim {
		value = strings.TrimSpace(value)
	}
	length := len(utf16.Encode([]rune(value)))
	return length >= min && length <= max
}
func (content WebsiteTutorialContent) Validate() error {
	invalid := output.Validation("WEBSITE_TUTORIAL_INPUT_INVALID", "tutorial JSON does not match the website tutorial schema")
	if content.SchemaVersion != 1 || !tutorialTextValid(content.Title, 1, 200, true) || !tutorialTextValid(content.Summary, 0, 2000, false) || content.Prerequisites == nil || len(content.Prerequisites) > 30 || len(content.Steps) < 1 || len(content.Steps) > 100 {
		return invalid
	}
	for _, item := range content.Prerequisites {
		if !tutorialTextValid(item, 0, 2000, false) {
			return invalid
		}
	}
	for _, step := range content.Steps {
		if !tutorialTextValid(step.Title, 1, 200, true) || !tutorialTextValid(step.Explanation, 0, 20000, false) || !tutorialTextValid(step.Prompt, 1, 50000, false) || (step.Model != nil && !tutorialTextValid(*step.Model, 0, 200, true)) {
			return invalid
		}
	}
	data, err := json.Marshal(content)
	if err != nil || len(data) > 500000 {
		return invalid
	}
	return nil
}
func (c *Client) GetWebsiteTutorial(ctx context.Context, workID string, version int, public bool) (WebsiteTutorialView, error) {
	var response WebsiteTutorialView
	if !uuidPattern.MatchString(workID) || version < 0 {
		return response, output.Validation("WEBSITE_TUTORIAL_TARGET_INVALID", "a Work UUID and a nonnegative version are required")
	}
	prefix, credential := "/v1/cli/works/", "@stored"
	if public {
		prefix, credential = "/v1/public/works/", ""
	}
	endpoint := prefix + workID + "/tutorial"
	if version > 0 {
		endpoint += "?version=" + strconv.Itoa(version)
	}
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, credential)
	if err == nil {
		err = response.validate(workID, version, public)
	}
	return response, err
}
func (c *Client) PublishWebsiteTutorial(ctx context.Context, workID string, input PublishWebsiteTutorialRequest) (WebsiteTutorialView, error) {
	var response WebsiteTutorialView
	if !uuidPattern.MatchString(workID) || !uuidPattern.MatchString(input.MerchantAccountID) || !uuidPattern.MatchString(input.ClientRequestID) || input.ExpectedVersion < 0 || input.PriceCents < 0 || input.PriceCents > 10000000 || input.PreviewStepCount < 0 || input.PreviewStepCount > len(input.Content.Steps) {
		return response, output.Validation("WEBSITE_TUTORIAL_INPUT_INVALID", "invalid tutorial publication identifiers, version, price or preview count")
	}
	if err := input.Content.Validate(); err != nil {
		return response, err
	}
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/merchant/works/"+workID+"/tutorial", input, &response, "@stored")
	if err == nil {
		err = response.validate(workID, input.ExpectedVersion+1, false)
	}
	return response, err
}
func (view WebsiteTutorialView) validate(workID string, version int, public bool) error {
	invalid := output.Validation("WEBSITE_TUTORIAL_RESPONSE_INVALID", "server returned an invalid tutorial view")
	if view.WorkID != workID || view.Version < 1 || (version > 0 && view.Version != version) || view.PriceCents < 0 || view.PreviewStepCount < 0 || view.PreviewStepCount > len(view.Steps) || len(view.Steps) < 1 || len(view.Steps) > 100 || !tutorialTextValid(view.Title, 1, 200, true) || !tutorialTextValid(view.Summary, 0, 2000, false) || view.Prerequisites == nil || len(view.Prerequisites) > 30 {
		return invalid
	}
	if view.PriceCents > 0 && (view.ProductID == nil || !uuidPattern.MatchString(*view.ProductID)) {
		return invalid
	}
	if public && view.PriceCents > 0 && view.Unlocked {
		return invalid
	}
	if view.PriceCents == 0 && !view.Unlocked {
		return invalid
	}
	for index, step := range view.Steps {
		visible := view.Unlocked || index < view.PreviewStepCount
		if !tutorialTextValid(step.Title, 1, 200, true) || visible != (step.Content != nil) {
			return invalid
		}
		if step.Content != nil {
			if step.Content.Title != step.Title {
				return invalid
			}
			content := WebsiteTutorialContent{SchemaVersion: 1, Title: view.Title, Summary: view.Summary, Prerequisites: view.Prerequisites, Steps: []WebsiteTutorialStep{*step.Content}}
			if content.Validate() != nil {
				return invalid
			}
		}
	}
	return nil
}
