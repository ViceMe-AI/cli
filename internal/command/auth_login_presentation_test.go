package command

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/api"
)

func TestDeviceLoginPresentationCreatesPrivateChatQRCode(t *testing.T) {
	t.Parallel()
	runtime := &Runtime{configBase: t.TempDir()}
	authorization := api.DeviceAuthorization{
		DeviceCode:              "device-code-that-must-not-appear-in-the-file-name",
		VerificationURIComplete: "https://viceme.cn/cli/authorize?user_code=ABCD-EFGH",
		WechatMPQRCodeURL:       "https://mp.weixin.qq.com/cgi-bin/showqrcode?ticket=device-ticket",
		ExpiresIn:               600,
	}

	presentation, err := createDeviceLoginPresentation(runtime, authorization, testLoginQRImage(t))
	if err != nil {
		t.Fatalf("create device login presentation: %v", err)
	}
	t.Cleanup(func() { _ = removeDeviceLoginPresentation(presentation) })
	if !filepath.IsAbs(presentation.ImagePath) {
		t.Fatalf("image path must be absolute: %q", presentation.ImagePath)
	}
	if presentation.ImageChatSrc != authorization.WechatMPQRCodeURL {
		t.Fatalf("unexpected chat image source: %q", presentation.ImageChatSrc)
	}
	if strings.HasPrefix(presentation.ImageChatSrc, "local-file://") {
		t.Fatalf("WorkBuddy must receive the validated HTTPS QR image: %q", presentation.ImageChatSrc)
	}
	if strings.Contains(filepath.Base(presentation.ImagePath), authorization.DeviceCode) {
		t.Fatalf("image filename leaked device code: %q", presentation.ImagePath)
	}
	image, err := os.ReadFile(presentation.ImagePath)
	if err != nil || len(image) < 8 || !bytes.Equal(image[:8], []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("presentation is not a PNG: %v", err)
	}
	if private, err := commercePaymentPresentationIsPrivate(presentation.ImagePath); err != nil || !private {
		t.Fatalf("presentation image is not private: private=%v err=%v", private, err)
	}
	if private, err := commercePaymentDirectoryIsPrivate(filepath.Dir(presentation.ImagePath)); err != nil || !private {
		t.Fatalf("presentation directory is not private: private=%v err=%v", private, err)
	}

	var progress bytes.Buffer
	writeHumanLoginStart(&progress, authorization, presentation)
	if !strings.Contains(progress.String(), "VICEME_LOGIN_QR_PRESENTATION=") || strings.Contains(progress.String(), "\n  https://") {
		t.Fatalf("login progress must expose a structured QR presentation without a bare URL: %q", progress.String())
	}
}

func TestDeviceLoginPresentationIsRemovedWhenLoginEnds(t *testing.T) {
	t.Parallel()
	presentation, err := createDeviceLoginPresentation(&Runtime{configBase: t.TempDir()}, api.DeviceAuthorization{
		DeviceCode:              "device-code",
		VerificationURIComplete: "https://viceme.cn/cli/authorize?user_code=ABCD-EFGH",
		WechatMPQRCodeURL:       "https://mp.weixin.qq.com/cgi-bin/showqrcode?ticket=device-ticket",
	}, testLoginQRImage(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := removeDeviceLoginPresentation(presentation); err != nil {
		t.Fatalf("remove device login presentation: %v", err)
	}
	if _, err := os.Stat(presentation.ImagePath); !os.IsNotExist(err) {
		t.Fatalf("one-time login QR remained after login ended: %v", err)
	}
}

func TestDeviceLoginPresentationRefusesToTurnTheBrowserURLIntoAQRCode(t *testing.T) {
	t.Parallel()
	authorization := api.DeviceAuthorization{
		DeviceCode:              "legacy-device-code",
		VerificationURIComplete: "https://viceme.cn/cli/authorize?user_code=ABCD-EFGH",
	}
	presentation, err := createDeviceLoginPresentation(
		&Runtime{configBase: t.TempDir()},
		authorization,
		nil,
	)
	if err == nil {
		t.Fatal("legacy browser authorization URL must not become a misleading chat QR")
	}
	if presentation.ImagePath != "" || presentation.AuthorizationURL != authorization.VerificationURIComplete {
		t.Fatalf("unexpected fallback presentation: %#v", presentation)
	}
}

func testLoginQRImage(t *testing.T) []byte {
	t.Helper()
	imageData := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			imageData.Set(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 4), B: 80, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, imageData); err != nil {
		t.Fatalf("encode test QR image: %v", err)
	}
	return encoded.Bytes()
}
