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
	var item struct {
		ProductTitle string `json:"productTitle"`
	}
	_ = json.Unmarshal(order.Item, &item)
	title := item.ProductTitle
	if strings.TrimSpace(title) == "" {
		title = "订单支付"
	}
	presentation, err := newCommercePaymentPresentation(runtime, order.OrderNo, order.ExpiresAt, action.Content, paymentWidgetData{
		Title: title, AmountCents: &order.AmountCents, Currency: order.Currency,
		PaymentMethodLabel: "微信支付", Status: order.Status, ExpiresAt: order.ExpiresAt,
		Locale: localeForRuntimeMarket(runtime),
	})
	if err != nil {
		return err
	}
	order.PaymentPresentation = presentation
	// The Agent only receives a local image path. Keeping the provider URI out
	// of stdout prevents accidental plaintext display or third-party QR upload.
	order.PaymentAction = json.RawMessage(`{"type":"QR_CODE"}`)
	return nil
}

func newCommercePaymentPresentation(runtime *Runtime, orderNo, expiresAt, content string, details ...paymentWidgetData) (*api.CommercePaymentPresentation, error) {
	absolutePath, err := createCommercePaymentQRImage(runtime, orderNo, content)
	if err != nil {
		return nil, err
	}
	data := paymentWidgetData{Title: "订单支付", PaymentMethodLabel: "微信支付", Status: "PENDING", ExpiresAt: expiresAt, Locale: localeForRuntimeMarket(runtime)}
	if len(details) > 0 {
		data = details[0]
	}
	widgetPath, err := createPaymentWidget(runtime, absolutePath, content, data)
	if err != nil {
		return nil, err
	}
	presentation := commercePaymentPresentation(absolutePath, expiresAt)
	presentation.WidgetPath = widgetPath
	presentation.WidgetMIMEType = "text/html"
	return presentation, nil
}

func commercePaymentPresentation(absolutePath, expiresAt string) *api.CommercePaymentPresentation {
	return &api.CommercePaymentPresentation{
		Type:      "LOCAL_IMAGE",
		Purpose:   "PAYMENT_QR_CODE",
		MIMEType:  "image/png",
		ImagePath: absolutePath,
		AltText:   "微信支付二维码",
		ExpiresAt: expiresAt,
	}
}

func createCommercePaymentQRImage(runtime *Runtime, orderNo, content string) (string, error) {
	png, err := encodeCommercePaymentQR(content)
	if err != nil {
		return "", err
	}
	directory := filepath.Join(runtime.configBase, commercePaymentPresentationDirectory)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create payment presentation directory: %w", err)
	}
	if err := secureCommercePaymentDirectory(directory); err != nil {
		return "", fmt.Errorf("secure payment presentation directory: %w", err)
	}
	if err := pruneCommercePaymentPresentations(runtime); err != nil {
		return "", fmt.Errorf("prune payment presentations: %w", err)
	}
	imagePath := filepath.Join(directory, commercePaymentPresentationFilename(orderNo))
	if err := writeCommercePaymentPresentation(imagePath, png); err != nil {
		return "", err
	}
	absolutePath, err := filepath.Abs(imagePath)
	if err != nil {
		return "", fmt.Errorf("resolve payment presentation path: %w", err)
	}
	return absolutePath, nil
}

func encodeCommercePaymentQR(content string) ([]byte, error) {
	if len(content) > 4096 {
		return nil, errors.New("payment QR content is too large")
	}
	parsed, err := url.Parse(content)
	if err != nil || !strings.EqualFold(parsed.Scheme, "weixin") {
		return nil, errors.New("payment QR content is not a WeChat payment URI")
	}
	png, err := qrcode.Encode(content, qrcode.Medium, 512)
	if err != nil {
		return nil, fmt.Errorf("encode payment QR image: %w", err)
	}
	return png, nil
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
	for _, path := range []string{filename, strings.TrimSuffix(filename, ".png") + ".html"} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
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
		isPaymentArtifact := strings.HasPrefix(entry.Name(), "wechat-") && (strings.HasSuffix(entry.Name(), ".png") || strings.HasSuffix(entry.Name(), ".html"))
		if !isStaging && !isPaymentArtifact {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("inspect %s: %w", entry.Name(), err))
			continue
		}
		stale := (isStaging && info.ModTime().Before(staleStagingBefore)) ||
			(isPaymentArtifact && info.ModTime().Before(staleBefore))
		if stale {
			if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("remove %s: %w", entry.Name(), err))
			}
		}
	}
	return errors.Join(cleanupErrors...)
}
