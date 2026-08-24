package command

import (
	"fmt"

	"github.com/ViceMe-AI/cli/internal/api"
)

const publicationWorkflowVersion = "skill-publication.v1"

type publicationWorkflow struct {
	Version            string                      `json:"version"`
	Phase              string                      `json:"phase"`
	UserActionRequired bool                        `json:"userActionRequired"`
	Actions            []publicationWorkflowAction `json:"actions"`
}

type publicationWorkflowAction struct {
	Type                             string `json:"type"`
	Command                          string `json:"command,omitempty"`
	Reason                           string `json:"reason,omitempty"`
	RequiresExplicitUserConfirmation bool   `json:"requiresExplicitUserConfirmation,omitempty"`
}

const (
	workflowPhaseContinueDraft = "CONTINUE_DRAFT"
	workflowPhaseEnrichDraft   = "ENRICH_DRAFT"
)

func workflowForPublication(current api.SkillPublication, phaseOverride string) publicationWorkflow {
	workflow := publicationWorkflow{Version: publicationWorkflowVersion, Actions: []publicationWorkflowAction{}}
	publicationID := current.ID

	if current.RequiresCreatorMonthlyPrice && current.Status != "PUBLISHED" && current.Status != "CANCELLED" {
		workflow.Phase = "CREATOR_PRICE_REQUIRED"
		workflow.UserActionRequired = true
		workflow.Actions = append(workflow.Actions, publicationWorkflowAction{
			Type:    "ASK_CREATOR_MONTHLY_PRICE",
			Command: fmt.Sprintf("viceme skill publish --resume %s --creator-monthly-price-cents <fen>", publicationID),
			Reason:  "Ask once for the creator's shared monthly CNY subscription price, then continue this same private Publication.",
		})
		return workflow
	}

	switch current.Status {
	case "PUBLISHED":
		workflow.Phase = "COMPLETE"
	case "CANCELLED":
		workflow.Phase = "CANCELLED"
	case "READY":
		workflow.Phase = "PUBLISH_AUTHORIZED"
		workflow.Actions = append(workflow.Actions, publicationWorkflowAction{
			Type:    "PUBLISH",
			Command: fmt.Sprintf("viceme publication publish %s --review-digest <review-digest>", publicationID),
			Reason:  "The reviewed draft is confirmed; complete the already-authorized public release.",
		})
	case "REVIEW_REQUIRED":
		workflow.Phase = "USER_REVIEW_REQUIRED"
		workflow.UserActionRequired = true
		workflow.Actions = append(workflow.Actions,
			publicationWorkflowAction{
				Type:   "ASK_USER_TO_REVIEW",
				Reason: "Show the latest bilingual copy, access mode, effective creator price, cover, and ordered gallery. The user may request more changes.",
			},
			publicationWorkflowAction{
				Type:    "MODIFY_DRAFT",
				Command: fmt.Sprintf("viceme publication update %s --input <draft.json>", publicationID),
				Reason:  "Apply requested edits, then fetch and display a fresh review with its new digest.",
			},
			publicationWorkflowAction{
				Type:                             "CONFIRM_REVIEW",
				Command:                          fmt.Sprintf("viceme publication confirm %s --review-digest <review-digest>", publicationID),
				Reason:                           "Run only after the user explicitly confirms the currently displayed review and immediate public publication.",
				RequiresExplicitUserConfirmation: true,
			},
		)
	case "DRAFT", "":
		switch phaseOverride {
		case workflowPhaseContinueDraft:
			workflow.Phase = workflowPhaseContinueDraft
			workflow.Actions = append(workflow.Actions, publicationWorkflowAction{
				Type:    "CONTINUE_DRAFT",
				Command: fmt.Sprintf("viceme skill publish --resume %s", publicationID),
				Reason:  "Continue the same private publication to upload verified media candidates.",
			})
		default:
			workflow.Phase = workflowPhaseEnrichDraft
			workflow.Actions = append(workflow.Actions, publicationWorkflowAction{
				Type:    "REVIEW_FOR_ENRICHMENT",
				Command: fmt.Sprintf("viceme publication review %s", publicationID),
				Reason:  "Read the latest revision and verified uploads before preparing one revision-protected Agent suggestion.",
			})
		}
	case "FAILED":
		workflow.Phase = "RECOVERY_REQUIRED"
		workflow.UserActionRequired = true
		workflow.Actions = append(workflow.Actions, publicationWorkflowAction{
			Type:    "INSPECT_FAILURE",
			Command: fmt.Sprintf("viceme publication review %s", publicationID),
			Reason:  "Inspect the authoritative failure and correct its input or Draft without creating another Publication.",
		})
	default:
		workflow.Phase = "STATE_INSPECTION_REQUIRED"
		workflow.UserActionRequired = true
		workflow.Actions = append(workflow.Actions, publicationWorkflowAction{
			Type:    "GET_PUBLICATION",
			Command: fmt.Sprintf("viceme publication get %s", publicationID),
			Reason:  "The state is not recognized by this workflow version; inspect it instead of guessing the next write.",
		})
	}

	return workflow
}
