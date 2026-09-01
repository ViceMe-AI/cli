package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	qrcode "github.com/skip2/go-qrcode"
)

const (
	commercePaymentPresentationDirectory = "payment-presentations"
	commercePaymentPresentationMaxAge    = 24 * time.Hour
	commercePaymentStagingMaxAge         = 5 * time.Minute
)

type commerceQRPaymentAction struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

func prepareCommercePaymentPresentation(runtime *Runtime, order *api.CommerceOrder) error {
	if order == nil || order.AmountCents == 0 {
		return nil
	}
	if order.Status != "PENDING" || !runtime.deps.Now().Before(parseCommercePaymentExpiry(order.ExpiresAt)) {
		order.PaymentAction = nil
		order.PaymentPresentation = nil
		return removeCommercePaymentPresentation(runtime, order.OrderNo)
	}
	if order.PaymentProvider != "WECHAT_PAY" {
		return errors.New("paid Commerce Skill order did not use WeChat payment")
	}
	var action commerceQRPaymentAction
	if err := json.Unmarshal(order.PaymentAction, &action); err != nil {
		return fmt.Errorf("decode payment action: %w", err)
	}
	if action.Type != "QR_CODE" || strings.TrimSpace(action.Content) == "" {
		return errors.New("paid WeChat NATIVE order did not return a QR_CODE action")
	}
	if len(action.Content) > 4096 {
		return errors.New("payment QR content is too large")
	}
	parsed, err := url.Parse(action.Content)
	if err != nil || !strings.EqualFold(parsed.Scheme, "weixin") {
		return errors.New("payment QR content is not a WeChat payment URI")
	}
	png, err := qrcode.Encode(action.Content, qrcode.Medium, 512)
	if err != nil {
		return fmt.Errorf("encode payment QR image: %w", err)
	}
	directory := filepath.Join(runtime.configBase, commercePaymentPresentationDirectory)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create payment presentation directory: %w", err)
	}
	if err := secureCommercePaymentDirectory(directory); err != nil {
		return fmt.Errorf("secure payment presentation directory: %w", err)
	}
	if err := pruneCommercePaymentPresentations(runtime); err != nil {
		return fmt.Errorf("prune payment presentations: %w", err)
	}
	imagePath := filepath.Join(directory, commercePaymentPresentationFilename(order.OrderNo))
	if err := writeCommercePaymentPresentation(imagePath, png); err != nil {
		return err
	}
	absolutePath, err := filepath.Abs(imagePath)
	if err != nil {
		return fmt.Errorf("resolve payment presentation path: %w", err)
	}
	order.PaymentPresentation = &api.CommercePaymentPresentation{
		Type:      "LOCAL_IMAGE",
		Purpose:   "PAYMENT_QR_CODE",
		MIMEType:  "image/png",
		ImagePath: absolutePath,
		AltText:   "微信支付二维码",
		ExpiresAt: order.ExpiresAt,
	}
	// The Agent only receives a local image path. Keeping the provider URI out
	// of stdout prevents accidental plaintext display or third-party QR upload.
	order.PaymentAction = json.RawMessage(`{"type":"QR_CODE"}`)
	return nil
}

func writeCommercePaymentPresentation(filename string, data []byte) error {
	if existing, err := os.ReadFile(filename); err == nil && bytes.Equal(existing, data) {
		if err := secureCommercePaymentFile(filename); err != nil {
			return fmt.Errorf("secure existing payment QR image: %w", err)
		}
		return nil
	}
	// The shared degraded write replaces the previous Windows remove-and-retry:
	// the direct-write fallback also covers a sandbox or filesystem that cannot
	// replace the target through a rename.
	if err := privatefile.Write(filename, data, ".payment-qr-*.tmp"); err != nil {
		return fmt.Errorf("write payment QR image: %w", err)
	}
	if err := secureCommercePaymentFile(filename); err != nil {
		return fmt.Errorf("secure active payment QR image: %w", err)
	}
	return nil
}

func parseCommercePaymentExpiry(value string) time.Time {
	expiresAt, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return expiresAt
}

func commercePaymentPresentationFilename(orderNo string) string {
	digest := sha256.Sum256([]byte(orderNo))
	return "wechat-" + hex.EncodeToString(digest[:16]) + ".png"
}

func removeCommercePaymentPresentation(runtime *Runtime, orderNo string) error {
	filename := filepath.Join(
		runtime.configBase,
		commercePaymentPresentationDirectory,
		commercePaymentPresentationFilename(orderNo),
	)
	if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func pruneCommercePaymentPresentations(runtime *Runtime) error {
	directory := filepath.Join(runtime.configBase, commercePaymentPresentationDirectory)
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	staleBefore := runtime.deps.Now().Add(-commercePaymentPresentationMaxAge)
	staleStagingBefore := runtime.deps.Now().Add(-commercePaymentStagingMaxAge)
	var cleanupErrors []error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		isStaging := strings.HasPrefix(entry.Name(), ".payment-qr-") && strings.HasSuffix(entry.Name(), ".tmp")
		isPaymentPNG := strings.HasPrefix(entry.Name(), "wechat-") && strings.HasSuffix(entry.Name(), ".png")
		if !isStaging && !isPaymentPNG {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("inspect %s: %w", entry.Name(), err))
			continue
		}
		stale := (isStaging && info.ModTime().Before(staleStagingBefore)) ||
			(isPaymentPNG && info.ModTime().Before(staleBefore))
		if stale {
			if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("remove %s: %w", entry.Name(), err))
			}
		}
	}
	return errors.Join(cleanupErrors...)
}
