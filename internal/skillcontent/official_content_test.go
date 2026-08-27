package skillcontent_test

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
	"unicode"

	cliembed "github.com/ViceMe-AI/cli"
)

var officialSkillNames = []string{
	"viceme-access",
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
			name: "viceme-publish",
			machine: []string{
				"viceme merchant accounts", "viceme merchant onboarding status", "claim-github", "claim-xiaohongshu",
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
		"viceme-publish/SKILL.md",
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

func TestPublishSkillStopsAtHumanReviewAndUsesPlainLanguage(t *testing.T) {
	t.Parallel()

	publish, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(publish)
	for _, required := range []string{
		"第一条进程命令必须是 `viceme auth status`",
		"原发布请求不等于授权提交申请",
		"异步人工审核边界",
		"同一回合不得再次查询",
		"面向用户的表达",
		"不得直接展示",
		"用于个人主页链接的英文名称",
		"申请已经提交，接下来需要工作人员审核",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("publish Skill omitted human-review guard %q", required)
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

func TestWebsitePublicationUsesCurrentWorkBoundary(t *testing.T) {
	t.Parallel()

	publish, err := fs.ReadFile(cliembed.EmbeddedSkills(), "viceme-publish/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(publish)
	for _, required := range []string{
		"创作者自己的网站",
		"Website Work",
		"本版本不包含网站支付",
		"未来能力",
		"不得为它发布 Product",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("publish Skill omitted the current website Work boundary %q", required)
		}
	}
	if strings.Contains(text, "references/website-workflow.md") {
		t.Fatal("publish Skill still routes to the superseded SdkWork website workflow")
	}
}
