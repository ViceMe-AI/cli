package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestWithdrawalEmptyBalanceDoesNotCreatePayout(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
	creates := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/cli/auth/status":
			writeJSONResponse(w, map[string]any{"authenticated": true, "user": map[string]any{"id": "owner"}, "scopes": []string{"withdrawal:read", "withdrawal:write"}, "expiresAt": time.Now().Add(time.Hour).Format(time.RFC3339)})
		case "/v1/cli/withdrawals/by-source-reference":
			writeJSONResponse(w, map[string]any{"payout": nil})
		case "/v1/cli/withdrawals/context":
			writeJSONResponse(w, map[string]any{
				"capabilities":             map[string]any{"provider": "YUNZHANGHU", "identityReady": true, "requiresPayoutAccount": true, "minAmount": 1, "maxAmount": 100000000, "agreementVersion": "2026-08-05"},
				"withdrawableBalanceCents": 0, "payoutMethods": []any{}, "setupUrl": "https://viceme.cn/me/creator-center/wallet/withdraw",
			})
		case "/v1/cli/withdrawals":
			creates++
			w.WriteHeader(500)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	exit := Execute([]string{"withdraw", "create", "--request-id", "empty-income"}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	var result struct {
		Data struct {
			Outcome string `json:"outcome"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || exit != 0 || result.Data.Outcome != "NO_FUNDS" || creates != 0 {
		t.Fatalf("empty income must not create a payout: exit=%d creates=%d output=%s", exit, creates, stdout.String())
	}
}

func TestWithdrawalRecoversOriginalAmountAfterLostResponse(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
	var amounts []int64
	contexts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/cli/auth/status":
			writeJSONResponse(w, map[string]any{"authenticated": true, "user": map[string]any{"id": "owner"}, "scopes": []string{"withdrawal:read", "withdrawal:write"}})
		case "/v1/cli/withdrawals/by-source-reference":
			writeJSONResponse(w, map[string]any{"payout": nil})
		case "/v1/cli/withdrawals/context":
			contexts++
			balance := 2500
			revision := strings.Repeat("a", 64)
			if contexts > 1 {
				balance = 9999
				revision = strings.Repeat("b", 64)
			}
			writeJSONResponse(w, map[string]any{
				"capabilities":              map[string]any{"provider": "YUNZHANGHU", "identityReady": true, "requiresPayoutAccount": true, "minAmount": 1, "maxAmount": 100000000, "agreementVersion": "2026-08-05"},
				"withdrawableBalanceCents":  balance,
				"payoutMethods":             []any{map[string]any{"id": "account", "channel": "BANK", "account": "****1234", "recipientRevision": revision, "minAmount": 1, "maxAmount": 100000000}},
				"recommendedPayoutMethodId": "account", "setupUrl": "https://viceme.cn/me/creator-center/wallet/withdraw",
			})
		case "/v1/cli/withdrawals":
			var body struct {
				Amount            int64  `json:"amount"`
				RecipientRevision string `json:"recipientRevision"`
				SourceReference   string `json:"sourceReference"`
				PayoutMethodID    string `json:"payoutMethodId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			amounts = append(amounts, body.Amount)
			if body.SourceReference != "recover-income" || body.PayoutMethodID != "account" || body.RecipientRevision != strings.Repeat("a", 64) {
				t.Errorf("wrong payout intent: %+v", body)
			}
			if len(amounts) == 1 {
				w.WriteHeader(503)
				writeJSONResponse(w, map[string]any{"code": "SETTLEMENT_PROVIDER_UNAVAILABLE", "message": "result unknown"})
				return
			}
			writeJSONResponse(w, map[string]any{"id": "payout", "sourceReference": body.SourceReference, "orderId": "VM1", "provider": "YUNZHANGHU", "amount": body.Amount, "status": "PROCESSING", "pendingAmount": body.Amount})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	deps := Dependencies{Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")}}
	args := []string{"withdraw", "create", "--request-id", "recover-income"}
	if exit := Execute(args, deps); exit == 0 {
		t.Fatal("an unknown provider outcome must not report success")
	}
	stdout.Reset()
	if exit := Execute(args, deps); exit != 0 {
		t.Fatalf("resume failed: %s", stdout.String())
	}
	if len(amounts) != 2 || amounts[0] != 2500 || amounts[1] != 2500 {
		t.Fatalf("recovery changed withdrawal amount: %v", amounts)
	}
	var result struct {
		Data struct {
			Outcome string `json:"outcome"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Data.Outcome != "PENDING" {
		t.Fatalf("processing was not returned as pending: %s", stdout.String())
	}
}

func TestWithdrawalAmountAccountAndSetupBoundaries(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
	for _, tc := range []struct {
		name           string
		amount         string
		method         string
		ready          bool
		balance        int64
		maximum        int64
		expectedAmount int64
		outcome        string
		code           string
	}{
		{name: "默认金额不超过渠道上限", ready: true, balance: 9000, maximum: 2000, expectedAmount: 2000, outcome: "PENDING"},
		{name: "指定金额精确到分", ready: true, balance: 9000, maximum: 10000, amount: "12.34", expectedAmount: 1234, outcome: "PENDING"},
		{name: "指定金额不能缩减", ready: true, balance: 9000, maximum: 2000, amount: "30", code: "WITHDRAWAL_AMOUNT_OUT_OF_RANGE"},
		{name: "指定金额不能使用充值资金", ready: true, balance: 500, maximum: 10000, amount: "10", code: "WITHDRAWAL_INSUFFICIENT_FUNDS"},
		{name: "指定账户不可用时不替换", ready: true, balance: 9000, maximum: 10000, method: "missing", code: "WITHDRAWAL_ACCOUNT_UNAVAILABLE"},
		{name: "首次签约返回办理入口", ready: false, balance: 9000, maximum: 10000, outcome: "ACTION_REQUIRED"},
		{name: "指定金额余额不足时不先要求签约", ready: false, balance: 0, maximum: 10000, amount: "10", code: "WITHDRAWAL_INSUFFICIENT_FUNDS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			creates := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/cli/auth/status":
					writeJSONResponse(w, map[string]any{"authenticated": true, "user": map[string]any{"id": "owner"}, "scopes": []string{"withdrawal:read", "withdrawal:write"}})
				case "/v1/cli/withdrawals/by-source-reference":
					writeJSONResponse(w, map[string]any{"payout": nil})
				case "/v1/cli/withdrawals/context":
					writeJSONResponse(w, map[string]any{
						"capabilities":             map[string]any{"provider": "YUNZHANGHU", "identityReady": tc.ready, "requiresPayoutAccount": true, "minAmount": 1, "maxAmount": 100000000, "agreementVersion": "2026-08-05"},
						"withdrawableBalanceCents": tc.balance, "payoutMethods": []any{map[string]any{"id": "chosen", "channel": "BANK", "account": "****1234", "recipientRevision": strings.Repeat("a", 64), "minAmount": 1, "maxAmount": tc.maximum}}, "recommendedPayoutMethodId": "chosen", "setupUrl": "https://viceme.cn/me/creator-center/wallet/withdraw",
					})
				case "/v1/cli/withdrawals":
					creates++
					var input map[string]any
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						t.Error(err)
					}
					if input["amount"] != float64(tc.expectedAmount) || input["payoutMethodId"] != "chosen" {
						t.Errorf("unexpected request: %v", input)
					}
					writeJSONResponse(w, map[string]any{"id": "payout", "orderId": "VM1", "sourceReference": "case", "provider": "YUNZHANGHU", "amount": input["amount"], "status": "PROCESSING", "pendingAmount": input["amount"]})
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			root := t.TempDir()
			var stdout, stderr bytes.Buffer
			args := []string{"withdraw", "create", "--request-id", "case"}
			if tc.amount != "" {
				args = append(args, "--amount", tc.amount)
			}
			if tc.method != "" {
				args = append(args, "--payout-method", tc.method)
			}
			exit := Execute(args, Dependencies{Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), APIBaseURL: server.URL, Region: config.RegionCN, Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")}})
			var result struct {
				Data struct {
					Outcome string `json:"outcome"`
					URL     string `json:"url"`
				} `json:"data"`
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Data.Outcome != tc.outcome || result.Error.Code != tc.code {
				t.Fatalf("wrong result: exit=%d output=%s", exit, stdout.String())
			}
			if tc.outcome == "PENDING" {
				if creates != 1 {
					t.Fatalf("expected one payout, got %d", creates)
				}
			} else if creates != 0 {
				t.Fatalf("unexpected payout: %d", creates)
			}
		})
	}
}

func TestWithdrawalWechatConfirmationAndTerminalOutcomes(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
	for _, tc := range []struct{ status, outcome string }{
		{"WAIT_USER_CONFIRM", "ACTION_REQUIRED"}, {"SUCCEEDED", "SUCCEEDED"},
		{"PARTIALLY_SUCCEEDED", "PARTIALLY_SUCCEEDED"}, {"FAILED", "FAILED"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/cli/auth/status":
					writeJSONResponse(w, map[string]any{"user": map[string]any{"id": "owner"}, "scopes": []string{"withdrawal:read"}})
				case "/v1/cli/withdrawals/by-source-reference/refresh":
					if r.Method != http.MethodPost || r.URL.Query().Get("sourceReference") != "wechat-income" {
						t.Errorf("wrong refresh: %s", r.URL)
					}
					payout := map[string]any{"id": "payout-id", "sourceReference": "wechat-income", "provider": "WECHAT_PAY", "amount": 2500, "status": tc.status, "recipientAccount": "微信用户", "succeededAmount": 1000, "releasedAmount": 1500, "pendingAmount": 0}
					if tc.status == "WAIT_USER_CONFIRM" {
						payout["actionRequired"] = map[string]any{"kind": "WECHAT_CONFIRM_RECEIPT", "itemId": "item-id"}
					}
					writeJSONResponse(w, map[string]any{"payout": payout})
				case "/v1/cli/withdrawals/context":
					writeJSONResponse(w, map[string]any{"withdrawableBalanceCents": 1500, "setupUrl": "https://viceme.cn/me/creator-center/wallet/withdraw"})
				default:
					t.Errorf("status must never create another payout: %s", r.URL)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			root := t.TempDir()
			var stdout, stderr bytes.Buffer
			exit := Execute([]string{"withdraw", "status", "--request-id", "wechat-income"}, Dependencies{Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), APIBaseURL: server.URL, Region: config.RegionCN, Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")}})
			var result struct {
				Data struct {
					Outcome         string
					ActionURL       string
					RemainingAmount int64
					Payout          struct{ SucceededAmount, ReleasedAmount int64 }
				}
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || exit != 0 || result.Data.Outcome != tc.outcome || result.Data.RemainingAmount != 1500 || result.Data.Payout.SucceededAmount != 1000 || result.Data.Payout.ReleasedAmount != 1500 {
				t.Fatalf("wrong outcome: %s", stdout.String())
			}
			if tc.status == "WAIT_USER_CONFIRM" && result.Data.ActionURL != "https://viceme.cn/me/creator-center/wallet/withdraw/payouts/payout-id?itemId=item-id" {
				t.Fatalf("confirmation must open the original payout: %s", result.Data.ActionURL)
			}
		})
	}
}

func TestWithdrawalSetupResumeAndLoginUpgrade(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
	stage, creates := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/cli/auth/status":
			scopes := []string{"profile:read"}
			if stage > 0 {
				scopes = append(scopes, "withdrawal:read", "withdrawal:write")
			}
			writeJSONResponse(w, map[string]any{"user": map[string]any{"id": "owner"}, "scopes": scopes})
		case "/v1/cli/withdrawals/by-source-reference":
			writeJSONResponse(w, map[string]any{"payout": nil})
		case "/v1/cli/withdrawals/context":
			writeJSONResponse(w, map[string]any{"capabilities": map[string]any{"provider": "WECHAT_PAY", "identityReady": stage == 2, "requiresPayoutAccount": false, "minAmount": 10, "maxAmount": 200000, "remainingDailyAmount": 1200, "agreementVersion": "2026-09-10"}, "withdrawableBalanceCents": 2500, "recipientRevision": strings.Repeat("a", 64), "setupUrl": "https://viceme.cn/me/creator-center/wallet/withdraw"})
		case "/v1/cli/withdrawals":
			creates++
			var input map[string]any
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			if len(input) != 5 || input["recipientRevision"] != strings.Repeat("a", 64) || input["sourceReference"] != "setup-resume" || input["amount"] != float64(1200) || input["provider"] != "WECHAT_PAY" {
				t.Errorf("unexpected Wechat request: %v", input)
			}
			writeJSONResponse(w, map[string]any{"id": "payout-id", "provider": "WECHAT_PAY", "sourceReference": "setup-resume", "amount": 1200, "status": "ACCEPTED"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	deps := Dependencies{Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), APIBaseURL: server.URL, Region: config.RegionCN, Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")}}
	for stage = 0; stage < 3; stage++ {
		stdout.Reset()
		Execute([]string{"withdraw", "create", "--request-id", "setup-resume"}, deps)
		var result struct {
			Data  struct{ Outcome, Action, URL string }
			Error struct{ Code string }
		}
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if stage == 0 && result.Error.Code != "WITHDRAWAL_SCOPE_REQUIRED" {
			t.Fatalf("old login must reauthenticate: %s", stdout.String())
		}
		if stage == 1 && (result.Data.Outcome != "ACTION_REQUIRED" || result.Data.Action != "COMPLETE_SETUP" || result.Data.URL == "") {
			t.Fatalf("missing setup action: %s", stdout.String())
		}
		if stage == 2 && result.Data.Outcome != "PENDING" {
			t.Fatalf("setup resume failed: %s", stdout.String())
		}
		if (stage < 2 && creates != 0) || (stage == 2 && creates != 1) {
			t.Fatalf("wrong number of payouts after setup stage %d: %d", stage, creates)
		}
	}
}

func TestWithdrawalUsesProfileMarketInsteadOfDownloadRegion(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
	for _, tc := range []struct {
		market, distribution config.Region
		expectedCode         string
	}{
		{config.RegionGlobal, config.RegionCN, "WITHDRAWAL_CN_ONLY"},
		{config.RegionCN, config.RegionGlobal, ""},
	} {
		t.Run(string(tc.market), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path == "/v1/cli/auth/status" {
					writeJSONResponse(w, map[string]any{"user": map[string]any{"id": "owner"}, "scopes": []string{"withdrawal:read"}})
				} else {
					writeJSONResponse(w, map[string]any{"withdrawableBalanceCents": 2500})
				}
			}))
			defer server.Close()
			root := t.TempDir()
			configDir := filepath.Join(root, "config")
			configured := config.Default(tc.distribution)
			configured.Profiles[0].MarketRegion = tc.market
			configured.Profiles[0].APIBaseURL = server.URL
			configured.Profiles[0].WebBaseURL = server.URL
			if _, err := config.Save(configDir, configured); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			exit := Execute([]string{"withdraw", "context"}, Dependencies{Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), Environment: skillcontent.Environment{Home: root, ConfigDir: configDir}})
			var result struct {
				Error struct{ Code string }
				Data  struct{ WithdrawableBalanceCents int64 }
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Error.Code != tc.expectedCode {
				t.Fatalf("market=%s distribution=%s exit=%d output=%s", tc.market, tc.distribution, exit, stdout.String())
			}
			if tc.market == config.RegionGlobal && requests != 0 {
				t.Fatalf("global profile performed %d requests", requests)
			}
			if tc.market == config.RegionCN && (exit != 0 || result.Data.WithdrawableBalanceCents != 2500) {
				t.Fatalf("CN profile cannot withdraw with global downloads: %s", stdout.String())
			}
		})
	}
}
