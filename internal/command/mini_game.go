package command

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

const miniGameRuntimePath = "viceme/mini-game-commerce.js"
const miniGameConfigPath = "viceme/mini-game-config.js"
const miniGameGuideURL = "https://miniapp-sandbox.xiaohongshu.com/minitool/doc"

type miniGameSelection struct {
	WorkID            string `json:"workId"`
	MerchantAccountID string `json:"merchantAccountId"`
	Environment       string `json:"environment"`
}

type miniGameIssue struct {
	Code  string `json:"code"`
	File  string `json:"file,omitempty"`
	Alias string `json:"alias,omitempty"`
	Line  int    `json:"line,omitempty"`
	Fix   string `json:"fix"`
}

type miniGameReport struct {
	miniGameSelection
	RuntimeVersion   string             `json:"runtimeVersion"`
	Installed        bool               `json:"installed"`
	Ready            bool               `json:"ready"`
	UpdatedFiles     []string           `json:"updatedFiles"`
	Items            []api.MiniGameItem `json:"items"`
	Issues           []miniGameIssue    `json:"issues"`
	Next             string             `json:"next"`
	PlatformGuideURL string             `json:"platformGuideUrl"`
}

func newMiniGameCommand(runtime *Runtime) *cobra.Command {
	root := &cobra.Command{Use: "mini-game", Short: "在本地小游戏中接入或检查 ViceMe 离线购买与兑换"}
	for _, mode := range []string{"integrate", "check"} {
		var selection miniGameSelection
		var project string
		command := &cobra.Command{Use: mode, Args: cobra.NoArgs, Short: map[string]string{"integrate": "安装或增量更新一份作品运行库和配置", "check": "只读检查作品配置、入口脚本和道具引用"}[mode]}
		command.RunE = func(command *cobra.Command, _ []string) error {
			return runMiniGame(command, runtime, mode, project, selection)
		}
		command.Flags().StringVar(&selection.WorkID, "work", "", "已发布小游戏作品 UUID；可从受管状态恢复")
		command.Flags().StringVar(&selection.MerchantAccountID, "merchant-account", "", "资格守卫返回的商家 UUID；可从受管状态恢复")
		// Kept only so previously copied commands keep working; production is the sole environment.
		command.Flags().StringVar(&selection.Environment, "environment", "", "仅支持 production（默认）；小游戏已无沙箱")
		command.Flags().StringVar(&project, "project", ".", "已有本地小游戏项目目录")
		root.AddCommand(command)
	}
	return root
}

func runMiniGame(command *cobra.Command, runtime *Runtime, mode, project string, selection miniGameSelection) error {
	project, err := miniGameProject(project)
	if err != nil {
		return err
	}
	lock, err := lockMiniGameProject(project, mode == "integrate")
	if err != nil {
		return err
	}
	if lock != nil {
		defer lock.Unlock()
	}
	state, err := loadMiniGameState(project)
	if err != nil {
		return err
	}
	if state != nil && (state.EndpointOrigin != config.APIStateBaseURL(runtime.apiBaseURL) || state.ProjectPath != project) {
		return output.Validation("MINI_GAME_BINDING_CONFLICT", "受管状态绑定了不同的项目或 API 环境").WithHint("使用原项目路径和原 ViceMe profile；不要删除状态后覆盖已有文件。需要独立环境时复制未接入的项目。")
	}
	selection, legacySandbox, err := resolveMiniGameSelection(selection, state)
	if err != nil {
		return err
	}
	if legacySandbox {
		if mode == "check" {
			return output.Validation("MINI_GAME_SANDBOX_REMOVED", "小游戏已无沙箱，此项目仍绑定沙箱环境").WithHint("运行 viceme mini-game integrate --project <原项目路径> 切换到正式环境；作品与商家绑定保持不变。")
		}
		// Same Work and merchant; only the removed sandbox binding is replaced.
		state.Selection = selection
	}
	manifest, err := runtime.client().GetMiniGameIntegration(command.Context(), selection.WorkID, selection.MerchantAccountID)
	if err != nil {
		cliErr := output.AsError(err)
		if cliErr.Subtype == "RESPONSE_INVALID" {
			cliErr.Hint = "请更新 CLI 后重试；若仍失败，请检查创作者中心的作品、道具 ID 和别名，不能手改配置绕过校验。"
		}
		return err
	}
	runtimeBytes, _, err := runtime.deps.Skills.Read("viceme-mini-game-commerce", "templates/mini-game-commerce.js")
	if err != nil || len(runtimeBytes) == 0 {
		return output.Internal("MINI_GAME_RUNTIME_MISSING", "当前 CLI 缺少官方小游戏运行库", err).WithHint("执行 viceme update 安装修复版本，再重跑同一接入口令；不要下载其他来源的运行库。")
	}
	configBytes, err := miniGameConfiguration(manifest)
	if err != nil {
		return err
	}
	files := map[string][]byte{miniGameRuntimePath: runtimeBytes, miniGameConfigPath: configBytes}
	if err := validateMiniGameManagedFiles(project, state); err != nil {
		return err
	}
	report := miniGameReport{miniGameSelection: selection, RuntimeVersion: manifest.RuntimeVersion, Items: manifest.Items, Installed: state != nil && state.Pending == nil, UpdatedFiles: []string{}, Issues: []miniGameIssue{}, PlatformGuideURL: miniGameGuideURL}
	if mode == "integrate" {
		if state == nil {
			state = &miniGameState{SchemaVersion: 1, EndpointOrigin: config.APIStateBaseURL(runtime.apiBaseURL), ProjectPath: project, Selection: selection, Files: map[string]string{}}
		}
		report.UpdatedFiles, err = installMiniGameFiles(project, state, files, legacySandbox)
		if err != nil {
			return err
		}
		report.Installed = true
	} else {
		if state == nil || state.Pending != nil {
			report.Issues = append(report.Issues, miniGameIssue{Code: "INSTALL_REQUIRED", Fix: "运行同一选择参数的 viceme mini-game integrate，完成运行库与配置安装。"})
		} else {
			for _, name := range []string{miniGameRuntimePath, miniGameConfigPath} {
				if state.Files[name] != miniGameDigest(files[name]) {
					report.Issues = append(report.Issues, miniGameIssue{Code: "CONFIGURATION_STALE", File: name, Fix: "运行 viceme mini-game integrate --project <原项目路径> 增量更新当前作品配置；保留停用道具以恢复永久权益。"})
				}
			}
		}
	}
	issues, err := checkMiniGameReferences(project, manifest)
	if err != nil {
		return err
	}
	report.Issues = append(report.Issues, issues...)
	report.Ready = len(report.Issues) == 0
	report.Next = "按 issues 修复原项目入口和付费点，然后运行 viceme mini-game check；命令不会改写宿主代码。"
	if report.Ready {
		report.Next = "静态接入检查通过；请在原游戏中实际验证付款卡、六位动态码兑换与刷新恢复，普通浏览器可直接验收。仅在发布小红书时使用上传页当前官方口令与 Skill 完成平台检查和打包。此命令不上传、托管、打包或发布。"
	}
	if mode == "check" && !report.Ready {
		return output.Validation("MINI_GAME_CHECK_FAILED", "小游戏仍有接入问题").WithHint(report.Next).WithDetails(report)
	}
	return runtime.success(report)
}

// resolveMiniGameSelection returns the production selection and whether the
// stored state still carries the removed SANDBOX environment for the same Work.
func resolveMiniGameSelection(selection miniGameSelection, state *miniGameState) (miniGameSelection, bool, error) {
	selection.WorkID = strings.ToLower(selection.WorkID)
	selection.MerchantAccountID = strings.ToLower(selection.MerchantAccountID)
	switch selection.Environment {
	case "", "production":
	case "sandbox":
		return selection, false, output.Validation("MINI_GAME_SANDBOX_REMOVED", "小游戏已无沙箱，接入即正式版").WithHint("去掉 --environment 或使用 --environment production 重跑；验证兑换需要一笔真实付款。")
	default:
		return selection, false, output.Validation("MINI_GAME_ENVIRONMENT_INVALID", "--environment 只支持 production").WithHint("去掉 --environment 重跑；小游戏只有正式环境。")
	}
	selection.Environment = api.MiniGameEnvironment
	legacySandbox := false
	if state != nil {
		if (selection.WorkID != "" && selection.WorkID != state.Selection.WorkID) || (selection.MerchantAccountID != "" && selection.MerchantAccountID != state.Selection.MerchantAccountID) {
			return selection, false, output.Validation("MINI_GAME_BINDING_CONFLICT", "不能用另一作品或商家覆盖现有接入").WithHint("省略选择参数继续原绑定；如需另一个作品，请使用独立的未接入项目目录。")
		}
		switch state.Selection.Environment {
		case api.MiniGameEnvironment:
		case "SANDBOX":
			legacySandbox = true
		default:
			return selection, false, output.Validation("MINI_GAME_STATE_INVALID", "小游戏受管状态包含未知环境").WithHint("从版本控制或备份恢复 .viceme/mini-game.v1.json 及配套两份受管文件；不要手工猜写哈希。")
		}
		selection = miniGameSelection{WorkID: state.Selection.WorkID, MerchantAccountID: state.Selection.MerchantAccountID, Environment: api.MiniGameEnvironment}
	}
	if !replicaUUIDPattern.MatchString(selection.WorkID) || !replicaUUIDPattern.MatchString(selection.MerchantAccountID) {
		return selection, false, output.Validation("MINI_GAME_SELECTION_REQUIRED", "首次接入需要有效的作品与商家").WithHint("使用 viceme mini-game integrate --work <作品UUID> --merchant-account <资格守卫返回的商家UUID> --project <路径>。")
	}
	return selection, legacySandbox, nil
}

func miniGameConfiguration(manifest api.MiniGameIntegration) ([]byte, error) {
	// 按运行库的 exactKeys 合同投影：不透传 schemaVersion、runtimeVersion 或道具 status。
	type runtimeItem struct {
		ID    string `json:"id"`
		Alias string `json:"alias"`
		Title string `json:"title"`
	}
	items := make([]runtimeItem, 0, len(manifest.Items))
	for _, item := range manifest.Items {
		items = append(items, runtimeItem{item.ID, item.Alias, item.Title})
	}
	configuration := struct {
		ProtocolVersion int           `json:"protocolVersion"`
		WorkID          string        `json:"workId"`
		WorkTitle       string        `json:"workTitle"`
		Environment     string        `json:"environment"`
		PublicClientID  string        `json:"publicClientId"`
		SharedSecret    string        `json:"sharedSecret"`
		CheckoutOrigin  string        `json:"checkoutOrigin"`
		Items           []runtimeItem `json:"items"`
	}{2, manifest.WorkID, manifest.WorkTitle, manifest.Environment, manifest.PublicClientID, manifest.SharedSecret, manifest.CheckoutOrigin, items}
	data, err := json.MarshalIndent(configuration, "  ", "  ")
	if err != nil {
		return nil, output.Internal("MINI_GAME_CONFIG_INVALID", "无法生成小游戏配置", err)
	}
	return []byte(fmt.Sprintf("// ViceMe 受管文件：请通过 viceme mini-game integrate 更新，不要手改。\n(function () {\n  \"use strict\";\n  if (window.ViceMeMiniGame) throw new Error(\"ViceMeMiniGame 已初始化，请删除重复的配置脚本引用\");\n  var configuration = %s;\n  window.ViceMeMiniGame = ViceMeMiniGameCommerce.createMiniGameRuntime(configuration, ViceMeMiniGameCommerce.createMiniGamePlatform(window));\n})();\n", data)), nil
}
