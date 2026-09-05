package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	cliembed "github.com/ViceMe-AI/cli"
	qrcode "github.com/skip2/go-qrcode"
)

// General presentation input; callers supply authoritative order snapshots.
// It has no Skill, entitlement, installation or task-continuation semantics.
type paymentWidgetData struct {
	Title              string `json:"title"`
	AmountCents        *int   `json:"amountCents"`
	Currency           string `json:"currency"`
	PaymentMethodLabel string `json:"paymentMethodLabel"`
	Status             string `json:"status"`
	ExpiresAt          string `json:"expiresAt"`
	Locale             string `json:"locale"`
}

func renderPaymentWidget(data paymentWidgetData, content string) ([]byte, error) {
	if data.AmountCents != nil && (*data.AmountCents < 0 || (data.Currency != "CNY" && data.Currency != "USD")) {
		return nil, errors.New("invalid payment amount or currency")
	}
	if data.Status != "PENDING" && data.Status != "PAID" && data.Status != "CLOSED" {
		return nil, errors.New("invalid payment status")
	}
	svg := ""
	if data.Status == "PENDING" && content != "" {
		// Reuse the payment URI validation, never interpret arbitrary SVG input.
		if _, err := encodeCommercePaymentQR(content); err != nil {
			return nil, err
		}
		code, err := qrcode.New(content, qrcode.Medium)
		if err != nil {
			return nil, err
		}
		matrix := code.Bitmap() // Includes the quiet zone required for scanning.
		var path strings.Builder
		for y, row := range matrix {
			for x, dark := range row {
				if dark {
					fmt.Fprintf(&path, "M%d %dh1v1h-1z", x, y)
				}
			}
		}
		svg = fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" role="img" aria-label="微信支付二维码" viewBox="0 0 %[1]d %[1]d" shape-rendering="crispEdges"><title>微信支付二维码</title><desc>使用微信扫一扫完成订单支付</desc><path fill="#fff" d="M0 0h%[1]dv%[1]dH0z"/><path fill="#000" d="%[2]s"/></svg>`, len(matrix), path.String())
	}
	template, err := fs.ReadFile(cliembed.EmbeddedWidgets(), "payment.html")
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(data) // HTML and U+2028/2029 escaping stays enabled.
	if err != nil {
		return nil, err
	}
	return []byte(strings.ReplaceAll(strings.ReplaceAll(string(template), "__QR_SVG__", svg), "__WIDGET_DATA__", string(encoded))), nil
}

func createPaymentWidget(runtime *Runtime, imagePath, content string, data paymentWidgetData) (string, error) {
	widget, err := renderPaymentWidget(data, content)
	if err != nil {
		return "", err
	}
	path := strings.TrimSuffix(imagePath, filepath.Ext(imagePath)) + ".html"
	if err := writeCommercePaymentPresentation(path, widget); err != nil {
		return "", err
	}
	return path, nil
}
