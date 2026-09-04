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

	"github.com/gofrs/flock"
)

// Skill 试用消耗的请求级幂等键:每次使用一个 requestId,服务端对同一键
// 原样回放首次结果;结果未知时保留 pending,下一次 use 复用同一键,只有
// 拿到权威业务结果才换新键。这替代了服务端旧的时间窗口语义——连续的新
// 使用无论间隔多短都会各扣各的,而真正的重试不会重复扣。
type trialUsePending struct {
	ProductID string `json:"productId"`
	RequestID string `json:"requestId"`
	CreatedAt int64  `json:"createdAtMs"`
}

// 未确认的 pending 无 TTL、持续复用:复用旧键最坏漏扣一次,而按计时器
// 换新键可能对服务端已扣过的使用二次扣——结果未知不能由计时器变成结果
// 已知。锁等待上限远大于临界区(读写一个小 JSON)。
const (
	trialUseLockTimeout = 5 * time.Second
	trialUseLockRetry   = 5 * time.Millisecond
)

func trialUsePendingPath(configBase, apiBaseURL, productID string) string {
	digest := sha256.Sum256([]byte(apiBaseURL + "\x00" + productID))
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

// beginTrialUsePending 返回本次使用的 requestId:存在未确认的 pending
// (上次结果未知)时复用其键,否则生成新键并落盘。
//
// 读-判-写的整个临界区由 flock 互斥:并发进程串行进入,读到的总是
// 完整的 pending,重复调用稳定收敛到同一个 requestId。等锁超时返回
// 错误,让本次命令失败而不是带着分叉的键去扣次。
func beginTrialUsePending(
	configBase, apiBaseURL, productID string,
	now time.Time,
) (string, error) {
	path := trialUsePendingPath(configBase, apiBaseURL, productID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	lock, err := lockTrialUsePending(path)
	if err != nil {
		return "", err
	}
	defer lock.Unlock()

	if reusable := readReusableTrialUsePending(path, productID); reusable != "" {
		return reusable, nil
	}
	pending := trialUsePending{
		ProductID: productID,
		RequestID: newTrialUseRequestID(),
		CreatedAt: now.UnixMilli(),
	}
	payload, err := json.Marshal(pending)
	if err != nil {
		return "", err
	}
	// 持锁临界区:旧文件(外域或残留的半成品)可以被安全替换。
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return "", err
	}
	return pending.RequestID, nil
}

// confirmTrialUsePending 在本次 requestId 拿到权威业务结果(成功解析的
// 响应)后删除 pending:下一次使用从新键开始。服务端可能已经扣次但响应
// 丢失(网络错误、5xx、无效响应)的情况一律不确认——键保留,重试复用
// 同一键由服务端回放,不会二次扣。
//
// 删除与 begin 持同一把锁且只删属于自己的键:迟到的旧响应确认时,盘上
// 可能已经是新使用的 pending,无条件删除会让新使用的结果未知重试分叉
// 出新键、被服务端二次扣次。
func confirmTrialUsePending(
	configBase, apiBaseURL, productID, requestID string,
) {
	path := trialUsePendingPath(configBase, apiBaseURL, productID)
	lock, err := lockTrialUsePending(path)
	if err != nil {
		// 拿不到锁说明另一进程正在 begin/confirm;它落盘或删除的
		// pending 一定覆盖本键的处置,无需再动。
		return
	}
	defer lock.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var pending trialUsePending
	if json.Unmarshal(data, &pending) != nil || pending.RequestID != requestID {
		return
	}
	_ = os.Remove(path)
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
