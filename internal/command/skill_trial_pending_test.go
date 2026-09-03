package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		confirmTrialUsePending(base, apiBaseURL, productID)
		second, err := beginTrialUsePending(base, apiBaseURL, productID, now.Add(time.Second))
		if err != nil {
			t.Fatalf("begin after confirm: %v", err)
		}
		if first == second {
			t.Fatalf("a confirmed use must start a new request id")
		}
	})

	t.Run("expires a stale pending instead of reusing it forever", func(t *testing.T) {
		first, err := beginTrialUsePending(base, apiBaseURL, productID, now)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		stale, err := beginTrialUsePending(base, apiBaseURL, productID, now.Add(trialUsePendingTTL+time.Minute))
		if err != nil {
			t.Fatalf("stale begin: %v", err)
		}
		if first == stale {
			t.Fatalf("a pending older than the TTL must not be reused")
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
