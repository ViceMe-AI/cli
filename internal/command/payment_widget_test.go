package command

import (
	"bytes"
	"strings"
	"testing"
)

func TestPaymentWidgetUsesSafeJSONAndInlineEncodedQR(t *testing.T) {
	amount := 1990
	data := paymentWidgetData{Title: "Generic </script><script>alert(1)</script>\u2028", AmountCents: &amount, Currency: "CNY", Status: "PENDING", ExpiresAt: "2099-01-01T00:00:00Z", PaymentMethodLabel: "微信支付", Locale: "zh-CN"}
	widget, err := renderPaymentWidget(data, "weixin://pay/test-only")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{data.Title, "weixin://", "__QR_SVG__", "__WIDGET_DATA__", "<img", "<button", "sendPrompt", "安装", "试用"} {
		if bytes.Contains(widget, []byte(forbidden)) {
			t.Fatalf("Widget contains unsafe or business-specific content %q", forbidden)
		}
	}
	if !bytes.Contains(widget, []byte("<svg")) || !bytes.Contains(widget, []byte("\\u003c/script\\u003e")) {
		t.Fatal("missing encoded QR or JSON escaping")
	}
	data.Status = "PAID"
	paid, err := renderPaymentWidget(data, "weixin://pay/test-only")
	if err != nil || bytes.Contains(paid, []byte("<svg")) {
		t.Fatal("paid Widget must not show a live QR")
	}
}

func TestPaymentWidgetRejectsInvalidMoneyAndProviderURIs(t *testing.T) {
	amount := -1
	data := paymentWidgetData{AmountCents: &amount, Currency: "CNY", Status: "PENDING"}
	if _, err := renderPaymentWidget(data, "weixin://pay/test-only"); err == nil {
		t.Fatal("negative money accepted")
	}
	amount = 1
	for _, uri := range []string{"https://example.invalid", "javascript:alert(1)", "weixin:" + strings.Repeat("a", 4097)} {
		if _, err := renderPaymentWidget(data, uri); err == nil {
			t.Fatalf("invalid URI accepted")
		}
	}
}
