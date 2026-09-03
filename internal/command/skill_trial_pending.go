package command

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Skill 试用消耗的请求级幂等键:每次使用一个 requestId,服务端对同一键
// 原样回放首次结果;响应未送达时保留 pending,下一次 use 复用同一键,
// 只有拿到权威业务结果才换新键。这替代了服务端旧的时间窗口语义——连续
// 的新使用无论间隔多短都会各扣各的,而真正的重试不会重复扣。
type trialUsePending struct {
	ProductID string `json:"productId"`
	RequestID string `json:"requestId"`
	CreatedAt int64  `json:"createdAtMs"`
}

// 崩溃残留兜底:超过 TTL 的 pending 视为历史残留并换新键。复用未确认的
// 旧键最坏漏扣一次,换新键可能对服务端已扣过的使用再扣一次,所以 TTL
// 取一个远超单次命令生命周期的值,把换键概率压到实际为零。
const trialUsePendingTTL = 24 * time.Hour

// 跨进程锁参数:等待持锁者的上限远大于临界区(读写一个小 JSON);锁文件
// 自身的崩溃残留按 staleness 清理,超过该年龄的锁视为已死。
const (
	trialUseLockTimeout = 5 * time.Second
	trialUseLockStale   = 10 * time.Second
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

// beginTrialUsePending 返回本次使用的 requestId:存在未确认且未过期的
// pending(上次响应未送达)时复用其键,否则生成新键并落盘。
//
// 读-判-写的整个临界区由同目录的锁文件(O_EXCL 原子创建)保护:并发
// 进程串行进入,读到的一定是完整的 pending,重复调用稳定收敛到同一个
// requestId。锁文件崩溃残留按 staleness 清理;等锁超时返回错误,让本次
// 命令失败而不是带着分叉的键去扣次。
func beginTrialUsePending(
	configBase, apiBaseURL, productID string,
	now time.Time,
) (string, error) {
	path := trialUsePendingPath(configBase, apiBaseURL, productID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	unlock, err := lockTrialUsePending(path, now)
	if err != nil {
		return "", err
	}
	defer unlock()

	if reusable := readReusableTrialUsePending(path, productID, now); reusable != "" {
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
	// 持锁临界区:旧文件(过期、外域或残留的半成品)可以被安全替换。
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return "", err
	}
	return pending.RequestID, nil
}

// confirmTrialUsePending 在拿到权威业务结果(成功解析的响应)后删除
// pending:下一次使用从新键开始。服务端可能已经扣次但响应丢失(网络错
// 误、5xx、无效响应)的情况一律不确认——键保留,重试复用同一键由服务端
// 回放,不会二次扣。删除失败不影响主流程,残留由 TTL 兜底。
func confirmTrialUsePending(configBase, apiBaseURL, productID string) {
	_ = os.Remove(trialUsePendingPath(configBase, apiBaseURL, productID))
}

// lockTrialUsePending 以 O_EXCL 锁文件实现跨进程互斥,覆盖 pending 文件
// 的读取、判定与替换。返回解锁函数;等锁超时返回错误。内部使用真实
// 时钟:等待循环必须随墙钟推进,不能用调用方传入的业务时刻。
func lockTrialUsePending(pendingPath string, now time.Time) (func(), error) {
	lockPath := pendingPath + ".lock"
	deadline := time.Now().Add(trialUseLockTimeout)
	for {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			file.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		if info, statErr := os.Stat(lockPath); statErr == nil {
			// 崩溃残留的锁:持有者早已死亡,清理后重试竞争。
			if time.Since(info.ModTime()) > trialUseLockStale {
				_ = os.Remove(lockPath)
				continue
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf(
				"trial use pending lock timed out after %s: %s",
				trialUseLockTimeout,
				lockPath,
			)
		}
		time.Sleep(trialUseLockRetry)
	}
}

func readReusableTrialUsePending(path, productID string, now time.Time) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pending trialUsePending
	if json.Unmarshal(data, &pending) != nil || pending.ProductID != productID {
		return ""
	}
	if pending.RequestID == "" ||
		now.UnixMilli()-pending.CreatedAt >= trialUsePendingTTL.Milliseconds() {
		return ""
	}
	return pending.RequestID
}
