package command

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

var withdrawalRecipientRevision = regexp.MustCompile(`^[a-f0-9]{64}$`)
var withdrawalReference = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)
var withdrawalAmount = regexp.MustCompile(`^[0-9]{1,9}(\.[0-9]{1,2})?$`)

type withdrawalOptions struct{ requestID, amount, methodID string }

func newWithdrawalCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "withdraw", Short: "自主提取创作者收入并查询进度"}
	command.AddCommand(&cobra.Command{
		Use: "context", Short: "查询可提现收入、可用账户和首次办理入口", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			client, _, err := runtime.withdrawalClient(command.Context(), false)
			if err != nil {
				return err
			}
			current, err := client.GetWithdrawalContext(command.Context())
			if err != nil {
				return err
			}
			return runtime.business(current)
		},
	})
	var options withdrawalOptions
	create := &cobra.Command{
		Use: "create", Short: "发起或恢复同一笔收入提现", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runtime.createWithdrawal(command.Context(), options)
		},
	}
	create.Flags().StringVar(&options.requestID, "request-id", "", "本次任务的稳定请求标识，重试时复用（必填）")
	create.Flags().StringVar(&options.amount, "amount", "", "人民币金额（元，最多两位小数）；省略时提取允许的最大金额")
	create.Flags().StringVar(&options.methodID, "payout-method", "", "指定收款账户 ID；省略时采用服务端推荐账户")
	var requestID string
	status := &cobra.Command{
		Use: "status", Short: "刷新并查询原提现单，不创建新单", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if !withdrawalReference.MatchString(requestID) {
				return invalidWithdrawalReference()
			}
			client, _, err := runtime.withdrawalClient(command.Context(), false)
			if err != nil {
				return err
			}
			payout, err := client.FindWithdrawal(command.Context(), requestID, true)
			if err != nil {
				return err
			}
			if payout == nil {
				return output.Validation("WITHDRAWAL_NOT_FOUND", "尚未找到本次请求的提现单，请使用原 create 命令继续")
			}
			return runtime.reportWithdrawal(command.Context(), client, *payout)
		},
	}
	status.Flags().StringVar(&requestID, "request-id", "", "原提现任务的稳定请求标识（必填）")
	command.AddCommand(create, status)
	return command
}

func invalidWithdrawalReference() error {
	return output.Validation("WITHDRAWAL_REQUEST_ID_REQUIRED", "--request-id 必须是 1 到 64 位字母、数字或 . _ : -；同一任务重试必须复用")
}

func parseWithdrawalAmount(value string) (int64, error) {
	if !withdrawalAmount.MatchString(value) {
		return 0, output.Validation("WITHDRAWAL_AMOUNT_INVALID", "--amount 必须是正数人民币金额，最多两位小数")
	}
	parts := strings.SplitN(value, ".", 2)
	yuan, _ := strconv.ParseInt(parts[0], 10, 64)
	var cents int64
	if len(parts) == 2 {
		cents, _ = strconv.ParseInt((parts[1] + "0")[:2], 10, 64)
	}
	amount := yuan*100 + cents
	if amount <= 0 {
		return 0, output.Validation("WITHDRAWAL_AMOUNT_INVALID", "提现金额必须大于零")
	}
	return amount, nil
}

func (runtime *Runtime) createWithdrawal(ctx context.Context, options withdrawalOptions) error {
	if !withdrawalReference.MatchString(options.requestID) {
		return invalidWithdrawalReference()
	}
	var requested int64
	var err error
	if options.amount != "" {
		requested, err = parseWithdrawalAmount(options.amount)
		if err != nil {
			return err
		}
	}
	client, userID, err := runtime.withdrawalClient(ctx, true)
	if err != nil {
		return err
	}
	unlock, err := runtime.lockCommerceIntent(ctx, "withdrawal", userID, options.requestID, nil)
	if err != nil {
		return err
	}
	defer unlock()
	intent, err := runtime.loadWithdrawalIntent(userID, options.requestID)
	if err != nil {
		return err
	}
	if intent != nil && (intent.RequestedAmount != options.amount || intent.RequestedMethodID != options.methodID) {
		return output.Validation("WITHDRAWAL_INTENT_CONFLICT", "该请求已绑定另一组提现参数，请恢复原命令；新提现使用新请求标识")
	}
	if intent != nil {
		return runtime.submitWithdrawal(ctx, client, intent.Request)
	}
	// 查询先于重新选额。另一进程或另一设备已提交相同请求时，服务端原单优先。
	existing, err := client.FindWithdrawal(ctx, options.requestID, false)
	if err != nil {
		return err
	}
	if existing != nil {
		if requested != 0 && requested != existing.Amount {
			return output.Validation("WITHDRAWAL_INTENT_CONFLICT", "该请求标识已绑定不同金额")
		}
		if options.methodID != "" {
			return output.Validation("WITHDRAWAL_INTENT_RECOVERY_REQUIRED", "原请求已存在，请使用 withdraw status 查询原单，或在原设备恢复")
		}
		return runtime.reportWithdrawal(ctx, client, *existing)
	}
	current, err := client.GetWithdrawalContext(ctx)
	if err != nil {
		return err
	}
	if requested == 0 && current.WithdrawableBalanceCents == 0 {
		return runtime.business(map[string]any{"outcome": "NO_FUNDS", "sourceReference": options.requestID, "remainingAmount": 0})
	}
	if requested > current.WithdrawableBalanceCents {
		return output.Validation("WITHDRAWAL_INSUFFICIENT_FUNDS", "可提现收入不足")
	}
	if !current.Capabilities.IdentityReady {
		return runtime.business(map[string]any{"outcome": "ACTION_REQUIRED", "sourceReference": options.requestID, "action": "COMPLETE_SETUP", "url": current.SetupURL})
	}
	input := api.WithdrawalRequest{Provider: current.Capabilities.Provider, SourceReference: options.requestID, AgreementVersion: current.Capabilities.AgreementVersion}
	minimum, maximum := current.Capabilities.MinAmount, current.Capabilities.MaxAmount
	if current.RecipientRevision != nil {
		input.RecipientRevision = *current.RecipientRevision
	}
	if current.Capabilities.RequiresPayoutAccount {
		methodID := options.methodID
		if methodID == "" && current.RecommendedPayoutMethodID != nil {
			methodID = *current.RecommendedPayoutMethodID
		}
		var selected *api.WithdrawalMethod
		for i := range current.PayoutMethods {
			if current.PayoutMethods[i].ID == methodID {
				selected = &current.PayoutMethods[i]
				break
			}
		}
		if selected == nil {
			if options.methodID != "" {
				return output.Validation("WITHDRAWAL_ACCOUNT_UNAVAILABLE", "指定收款账户不可用")
			}
			return runtime.business(map[string]any{"outcome": "ACTION_REQUIRED", "sourceReference": options.requestID, "action": "COMPLETE_SETUP", "url": current.SetupURL})
		}
		input.PayoutMethodID = selected.ID
		input.RecipientRevision = selected.RecipientRevision
		minimum, maximum = selected.MinAmount, selected.MaxAmount
	} else if options.methodID != "" {
		return output.Validation("WITHDRAWAL_ACCOUNT_UNAVAILABLE", "当前渠道使用服务端绑定身份，不接受指定收款账户")
	}
	if current.Capabilities.RemainingDailyAmount != nil {
		maximum = min(maximum, *current.Capabilities.RemainingDailyAmount)
	}
	if !withdrawalRecipientRevision.MatchString(input.RecipientRevision) || minimum <= 0 || current.Capabilities.MaxAmount <= 0 || current.Capabilities.AgreementVersion == "" || current.WithdrawableBalanceCents < 0 {
		return output.Internal("WITHDRAWAL_CONTEXT_INVALID", "服务端提现能力响应不完整", nil)
	}
	input.Amount = requested
	if requested == 0 {
		input.Amount = min(current.WithdrawableBalanceCents, maximum)
	}
	if input.Amount < minimum || input.Amount > maximum {
		return output.Validation("WITHDRAWAL_AMOUNT_OUT_OF_RANGE", "金额不满足当前渠道或当日额度限制").WithDetails(map[string]any{"minAmount": minimum, "maxAmount": maximum})
	}
	if input.Amount > current.WithdrawableBalanceCents {
		return output.Validation("WITHDRAWAL_INSUFFICIENT_FUNDS", "可提现收入不足")
	}
	intent = &withdrawalIntent{RequestedAmount: options.amount, RequestedMethodID: options.methodID, Request: input}
	if err := runtime.saveWithdrawalIntent(userID, options.requestID, *intent); err != nil {
		return err
	}
	return runtime.submitWithdrawal(ctx, client, input)
}

func (runtime *Runtime) submitWithdrawal(ctx context.Context, client *api.Client, input api.WithdrawalRequest) error {
	payout, err := client.CreateWithdrawal(ctx, input)
	if err != nil {
		return output.AsError(err).WithHint("保留原 --request-id；先运行 withdraw status 查询，未找到时重试原 create 命令").WithDetails(map[string]any{"sourceReference": input.SourceReference})
	}
	if payout.SourceReference != input.SourceReference || payout.Amount != input.Amount || payout.Provider != input.Provider {
		return output.Internal("WITHDRAWAL_RESPONSE_MISMATCH", "提现响应与原请求不一致，请按原请求标识查询", nil)
	}
	return runtime.reportWithdrawal(ctx, client, payout)
}

func (runtime *Runtime) reportWithdrawal(ctx context.Context, client *api.Client, payout api.WithdrawalPayout) error {
	outcome := "PENDING"
	switch payout.Status {
	case "SUCCEEDED":
		outcome = "SUCCEEDED"
	case "FAILED", "RETURNED", "CANCELED", "INVALIDATED":
		outcome = "FAILED"
	case "PARTIALLY_SUCCEEDED":
		outcome = "PARTIALLY_SUCCEEDED"
	case "WAIT_USER_CONFIRM":
		outcome = "ACTION_REQUIRED"
	case "SUBMITTING", "ACCEPTED", "PROCESSING", "HELD":
	default:
		return output.Internal("WITHDRAWAL_STATUS_INVALID", "服务端返回未知提现状态", nil)
	}
	if payout.ActionRequired != nil {
		outcome = "ACTION_REQUIRED"
	}
	result := map[string]any{"outcome": outcome, "payout": payout, "sourceReference": payout.SourceReference, "remainingAmount": nil}
	if outcome == "PENDING" || outcome == "ACTION_REQUIRED" {
		result["nextPollAfterSeconds"] = 5
		result["nextCommand"] = "viceme withdraw status --request-id " + payout.SourceReference
	}
	setupURL, _ := url.JoinPath(runtime.profile.ResolvedWebBaseURL(), "me/creator-center/wallet/withdraw")
	if current, err := client.GetWithdrawalContext(ctx); err == nil {
		result["remainingAmount"] = current.WithdrawableBalanceCents
		setupURL = current.SetupURL
	}
	if outcome == "ACTION_REQUIRED" {
		payoutURL, err := url.JoinPath(setupURL, "payouts", payout.ID)
		if err != nil {
			return output.Internal("WITHDRAWAL_ACTION_URL_INVALID", "无法生成原提现单的操作入口，请按请求标识查询", nil)
		}
		if payout.ActionRequired != nil {
			payoutURL += "?itemId=" + url.QueryEscape(payout.ActionRequired.ItemID)
		}
		result["actionUrl"] = payoutURL
	}
	return runtime.business(result)
}

func (runtime *Runtime) withdrawalClient(ctx context.Context, write bool) (*api.Client, string, error) {
	market, err := runtime.profile.ResolvedMarketRegion()
	if err != nil || market != config.RegionCN {
		return nil, "", output.Policy("WITHDRAWAL_CN_ONLY", "创作者提现仅支持国内站")
	}
	client := runtime.client()
	token, err := client.Tokens.Token(ctx)
	if err != nil {
		return nil, "", err
	}
	client.Tokens = processTokenSource(token)
	status, err := client.AuthStatus(ctx)
	if err != nil {
		return nil, "", err
	}
	granted := make(map[string]bool)
	for _, scope := range status.Scopes {
		granted[scope] = true
	}
	if !granted["withdrawal:read"] || (write && !granted["withdrawal:write"]) {
		return nil, "", output.Authorization("WITHDRAWAL_SCOPE_REQUIRED", "请重新登录以启用提现能力").WithHint("运行 viceme auth login 后继续原提现任务")
	}
	if status.User.ID == "" {
		return nil, "", output.Authentication("WITHDRAWAL_IDENTITY_REQUIRED", "登录身份缺失")
	}
	return client, status.User.ID, nil
}
