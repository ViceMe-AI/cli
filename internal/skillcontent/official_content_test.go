package skillcontent_test

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
	"unicode"

	cliembed "github.com/ViceMe-AI/cli"
)

var officialSkillNames = []string{
	"viceme-access",
	"viceme-creator-onboarding",
	"viceme-danmaku",
	"viceme-engagement",
	"viceme-publish",
	"viceme-shared",
	"viceme-skill-use",
	"viceme-tip",
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
			name: "viceme-shared",
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
			name: "viceme-creator-onboarding",
			machine: []string{
				"viceme auth status", "viceme auth login", "present_files", "viceme merchant accounts",
				"viceme merchant onboarding status", "viceme merchant onboarding apply", "claim-github",
				"claim-xiaohongshu", "MerchantAccountMember(role=OWNER)",
			},
			semantics: []string{
				"不得创建平行申请", "不等于用户同意提交申请", "同一回合不得再次查询",
				"申请已经提交，接下来需要工作人员审核", "回到调用本 Skill 的发布",
			},
		},
		{
			name: "viceme-publish",
			machine: []string{
				"$viceme-creator-onboarding",
				"MerchantAccountMember(role=OWNER)", "publication confirm", "publication publish", "reviewDigest",
				"SKILL_LISTING_DRAFT_CHANGED", "priceMinor", "--new-listing",
			},
			semantics: []string{
				"公开且不可逆", "响应丢失时读取同一资源恢复", "支付成功与履约成功是两个不同状态",
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
			name: "viceme-access",
			machine: []string{
				"access.require()", "access.getFeatures()", "<viceme-access-layer>", "FOLLOW_OWNER", "WORK_ENTITLEMENT",
				"ACTIVE_CREATOR_SUBSCRIPTION", "window.open", "access.check()",
			},
			semantics: []string{
				"只有 `access.check()` 能授予权限", "不得改变其参数、返回值、错误或副作用",
			},
		},
		{
			name:    "viceme-danmaku",
			machine: []string{"data.embedSnippet", "--danmaku", "workKey", "'unsafe-eval'"},
			semantics: []string{
				"不得手改配置，也不得创建第二个 Work", "只挂载一个 SDK 根节点",
			},
		},
		{
			name: "viceme-engagement",
			machine: []string{
				"data.engagementEmbedSnippet", "creator-app domain verify", "--locale <zh-CN-or-en-US>", "unsafe-eval",
			},
			semantics: []string{
				"不得因为 apply 失败就创建第二个 Work", "真实支付尚未验证",
			},
		},
		{
			name: "viceme-tip",
			machine: []string{
				"creator-app domain add", "creator-app domain verify", "creator-app show", "data.embedSnippet",
				"data-creator-app-id", "verified: true",
			},
			semantics: []string{
				"不得仅因 HTML 移动就创建重复项", "打开界面不代表支付已经结算",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			bundle := readOfficialSkillBundle(t, test.name)
			if !strings.Contains(bundle, "跟随用户当前") {
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

func TestTipTemplateUsesCLIResponseAsItsOnlyEmbedSource(t *testing.T) {
	t.Parallel()

	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-tip/templates/single-html.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, required := range []string{"当前固定 Profile", "creator-app show", "data.embedSnippet", "不要手工推导"} {
		if !strings.Contains(text, required) {
			t.Fatalf("tip template omitted authoritative CLI source guard %q", required)
		}
	}
	if strings.Contains(text, "Creator Center") {
		t.Fatal("tip template still treats Creator Center as the embed value source")
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
		"viceme-creator-onboarding/SKILL.md",
		"viceme-publish/references/workflow.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatalf("read embedded %s: %v", relativePath, err)
		}
		text := string(content)
		for _, required := range []string{
			"记忆",
			"旧对话",
			"当前 CLI 上下文",
			"已登录用户",
		} {
			if !strings.Contains(text, required) {
				t.Fatalf("embedded %s does not contain profile precedence guard %q", relativePath, required)
			}
		}
	}

	for _, relativePath := range []string{
		"viceme-creator-onboarding/SKILL.md",
		"viceme-publish/SKILL.md",
		"viceme-publish/references/workflow.md",
		"viceme-publish/references/errors.md",
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

	onboarding, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-creator-onboarding/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(onboarding)
	for _, required := range []string{
		"第一条进程命令必须是 `viceme auth status`",
		"不等于用户同意提交申请",
		"先用白话说明原因并询问是否现在申请",
		"可以直接提交，不重复确认",
		"没有有效商家时运行一次 `viceme merchant onboarding status`",
		"把返回的商家交回原任务继续使用",
		"merchant-commerce:read",
		"merchant-commerce:write",
		"MERCHANT_COMMERCE_SCOPE_REQUIRED",
		"PUBLICATION_SCOPE_REQUIRED",
		"浏览器结果页明确显示授权完成后",
		"只运行一次 `viceme merchant onboarding status --merchant <merchant-account-id>`",
		"evidence <onboarding-id> --path <截图路径> --lock-version <当前版本>",
		"submit <onboarding-id> --lock-version <新版本>",
		"人工审核边界",
		"同一回合不得再次查询",
		"中文交流必须使用自然白话",
		"不得告诉用户正在使用哪个内置 Skill",
		"Profile 或 CLI 命令",
		"用于个人主页链接的英文名称",
		"申请已经提交，接下来需要工作人员审核",
		"`error.code=INTERNAL_ERROR` 且 `retryable=true`",
		"立即重试一次同一条读取",
		"不得 `sleep`、轮询、启动后台进程",
		"不得读取发布流程的错误说明",
		"第二次返回任何失败都停止本次操作",
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

	errorsContent, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/references/errors.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(errorsContent)
	for _, required := range []string{
		"发布命令（包括首次创建）的读取或写入",
		"本地已持久化的同一 Publication 或 client request identity",
		"不得因为首次响应未返回 Publication ID 就改变输入或创建另一项",
		"商家账户读取的错误全部交回 `$viceme-creator-onboarding`",
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
		"viceme-shared/SKILL.md",
		"viceme-creator-onboarding/SKILL.md",
		"viceme-publish/SKILL.md",
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
		"viceme-shared/SKILL.md",
		"viceme-creator-onboarding/SKILL.md",
		"viceme-publish/SKILL.md",
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

	onboarding, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-creator-onboarding/SKILL.md")
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

	shared, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-shared/SKILL.md")
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

	publish, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(publish), "它仍在等待登录时，本发布流程不得结束当前回合或给出最终答复") {
		t.Fatal("publish skill may finish while creator onboarding is still waiting for login")
	}

	workflow, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/references/workflow.md")
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

	workflow, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/references/workflow.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	for _, required := range []string{
		"在读取任何仓库内容或执行发布命令前",
		"viceme merchant channel github <merchant-id>",
		"只启动一次等待式 `viceme merchant channel github <merchant-id>`",
		"用 Bash 后台启动并保存 `task_id`",
		"用一次短时 `TaskOutput` 读取当前命令输出的完整授权链接",
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
		"不得读取仓库后自行编造这些参数",
		terminalReply,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("publish GitHub workflow omitted guard %q", required)
		}
	}

	errorsContent, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/references/errors.md")
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

	skillContent, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/SKILL.md")
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
		"viceme-publish/SKILL.md",
		"viceme-publish/references/workflow.md",
		"viceme-publish/references/generic-product.md",
		"viceme-publish/references/errors.md",
	} {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), relativePath)
		if err != nil {
			t.Fatal(err)
		}
		text := string(content)
		if !strings.Contains(text, "$viceme-creator-onboarding") {
			t.Fatalf("%s does not delegate creator qualification", relativePath)
		}
		for _, forbidden := range []string{"viceme auth login", "merchant onboarding apply", "claim-github", "claim-xiaohongshu"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s still owns creator onboarding command %q", relativePath, forbidden)
			}
		}
	}
	if _, err := fs.Stat(cliembed.EmbeddedSkills(), "viceme-publish/references/website-workflow.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("superseded website workflow is still embedded: %v", err)
	}
	errorsContent, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/references/errors.md")
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

func TestDanmakuSkillUsesOnlyHostedIntegration(t *testing.T) {
	t.Parallel()
	content, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-danmaku/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, required := range []string{"access init", "--danmaku", "data.embedSnippet", "不得复制"} {
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

func TestPublishDoesNotAdvertiseLegacyWebsitePublication(t *testing.T) {
	t.Parallel()

	publish, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(publish)
	for _, required := range []string{
		"网站发布目前尚未开放",
		"不是当前可执行的发布路线",
		"不得运行旧的 `viceme website publish`",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("publish Skill omitted the current website publication boundary %q", required)
		}
	}
	if strings.Contains(text, "references/website-workflow.md") {
		t.Fatal("publish Skill still routes to the superseded SdkWork website workflow")
	}
	metadata, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/agents/openai.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(metadata), "网站作品") {
		t.Fatal("publish Skill metadata still advertises unavailable website publication")
	}
}
