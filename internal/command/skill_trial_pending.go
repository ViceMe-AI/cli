package command

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/gofrs/flock"
)

// Older CLI versions stored their unconfirmed key separately from Python.
// New uses persist only scriptTrialState.PendingRequestID under ProductLock;
// these helpers read and retire legacy files during migration.
type trialUsePending struct {
	ProductID string `json:"productId"`
	RequestID string `json:"requestId"`
	CreatedAt int64  `json:"createdAtMs"`
}

// 未确认的 pending 无 TTL、持续复用:复用旧键最坏漏扣一次,而按计时器
// 换新键可能对服务端已扣过的使用二次扣——结果未知不能由计时器变成结果
// 已知。锁等待上限远大于临界区(读写一个小 JSON)。
var (
	trialUseLockTimeout = 5 * time.Second
	trialUseLockRetry   = 5 * time.Millisecond
)

func trialUsePendingPath(configBase, apiBaseURL, productID string) string {
	digest := sha256.Sum256([]byte(config.APIStateBaseURL(apiBaseURL) + "\x00" + productID))
	return filepath.Join(
		configBase,
		"trial-use-pending",
		hex.EncodeToString(digest[:16])+".json",
	)
}

func newTrialUseRequestID() string {
	entropy := make([]byte, 16)
	if _, err := rand.Read(entropy); err != nil {
		return fmt.Sprintf("pending-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(entropy)
}

// lockTrialUsePending 获取 pending 文件的跨进程互斥锁。gofrs/flock 的锁
// 由 OS 持有,进程退出即释放,无残留清理问题;调用方 defer Unlock。
func lockTrialUsePending(pendingPath string) (*flock.Flock, error) {
	lock := flock.New(pendingPath + ".lock")
	lockContext, cancelLock := context.WithTimeout(
		context.Background(),
		trialUseLockTimeout,
	)
	defer cancelLock()
	if _, err := lock.TryLockContext(lockContext, trialUseLockRetry); err != nil {
		return nil, fmt.Errorf("trial use pending lock failed: %w", err)
	}
	return lock, nil
}

func readReusableTrialUsePending(path, productID string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pending trialUsePending
	if json.Unmarshal(data, &pending) != nil || pending.ProductID != productID {
		return ""
	}
	// 未确认的 pending 不设时效:只要还是同一款目就一直复用。
	if pending.RequestID == "" {
		return ""
	}
	return pending.RequestID
}

// hasPendingTrialUse treats the shared journal as authoritative after migration.
// A legacy copy can survive a crash between saving the shared state and deleting
// that copy; Python can then settle the shared request without removing it. Only
// trust its migration marker when the full local trial identity matches.
func hasPendingTrialUse(runtime *Runtime, productID string, credential skillTrialCredential, state scriptTrialState) bool {
	legacyID := readReusableTrialUsePending(trialUsePendingPath(runtime.configBase, runtime.apiBaseURL, productID), productID)
	if credential.InstallID != "" && credential.Secret != "" &&
		validateScriptTrialStateIdentity(runtime, productID, state) == nil &&
		state.InstallID == credential.InstallID && state.Secret == credential.Secret {
		return state.PendingRequestID != "" || (legacyID != "" && legacyID != state.MigratedCLIRequestID)
	}
	return legacyID != ""
}
