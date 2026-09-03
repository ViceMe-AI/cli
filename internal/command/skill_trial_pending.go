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
// 只有拿到响应才换新键。这替代了服务端旧的时间窗口语义——连续的新使用
// 无论间隔多短都会各扣各的,而真正的重试不会重复扣。
type trialUsePending struct {
	ProductID string `json:"productId"`
	RequestID string `json:"requestId"`
	CreatedAt int64  `json:"createdAtMs"`
}

// 崩溃残留兜底:超过 TTL 的 pending 视为历史残留并换新键。宁可给用户
// 漏扣一次,不能把残留键一直复用下去。
const trialUsePendingTTL = 15 * time.Minute

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
// pending(上次响应未送达)时复用其键,否则生成新键并原子落盘。并发对手
// 抢先落盘时复用对手的键,让并发的重复调用收敛到同一 requestId。
func beginTrialUsePending(
	configBase, apiBaseURL, productID string,
	now time.Time,
) (string, error) {
	path := trialUsePendingPath(configBase, apiBaseURL, productID)
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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			// 并发对手刚写入了 pending:复用它的键。
			if reusable := readReusableTrialUsePending(path, productID, now); reusable != "" {
				return reusable, nil
			}
			// 残留(已过期或不是本款目)挡住了新键:移除后重试一次创建。
			if removeErr := os.Remove(path); removeErr == nil {
				file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			}
		}
		if err != nil {
			return "", err
		}
	}
	if _, err := file.Write(payload); err != nil {
		file.Close()
		return "", err
	}
	return pending.RequestID, file.Close()
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

// confirmTrialUsePending 响应已送达(成功或明确的业务失败)后删除 pending:
// 下一次使用从新键开始。删除失败不影响主流程,残留由 TTL 兜底。
func confirmTrialUsePending(configBase, apiBaseURL, productID string) {
	_ = os.Remove(trialUsePendingPath(configBase, apiBaseURL, productID))
}
