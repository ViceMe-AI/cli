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
		"viceme-publish/references/creator-workflow.md",
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

func TestPublishSkillRoutesTopLevelCreatorWorkflow(t *testing.T) {
	publishEntrypoint, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/SKILL.md")
	if err != nil {
		t.Fatalf("read embedded publish Skill: %v", err)
	}
	if !strings.Contains(string(publishEntrypoint), "references/creator-workflow.md") {
		t.Fatal("publish Skill does not route local Skill requests to the creator workflow")
	}

	creatorWorkflow, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/references/creator-workflow.md")
	if err != nil {
		t.Fatalf("read embedded creator workflow: %v", err)
	}
	for _, required := range []string{
		"viceme publish <path>",
		"CREATOR_SUBSCRIPTION",
		"shared monthly CNY price",
		"Do not offer 1/3/6/12-month variants",
		"same returned Publication ID",
		"untrusted data",
	} {
		if !strings.Contains(string(creatorWorkflow), required) {
			t.Fatalf("embedded creator workflow is missing %q", required)
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
		"Do not run `viceme doctor`, `viceme version`, `viceme install`",
		"stop immediately",
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

func TestDanmakuPOCSkillPinsStandaloneExecutable(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-danmaku/SKILL.md")
	if err != nil {
		t.Fatalf("read embedded danmaku Skill: %v", err)
	}
	if !strings.Contains(string(content), `"$HOME/.local/bin/viceme" access init`) {
		t.Fatal("embedded POC danmaku Skill can be shadowed by another viceme executable")
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
		for _, required := range []string{"$viceme-publish", "never publishes"} {
			if !strings.Contains(string(content), required) {
				t.Fatalf("embedded %s does not enforce explicit publication through %q", relativePath, required)
			}
		}
	}
}

func TestPOCSkillsUseOnlyPOCSDKPackage(t *testing.T) {
	t.Parallel()

	for _, relativePath := range []string{
		"viceme-access/SKILL.md",
		"viceme-access/references/integration.md",
		"viceme-danmaku/references/cdn-sdk.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatalf("read embedded %s: %v", relativePath, err)
		}
		text := string(content)
		if !strings.Contains(text, "@viceme-ai/sdk-poc") {
			t.Fatalf("embedded %s does not reference the POC SDK package", relativePath)
		}
		if strings.Contains(strings.ReplaceAll(text, "@viceme-ai/sdk-poc", ""), "@viceme-ai/sdk") {
			t.Fatalf("embedded %s references the formal SDK package", relativePath)
		}
	}

	accessSkill, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-access/SKILL.md")
	if err != nil {
		t.Fatalf("read embedded access Skill: %v", err)
	}
	if !strings.Contains(string(accessSkill), "@viceme-ai/sdk-poc@0.1.7-poc.16") {
		t.Fatal("embedded access Skill does not pin the exact POC SDK version")
	}
}
