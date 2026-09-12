package command

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

// Enrichment accepts existing platform media references, never local filenames
// or unverified upload URLs. Media upload remains owned by the Work media API.
type websiteEnrichInput struct {
	Summary           *string          `json:"summary,omitempty"`
	BodyMarkdown      *string          `json:"bodyMarkdown,omitempty"`
	Tags              *json.RawMessage `json:"tags,omitempty"`
	Media             *json.RawMessage `json:"media,omitempty"`
	UsageInstructions *string          `json:"usageInstructions,omitempty"`
}

func newWebsiteEnrichCommand(runtime *Runtime) *cobra.Command {
	var project, inputFile string
	command := &cobra.Command{Use: "enrich", Short: "Merge confirmed content into the bound Work and verify publication", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		canonical, lock, err := openWebsiteAccessState(runtime, project)
		if err != nil {
			return err
		}
		defer lock.Unlock()
		state, found, err := loadWebsiteAccessState(runtime, canonical)
		if err != nil {
			return err
		}
		if !found || state.Result == nil || state.Result.Access == nil {
			return output.Validation("WEBSITE_ACCESS_NOT_CONFIGURED", "configure website access before enriching its Work")
		}
		work, err := runtime.client().GetMerchantWork(command.Context(), state.Result.WorkID, state.Result.MerchantAccountID)
		if err != nil {
			return websiteAccessActionError(runtime, canonical, state, err)
		}
		if err = validateWebsiteEnrichTarget(work, state); err != nil {
			return err
		}
		if state.Enrichment == nil {
			if inputFile == "" {
				return output.Validation("WEBSITE_ENRICH_INPUT_REQUIRED", "provide the confirmed content delta with --input")
			}
			input, err := readStrictJSONObject[websiteEnrichInput](inputFile, "WEBSITE_ENRICH_INPUT_INVALID")
			if err != nil {
				return err
			}
			content, err := websiteEnrichedContent(work.ActiveRevision, input)
			if err != nil {
				return err
			}
			state.Enrichment = &websiteEnrichmentState{ExpectedRevision: work.Revision, Content: content}
			if err = saveWebsiteAccessState(canonical, state); err != nil {
				return err
			}
		} else if inputFile != "" {
			input, err := readStrictJSONObject[websiteEnrichInput](inputFile, "WEBSITE_ENRICH_INPUT_INVALID")
			if err != nil {
				return err
			}
			content, err := websiteEnrichedContent(work.ActiveRevision, input)
			if err != nil {
				return err
			}
			if !websiteJSONEqual(content, state.Enrichment.Content) {
				return output.Validation("WEBSITE_ENRICH_RECOVERY_CONFLICT", "resume the pending enrichment without a new input before changing content")
			}
		}
		return performWebsiteEnrich(command.Context(), runtime, canonical, &state, work)
	}}
	command.Flags().StringVar(&project, "project", ".", "bound website project")
	command.Flags().StringVar(&inputFile, "input", "", "confirmed summary, body, tags, media or usage instructions delta; omit to resume")
	return command
}

func validateWebsiteEnrichTarget(work api.MerchantWork, state websiteAccessState) error {
	if work.ID != state.Result.WorkID || work.Kind != "WEBSITE" || work.Status != "PUBLISHED" || work.Owner.Kind != "MERCHANT" || work.Owner.MerchantAccountID == nil || *work.Owner.MerchantAccountID != state.Result.MerchantAccountID || bytes.Equal(bytes.TrimSpace(work.ActiveRevision), []byte("null")) {
		return output.Validation("WEBSITE_ENRICH_TARGET_INVALID", "bound Work is not a published website owned by the selected merchant")
	}
	if !bytes.Equal(bytes.TrimSpace(work.DraftRevision), []byte("null")) {
		return output.Policy("WEBSITE_ENRICH_DRAFT_EXISTS", "the Work has an existing draft; preserve it before enriching")
	}
	return nil
}

func websiteActiveContent(active json.RawMessage) (map[string]json.RawMessage, error) {
	var source map[string]json.RawMessage
	if err := json.Unmarshal(active, &source); err != nil {
		return nil, output.Validation("WEBSITE_ENRICH_CONTENT_INVALID", "Work has no readable active content")
	}
	content := map[string]json.RawMessage{}
	for _, key := range []string{"summary", "bodyMarkdown", "templateType", "tags", "media", "actionConfig"} {
		value, ok := source[key]
		if !ok || bytes.Equal(value, []byte("null")) {
			return nil, output.Validation("WEBSITE_ENRICH_CONTENT_INVALID", "Work is missing a required active content field")
		}
		content[key] = value
	}
	if value, ok := source["usageInstructions"]; ok && !bytes.Equal(value, []byte("null")) {
		content["usageInstructions"] = value
	}
	return content, nil
}

func websiteEnrichedContent(active json.RawMessage, input websiteEnrichInput) (json.RawMessage, error) {
	content, err := websiteActiveContent(active)
	if err != nil {
		return nil, err
	}
	delta, _ := json.Marshal(input)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(delta, &fields)
	if len(fields) == 0 {
		return nil, output.Validation("WEBSITE_ENRICH_INPUT_INVALID", "provide at least one confirmed content field")
	}
	if input.Summary != nil && (len([]rune(*input.Summary)) == 0 || len([]rune(*input.Summary)) > 500) {
		return nil, output.Validation("WEBSITE_ENRICH_INPUT_INVALID", "summary must contain 1 to 500 characters")
	}
	for _, key := range []string{"tags", "media"} {
		if raw, ok := fields[key]; ok {
			var list []json.RawMessage
			if json.Unmarshal(raw, &list) != nil || list == nil {
				return nil, output.Validation("WEBSITE_ENRICH_INPUT_INVALID", "tags and media must be arrays")
			}
		}
	}
	for key, value := range fields {
		content[key] = value
	}
	data, err := json.Marshal(content)
	return data, err
}

func performWebsiteEnrich(ctx context.Context, runtime *Runtime, project string, state *websiteAccessState, current api.MerchantWork) error {
	actual, err := websiteActiveContent(current.ActiveRevision)
	if err != nil {
		return err
	}
	actualJSON, _ := json.Marshal(actual)
	if !websiteJSONEqual(actualJSON, state.Enrichment.Content) {
		if current.Revision != state.Enrichment.ExpectedRevision {
			return output.Policy("WORK_REVISION_CONFLICT", "the Work changed; pending enrichment was preserved without overwriting it")
		}
		request, _ := json.Marshal(map[string]any{"merchantAccountId": state.Result.MerchantAccountID, "expectedRevision": state.Enrichment.ExpectedRevision, "content": state.Enrichment.Content, "status": "PUBLISHED"})
		_, writeErr := runtime.client().UpdateMerchantWork(ctx, state.Result.WorkID, request)
		// A lost response is resolved by one authoritative read, never another write.
		observed, readErr := runtime.client().GetMerchantWork(ctx, state.Result.WorkID, state.Result.MerchantAccountID)
		if readErr != nil {
			if writeErr != nil {
				return writeErr
			}
			return readErr
		}
		if err = validateWebsiteEnrichTarget(observed, *state); err != nil {
			return err
		}
		observedContent, err := websiteActiveContent(observed.ActiveRevision)
		if err != nil {
			return err
		}
		observedJSON, _ := json.Marshal(observedContent)
		if !websiteJSONEqual(observedJSON, state.Enrichment.Content) {
			if writeErr != nil {
				return writeErr
			}
			return output.Validation("WEBSITE_ENRICH_READBACK_MISMATCH", "the confirmed content is not the active Work revision")
		}
	}
	state.Enrichment = nil
	if err = saveWebsiteAccessState(project, *state); err != nil {
		return err
	}
	return runtime.business(map[string]any{"workId": state.Result.WorkID, "saved": true, "nextAction": "CONTINUE_CREATING", "accessPhase": state.Phase})
}

func websiteJSONEqual(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && reflect.DeepEqual(a, b)
}
