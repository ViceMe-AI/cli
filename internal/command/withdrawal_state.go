package command

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/privatepath"
)

type withdrawalIntent struct {
	RequestedAmount   string                `json:"requestedAmount"`
	RequestedMethodID string                `json:"requestedMethodId"`
	Request           api.WithdrawalRequest `json:"request"`
}

func (runtime *Runtime) withdrawalIntentPath(userID, requestID string) string {
	digest := sha256.Sum256([]byte(joinStateParts([]string{runtime.profile.ID, runtime.credentialScope, userID, requestID})))
	return filepath.Join(runtime.configBase, "withdrawals", hex.EncodeToString(digest[:])+".json")
}

func (runtime *Runtime) loadWithdrawalIntent(userID, requestID string) (*withdrawalIntent, error) {
	raw, err := os.ReadFile(runtime.withdrawalIntentPath(userID, requestID))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, output.Internal("WITHDRAWAL_INTENT_READ_FAILED", "无法读取原提现请求", err)
	}
	var intent withdrawalIntent
	if json.Unmarshal(raw, &intent) != nil || intent.Request.SourceReference != requestID || intent.Request.Amount <= 0 || !withdrawalRecipientRevision.MatchString(intent.Request.RecipientRevision) || intent.Request.AgreementVersion == "" {
		return nil, output.Internal("WITHDRAWAL_INTENT_INVALID", "原提现请求已损坏，停止创建，请按请求标识查询原单", nil)
	}
	return &intent, nil
}

func (runtime *Runtime) saveWithdrawalIntent(userID, requestID string, intent withdrawalIntent) error {
	filename := runtime.withdrawalIntentPath(userID, requestID)
	if _, err := privatepath.EnsureDirectory(filepath.Dir(filename)); err != nil {
		return output.Internal("WITHDRAWAL_INTENT_WRITE_FAILED", "无法创建提现恢复目录", err)
	}
	raw, err := json.Marshal(intent)
	if err != nil {
		return output.Internal("WITHDRAWAL_INTENT_WRITE_FAILED", "无法编码提现请求", err)
	}
	if err := privatefile.Write(filename, raw, ".withdrawal-*"); err != nil {
		return output.Internal("WITHDRAWAL_INTENT_WRITE_FAILED", "无法在提交前保存提现请求", err)
	}
	return nil
}
