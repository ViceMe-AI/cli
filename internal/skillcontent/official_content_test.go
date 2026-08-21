package skillcontent_test

import (
	"io/fs"
	"regexp"
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

func TestDanmakuSkillUsesProfileDerivedHostedSnippet(t *testing.T) {
	t.Parallel()

	skill, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-danmaku/SKILL.md")
	if err != nil {
		t.Fatalf("read embedded danmaku Skill: %v", err)
	}
	reference, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-danmaku/references/cdn-sdk.md")
	if err != nil {
		t.Fatalf("read embedded danmaku reference: %v", err)
	}
	text := string(skill) + "\n" + string(reference)
	for _, required := range []string{
		"profile list",
		"access init",
		"data.workKey",
		"data.embedSnippet",
		"webBaseUrl",
		"must not guess",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("embedded danmaku hosted flow is missing %q", required)
		}
	}
	if hardcodedURL := regexp.MustCompile(`https?://\S+`).FindString(text); hardcodedURL != "" {
		t.Fatalf("embedded danmaku flow hard-codes a hosted SDK URL: %q", hardcodedURL)
	}
}

func TestSharedSkillDoesNotPreflightBusinessCommands(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-shared/SKILL.md")
	if err != nil {
		t.Fatalf("read embedded shared Skill: %v", err)
	}
	for _, required := range []string{
		"Do not run setup, Doctor",
		"stop on its structured error",
	} {
		if !strings.Contains(string(content), required) {
			t.Fatalf("embedded shared Skill does not contain business preflight guard %q", required)
		}
	}
}

func TestEngagementSkillRequiresCreatorOwnedCompleteFlow(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-engagement/SKILL.md")
	if err != nil {
		t.Fatalf("read embedded engagement Skill: %v", err)
	}
	text := string(content)
	for _, required := range []string{
		"website publish",
		"access init",
		"creator-app create",
		"creator-app domain verify",
		"engagement-embed.js",
		"Do not use a shared or pre-provisioned `workKey`",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("embedded engagement Skill is missing complete-flow guard %q", required)
		}
	}
}
func TestWebsitePublicationBelongsToPublishSkill(t *testing.T) {
	t.Parallel()

	publishEntrypoint, err := fs.ReadFile(
		cliembed.EmbeddedSkills(),
		"viceme-publish/SKILL.md",
	)
	if err != nil {
		t.Fatalf("read embedded publish Skill: %v", err)
	}
	if !strings.Contains(string(publishEntrypoint), "references/website-workflow.md") {
		t.Fatal("publish Skill does not route website publication")
	}

	publishWorkflow, err := fs.ReadFile(
		cliembed.EmbeddedSkills(),
		"viceme-publish/references/website-workflow.md",
	)
	if err != nil {
		t.Fatalf("read embedded website publication workflow: %v", err)
	}
	for _, required := range []string{
		"viceme website publish",
		"clientWorkId",
		"--creator-display-name",
		"$viceme-danmaku",
	} {
		if !strings.Contains(string(publishWorkflow), required) {
			t.Fatalf("website publication workflow is missing %q", required)
		}
	}
}
