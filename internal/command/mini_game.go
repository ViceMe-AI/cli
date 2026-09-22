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
		command.Flags().StringVar(&selection.Environment, "environment", "", "sandbox 或 production；可从受管状态恢复")
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
	selection, err = resolveMiniGameSelection(selection, state)
	if err != nil {
		return err
	}
	manifest, err := runtime.client().GetMiniGameIntegration(command.Context(), selection.WorkID, selection.MerchantAccountID, selection.Environment)
	if err != nil {
		cliErr := output.AsError(err)
		if cliErr.Subtype == "RESPONSE_INVALID" {
			cliErr.Hint = "请更新 CLI 后重试；若仍失败，请检查创作者中心的作品、环境、道具 ID 和别名，不能手改配置绕过校验。"
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
		report.UpdatedFiles, err = installMiniGameFiles(project, state, files)
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
		report.Next = "静态接入检查通过；请实际验证付款卡和六位动态码兑换，再使用小红书上传页当前官方口令与 Skill 完成平台检查和打包。此命令不上传、托管、打包或发布。"
	}
	if mode == "check" && !report.Ready {
		return output.Validation("MINI_GAME_CHECK_FAILED", "小游戏仍有接入问题").WithHint(report.Next).WithDetails(report)
	}
	return runtime.success(report)
}

func resolveMiniGameSelection(selection miniGameSelection, state *miniGameState) (miniGameSelection, error) {
	selection.WorkID = strings.ToLower(selection.WorkID)
	selection.MerchantAccountID = strings.ToLower(selection.MerchantAccountID)
	if selection.Environment != "" && selection.Environment != "sandbox" && selection.Environment != "production" {
		return selection, output.Validation("MINI_GAME_ENVIRONMENT_INVALID", "--environment 必须是 sandbox 或 production").WithHint("用作品口令指定的环境重跑；不要混用测试与生产动态码。")
	}
	selection.Environment = strings.ToUpper(selection.Environment)
	if state != nil {
		if (selection.WorkID != "" && selection.WorkID != state.Selection.WorkID) || (selection.MerchantAccountID != "" && selection.MerchantAccountID != state.Selection.MerchantAccountID) || (selection.Environment != "" && selection.Environment != state.Selection.Environment) {
			return selection, output.Validation("MINI_GAME_BINDING_CONFLICT", "不能用另一作品、商家或环境覆盖现有接入").WithHint("省略选择参数继续原绑定；如需另一个作品或环境，请使用独立的未接入项目目录。")
		}
		selection = state.Selection
	}
	if !replicaUUIDPattern.MatchString(selection.WorkID) || !replicaUUIDPattern.MatchString(selection.MerchantAccountID) || (selection.Environment != "SANDBOX" && selection.Environment != "PRODUCTION") {
		return selection, output.Validation("MINI_GAME_SELECTION_REQUIRED", "首次接入需要有效的作品、商家与环境").WithHint("使用 viceme mini-game integrate --work <作品UUID> --merchant-account <资格守卫返回的商家UUID> --environment sandbox|production --project <路径>。")
	}
	return selection, nil
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
	return []byte(fmt.Sprintf("// ViceMe 受管文件：请通过 viceme mini-game integrate 更新，不要手改。\n(function () {\n  \"use strict\";\n  if (window.ViceMeMiniGame) throw new Error(\"ViceMeMiniGame 已初始化，请删除重复的配置脚本引用\");\n  var configuration = %s;\n  window.ViceMeMiniGame = ViceMeMiniGameCommerce.createMiniGameRuntime(configuration, ViceMeMiniGameCommerce.createXiaohongshuPlatform(window));\n})();\n", data)), nil
}
