package command

import (
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/api"
)

func TestWorkflowForPublicationPreservesDraftReviewAndAuthorizationBoundaries(t *testing.T) {
	t.Parallel()
	publicationID := "22222222-2222-4222-8222-222222222222"

	tests := []struct {
		name               string
		status             string
		override           string
		phase              string
		userActionRequired bool
		actionType         string
		commandContains    string
		requiresApproval   bool
	}{
		{name: "initial private draft", status: "DRAFT", override: workflowPhaseContinueDraft, phase: "CONTINUE_DRAFT", actionType: "CONTINUE_DRAFT", commandContains: "skill publish --resume"},
		{name: "creator price", status: "DRAFT", phase: "CREATOR_PRICE_REQUIRED", userActionRequired: true, actionType: "ASK_CREATOR_MONTHLY_PRICE", commandContains: "--creator-monthly-price-cents"},
		{name: "agent enrichment", status: "DRAFT", override: workflowPhaseEnrichDraft, phase: "ENRICH_DRAFT", actionType: "REVIEW_FOR_ENRICHMENT", commandContains: "publication review"},
		{name: "user review loop", status: "REVIEW_REQUIRED", phase: "USER_REVIEW_REQUIRED", userActionRequired: true, actionType: "CONFIRM_REVIEW", commandContains: "publication confirm", requiresApproval: true},
		{name: "confirmed release", status: "READY", phase: "PUBLISH_AUTHORIZED", actionType: "PUBLISH", commandContains: "publication publish"},
		{name: "complete", status: "PUBLISHED", phase: "COMPLETE"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			current := api.SkillPublication{ID: publicationID, Status: test.status}
			if test.name == "creator price" {
				current.RequiresCreatorMonthlyPrice = true
			}
			workflow := workflowForPublication(current, test.override)
			if workflow.Version != publicationWorkflowVersion || workflow.Phase != test.phase || workflow.UserActionRequired != test.userActionRequired {
				t.Fatalf("unexpected workflow: %#v", workflow)
			}
			if test.actionType == "" {
				if len(workflow.Actions) != 0 {
					t.Fatalf("terminal workflow exposed actions: %#v", workflow)
				}
				return
			}
			for _, action := range workflow.Actions {
				if action.Type == test.actionType {
					if !strings.Contains(action.Command, test.commandContains) || action.RequiresExplicitUserConfirmation != test.requiresApproval {
						t.Fatalf("unexpected action: %#v", action)
					}
					return
				}
			}
			t.Fatalf("action %q missing from %#v", test.actionType, workflow)
		})
	}
}
