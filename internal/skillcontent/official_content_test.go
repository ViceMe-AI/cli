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

func TestDanmakuSkillUsesOnlyHostedIntegration(t *testing.T) {
	t.Parallel()
	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-danmaku/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, required := range []string{"access init", "--danmaku", "data.embedSnippet", "must not copy"} {
		if !strings.Contains(text, required) {
			t.Fatalf("hosted danmaku Skill omitted %q", required)
		}
	}
	for _, forbidden := range []string{"FOLLOW_OWNER", "WORK_ENTITLEMENT", "React blueprint"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("hosted danmaku Skill retained excluded capability %q", forbidden)
		}
	}
	if _, err := fs.Stat(cliembed.EmbeddedSkills(), "viceme-danmaku/references/cdn-sdk.md"); err != nil {
		t.Fatalf("hosted SDK contract is missing: %v", err)
	}
}

func TestEngagementSkillConsumesAuthoritativeCombinedSnippet(t *testing.T) {
	t.Parallel()
	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-engagement/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, required := range []string{
		"access init",
		"creator-app create",
		"creator-app domain verify",
		"creator-app show <app-id> --work-key <work-key> --locale <zh-CN-or-en-US>",
		"data.engagementEmbedSnippet",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("engagement Skill omitted %q", required)
		}
	}
	for _, forbidden := range []string{"website publish", "FOLLOW_OWNER", "WORK_ENTITLEMENT"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("engagement Skill retained excluded flow %q", forbidden)
		}
	}
}
