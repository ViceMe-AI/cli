package skillcontent_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unicode"

	cliembed "github.com/ViceMe-AI/cli"
)

var officialSkillNames = []string{
	"charge-for-your-work",
	"become-a-creator",
	"sell-a-skill",
	"creator-tools",
	"viceme-skill-use",
	"let-people-interact",
}

func readOfficialSkillBundle(t *testing.T, skillName string) string {
	t.Helper()

	var bundle strings.Builder
	err := fs.WalkDir(cliembed.EmbeddedSkills(), skillName, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		switch path.Ext(filePath) {
		case ".html", ".json", ".md", ".yaml", ".yml":
		default:
			return nil
		}

		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), filePath)
		if err != nil {
			return err
		}
		bundle.WriteString("\n--- ")
		bundle.WriteString(filePath)
		bundle.WriteString(" ---\n")
		bundle.Write(content)
		return nil
	})
	if err != nil {
		t.Fatalf("read embedded Skill bundle %s: %v", skillName, err)
	}
	return bundle.String()
}

func containsHan(text string) bool {
	for _, char := range text {
		if unicode.Is(unicode.Han, char) {
			return true
		}
	}
	return false
}

func TestOfficialSkillsKeepOneChineseSourceAndMachineContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		machine   []string
		semantics []string
	}{
		{
			name: "creator-tools",
			machine: []string{
				"viceme auth status", "viceme auth login", "present_files", "VICEME_ACCESS_TOKEN",
				"AUTO_UPDATE_RESTART_REQUIRED", "error.code",
			},
			semantics: []string{
				"不得为了复用其他登录而替用户切换 Profile",
				"不得使用只完成一部分的代次继续",
			},
		},
		{
			name: "become-a-creator",
			machine: []string{
				"viceme auth status", "viceme auth login", "present_files", "viceme merchant accounts",
				"viceme merchant onboarding status", "viceme merchant onboarding apply", "claim-github",
				"claim-xiaohongshu", "MerchantAccountMember(role=OWNER)",
			},
			semantics: []string{
				"不创建平行申请", "直接申请模式不再确认", "玩法守卫模式",
				"申请中", "交回调用玩法",
			},
		},
		{
			name: "sell-a-skill",
			machine: []string{
				"$become-a-creator",
				"MerchantAccountMember(role=OWNER)", "publication confirm", "publication publish", "reviewDigest",
				"SKILL_LISTING_DRAFT_CHANGED", "priceMinor", "--new-listing",
			},
			semantics: []string{
				"公开且不可逆", "响应丢失时读取同一资源恢复", "只支持以下来源",
			},
		},
		{
			name: "viceme-skill-use",
			machine: []string{
				"owned=true", "nextAction=CONTINUE_ORIGINAL_TASK_WITH_INSTALLED_SKILL", "--wait 10m", "--agent auto",
				"?product=<product-id>",
			},
			semantics: []string{"不得要求再次购买", "不得停在“安装成功”"},
		},
		{
			name: "charge-for-your-work",
			machine: []string{
				"access.require()", "access.getFeatures()", "<viceme-access-layer>", "FOLLOW_OWNER", "WORK_ENTITLEMENT",
				"Hosted Checkout", "Product ID", "access.check()", "$become-a-creator",
			},
			semantics: []string{
				"登录不等于关注", "不得改变其参数、返回值、错误或副作用",
			},
		},
		{
			name: "let-people-interact",
			machine: []string{
				"website-verification create", "website-verification verify", "sdk-access", "keys.test", "keys.live",
				"--feature danmaku", "--feature tip", "0.5.0", "createViceMe", "mountDanmaku", "mountTip", "createTip",
			},
			semantics: []string{
				"仅弹幕", "仅赞赏", "弹幕加赞赏", "SANDBOX", "Headless", "宿主页与被赞赏 Work 是独立资源",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			bundle := readOfficialSkillBundle(t, test.name)
			if !strings.Contains(bundle, "跟随") {
				t.Fatal("missing the single-source language rule")
			}
			for _, required := range append(test.machine, test.semantics...) {
				if !strings.Contains(bundle, required) {
					t.Fatalf("official Skill bundle omitted contract %q", required)
				}
			}
		})
	}
}

func TestOfficialSkillEntryMetadataIsChinese(t *testing.T) {
	t.Parallel()

	for _, skillName := range officialSkillNames {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), path.Join(skillName, "SKILL.md"))
		if err != nil {
			t.Fatalf("read embedded %s metadata: %v", skillName, err)
		}
		frontmatterEnd := strings.Index(string(content[4:]), "\n---")
		if frontmatterEnd < 0 {
			t.Fatalf("embedded %s has no closing frontmatter delimiter", skillName)
		}
		frontmatter := string(content[:frontmatterEnd+4])
		if !containsHan(frontmatter) {
			t.Fatalf("embedded %s frontmatter has no Chinese description", skillName)
		}
	}
}

func TestInteractionTemplateUsesExactMountedTipESM(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "let-people-interact/templates/single-html.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, required := range []string{
		`import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.5.0/index.js";`,
		`import { mountTip } from "https://s3.viceme.cn/viceme-sdk/0.5.0/tip.js";`,
		`workKey: "wrk_test_REPLACE_WITH_PUBLIC_TEST_KEY"`, "tipHandle.destroy();", "client.destroy();",
		"公开 PUBLISHED Work", "宿主页不因此成为 Website Work", "静态文档没有页面内卸载",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("interaction template omitted exact Mounted Tip contract %q", required)
		}
	}
	if strings.Index(text, "tipHandle.destroy();") > strings.Index(text, "client.destroy();") {
		t.Fatal("interaction template destroys the client before its Tip mount")
	}
	for _, forbidden := range []string{"REPLACE_WITH_SDK_SCRIPT_URL", "data-viceme-", "window.ViceMe"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("interaction template retained legacy integration %q", forbidden)
		}
	}
}

func TestOfficialSkillMarkdownRelativeLinksResolve(t *testing.T) {
	t.Parallel()

	linkPattern := regexp.MustCompile(`\]\(([^)]+)\)`)
	for _, skillName := range officialSkillNames {
		err := fs.WalkDir(cliembed.EmbeddedSkills(), skillName, func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || path.Ext(filePath) != ".md" {
				return nil
			}
			content, err := fs.ReadFile(cliembed.EmbeddedSkills(), filePath)
			if err != nil {
				return err
			}
			for _, match := range linkPattern.FindAllStringSubmatch(string(content), -1) {
				target := strings.TrimSpace(strings.SplitN(match[1], "#", 2)[0])
				if target == "" || strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
					continue
				}
				resolved := path.Clean(path.Join(path.Dir(filePath), target))
				if _, err := fs.Stat(cliembed.EmbeddedSkills(), resolved); err != nil {
					t.Errorf("embedded %s has unresolved relative link %q: %v", filePath, match[1], err)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk embedded %s links: %v", skillName, err)
		}
	}
}

func TestPublishSkillKeepsHistoricalContextOutOfProfileSelection(t *testing.T) {
	t.Parallel()

	for _, relativePath := range []string{
		"become-a-creator/SKILL.md",
		"sell-a-skill/references/workflow.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatalf("read embedded %s: %v", relativePath, err)
		}
		text := string(content)
		for _, required := range []string{"当前 CLI 上下文", "旧对话"} {
			if !strings.Contains(text, required) {
				t.Fatalf("embedded %s does not contain profile precedence guard %q", relativePath, required)
			}
		}
	}

	for _, relativePath := range []string{
		"become-a-creator/SKILL.md",
		"sell-a-skill/SKILL.md",
		"sell-a-skill/references/workflow.md",
		"sell-a-skill/references/errors.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatalf("read embedded %s: %v", relativePath, err)
		}
		text := string(content)
		for _, forbidden := range []string{"profile list", "profile use"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("embedded %s exposes environment selection through %q", relativePath, forbidden)
			}
		}
		if regexp.MustCompile(`--profile(?:[ =]|$)`).MatchString(text) {
			t.Fatalf("embedded %s exposes environment selection through --profile", relativePath)
		}
	}
}

func TestCreatorOnboardingStopsAtHumanReviewAndUsesPlainLanguage(t *testing.T) {
	t.Parallel()

	onboarding, err := fs.ReadFile(cliembed.EmbeddedSkills(), "become-a-creator/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(onboarding)
	for _, required := range []string{
		"第一条进程命令运行 `viceme auth status`",
		"直接申请模式不再确认",
		"玩法守卫模式",
		"现在帮你申请成为创作者吗？",
		"viceme auth login --purpose creator-onboarding",
		"没有有效商家时运行一次 `viceme merchant onboarding status`",
		"merchant-commerce:read",
		"merchant-commerce:write",
		"creatorIdentity.markdownPath",
		"MERCHANT_APPLICATION_HANDLE_REQUIRED",
		"人工审核边界",
		"同一回合不得再次查询",
		"不把 DRAFT 创作者身份误当成资格",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("creator onboarding Skill omitted human-review guard %q", required)
		}
	}
	if currentUser := strings.TrimSpace(os.Getenv("USER")); currentUser != "" && strings.Contains(strings.ToLower(text), strings.ToLower(currentUser)) {
		t.Fatal("creator onboarding Skill uses the current developer username as an example")
	}
}

func TestPublishRetryPolicyDoesNotOwnCreatorOnboardingReads(t *testing.T) {
	t.Parallel()

	errorsContent, err := fs.ReadFile(cliembed.EmbeddedSkills(), "sell-a-skill/references/errors.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(errorsContent)
	for _, required := range []string{
		"发布命令（包括首次创建）的读取或写入",
		"本地已持久化的同一 Publication 或 client request identity",
		"不得因为首次响应未返回 Publication ID 就改变输入或创建另一项",
		"商家账户读取的错误全部交回 `$become-a-creator`",
		"不得套用本发布错误说明",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("publish retry policy omitted ownership boundary %q", required)
		}
	}
}

func TestCoreSkillsKeepInternalToolNamesOutOfUserFacingProgress(t *testing.T) {
	t.Parallel()

	for _, relativePath := range []string{
		"creator-tools/SKILL.md",
		"become-a-creator/SKILL.md",
		"sell-a-skill/SKILL.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatalf("read embedded %s: %v", relativePath, err)
		}
		text := string(content)
		for _, required := range []string{
			"自然白话",
			"不得告诉用户正在使用哪个内置 Skill",
		} {
			if !strings.Contains(text, required) {
				t.Fatalf("embedded %s omitted user-facing language guard %q", relativePath, required)
			}
		}
	}
}

func TestCoreSkillsForbidWorkBuddyTaskListsAndKeepBlockingStepsGuided(t *testing.T) {
	t.Parallel()

	for _, relativePath := range []string{
		"creator-tools/SKILL.md",
		"become-a-creator/SKILL.md",
		"sell-a-skill/SKILL.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatalf("read embedded %s: %v", relativePath, err)
		}
		text := string(content)
		for _, required := range []string{"不得调用 `TaskCreate`", "`TaskUpdate`", "`TaskList`"} {
			if !strings.Contains(text, required) {
				t.Fatalf("embedded %s omitted WorkBuddy task-list guard %q", relativePath, required)
			}
		}
		if !regexp.MustCompile(`不得[^。\n]*计划`).MatchString(text) {
			t.Fatalf("embedded %s omitted the no-plan-display guard", relativePath)
		}
	}

	onboarding, err := fs.ReadFile(cliembed.EmbeddedSkills(), "become-a-creator/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	onboardingText := string(onboarding)
	for _, required := range []string{
		"需要重新登录，我现在为你打开登录页面。",
		"请在右侧完成登录，完成后我会自动继续。",
		"也可以在外部浏览器打开下面这个链接",
		"另起一行用 Markdown 链接格式输出",
		"当前命令实际返回的完整链接：`[打开登录页面](https://…)`",
		"登录完成，我继续确认创作者资格。",
		"保存返回的 `task_id`",
		"`present_files` 返回也不代表登录完成",
		"`TaskOutput(task_id=<同一个任务>, timeout=180000)`",
		"只要任务仍在运行，就不得结束当前回合、给出最终答复",
		"不能把一次 `TaskOutput` 的读取超时当成登录流程完成",
		"不得编造路径或用户名",
	} {
		if !strings.Contains(onboardingText, required) {
			t.Fatalf("creator onboarding omitted guided blocking-step contract %q", required)
		}
	}
	waitStages := []string{
		"后台启动一次 `viceme auth login`",
		"用一次短时 `TaskOutput`",
		"立即用内置 `present_files`",
		"页面打开后立即说",
		"必须立刻再次调用 `TaskOutput",
		"继续对同一个 `task_id` 调用 `TaskOutput`",
		"只有登录命令成功返回后",
	}
	previousWaitStage := -1
	for _, stage := range waitStages {
		current := strings.Index(onboardingText, stage)
		if current < 0 {
			t.Fatalf("creator onboarding omitted deterministic login wait stage %q", stage)
		}
		if current <= previousWaitStage {
			t.Fatalf("creator onboarding login wait stage %q is out of order", stage)
		}
		previousWaitStage = current
	}

	shared, err := fs.ReadFile(cliembed.EmbeddedSkills(), "creator-tools/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	sharedText := string(shared)
	for _, required := range []string{
		"`TaskOutput(task_id=<同一个任务>, timeout=180000)`",
		"发送提示不等于继续等待",
		"只要它仍在运行，就不得结束当前回合、给出最终答复",
		"一次 `TaskOutput` 的读取超时不是登录失败",
		"也可以在外部浏览器打开下面这个链接",
		"另起一行用 Markdown 链接格式输出",
		"不要直接贴裸链接，不得重建、缩短或复用旧链接",
	} {
		if !strings.Contains(sharedText, required) {
			t.Fatalf("shared skill omitted deterministic login wait contract %q", required)
		}
	}

	publish, err := fs.ReadFile(cliembed.EmbeddedSkills(), "sell-a-skill/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(publish), "它仍在等待登录时，本发布流程不得结束当前回合或给出最终答复") {
		t.Fatal("publish skill may finish while creator onboarding is still waiting for login")
	}

	workflow, err := fs.ReadFile(cliembed.EmbeddedSkills(), "sell-a-skill/references/workflow.md")
	if err != nil {
		t.Fatal(err)
	}
	workflowText := string(workflow)
	stages := []string{
		"确认当前登录和创作者资格",
		"按来源完成且只完成必要的渠道确认",
		"创建或恢复同一私有草稿",
		"一次性补齐缺少的价格、文案和媒体",
		"只询问一次是否确认公开发布",
		"连续完成确认与公开发布",
	}
	previous := -1
	for _, stage := range stages {
		current := strings.Index(workflowText, stage)
		if current < 0 {
			t.Fatalf("publish workflow omitted fixed stage %q", stage)
		}
		if current <= previous {
			t.Fatalf("publish workflow stage %q is out of order", stage)
		}
		previous = current
	}
}

func TestPublishGithubFlowVerifiesOwnershipBeforeReadingSource(t *testing.T) {
	t.Parallel()
	const terminalReply = "最终答复只能是“当前环境还没有接好 GitHub 登录，暂时不能从 GitHub 发布。”这一句话"

	workflow, err := fs.ReadFile(cliembed.EmbeddedSkills(), "sell-a-skill/references/workflow.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	for _, required := range []string{
		"在读取任何仓库内容或执行发布命令前",
		"viceme merchant channel github <merchant-id>",
		"只启动一次等待式 `viceme merchant channel github <merchant-id>`",
		"用 Bash 后台启动并保存 `task_id`",
		"用一次短时 `TaskOutput` 读取当前命令输出",
		"这次检查默认静默",
		"立即使用内置 `present_files` 在当前任务浏览器打开同一个链接",
		"也可以在外部浏览器打开下面这个链接",
		"另起一行用 Markdown 链接格式输出当前命令实际返回的完整 `https://` 链接：`[打开 GitHub 授权页面](https://…)`",
		"`TaskOutput(task_id=<同一个任务>, timeout=180000)`",
		"只要命令仍在运行，就继续读取同一个 `task_id`",
		"命令成功返回 `kind=verified` 后立即继续发布",
		"不得再次运行渠道命令、轮询状态、`sleep` 或启动另一后台任务",
		"不得要求用户回复“完成了”",
		"也不得创建任务列表",
		"绝不能使用 `curl`、`gh`、`git`、WebFetch、浏览器抓取或 raw GitHub URL 代替这一步",
		"用户未指定分支时省略 `--github-ref`",
		"全部由 Agent 自动派生，绝不向用户询问",
		terminalReply,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("publish GitHub workflow omitted guard %q", required)
		}
	}

	errorsContent, err := fs.ReadFile(cliembed.EmbeddedSkills(), "sell-a-skill/references/errors.md")
	if err != nil {
		t.Fatal(err)
	}
	errorsText := string(errorsContent)
	for _, required := range []string{
		"OAUTH_PROVIDER_NOT_CONFIGURED",
		"确定性重试不会恢复",
		"不得 sleep、轮询、再次运行渠道命令",
		"不得继续追问是否切换来源",
		terminalReply,
	} {
		if !strings.Contains(errorsText, required) {
			t.Fatalf("publish error contract omitted OAuth configuration guard %q", required)
		}
	}
	for _, forbidden := range []string{
		"可以询问是否改用本地文件",
		"是否改用本地目录",
		"是否改用 ZIP",
		"用户之后主动提供本地目录",
	} {
		if strings.Contains(errorsText, forbidden) {
			t.Fatalf("publish error contract must end immediately instead of inviting source fallback %q", forbidden)
		}
	}

	skillContent, err := fs.ReadFile(cliembed.EmbeddedSkills(), "sell-a-skill/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skillContent), terminalReply) {
		t.Fatalf("publish entrypoint omitted terminal GitHub configuration reply contract %q", terminalReply)
	}
}

func TestPublishDelegatesCreatorQualification(t *testing.T) {
	t.Parallel()
	for _, relativePath := range []string{
		"sell-a-skill/SKILL.md",
		"sell-a-skill/references/workflow.md",
		"sell-a-skill/references/errors.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatal(err)
		}
		text := string(content)
		if !strings.Contains(text, "$become-a-creator") {
			t.Fatalf("%s does not delegate creator qualification", relativePath)
		}
		for _, forbidden := range []string{"viceme auth login", "merchant onboarding apply", "claim-github", "claim-xiaohongshu"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s still owns creator onboarding command %q", relativePath, forbidden)
			}
		}
	}
	for _, removed := range []string{
		"sell-a-skill/references/generic-product.md",
		"sell-a-skill/references/website-workflow.md",
	} {
		if _, err := fs.Stat(cliembed.EmbeddedSkills(), removed); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("removed publish route is still bundled: %s", removed)
		}
	}
	errorsContent, err := fs.ReadFile(cliembed.EmbeddedSkills(), "sell-a-skill/references/errors.md")
	if err != nil {
		t.Fatal(err)
	}
	errorsText := string(errorsContent)
	for _, required := range []string{
		"MERCHANT_COMMERCE_SCOPE_REQUIRED",
		"MERCHANT_SELECTION_REQUIRED",
		"这个错误发生在创建发布记录之前",
		"不得声称存在可恢复的原 Publication",
		"恢复已有 Publication 时，原商家不可更换",
		"用户明确希望另起一次独立发布时",
		"本发布流程也不得自行查询或选择账户",
	} {
		if !strings.Contains(errorsText, required) {
			t.Fatalf("publish error contract omitted delegated Merchant selection guard %q", required)
		}
	}
	if strings.Contains(errorsText, "展示返回的有效商家") {
		t.Fatal("publish error contract still performs Merchant account selection")
	}
}

func TestPublishDoesNotAdvertiseLegacyWebsitePublication(t *testing.T) {
	t.Parallel()

	publish, err := fs.ReadFile(cliembed.EmbeddedSkills(), "sell-a-skill/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(publish)
	for _, forbidden := range []string{"website-workflow.md", "generic-product.md", "merchant work create", "sdk-access create"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("paid Skill still owns a non-downloadable publish route %q", forbidden)
		}
	}
	metadata, err := fs.ReadFile(cliembed.EmbeddedSkills(), "sell-a-skill/agents/openai.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"网站作品", "商品或", "服务与"} {
		if strings.Contains(string(metadata), forbidden) {
			t.Fatalf("paid Skill metadata advertises a removed route %q", forbidden)
		}
	}
}

func TestInteractionSkillKeepsThreeBranchBoundaries(t *testing.T) {
	t.Parallel()
	bundle := readOfficialSkillBundle(t, "let-people-interact")
	for _, required := range []string{
		"仅弹幕", "仅赞赏", "弹幕加赞赏", "$become-a-creator", "MerchantAccountMember(role=OWNER)",
		"任意 kind Work", "PUBLISHED + VERIFIED Website Work", "marketRegion: cn", "页面 locale 不选择市场",
		"仅弹幕不受 CN/CNY 限制", "GLOBAL 必须立即停止", "Tip 本身不增加 Origin 或 Application 门禁",
		"只承载 Tip UI 的宿主页不是作品证据", "只有真实作品是可下载 Skill 时才转交 `$sell-a-skill`",
	} {
		if !strings.Contains(bundle, required) {
			t.Fatalf("interaction Skill omitted branch contract %q", required)
		}
	}
	for _, forbidden := range []string{
		"$viceme-creator-onboarding", "$viceme-publish", "目标特性集合固定",
		"merchant commerce-application create", "merchant commerce-application update",
		"merchant commerce-application suspend", "merchant commerce-application activate",
	} {
		if strings.Contains(bundle, forbidden) {
			t.Fatalf("interaction Skill retained retired or over-broad contract %q", forbidden)
		}
	}
}

func TestInteractionTipOnlyUsesAnyPublishedMerchantWorkWithoutOriginGate(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "let-people-interact/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	section := sectionBetween(string(content), "## 仅赞赏的 Work 选择", "## Website Work 与安全迁移")
	for _, required := range []string{
		"merchant work list", "owner.kind: MERCHANT", "owner.merchantAccountId", "status: PUBLISHED",
		"Work kind 不受限制", "最终创作者发布流程", "$sell-a-skill", "宿主页不是作品证据",
		"不执行 Website ownership verification", "已有可选 Commerce Application 只能提供来源归因，不是 Tip 门禁",
	} {
		if !strings.Contains(section, required) {
			t.Fatalf("tip-only branch omitted Work boundary %q", required)
		}
	}
	for _, forbidden := range []string{
		`"kind": "WEBSITE"`, "canonicalOrigin", "website-verification create",
		"merchant commerce-application create", "merchant commerce-application update",
		"merchant commerce-application suspend", "merchant commerce-application activate",
	} {
		if strings.Contains(section, forbidden) {
			t.Fatalf("tip-only branch retained host gate %q", forbidden)
		}
	}
}

func TestTipBearingInteractionPreflightsExactReleaseBeforeMutation(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "let-people-interact/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, required := range []string{
		"https://s3.viceme.cn/viceme-sdk/0.5.0/index.js",
		"https://s3.viceme.cn/viceme-sdk/0.5.0/tip.js",
		"https://s3.viceme.ai/viceme-sdk/0.5.0/index.js",
		"https://s3.viceme.ai/viceme-sdk/0.5.0/tip.js",
		"https://s3.viceme.cn/viceme-sdk/0.5.0/danmaku.js",
		"--connect-timeout 5", "--max-time 15", "--write-out '%{http_code}'",
		`"$asset_url")" || exit 1`, `test "$http_code" = "200" || exit 1`,
		"不得跟随或接受重定向", "npm view @viceme-ai/sdk@0.5.0 version --json",
		"--fetch-timeout=15000 --fetch-retries=0", "--registry=https://registry.npmjs.org",
		"--@viceme-ai:registry=https://registry.npmjs.org", "latest", "声明式或全局 loader",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Tip preflight omitted exact release guard %q", required)
		}
	}
	preflight := strings.Index(text, "https://s3.viceme.cn/viceme-sdk/0.5.0/index.js")
	mutation := strings.Index(text, "merchant work sdk-access create")
	if preflight < 0 || mutation < 0 || preflight >= mutation {
		t.Fatal("Tip release preflight does not precede SDK access mutation")
	}
}

func TestInteractionPreservesCompleteSDKAccessSnapshotAndPermanentKeys(t *testing.T) {
	t.Parallel()

	bundle := readOfficialSkillBundle(t, "let-people-interact")
	for _, required := range []string{
		"完整 hosted `features`", "完整 `accessFeatures`", "精确 `configVersion`",
		"现有 `features` 与本次分支请求的并集", "--feature danmaku", "--feature tip",
		"不传 `--follow`、`--purchase` 或 `--clear-access`", "原样写回",
		"create 一次返回 `keys.test` 与 `keys.live`", "不得轮换", "configVersion` 单调增加",
		"--clear-hosted", "sdk-access disable",
	} {
		if !strings.Contains(bundle, required) {
			t.Fatalf("interaction omitted complete SDK access contract %q", required)
		}
	}
}

func TestInteractionHeadlessContractExposesOnlyPublicFacade(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "let-people-interact/references/integration-contract.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, required := range []string{
		"GET /v1/work-sdk/:workKey/tip-config", `credentials: "omit"`, `redirect: "error"`, "AbortSignal",
		"8 秒", "16 KiB", "TIP_CONFIG_CREDENTIALS_NOT_ALLOWED", "sourceOrigin", "no-referrer", "fail closed",
		"viceme:tip-headless-ready", "viceme:tip-headless-init", "viceme:tip-headless-result",
		"event.origin", "event.source", "channel", "mode=headless", "PAID", "CANCELLED", "UNKNOWN",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Headless contract omitted public boundary %q", required)
		}
	}

	foundHeadless := false
	for _, example := range fencedBlocks(text, "js") {
		if !strings.Contains(example, "createTip(") || !strings.Contains(example, ".open(") {
			continue
		}
		foundHeadless = true
		for _, required := range []string{"createViceMe", "createTip", "getConfig", ".open(", "PAID", "CANCELLED", "UNKNOWN", "destroy"} {
			if !strings.Contains(example, required) {
				t.Fatalf("Headless example omitted facade token %q", required)
			}
		}
	}
	if !foundHeadless {
		t.Fatal("interaction reference contains no Headless Tip example")
	}
}

func TestInteractionExactESMExamplesOwnTheirLifecycles(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "let-people-interact/references/integration-contract.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, required := range []string{
		`from "https://s3.viceme.cn/viceme-sdk/0.5.0/index.js"`,
		`from "https://s3.viceme.cn/viceme-sdk/0.5.0/danmaku.js"`,
		`from "https://s3.viceme.cn/viceme-sdk/0.5.0/tip.js"`,
		"mountDanmaku(", "mountTip(", "createTip(", "Promise.allSettled", "tip.destroy();",
		"danmakuHandle.destroy();", "client.destroy();", "Local Fake", "SANDBOX", "keys.test", "keys.live",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("ESM lifecycle contract omitted %q", required)
		}
	}
}

func TestChargeForYourWorkUsesDualSDKKeys(t *testing.T) {
	t.Parallel()

	bundle := readOfficialSkillBundle(t, "charge-for-your-work")
	for _, required := range []string{
		"keys.test", "keys.live", "没有顶层单一 `workKey` 字段", "生产宿主把 `keys.live`",
		"完整 hosted `danmaku`/`tip` features", "完整 `accessFeatures`", "精确 `configVersion`", "没有发生轮换",
		"$become-a-creator", "拥有平台资源的发布流程",
	} {
		if !strings.Contains(bundle, required) {
			t.Fatalf("charge-for-your-work omitted dual-key boundary %q", required)
		}
	}
	for _, forbidden := range []string{"$viceme-publish", "$viceme-access"} {
		if strings.Contains(bundle, forbidden) {
			t.Fatalf("charge-for-your-work references retired Skill %q", forbidden)
		}
	}
}

func TestPaidSkillExcludesWebsiteAndGenericProductPublication(t *testing.T) {
	t.Parallel()

	publish, err := fs.ReadFile(cliembed.EmbeddedSkills(), "sell-a-skill/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(publish)
	for _, forbidden := range []string{"website-workflow.md", "generic-product.md", "merchant work", "commerce-application"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("paid Skill still contains removed publication route %q", forbidden)
		}
	}
}

func TestGameplaySkillsDelegateCreatorQualification(t *testing.T) {
	t.Parallel()
	for _, skillName := range []string{"sell-a-skill", "charge-for-your-work", "let-people-interact"} {
		text := readOfficialSkillBundle(t, skillName)
		if !strings.Contains(text, "$become-a-creator") {
			t.Fatalf("%s does not delegate creator qualification", skillName)
		}
		for _, forbidden := range []string{"viceme auth login", "viceme merchant accounts", "viceme merchant onboarding apply"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s duplicates onboarding command %q", skillName, forbidden)
			}
		}
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
	requireJSONBlockWithMarkers(t, relativePath, text, []string{marker}, expected)
}

func requireJSONBlockWithMarkers(t *testing.T, relativePath, text string, markers []string, expected string) {
	t.Helper()
	blocks := regexp.MustCompile("(?s)```json\\s*(.*?)\\s*```").FindAllStringSubmatch(text, -1)
	for _, block := range blocks {
		containsAll := true
		for _, marker := range markers {
			containsAll = containsAll && strings.Contains(block[1], marker)
		}
		if !containsAll {
			continue
		}
		var actualValue, expectedValue any
		if err := json.Unmarshal([]byte(block[1]), &actualValue); err != nil {
			t.Fatalf("standalone %s contains invalid JSON for %v: %v", relativePath, markers, err)
		}
		if err := json.Unmarshal([]byte(expected), &expectedValue); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actualValue, expectedValue) {
			t.Fatalf("standalone %s contains incomplete JSON for %v\nwant: %s\n got: %s", relativePath, markers, expected, block[1])
		}
		return
	}
	t.Fatalf("standalone %s omitted JSON block containing %v", relativePath, markers)
}

func sectionBetween(text, start, end string) string {
	startOffset := strings.Index(text, start)
	if startOffset < 0 {
		return ""
	}
	section := text[startOffset:]
	if endOffset := strings.Index(section, end); endOffset >= 0 {
		section = section[:endOffset]
	}
	return section
}

func fencedBlocks(text, language string) []string {
	matches := regexp.MustCompile("(?s)```" + regexp.QuoteMeta(language) + "\\s*(.*?)\\s*```").FindAllStringSubmatch(text, -1)
	blocks := make([]string, 0, len(matches))
	for _, match := range matches {
		blocks = append(blocks, match[1])
	}
	return blocks
}
