package command

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Legacy writers are kept only as test fixtures for the previous disk format.
// Production migration never starts a new use in this separate pending file.
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
//
// 确认失败必须向调用方报错而不是静默吞掉:本键已在服务端消费,留在
// pending 里会让下一次真实使用被当作重试回放旧响应,本地故障持续期间
// 会持续漏扣。锁获取、读取、删除任一失败都返回错误,让 `skill use`
// 失败;用户重跑时 begin 复用未确认键、服务端原样回放本次结果(不再
// 扣次),确认链路自愈。文件不存在或盘上已是别的键是安全的 no-op;
// 文件内容不可解析说明本地状态已损坏,删除后一并视为已处置——它不可
// 能再被安全复用,而本键确定已被服务端消费。
func confirmTrialUsePending(
	configBase, apiBaseURL, productID, requestID string,
) error {
	path := trialUsePendingPath(configBase, apiBaseURL, productID)
	lock, err := lockTrialUsePending(path)
	if err != nil {
		return err
	}
	defer lock.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	var pending trialUsePending
	if json.Unmarshal(data, &pending) != nil || pending.ProductID != productID {
		// 不可解析或外域残留:无法安全复用,本键确定已消费,一并清理。
		return os.Remove(path)
	}
	if pending.RequestID != requestID {
		// 盘上已是新使用的键:本键的处置已终结,不能动它。
		return nil
	}
	return os.Remove(path)
}
