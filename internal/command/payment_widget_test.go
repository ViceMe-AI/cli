package command

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	qrcode "github.com/skip2/go-qrcode"
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
	if !bytes.Contains(widget, []byte(`aria-label="微信支付二维码"`)) || !bytes.Contains(widget, []byte("\\u003c/script\\u003e")) {
		t.Fatal("missing encoded QR or JSON escaping")
	}
	data.Status = "PAID"
	paid, err := renderPaymentWidget(data, "weixin://pay/test-only")
	if err != nil || bytes.Contains(paid, []byte(`aria-label="微信支付二维码"`)) {
		t.Fatal("paid Widget must not show a live QR")
	}
}

func TestPaymentWidgetQRSurvivesHostLineTruncation(t *testing.T) {
	for _, uri := range []string{"weixin://pay/short-fixture", "weixin://pay/" + strings.Repeat("x", 500)} {
		widget, err := renderPaymentWidget(paymentWidgetData{Title: "Test", Status: "PENDING"}, uri)
		if err != nil {
			t.Fatal(err)
		}
		marker := []byte(`aria-label="微信支付二维码"`)
		at := bytes.Index(widget, marker)
		if at < 0 {
			t.Fatal("missing QR SVG")
		}
		start := bytes.LastIndex(widget[:at], []byte("<svg"))
		end := bytes.Index(widget[at:], []byte("</svg>"))
		if start < 0 || end < 0 {
			t.Fatal("QR SVG is not closed")
		}
		svg := string(widget[start : at+end+len("</svg>")])
		var capped []string
		for _, line := range strings.Split(svg, "\n") {
			if len(line) >= 1800 {
				t.Fatalf("unreadable SVG line: %d bytes", len(line))
			}
			capped = append(capped, line[:min(len(line), 2000)])
		}
		decoder := xml.NewDecoder(strings.NewReader(strings.Join(capped, "\n")))
		cells := map[[2]int]bool{}
		pattern := regexp.MustCompile(`M(\d+) (\d+)h1v1h-1z`)
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if element, ok := token.(xml.StartElement); ok && element.Name.Local == "path" {
				for _, attr := range element.Attr {
					if attr.Name.Local == "d" {
						for _, match := range pattern.FindAllStringSubmatch(attr.Value, -1) {
							x, _ := strconv.Atoi(match[1])
							y, _ := strconv.Atoi(match[2])
							cells[[2]int{x, y}] = true
						}
					}
				}
			}
		}
		code, _ := qrcode.New(uri, qrcode.Medium)
		code.DisableBorder = true
		for y, row := range code.Bitmap() {
			for x, dark := range row {
				if cells[[2]int{x, y}] != dark {
					t.Fatal("QR matrix changed after host read")
				}
				delete(cells, [2]int{x, y})
			}
		}
		if len(cells) != 0 {
			t.Fatal("unexpected extra QR cells")
		}
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
