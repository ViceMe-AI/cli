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
		"keys.test",
		"keys.live",
		"mountDanmaku",
		"mountTip",
		"createTip",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("engagement Skill omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"website publish",
		"merchant commerce-application",
		"WEBSITE_WIDGET",
		"orderNo",
		"PaymentAction",
		"testMode",
		"FOLLOW_OWNER",
		"WORK_ENTITLEMENT",
		"creator-app",
		"tip-embed.js",
		"engagement-embed.js",
		"/viceme-sdk/v1",
		"data-viceme-",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("engagement Skill retained excluded flow %q", forbidden)
		}
	}
}

func TestDanmakuBearingSkillsKeepWebsiteVerificationInstructions(t *testing.T) {
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

func TestTipSkillTargetsAnyPublishedMerchantWorkWithoutOriginGate(t *testing.T) {
	t.Parallel()
	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-tip/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	normalized := strings.Join(strings.Fields(strings.ReplaceAll(text, "\\\n", " ")), " ")
	requireOrderedSteps(t, "viceme-tip/SKILL.md", normalized, []string{
		"viceme profile list",
		"API base URL",
		"marketRegion",
		"viceme --profile <profile> auth status",
		"viceme --profile <profile> merchant accounts",
		"viceme --profile <profile> merchant work list --merchant <merchant-id>",
		"any Work kind",
		"owner.kind: MERCHANT",
		"status: PUBLISHED",
		"confirm the selected Work",
		"embedding page and selected Work as separate resources",
		"Do not infer the Work kind from the embedding page",
		"choose the official UI or Headless",
		"Only after the complete dual-region preflight",
		"merchant work sdk-access get",
		"--feature tip",
		"When status is `DISABLED`",
		"keys.test",
		"keys.live",
	})
	if !strings.Contains(text, "`viceme-publish`") {
		t.Fatal("tip Skill does not delegate missing Work publication to viceme-publish")
	}
	for _, forbidden := range []string{
		"merchant work website-verification",
		`"kind": "WEBSITE"`,
		"merchant commerce-application create",
		`"origins"`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("tip default flow retained gate %q", forbidden)
		}
	}
}

func TestTipOfficialUIIsSandboxFirstAndProductionNeedsConfirmation(t *testing.T) {
	t.Parallel()
	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-tip/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	blocks := fencedBlocks(text, "html")
	if len(blocks) < 2 {
		t.Fatalf("tip Skill must contain SANDBOX and PRODUCTION official UI snippets, got %d", len(blocks))
	}
	assertOfficialTipSnippet(t, blocks[0], "wrk_test_...")
	assertOfficialTipSnippet(t, blocks[1], "wrk_live_...")
	normalized := strings.Join(strings.Fields(text), " ")
	requireOrderedSteps(t, "viceme-tip/SKILL.md", normalized, []string{
		`workKey: "wrk_test_..."`,
		"Only after the user explicitly confirms the SANDBOX result",
		`workKey: "wrk_live_..."`,
	})
}

func TestTipSkillPreflightsSelectedReleaseBeforeAnyWorkOrSDKWrite(t *testing.T) {
	t.Parallel()
	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-tip/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(strings.ReplaceAll(string(content), "\\\n", " ")), " ")
	requireOrderedSteps(t, "viceme-tip/SKILL.md", text, []string{
		"merchant work list",
		"choose the official UI or Headless",
		"curl --fail --silent --show-error --output /dev/null https://s3.viceme.cn/viceme-sdk/0.4.0/index.js",
		"curl --fail --silent --show-error --output /dev/null https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js",
		"curl --fail --silent --show-error --output /dev/null https://s3.viceme.ai/viceme-sdk/0.4.0/index.js",
		"curl --fail --silent --show-error --output /dev/null https://s3.viceme.ai/viceme-sdk/0.4.0/tip.js",
		"npm view @viceme-ai/sdk@0.4.0 version --json",
		"--registry=https://registry.npmjs.org",
		"--@viceme-ai:registry=https://registry.npmjs.org",
		"Only after the complete dual-region preflight",
		"load `viceme-publish`",
		"merchant work sdk-access get",
		"merchant work sdk-access create",
		"merchant work sdk-access update",
	})
	for _, required := range []string{
		"Do not create, verify, or publish a Work",
		"snapshot its complete feature set",
		"restore that complete feature set",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("tip Skill omitted safe SDK access mutation boundary %q", required)
		}
	}
}

func TestTipHeadlessExamplesExposeOnlyThePublicFacade(t *testing.T) {
	t.Parallel()
	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-tip/references/integration-contract.md")
	if err != nil {
		t.Fatal(err)
	}
	blocks := fencedBlocks(string(content), "js")
	var npmExample, cdnExample string
	for _, block := range blocks {
		switch {
		case strings.Contains(block, `from "@viceme-ai/sdk"`):
			npmExample = block
		case strings.Contains(block, `/viceme-sdk/0.4.0/index.js`) && strings.Contains(block, "createTip"):
			cdnExample = block
		}
	}
	for name, example := range map[string]string{"npm": npmExample, "CDN ESM": cdnExample} {
		if example == "" {
			t.Fatalf("%s Headless example is missing", name)
		}
		for _, required := range []string{
			"createViceMe", "createTip", ".getConfig()", "config.amount.minCents",
			"config.amount.maxCents", "config.providers", "renderTipControls", ".open(",
			"resultPromise", "result.work", "result.amountCents", "result.currency",
			"tip.destroy()", "client.destroy()",
		} {
			if !strings.Contains(example, required) {
				t.Fatalf("%s Headless example omitted %q", name, required)
			}
		}
		statuses := regexp.MustCompile(`case ["']([A-Z]+)["']`).FindAllStringSubmatch(example, -1)
		gotStatuses := make([]string, 0, len(statuses))
		for _, status := range statuses {
			gotStatuses = append(gotStatuses, status[1])
		}
		if !reflect.DeepEqual(gotStatuses, []string{"PAID", "CANCELLED", "UNKNOWN"}) {
			t.Fatalf("%s Headless example handles statuses %v", name, gotStatuses)
		}
		for _, forbidden := range []string{"fetch(", "XMLHttpRequest", "/tip-orders", "/orders/", "orderNo", "token", "PaymentAction", "metadata", "scene", "testMode"} {
			if strings.Contains(example, forbidden) {
				t.Fatalf("%s Headless example exposes forbidden raw surface %q", name, forbidden)
			}
		}
	}
	if !strings.Contains(cdnExample, `from "https://s3.viceme.cn/viceme-sdk/0.4.0/index.js"`) ||
		!strings.Contains(cdnExample, `from "https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js"`) ||
		strings.Contains(cdnExample, "/v1/") {
		t.Fatal("CDN Headless example does not pin the immutable CN ESM release")
	}
	if !strings.Contains(string(content), `createTestTip } from "@viceme-ai/sdk/tip/testing"`) {
		t.Fatal("Headless contract does not use the official Local Fake adapter")
	}
	normalizedContent := strings.Join(strings.Fields(string(content)), " ")
	for _, required := range []string{
		"directly from the button click or keyboard activation stack",
		"Do not bind cleanup to `pagehide`",
	} {
		if !strings.Contains(normalizedContent, required) {
			t.Fatalf("Headless contract omitted lifecycle boundary %q", required)
		}
	}
	if strings.Contains(string(content), `addEventListener("pagehide"`) {
		t.Fatal("Headless contract destroys the SDK on bfcache pagehide")
	}
	for _, required := range []string{
		"npm view @viceme-ai/sdk@0.4.0 version --json",
		"--registry=https://registry.npmjs.org",
		"--@viceme-ai:registry=https://registry.npmjs.org",
		"curl --fail --silent --show-error --output /dev/null https://s3.viceme.cn/viceme-sdk/0.4.0/index.js",
		"curl --fail --silent --show-error --output /dev/null https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js",
		"curl --fail --silent --show-error --output /dev/null https://s3.viceme.ai/viceme-sdk/0.4.0/index.js",
		"curl --fail --silent --show-error --output /dev/null https://s3.viceme.ai/viceme-sdk/0.4.0/tip.js",
		"If the version is unavailable, stop",
		"If any object is unavailable, stop",
	} {
		if !strings.Contains(string(content), required) {
			t.Fatalf("Headless contract omitted immutable release preflight %q", required)
		}
	}
}

func TestTipReferenceOfficialUIUsesExactESMAndOwnedLifecycle(t *testing.T) {
	t.Parallel()
	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-tip/references/integration-contract.md")
	if err != nil {
		t.Fatal(err)
	}
	var officialExample string
	for _, block := range fencedBlocks(string(content), "js") {
		if strings.Contains(block, "mountTip") && !strings.Contains(block, "createTip") {
			officialExample = block
			break
		}
	}
	if officialExample == "" {
		t.Fatal("Tip integration reference omitted the official ESM example")
	}
	assertOfficialTipSnippet(t, officialExample, "wrk_test_...")
	normalized := strings.Join(strings.Fields(string(content)), " ")
	for _, required := range []string{
		"every real SPA, component, or route unmount",
		"mount is destroyed before the client",
		"Do not bind cleanup to `pagehide`",
	} {
		if !strings.Contains(normalized, required) {
			t.Fatalf("Tip integration reference omitted official lifecycle rule %q", required)
		}
	}
}

func TestCombinedEngagementChoosesExactlyOneTipUIAndRejectsGlobal(t *testing.T) {
	t.Parallel()
	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-engagement/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(content)), " ")
	for _, required := range []string{
		"either the official Tip UI or Headless Tip",
		"one selected Tip UI path",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("combined engagement discovery omitted UI choice %q", required)
		}
	}
	requireOrderedSteps(t, "viceme-engagement/SKILL.md", text, []string{
		"marketRegion",
		"unless `marketRegion` is exactly `cn`",
		"choose the official Tip UI or Headless Tip",
		"https://s3.viceme.cn/viceme-sdk/0.4.0/index.js",
		"https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js",
		"https://s3.viceme.ai/viceme-sdk/0.4.0/index.js",
		"https://s3.viceme.ai/viceme-sdk/0.4.0/tip.js",
		"https://s3.viceme.cn/viceme-sdk/0.4.0/danmaku.js",
		"Only after the complete dual-region preflight",
		"merchant work create",
		"merchant work website-verification create",
		"merchant work website-verification verify",
		"merchant work update",
		"merchant work sdk-access create",
		"--feature danmaku --feature tip",
		"merchant work sdk-access update",
		"For the official UI",
		"mountDanmaku",
		"mountTip",
		"For Headless Tip",
		"createTip",
	})
	for _, required := range []string{
		"one create or update",
		"Snapshot the previous complete feature set",
		"restore the complete pre-change feature set",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("combined engagement omitted atomic feature-set boundary %q", required)
		}
	}
}

func TestTipBearingFlowsRequireCompleteOfficialReleaseBeforeMutation(t *testing.T) {
	t.Parallel()

	for _, relativePath := range []string{
		"viceme-tip/SKILL.md",
		"viceme-engagement/SKILL.md",
		"viceme-tip/references/integration-contract.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatalf("read embedded %s: %v", relativePath, err)
		}
		text := string(content)
		for _, required := range []string{
			"https://s3.viceme.cn/viceme-sdk/0.4.0/index.js",
			"https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js",
			"https://s3.viceme.ai/viceme-sdk/0.4.0/index.js",
			"https://s3.viceme.ai/viceme-sdk/0.4.0/tip.js",
			"--registry=https://registry.npmjs.org",
			"--@viceme-ai:registry=https://registry.npmjs.org",
		} {
			if !strings.Contains(text, required) {
				t.Fatalf("embedded %s omitted complete release check %q", relativePath, required)
			}
		}
	}
}

func TestAllPublicationRoutesAndDanmakuCannotBypassTipPreflight(t *testing.T) {
	t.Parallel()

	publish, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	publishText := strings.Join(strings.Fields(string(publish)), " ")
	for _, required := range []string{
		"Before selecting any publication route",
		"load `viceme-tip` and complete its exact Tip release preflight",
		"before any Listing, Publication, Work, Product, Website verification, SDK access, or host write",
		"applies equally to Skill packages, generic offerings, and Websites",
	} {
		if !strings.Contains(publishText, required) {
			t.Fatalf("publish routing omitted Tip preflight boundary %q", required)
		}
	}
	if strings.Contains(publishText, "finish publication first and then") {
		t.Fatal("publish routing still performs Work writes before Tip preflight")
	}

	publicationRoutes := []struct {
		path       string
		preflight  string
		firstWrite string
	}{
		{"viceme-publish/references/workflow.md", "complete its exact Tip release preflight", "skill publish --path"},
		{"viceme-publish/references/generic-product.md", "complete its exact Tip release preflight", "viceme merchant work create"},
		{"viceme-publish/references/website-workflow.md", "Complete the exact Tip release preflight", "merchant work create"},
	}
	for _, route := range publicationRoutes {
		content, readErr := fs.ReadFile(cliembed.EmbeddedSkills(), route.path)
		if readErr != nil {
			t.Fatalf("read embedded %s: %v", route.path, readErr)
		}
		text := strings.Join(strings.Fields(string(content)), " ")
		preflightIndex := strings.Index(text, route.preflight)
		writeIndex := strings.Index(text, route.firstWrite)
		if preflightIndex < 0 || writeIndex < 0 || preflightIndex >= writeIndex {
			t.Fatalf("embedded %s does not require Tip preflight before its first write", route.path)
		}
	}

	danmaku, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-danmaku/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	danmakuText := strings.Join(strings.Fields(string(danmaku)), " ")
	for _, required := range []string{
		"feature snapshot contains `tip`",
		"before any Work creation, update, Website verification, publication, SDK access write, or page edit",
		"load `viceme-engagement`",
		"never preserve or re-enable `tip` from this flow",
	} {
		if !strings.Contains(danmakuText, required) {
			t.Fatalf("danmaku flow omitted Tip delegation boundary %q", required)
		}
	}
	if strings.Contains(danmakuText, "preserving `tip` if already enabled") {
		t.Fatal("danmaku flow still re-enables Tip without the Tip preflight")
	}
	sdkAccessIndex := strings.Index(danmakuText, "merchant work sdk-access get <work-id>")
	workCreateIndex := strings.Index(danmakuText, "merchant work create --input")
	verificationIndex := strings.Index(danmakuText, "merchant work website-verification create")
	if sdkAccessIndex < 0 || workCreateIndex < 0 || verificationIndex < 0 ||
		sdkAccessIndex >= workCreateIndex || sdkAccessIndex >= verificationIndex {
		t.Fatal("danmaku flow does not inspect existing Tip access before Work or verification writes")
	}
}

func TestCombinedEngagementUsesOwnedESMLifecycles(t *testing.T) {
	t.Parallel()
	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-engagement/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	blocks := fencedBlocks(string(content), "html")
	var officialExample, headlessExample string
	for _, block := range blocks {
		switch {
		case strings.Contains(block, "mountTip("):
			officialExample = block
		case strings.Contains(block, "createTip("):
			headlessExample = block
		}
	}
	if officialExample == "" || headlessExample == "" {
		t.Fatalf("combined engagement ESM examples are incomplete: official=%t headless=%t", officialExample != "", headlessExample != "")
	}
	for _, required := range []string{
		`from "https://s3.viceme.cn/viceme-sdk/0.4.0/index.js"`,
		`from "https://s3.viceme.cn/viceme-sdk/0.4.0/danmaku.js"`,
		`from "https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js"`,
		"mountDanmaku(", "mountTip(", "const mountHandles", "handle.destroy();", "client.destroy();",
	} {
		if !strings.Contains(officialExample, required) {
			t.Fatalf("combined official example omitted %q", required)
		}
	}
	requireOrderedSteps(t, "combined official example", officialExample, []string{
		"const mountHandles",
		"handle.destroy();",
		"client.destroy();",
	})
	for _, required := range []string{
		`from "https://s3.viceme.cn/viceme-sdk/0.4.0/index.js"`,
		`from "https://s3.viceme.cn/viceme-sdk/0.4.0/danmaku.js"`,
		`from "https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js"`,
		"mountDanmaku(", "createTip(", "tip.getConfig()", "tip.open(", "tip.destroy();", "danmakuHandle.destroy();", "client.destroy();",
	} {
		if !strings.Contains(headlessExample, required) {
			t.Fatalf("combined Headless example omitted %q", required)
		}
	}
	requireOrderedSteps(t, "combined Headless example", headlessExample, []string{
		"tip.destroy();",
		"danmakuHandle.destroy();",
		"client.destroy();",
	})
	if strings.Contains(officialExample, "createTip(") || strings.Contains(headlessExample, "mountTip(") {
		t.Fatal("combined engagement mounts both Tip UIs on one selected route")
	}
	for _, example := range []string{officialExample, headlessExample} {
		for _, forbidden := range []string{"/viceme-sdk/v1", "data-viceme-", "window.ViceMe"} {
			if strings.Contains(example, forbidden) {
				t.Fatalf("combined engagement example retained forbidden integration %q", forbidden)
			}
		}
	}
	normalized := strings.Join(strings.Fields(string(content)), " ")
	for _, required := range []string{"real SPA, component, or route unmount", "destroyed before `client.destroy()`", "Do not use `pagehide`"} {
		if !strings.Contains(normalized, required) {
			t.Fatalf("combined engagement omitted lifecycle boundary %q", required)
		}
	}
}

func TestTipIntegrationContractDefinesOpenValidationBoundary(t *testing.T) {
	t.Parallel()
	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-tip/references/integration-contract.md")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(content)), " ")
	for _, required := range []string{
		"CN and CNY",
		"100..20000 fen",
		"anonymous to the creator",
		"not anonymous to ViceMe or the payment provider",
		"provider is optional",
		"scene is platform-selected",
		"read-only confirmation layer",
		"cross-refresh order recovery",
		"UNKNOWN is not a failure",
		"Only `PAID` carries data",
		"`CANCELLED` and `UNKNOWN` carry only their status",
		"Local Fake",
		"SANDBOX key",
		"PRODUCTION key cannot simulate payment",
		"permanent public identifiers",
		"unverified Origin",
		"optional Commerce Application",
		"embedding page and selected Work are independent",
		"does not verify or claim ownership of the host",
		"never the full URL",
		"do not use WeChat JSAPI",
		"external browser or another available provider",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("tip integration contract omitted boundary %q", required)
		}
	}
}

func requireOrderedSteps(t *testing.T, relativePath, text string, steps []string) {
	t.Helper()
	offset := 0
	for _, step := range steps {
		index := strings.Index(text[offset:], step)
		if index < 0 {
			t.Fatalf("standalone %s omitted or misordered contract step %q", relativePath, step)
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

func fencedBlocks(text, language string) []string {
	matches := regexp.MustCompile("(?s)```"+regexp.QuoteMeta(language)+"\\s*(.*?)\\s*```").FindAllStringSubmatch(text, -1)
	blocks := make([]string, 0, len(matches))
	for _, match := range matches {
		blocks = append(blocks, match[1])
	}
	return blocks
}

func assertOfficialTipSnippet(t *testing.T, snippet, workKey string) {
	t.Helper()
	for _, required := range []string{
		`import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.4.0/index.js";`,
		`import { mountTip } from "https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js";`,
		`createViceMe({ workKey: "` + workKey + `", region: "cn" })`,
		"const mountHandle = await mountTip(client, {",
		"target,",
		`theme: "auto"`,
		"mountHandle.destroy();",
		"client.destroy();",
	} {
		if !strings.Contains(snippet, required) {
			t.Fatalf("official Tip snippet omitted %q\n%s", required, snippet)
		}
	}
	requireOrderedSteps(t, "official Tip snippet", snippet, []string{
		"const mountHandle = await mountTip",
		"mountHandle.destroy();",
		"client.destroy();",
	})
	for _, forbidden := range []string{"/viceme-sdk/v1", "data-viceme-", "window.ViceMe"} {
		if strings.Contains(snippet, forbidden) {
			t.Fatalf("official Tip snippet retained forbidden integration %q", forbidden)
		}
	}
}

func TestWebsitePublicationUsesCurrentWorkBoundary(t *testing.T) {
	t.Parallel()

	publish, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(publish)), " ")
	for _, required := range []string{
		"A creator-owned website",
		"A page used only to host Tip UI is integration context",
		"only when the user explicitly wants that website represented as a ViceMe Work",
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
