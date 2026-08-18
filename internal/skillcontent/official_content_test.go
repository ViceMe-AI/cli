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

func TestDanmakuSkillDefaultsToHostedFastPath(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-danmaku/SKILL.md")
	if err != nil {
		t.Fatalf("read embedded danmaku Skill: %v", err)
	}
	text := string(content)
	for _, required := range []string{
		"Hosted fast path, default",
		"Target completion within about 10 seconds",
		"Do not start a dev server",
		"Never fall back to a local or hand-written widget",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("embedded danmaku Skill does not contain fast-path guard %q", required)
		}
	}

	for _, forbidden := range []string{
		"Verify in a browser that the SDK mounted",
		"Run the target repository's format, lint, typecheck",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("embedded danmaku Skill still requires slow default work %q", forbidden)
		}
	}
}
