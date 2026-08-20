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
		"$viceme-access",
	} {
		if !strings.Contains(string(publishWorkflow), required) {
			t.Fatalf("website publication workflow is missing %q", required)
		}
	}

	for _, relativePath := range []string{
		"viceme-access/SKILL.md",
		"viceme-access/references/integration.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatalf("read embedded %s: %v", relativePath, err)
		}
		if strings.Contains(string(content), "viceme website publish") {
			t.Fatalf("embedded %s still owns website publication", relativePath)
		}
	}
}
