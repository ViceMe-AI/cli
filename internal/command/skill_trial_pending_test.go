package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestTrialUsePendingLifecycle(t *testing.T) {
	const productID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const otherProductID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	const apiBaseURL = "https://api.viceme.cn"
	now := time.Now()

	base := t.TempDir()

	t.Run("issues a fresh request id and persists it", func(t *testing.T) {
		requestID, err := beginTrialUsePending(base, apiBaseURL, productID, now)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if len(requestID) < 8 || strings.ContainsAny(requestID, "!@# /") {
			t.Fatalf("request id %q must be url-safe and at least 8 chars", requestID)
		}
		path := trialUsePendingPath(base, apiBaseURL, productID)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("pending file must exist: %v", err)
		}
		var pending trialUsePending
		if err := json.Unmarshal(raw, &pending); err != nil {
			t.Fatalf("pending payload: %v", err)
		}
		if pending.ProductID != productID || pending.RequestID != requestID {
			t.Fatalf("pending mismatch: %+v", pending)
		}
	})

	t.Run("reuses an unconfirmed pending id for the retry", func(t *testing.T) {
		first, err := beginTrialUsePending(base, apiBaseURL, productID, now)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		// 响应未送达:不 confirm,重试必须复用同一键。
		second, err := beginTrialUsePending(base, apiBaseURL, productID, now.Add(2*time.Minute))
		if err != nil {
			t.Fatalf("retry begin: %v", err)
		}
		if first != second {
			t.Fatalf("retry must reuse the pending id: %q vs %q", first, second)
		}
	})

	t.Run("issues a new id after the response was confirmed", func(t *testing.T) {
		first, err := beginTrialUsePending(base, apiBaseURL, productID, now)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		confirmTrialUsePending(base, apiBaseURL, productID, first)
		second, err := beginTrialUsePending(base, apiBaseURL, productID, now.Add(time.Second))
		if err != nil {
			t.Fatalf("begin after confirm: %v", err)
		}
		if first == second {
			t.Fatalf("a confirmed use must start a new request id")
		}
	})

	t.Run("a failed confirm surfaces an error and keeps the id for retry", func(t *testing.T) {
		// 评审复现:确认路径的本地故障(锁被长期占用、读写失败)不能
		// 静默吞掉——已消费的键留在 pending,下一次真实使用会被当作
		// 重试回放旧响应而持续漏扣。必须报错,且 pending 原样保留供
		// 重跑自愈(服务端对同一键回放,不重复扣)。
		id, err := beginTrialUsePending(base, apiBaseURL, productID, now)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		originalTimeout := trialUseLockTimeout
		trialUseLockTimeout = 250 * time.Millisecond
		defer func() { trialUseLockTimeout = originalTimeout }()

		pendingPath := trialUsePendingPath(base, apiBaseURL, productID)
		holder := flock.New(pendingPath + ".lock")
		locked, err := holder.TryLock()
		if err != nil || !locked {
			t.Fatalf("hold the pending lock: locked=%v err=%v", locked, err)
		}
		defer holder.Unlock()

		if err := confirmTrialUsePending(base, apiBaseURL, productID, id); err == nil {
			t.Fatalf("failed confirm silently left the consumed id reusable")
		}
		holder.Unlock()
		after, err := beginTrialUsePending(base, apiBaseURL, productID, now.Add(time.Second))
		if err != nil || after != id {
			t.Fatalf("pending must survive a failed confirm for retry: err=%v after=%q want=%q", err, after, id)
		}
	})

	t.Run("a late confirm never deletes a newer pending", func(t *testing.T) {
		// 评审复现:A、B 并发共用 ID-1,A 先返回确认删除;新使用 C 生成
		// ID-2;B 的响应迟到再来确认时,绝不能删掉 C 的 ID-2——否则 C
		// 的结果未知重试会分叉出 ID-3 被服务端二次扣。
		id1, err := beginTrialUsePending(base, apiBaseURL, productID, now)
		if err != nil {
			t.Fatalf("begin first: %v", err)
		}
		confirmTrialUsePending(base, apiBaseURL, productID, id1)
		id2, err := beginTrialUsePending(base, apiBaseURL, productID, now.Add(time.Second))
		if err != nil {
			t.Fatalf("begin second: %v", err)
		}
		if id2 == id1 {
			t.Fatalf("a confirmed use must start a new request id")
		}
		// 迟到的 ID-1 确认:盘上已是 ID-2,必须原样保留。
		confirmTrialUsePending(base, apiBaseURL, productID, id1)
		after, err := beginTrialUsePending(base, apiBaseURL, productID, now.Add(2*time.Second))
		if err != nil {
			t.Fatalf("begin after late confirm: %v", err)
		}
		if after != id2 {
			t.Fatalf("late confirmer deleted newer pending: %q vs %q", after, id2)
		}
		// 属于本键的确认照常删除。
		confirmTrialUsePending(base, apiBaseURL, productID, id2)
		next, err := beginTrialUsePending(base, apiBaseURL, productID, now.Add(3*time.Second))
		if err != nil {
			t.Fatalf("begin after own confirm: %v", err)
		}
		if next == id2 {
			t.Fatalf("own confirm must remove the pending")
		}
	})

	t.Run("keeps reusing an unconfirmed pending regardless of age", func(t *testing.T) {
		// 结果未知的重试不能靠计时器变成结果已知:换新键会对服务端
		// 已扣过的使用二次扣,未确认的 pending 无时效、持续复用。
		first, err := beginTrialUsePending(base, apiBaseURL, productID, now)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		daysLater, err := beginTrialUsePending(base, apiBaseURL, productID, now.Add(72*time.Hour))
		if err != nil {
			t.Fatalf("aged begin: %v", err)
		}
		if first != daysLater {
			t.Fatalf("an unconfirmed pending must stay reusable: %q vs %q", first, daysLater)
		}
	})

	t.Run("concurrent begins converge to a single request id", func(t *testing.T) {
		// 评审复现:读-判-写临界区没有互斥时,并发的重复调用会读到半成品
		// pending 并分叉出不同键,到服务端各扣一次。锁协议必须让它们收敛。
		for iteration := 0; iteration < 100; iteration++ {
			iterBase := filepath.Join(base, fmt.Sprintf("iter-%d", iteration))
			if err := os.MkdirAll(iterBase, 0o700); err != nil {
				t.Fatalf("mkdir iter base: %v", err)
			}
			ids := make([]string, 8)
			errs := make([]error, 8)
			var wg sync.WaitGroup
			for worker := 0; worker < len(ids); worker++ {
				wg.Add(1)
				go func(slot int) {
					defer wg.Done()
					ids[slot], errs[slot] = beginTrialUsePending(iterBase, apiBaseURL, productID, time.Now())
				}(worker)
			}
			wg.Wait()
			for slot, err := range errs {
				if err != nil {
					t.Fatalf("iteration %d worker %d: %v", iteration, slot, err)
				}
			}
			for slot := 1; slot < len(ids); slot++ {
				if ids[slot] != ids[0] {
					t.Fatalf("iteration %d forked request ids: %q vs %q", iteration, ids[0], ids[slot])
				}
			}
		}
	})

	t.Run("isolates pending files per product and endpoint", func(t *testing.T) {
		id, err := beginTrialUsePending(base, apiBaseURL, productID, now)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		other, err := beginTrialUsePending(base, apiBaseURL, otherProductID, now)
		if err != nil {
			t.Fatalf("begin other: %v", err)
		}
		otherEndpoint, err := beginTrialUsePending(base, "https://api.viceme.ai", productID, now)
		if err != nil {
			t.Fatalf("begin other endpoint: %v", err)
		}
		if id == other || id == otherEndpoint {
			t.Fatalf("pending must be isolated per product and endpoint")
		}
		if trialUsePendingPath(base, apiBaseURL, productID) == trialUsePendingPath(base, apiBaseURL, otherProductID) {
			t.Fatalf("pending paths must differ per product")
		}
	})

	t.Run("ignores a pending file written for another product", func(t *testing.T) {
		// 同哈希路径被外部内容覆盖的防御:ProductID 不匹配时换新键。
		path := trialUsePendingPath(base, apiBaseURL, productID)
		if err := os.WriteFile(path, []byte(`{"productId":"cccccccc-cccc-4ccc-8ccc-cccccccccccc","requestId":"foreign-id","createdAtMs":`+time.Now().Format("1500000000000")+`}`), 0o600); err != nil {
			t.Fatalf("seed foreign pending: %v", err)
		}
		requestID, err := beginTrialUsePending(base, apiBaseURL, productID, now)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if requestID == "foreign-id" {
			t.Fatalf("foreign pending must not be reused")
		}
	})
}

func TestTrialUsePendingFileModes(t *testing.T) {
	// POSIX 权限位在 Windows 上不具语义(Go 会把所有文件报成 0777);
	// Windows 的等价保证属于用户级 ACL,不在本测试范围。
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode assertions do not apply on Windows")
	}
	base := t.TempDir()
	const productID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	if _, err := beginTrialUsePending(base, "https://api.viceme.cn", productID, time.Now()); err != nil {
		t.Fatalf("begin: %v", err)
	}
	info, err := os.Stat(filepath.Join(base, "trial-use-pending"))
	if err != nil {
		t.Fatalf("pending dir: %v", err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("pending dir mode must be 0700, got %v", info.Mode().Perm())
	}
	path := trialUsePendingPath(base, "https://api.viceme.cn", productID)
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("pending file: %v", err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("pending file mode must be 0600, got %v", fileInfo.Mode().Perm())
	}
}
