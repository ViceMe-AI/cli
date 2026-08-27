package skillcontent_test

import (
	"io/fs"
	"strings"
	"testing"

	cliembed "github.com/ViceMe-AI/cli"
)

func TestPublishSkillKeepsHistoricalContextOutOfProfileSelection(t *testing.T) {
	t.Parallel()

	for _, relativePath := range []string{
		"viceme-publish/SKILL.md",
		"viceme-publish/references/workflow.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatalf("read embedded %s: %v", relativePath, err)
		}
		text := string(content)
		for _, required := range []string{
			"Memory",
			"prior conversations",
			"active CLI context",
			"authenticated user",
		} {
			if !strings.Contains(text, required) {
				t.Fatalf("embedded %s does not contain profile precedence guard %q", relativePath, required)
			}
		}
	}

	for _, relativePath := range []string{
		"viceme-publish/SKILL.md",
		"viceme-publish/references/workflow.md",
		"viceme-publish/references/errors.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatalf("read embedded %s: %v", relativePath, err)
		}
		for _, forbidden := range []string{"--profile", "profile list", "profile use"} {
			if strings.Contains(string(content), forbidden) {
				t.Fatalf("embedded %s exposes environment selection through %q", relativePath, forbidden)
			}
		}
	}
}

func TestPublishSkillRequiresAnalysisForEveryServiceWork(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/SKILL.md")
	if err != nil {
		t.Fatalf("read embedded publish skill: %v", err)
	}
	text := string(content)
	for _, required := range []string{
		"Every `SERVICE` Work",
		"recruitment",
		"`FULFILLMENT_ONLY`",
		"server-backed scenario analysis",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("embedded publish skill does not contain service analysis guard %q", required)
		}
	}
}

func TestPublishSkillDescribesPlatformAdaptiveRuntimeInputCollection(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/references/interaction-definition.md")
	if err != nil {
		t.Fatalf("read embedded interaction publication reference: %v", err)
	}
	text := string(content)
	for _, required := range []string{
		"native interactive controls",
		"genuinely free-form",
		"Never ask the participant to construct JSON",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("embedded interaction reference does not contain runtime input guard %q", required)
		}
	}
}

func TestPublishSkillRequiresVisibleAnalysisBeforeConfirmation(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/references/scenario-analysis.md")
	if err != nil {
		t.Fatalf("read embedded scenario analysis reference: %v", err)
	}
	text := string(content)
	for _, required := range []string{
		"Display the complete natural-language analysis overview",
		"Skill-specific omission, ambiguity, risk, or unresolved assumption",
		"The list is dynamic and may be empty",
		"Never replace the visible analysis with a generic prompt",
		"Review one Work's analysis at a time",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("embedded scenario analysis reference does not contain visible-analysis guard %q", required)
		}
	}
	for _, forbidden := range []string{
		"OUTCOME_AND_ENTRY",
		"ACTORS_AND_EXECUTION",
		"DATA_AND_AUDIENCE",
		"TRANSACTION_BOUNDARY",
		"EXCEPTIONS_AND_RECOVERY",
		"NOTIFICATIONS_AND_STATES",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("embedded scenario analysis reference still requires fixed review item %q", forbidden)
		}
	}
}
