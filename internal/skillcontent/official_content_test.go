package skillcontent_test

import (
	"encoding/json"
	"io/fs"
	"reflect"
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

func TestDanmakuSkillUsesOnlyHostedIntegration(t *testing.T) {
	t.Parallel()
	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-danmaku/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, required := range []string{
		"merchant work website-verification create",
		"merchant work sdk-access",
		`data-viceme-features="danmaku"`,
		"must not copy",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("hosted danmaku Skill omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"FOLLOW_OWNER",
		"WORK_ENTITLEMENT",
		"React blueprint",
		"access init",
		"sdk-work:",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("hosted danmaku Skill retained excluded capability %q", forbidden)
		}
	}
	if _, err := fs.Stat(cliembed.EmbeddedSkills(), "viceme-danmaku/references/cdn-sdk.md"); err != nil {
		t.Fatalf("hosted SDK contract is missing: %v", err)
	}
}

func TestEngagementSkillUsesOneWorkAndCombinedAccess(t *testing.T) {
	t.Parallel()
	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-engagement/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, required := range []string{
		"merchant work sdk-access create",
		"--feature danmaku --feature tip",
		"WEBSITE_WIDGET",
		`data-viceme-features="danmaku,tip"`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("engagement Skill omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"website publish",
		"FOLLOW_OWNER",
		"WORK_ENTITLEMENT",
		"creator-app",
		"tip-embed.js",
		"engagement-embed.js",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("engagement Skill retained excluded flow %q", forbidden)
		}
	}
}

func TestEngagementSkillsCanCreateWebsiteWorkFromTheirOwnInstructions(t *testing.T) {
	t.Parallel()
	createInput := `{
		"kind": "WEBSITE",
		"merchantAccountId": "<merchant-id>",
		"clientRequestId": "<stable-idempotency-key>",
		"slug": "website-slug",
		"title": "Website title",
		"canonicalOrigin": "https://creator.example",
		"content": {
			"summary": "Observed public purpose",
			"bodyMarkdown": "Observed public description",
			"templateType": "WEBSITE",
			"tags": [],
			"media": [],
			"actionConfig": {}
		}
	}`
	publishInput := `{
		"merchantAccountId": "<merchant-id>",
		"expectedRevision": 2,
		"status": "PUBLISHED"
	}`

	for _, relativePath := range []string{
		"viceme-danmaku/SKILL.md",
		"viceme-tip/SKILL.md",
		"viceme-engagement/SKILL.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatalf("read embedded %s: %v", relativePath, err)
		}
		text := string(content)
		normalized := strings.Join(strings.Fields(strings.ReplaceAll(text, "\\\n", " ")), " ")
		for _, required := range []string{
			"`merchant-commerce:read`",
			"`merchant-commerce:write`",
			"`website.canonicalOrigin` exactly equals the deployed Origin",
			"Whether the Work was reused or created",
			"latest verification status is `PENDING`",
			"`DRAFT` Work with a `PENDING` verification",
			"current Work status is `DRAFT`",
			"Work status is already `PUBLISHED`",
			"If it is `SUSPENDED` or `ARCHIVED`, stop and report it",
			"Publish the returned `challenge` verbatim at `dnsRecordName`",
			"After public DNS resolves exactly",
		} {
			if !strings.Contains(normalized, required) {
				t.Fatalf("standalone %s omitted Website Work constraint %q", relativePath, required)
			}
		}
		requireOrderedSteps(t, relativePath, normalized, []string{
			"viceme profile list",
			"viceme --profile <profile> auth status",
			"viceme --profile <profile> merchant accounts",
			"viceme --profile <profile> merchant work list --merchant <merchant-id>",
			`"kind": "WEBSITE"`,
			"viceme --profile <profile> merchant work create --input <json>",
			"response is lost, replay the identical request with the same `clientRequestId`; do not create a new identity",
			"viceme --profile <profile> merchant work get <work-id> --merchant <merchant-id>",
			"viceme --profile <profile> merchant work website-verification get <work-id> --merchant <merchant-id>",
			"viceme --profile <profile> merchant work website-verification create <work-id> --merchant <merchant-id> --expected-revision <work-revision>",
			"Publish the returned `challenge` verbatim at `dnsRecordName`",
			"viceme --profile <profile> merchant work website-verification verify <work-id> --merchant <merchant-id> --expected-verification-version <verification-version>",
			"Read the Work again after verify",
			`"status": "PUBLISHED"`,
			"viceme --profile <profile> merchant work update <work-id> --input <json>",
			"viceme --profile <profile> merchant work get <work-id> --merchant <merchant-id>",
		})
		requireJSONBlock(t, relativePath, text, `"kind": "WEBSITE"`, createInput)
		requireJSONBlock(t, relativePath, text, `"status": "PUBLISHED"`, publishInput)
	}
}

func TestTipSkillsIncludeRecoverableWebsiteWidgetWorkflow(t *testing.T) {
	t.Parallel()
	applicationInput := `{
		"merchantAccountId": "<merchant-id>",
		"workId": "<work-id>",
		"kind": "WEBSITE_WIDGET",
		"environment": "PRODUCTION",
		"displayName": "<website name>",
		"origins": ["https://creator.example"],
		"returnUrls": []
	}`

	for _, relativePath := range []string{
		"viceme-tip/SKILL.md",
		"viceme-engagement/SKILL.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatalf("read embedded %s: %v", relativePath, err)
		}
		text := string(content)
		normalized := strings.Join(strings.Fields(strings.ReplaceAll(text, "\\\n", " ")), " ")
		for _, required := range []string{
			"matching this Work, `kind: WEBSITE_WIDGET`, `environment: PRODUCTION`, the exact canonical Origin, empty return URLs, and no Products",
			"If its status is `DRAFT`, activate its exact revision",
			"If it is already `ACTIVE`, skip activation",
			"If it is `SUSPENDED` or `ARCHIVED`, stop and report it",
			"If a create response is lost, list again before another create",
		} {
			if !strings.Contains(normalized, required) {
				t.Fatalf("standalone %s omitted Website Widget constraint %q", relativePath, required)
			}
		}
		requireOrderedSteps(t, relativePath, normalized, []string{
			"viceme --profile <profile> merchant commerce-application list --merchant <merchant-id>",
			`"kind": "WEBSITE_WIDGET"`,
			"viceme --profile <profile> merchant commerce-application create --input <json>",
			"viceme --profile <profile> merchant commerce-application get <application-id> --merchant <merchant-id>",
			"viceme --profile <profile> merchant commerce-application activate <application-id> --merchant <merchant-id> --expected-revision <application-revision>",
		})
		requireJSONBlock(t, relativePath, text, `"kind": "WEBSITE_WIDGET"`, applicationInput)
	}
}

func requireOrderedSteps(t *testing.T, relativePath, text string, steps []string) {
	t.Helper()
	offset := 0
	for _, step := range steps {
		index := strings.Index(text[offset:], step)
		if index < 0 {
			t.Fatalf("standalone %s omitted or misordered Website Work step %q", relativePath, step)
		}
		offset += index + len(step)
	}
}

func requireJSONBlock(t *testing.T, relativePath, text, marker, expected string) {
	t.Helper()
	blocks := regexp.MustCompile("(?s)```json\\s*(.*?)\\s*```").FindAllStringSubmatch(text, -1)
	for _, block := range blocks {
		if !strings.Contains(block[1], marker) {
			continue
		}
		var actualValue, expectedValue any
		if err := json.Unmarshal([]byte(block[1]), &actualValue); err != nil {
			t.Fatalf("standalone %s contains invalid JSON for %s: %v", relativePath, marker, err)
		}
		if err := json.Unmarshal([]byte(expected), &expectedValue); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actualValue, expectedValue) {
			t.Fatalf("standalone %s contains incomplete JSON for %s\nwant: %s\n got: %s", relativePath, marker, expected, block[1])
		}
		return
	}
	t.Fatalf("standalone %s omitted JSON block containing %s", relativePath, marker)
}

func TestWebsitePublicationUsesCurrentWorkBoundary(t *testing.T) {
	t.Parallel()

	publish, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(publish)
	for _, required := range []string{
		"A creator-owned website",
		"website-workflow.md",
		"verified Website Work",
		"Publish no Product",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("publish Skill omitted the current website Work boundary %q", required)
		}
	}
	if strings.Contains(text, "Payment integration remains a documented future capability") {
		t.Fatal("publish Skill still describes Website Widget tips as unavailable")
	}
}

func TestOfficialEngagementSkillsContainNoLegacyDomain(t *testing.T) {
	t.Parallel()

	for _, relativePath := range []string{
		"viceme-danmaku/SKILL.md",
		"viceme-danmaku/references/cdn-sdk.md",
		"viceme-tip/SKILL.md",
		"viceme-tip/references/integration-contract.md",
		"viceme-engagement/SKILL.md",
		"viceme-publish/references/website-workflow.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatalf("read embedded %s: %v", relativePath, err)
		}
		for _, forbidden := range []string{
			"SdkWork",
			"CreatorApp",
			"creator-app",
			"sdk-work:",
			"creator-app:",
			"FOLLOW_OWNER",
			"WORK_ENTITLEMENT",
			"data-creator-app-id",
			"tip-embed.js",
			"engagement-embed.js",
			".viceme/access.yaml",
		} {
			if strings.Contains(string(content), forbidden) {
				t.Fatalf("embedded %s retained legacy term %q", relativePath, forbidden)
			}
		}
	}
}
