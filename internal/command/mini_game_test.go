package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/gofrs/flock"
)

func miniGameFixture() api.MiniGameIntegration {
	return api.MiniGameIntegration{SchemaVersion: 2, RuntimeVersion: "2.0.0", WorkID: "11111111-1111-4111-8111-111111111111", WorkTitle: "已有小游戏", Environment: "SANDBOX", PublicClientID: "vca_" + strings.Repeat("a", 32), SharedSecret: strings.Repeat("a", 64), CheckoutOrigin: "https://viceme.cn", Items: []api.MiniGameItem{{ID: "33333333-3333-4333-8333-333333333333", Alias: "hero-sword", Title: "英雄之剑", Status: "ACTIVE"}}}
}

func writeMiniGameHost(t *testing.T, project string) {
	t.Helper()
	miniGameWrite(t, project, "index.html", `<!doctype html><html><body><button id="buy">购买英雄之剑</button><script src="viceme/mini-game-commerce.js"></script><script src="viceme/mini-game-config.js"></script><script src="game.js"></script></body></html>`)
	miniGameWrite(t, project, "game.js", `async function start() { const result = await ViceMeMiniGame.initialize(); if (!result.ok) throw new Error(result.message); document.querySelector('#buy').onclick = async () => { const result = await ViceMeMiniGame.createPurchaseCard('hero-sword'); if (!result.ok) throw new Error(result.message); }; } start();`)
	miniGameWrite(t, project, "art.bin", "原游戏资源\x00不能覆盖")
}

func miniGameWrite(t *testing.T, project, name, content string) {
	t.Helper()
	filename := filepath.Join(project, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func miniGameRead(t *testing.T, project, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(project, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func miniGameArgs(mode, project string) []string {
	return []string{"mini-game", mode, "--project", project, "--work", miniGameFixture().WorkID, "--merchant-account", "22222222-2222-4222-8222-222222222222", "--environment", "sandbox"}
}

func miniGameServer(t *testing.T, body func() any) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/cli/merchant/works/"+miniGameFixture().WorkID+"/mini-game-integration" || r.URL.Query().Get("merchantAccountId") != "22222222-2222-4222-8222-222222222222" || r.URL.Query().Get("environment") != "SANDBOX" || r.Header.Get("Authorization") != "Bearer "+merchantEngagementToken {
			t.Errorf("错误请求: %s %s; credential present=%v", r.Method, r.URL.String(), r.Header.Get("Authorization") != "")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body())
	}))
	t.Cleanup(server.Close)
	return server
}

func miniGameReportFromOutput(t *testing.T, text string) miniGameReport {
	t.Helper()
	var envelope struct {
		OK    bool           `json:"ok"`
		Data  miniGameReport `json:"data"`
		Error struct {
			Details miniGameReport `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(text), &envelope); err != nil {
		t.Fatalf("输出不是单一 JSON: %v %s", err, text)
	}
	if envelope.OK {
		return envelope.Data
	}
	return envelope.Error.Details
}

func miniGameHasIssue(report miniGameReport, code, alias string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code && issue.Alias == alias && issue.Fix != "" {
			return true
		}
	}
	return false
}

func TestMiniGameIntegrateAndCheckLifecycle(t *testing.T) {
	project := t.TempDir()
	writeMiniGameHost(t, project)
	originalHTML, originalJS, originalArt := miniGameRead(t, project, "index.html"), miniGameRead(t, project, "game.js"), miniGameRead(t, project, "art.bin")
	manifest := miniGameFixture()
	server := miniGameServer(t, func() any { return manifest })
	exit, out := executeMerchantEngagementCommand(t, server, miniGameArgs("integrate", project))
	if exit != 0 {
		t.Fatal(out)
	}
	if strings.Contains(out, manifest.SharedSecret) || strings.Contains(out, "sharedSecret") {
		t.Fatal("接入输出泄露共享密钥")
	}
	report := miniGameReportFromOutput(t, out)
	if !report.Installed || !report.Ready {
		t.Fatalf("接入没有就绪: %s", out)
	}
	initial := map[string][]byte{}
	for _, name := range []string{miniGameRuntimePath, miniGameConfigPath, miniGameStatePath} {
		initial[name] = miniGameRead(t, project, name)
	}
	exit, out = executeMerchantEngagementCommand(t, server, []string{"mini-game", "integrate", "--project", project})
	if exit != 0 || len(miniGameReportFromOutput(t, out).UpdatedFiles) != 0 {
		t.Fatalf("重复接入不是幂等: %s", out)
	}
	exit, out = executeMerchantEngagementCommand(t, server, []string{"mini-game", "check", "--project", project})
	if exit != 0 || !miniGameReportFromOutput(t, out).Ready {
		t.Fatal(out)
	}
	if strings.Contains(out, manifest.SharedSecret) || strings.Contains(out, "sharedSecret") {
		t.Fatal("检查输出泄露共享密钥")
	}
	for name, before := range initial {
		if !bytes.Equal(before, miniGameRead(t, project, name)) {
			t.Errorf("重复接入/只读检查改写了 %s", name)
		}
	}
	manifest.Items[0].Title = "改名后的英雄之剑"
	manifest.Items[0].Status = "ARCHIVED"
	manifest.Items = append(manifest.Items, api.MiniGameItem{ID: "44444444-4444-4444-8444-444444444444", Alias: "magic-shield", Title: "新盾牌", Status: "ACTIVE"}, api.MiniGameItem{ID: "55555555-5555-4555-8555-555555555555", Alias: "retired-hat", Title: "已下架帽子", Status: "SUSPENDED"}, api.MiniGameItem{ID: "66666666-6666-4666-8666-666666666666", Alias: "draft-cloak", Title: "草稿披风", Status: "DRAFT"})
	exit, out = executeMerchantEngagementCommand(t, server, []string{"mini-game", "check", "--project", project})
	if exit == 0 || !miniGameHasIssue(miniGameReportFromOutput(t, out), "CONFIGURATION_STALE", "") {
		t.Fatalf("漏报过期配置: %s", out)
	}
	if strings.Contains(out, manifest.SharedSecret) || strings.Contains(out, "sharedSecret") {
		t.Fatal("检查失败输出泄露共享密钥")
	}
	if !bytes.Equal(initial[miniGameConfigPath], miniGameRead(t, project, miniGameConfigPath)) {
		t.Fatal("check 修改了旧配置")
	}
	exit, out = executeMerchantEngagementCommand(t, server, []string{"mini-game", "integrate", "--project", project})
	report = miniGameReportFromOutput(t, out)
	if exit != 0 || !miniGameHasIssue(report, "ACTIVE_ALIAS_MISSING", "magic-shield") || miniGameHasIssue(report, "ACTIVE_ALIAS_MISSING", "retired-hat") || miniGameHasIssue(report, "ACTIVE_ALIAS_MISSING", "draft-cloak") || miniGameHasIssue(report, "ACTIVE_ALIAS_MISSING", "hero-sword") {
		t.Fatalf("生命周期检查错误: %s", out)
	}
	if strings.Contains(out, manifest.SharedSecret) || strings.Contains(out, "sharedSecret") {
		t.Fatal("接入问题报告泄露共享密钥")
	}
	updated := string(miniGameRead(t, project, miniGameConfigPath))
	for _, required := range []string{"hero-sword", "改名后的英雄之剑", "retired-hat", "draft-cloak", "magic-shield"} {
		if !strings.Contains(updated, required) {
			t.Errorf("永久权益配置丢失 %s", required)
		}
	}
	for name, before := range map[string][]byte{"index.html": originalHTML, "game.js": originalJS, "art.bin": originalArt} {
		if !bytes.Equal(before, miniGameRead(t, project, name)) {
			t.Errorf("宿主文件被改写: %s", name)
		}
	}
	if strings.Contains(updated, merchantEngagementToken) || strings.Contains(updated, "merchantAccountId") || strings.Contains(updated, `"status"`) || strings.Contains(updated, `"schemaVersion"`) || strings.Contains(updated, `"price"`) {
		t.Fatal("配置泄露了非运行库字段")
	}
}

func TestMiniGameRequiresReferencesForRetainedEntitlements(t *testing.T) {
	project := t.TempDir()
	writeMiniGameHost(t, project)
	miniGameWrite(t, project, "game.js", `document.querySelector('#buy').hidden = true;`)
	manifest := miniGameFixture()
	manifest.Items[0].Status = "SUSPENDED"
	server := miniGameServer(t, func() any { return manifest })
	exit, out := executeMerchantEngagementCommand(t, server, miniGameArgs("integrate", project))
	report := miniGameReportFromOutput(t, out)
	if exit != 0 || report.Ready || !miniGameHasIssue(report, "ITEM_ALIAS_MISSING", "hero-sword") {
		t.Fatalf("下架道具失去本地权益入口却通过检查: %s", out)
	}
	miniGameWrite(t, project, "game.js", `document.querySelector('#buy').onclick = () => ViceMeMiniGame.createPurchaseCard('hero-sword');`)
	exit, out = executeMerchantEngagementCommand(t, server, []string{"mini-game", "check", "--project", project})
	if exit == 0 || !miniGameHasIssue(miniGameReportFromOutput(t, out), "ITEM_ALIAS_MISSING", "hero-sword") {
		t.Fatalf("下架后的无效购买按钮被当作权益恢复入口: %s", out)
	}
	miniGameWrite(t, project, "game.js", `async function restore() { await ViceMeMiniGame.initialize(); document.querySelector('#buy').onclick = () => ViceMeMiniGame.redeemCode('hero-sword', document.querySelector('#code').value); } restore();`)
	exit, out = executeMerchantEngagementCommand(t, server, []string{"mini-game", "check", "--project", project})
	if exit != 0 || !miniGameReportFromOutput(t, out).Ready {
		t.Fatalf("保留权益引用后仍要求下架道具购买入口: %s", out)
	}
}

func TestMiniGameRejectsInvalidManifestWithoutChangingFiles(t *testing.T) {
	cases := map[string]func(map[string]any){
		"重复别名": func(m map[string]any) {
			items := m["items"].([]any)
			duplicate := map[string]any{"id": "44444444-4444-4444-8444-444444444444", "alias": "hero-sword", "title": "重复", "status": "ACTIVE"}
			m["items"] = append(items, duplicate)
		},
		"重复道具ID": func(m map[string]any) {
			items := m["items"].([]any)
			m["items"] = append(items, map[string]any{"id": "33333333-3333-4333-8333-333333333333", "alias": "other-sword", "title": "重复", "status": "DRAFT"})
		},
		"缓存价格":      func(m map[string]any) { m["items"].([]any)[0].(map[string]any)["priceMinor"] = 100 },
		"未知顶层字段":    func(m map[string]any) { m["privateKey"] = "不能接受" },
		"不支持版本":     func(m map[string]any) { m["runtimeVersion"] = "1.0.0" },
		"旧协议版本":     func(m map[string]any) { m["schemaVersion"] = 1 },
		"错误环境":      func(m map[string]any) { m["environment"] = "PRODUCTION" },
		"错误作品":      func(m map[string]any) { m["workId"] = "99999999-9999-4999-8999-999999999999" },
		"缺失道具字段":    func(m map[string]any) { delete(m["items"].([]any)[0].(map[string]any), "status") },
		"旧公钥字段":     func(m map[string]any) { m["publicKey"] = strings.Repeat("A", 43) },
		"缺失密钥":      func(m map[string]any) { delete(m, "sharedSecret") },
		"大写密钥":      func(m map[string]any) { m["sharedSecret"] = strings.Repeat("A", 64) },
		"错误密钥长度":    func(m map[string]any) { m["sharedSecret"] = strings.Repeat("a", 63) },
		"非十六进制密钥":   func(m map[string]any) { m["sharedSecret"] = strings.Repeat("g", 64) },
		"尾斜杠Origin": func(m map[string]any) { m["checkoutOrigin"] = "https://viceme.cn/" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			writeMiniGameHost(t, project)
			body := any(miniGameFixture())
			server := miniGameServer(t, func() any { return body })
			exit, out := executeMerchantEngagementCommand(t, server, miniGameArgs("integrate", project))
			if exit != 0 {
				t.Fatal(out)
			}
			before := miniGameRead(t, project, miniGameConfigPath)
			state := miniGameRead(t, project, miniGameStatePath)
			encoded, _ := json.Marshal(body)
			var invalid map[string]any
			_ = json.Unmarshal(encoded, &invalid)
			mutate(invalid)
			body = invalid
			exit, out = executeMerchantEngagementCommand(t, server, []string{"mini-game", "integrate", "--project", project})
			if exit == 0 || !strings.Contains(out, "RESPONSE_INVALID") {
				t.Fatalf("接受了非法 manifest: %s", out)
			}
			if secret, ok := invalid["sharedSecret"].(string); ok && strings.Contains(out, secret) {
				t.Fatal("非法 manifest 错误输出泄露共享密钥")
			}
			if !bytes.Equal(before, miniGameRead(t, project, miniGameConfigPath)) || !bytes.Equal(state, miniGameRead(t, project, miniGameStatePath)) {
				t.Fatal("非法响应改写了已有安装")
			}
		})
	}
}

func TestMiniGameReferencesIgnoreTextAndReportRealCalls(t *testing.T) {
	project := t.TempDir()
	writeMiniGameHost(t, project)
	miniGameWrite(t, project, "game.js", "// ViceMeMiniGame.createPurchaseCard('hero-sword')\n/* ViceMeMiniGame.createPurchaseCard('hero-sword') */\nconst text = \"ViceMeMiniGame.createPurchaseCard('hero-sword')\";\nconst template = `ViceMeMiniGame.createPurchaseCard('hero-sword')`;\nconst pattern = /ViceMeMiniGame.createPurchaseCard('hero-sword')/;\n")
	miniGameWrite(t, project, "unused.js", `ViceMeMiniGame.createPurchaseCard('hero-sword')`)
	server := miniGameServer(t, func() any { return miniGameFixture() })
	exit, out := executeMerchantEngagementCommand(t, server, miniGameArgs("integrate", project))
	if exit != 0 || !miniGameHasIssue(miniGameReportFromOutput(t, out), "ACTIVE_ALIAS_MISSING", "hero-sword") {
		t.Fatalf("伪引用通过检查: %s", out)
	}
	miniGameWrite(t, project, "game.js", "const label = `${true ? `ViceMeMiniGame.createPurchaseCard('hero-sword')` : ''}`;\nconst probe = () => /ViceMeMiniGame.createPurchaseCard('hero-sword')/;\n")
	exit, out = executeMerchantEngagementCommand(t, server, []string{"mini-game", "check", "--project", project})
	if exit == 0 || !miniGameHasIssue(miniGameReportFromOutput(t, out), "ACTIVE_ALIAS_MISSING", "hero-sword") {
		t.Fatalf("嵌套模板或箭头函数的正则文本被当作真实购买调用: %s", out)
	}
	miniGameWrite(t, project, "game.js", `ViceMeMiniGame.createPurchaseCard(selectedAlias); ViceMeMiniGame.createPurchaseCard("not-an-item");`)
	exit, out = executeMerchantEngagementCommand(t, server, []string{"mini-game", "check", "--project", project})
	report := miniGameReportFromOutput(t, out)
	if exit == 0 || !miniGameHasIssue(report, "DYNAMIC_ALIAS", "") || !miniGameHasIssue(report, "UNKNOWN_ALIAS", "not-an-item") || miniGameHasIssue(report, "UNKNOWN_ALIAS", "selectedAlias") {
		t.Fatalf("动态或错误别名未正确诊断: %s", out)
	}
	miniGameWrite(t, project, "game.js", `import { buy } from './purchase.js'; document.querySelector('#buy').onclick = buy;`)
	miniGameWrite(t, project, "purchase.js", `export async function buy() { return ViceMeMiniGame.createPurchaseCard('hero-sword'); }`)
	exit, out = executeMerchantEngagementCommand(t, server, []string{"mini-game", "check", "--project", project})
	if exit != 0 || !miniGameReportFromOutput(t, out).Ready {
		t.Fatalf("真实导入调用未被识别: %s", out)
	}
	miniGameWrite(t, project, "index.html", `<!-- <script src="viceme/mini-game-commerce.js"></script> --><script src="viceme/mini-game-config.js"></script><script src="viceme/mini-game-config.js"></script><script src="game.js"></script>`)
	exit, out = executeMerchantEngagementCommand(t, server, []string{"mini-game", "check", "--project", project})
	if exit == 0 || !miniGameHasIssue(miniGameReportFromOutput(t, out), "HTML_SCRIPT_REFERENCES", "") {
		t.Fatalf("错误脚本顺序/重复被接受: %s", out)
	}
}

func TestMiniGameProtectsManagedChangesAndUnownedFiles(t *testing.T) {
	for _, installed := range []bool{false, true} {
		t.Run(map[bool]string{false: "未受管同名文件", true: "受管文件被手改"}[installed], func(t *testing.T) {
			project := t.TempDir()
			writeMiniGameHost(t, project)
			server := miniGameServer(t, func() any { return miniGameFixture() })
			if installed {
				exit, out := executeMerchantEngagementCommand(t, server, miniGameArgs("integrate", project))
				if exit != 0 {
					t.Fatal(out)
				}
			}
			miniGameWrite(t, project, miniGameConfigPath, "用户自定义配置，不可覆盖")
			exit, out := executeMerchantEngagementCommand(t, server, miniGameArgs("integrate", project))
			if exit == 0 || !strings.Contains(out, "MINI_GAME_MANAGED_FILE_MODIFIED") || string(miniGameRead(t, project, miniGameConfigPath)) != "用户自定义配置，不可覆盖" {
				t.Fatalf("未保护原文件: %s", out)
			}
		})
	}
}

func TestMiniGameProtectsWorkEnvironmentAndConcurrentWriter(t *testing.T) {
	project := t.TempDir()
	writeMiniGameHost(t, project)
	server := miniGameServer(t, func() any { return miniGameFixture() })
	exit, out := executeMerchantEngagementCommand(t, server, miniGameArgs("integrate", project))
	if exit != 0 {
		t.Fatal(out)
	}
	before := miniGameRead(t, project, miniGameConfigPath)
	for _, override := range [][]string{{"--environment", "production"}, {"--work", "99999999-9999-4999-8999-999999999999"}, {"--merchant-account", "99999999-9999-4999-8999-999999999999"}} {
		args := append([]string{"mini-game", "integrate", "--project", project}, override...)
		exit, out = executeMerchantEngagementCommand(t, server, args)
		if exit == 0 || !strings.Contains(out, "MINI_GAME_BINDING_CONFLICT") {
			t.Fatalf("错绑未拒绝: %s", out)
		}
	}
	lock := flock.New(filepath.Join(project, filepath.FromSlash(miniGameLockPath)))
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer lock.Unlock()
	for _, mode := range []string{"integrate", "check"} {
		exit, out = executeMerchantEngagementCommand(t, server, []string{"mini-game", mode, "--project", project})
		if exit == 0 || !strings.Contains(out, "MINI_GAME_BUSY") {
			t.Fatalf("并发未拒绝: %s", out)
		}
	}
	if !bytes.Equal(before, miniGameRead(t, project, miniGameConfigPath)) {
		t.Fatal("冲突调用改写配置")
	}
}

func TestMiniGameRejectsSymlinkAndCheckNeverCreatesProjectState(t *testing.T) {
	project := t.TempDir()
	writeMiniGameHost(t, project)
	server := miniGameServer(t, func() any { return miniGameFixture() })
	exit, out := executeMerchantEngagementCommand(t, server, miniGameArgs("check", project))
	if exit == 0 || !strings.Contains(out, "INSTALL_REQUIRED") {
		t.Fatalf("未安装项目误报成功: %s", out)
	}
	for _, name := range []string{".viceme", "viceme"} {
		if _, err := os.Lstat(filepath.Join(project, name)); !os.IsNotExist(err) {
			t.Fatalf("只读检查创建了 %s", name)
		}
	}
	outside := t.TempDir()
	miniGameWrite(t, outside, "mini-game-commerce.js", "项目外文件")
	if err := os.Symlink(outside, filepath.Join(project, "viceme")); err != nil {
		t.Skipf("当前系统无法创建符号链接: %v", err)
	}
	exit, out = executeMerchantEngagementCommand(t, server, miniGameArgs("integrate", project))
	if exit == 0 || !strings.Contains(out, "MINI_GAME_PATH_UNSAFE") {
		t.Fatalf("符号链接被接受: %s", out)
	}
	if string(miniGameRead(t, outside, "mini-game-commerce.js")) != "项目外文件" {
		t.Fatal("改写了项目外内容")
	}
}
