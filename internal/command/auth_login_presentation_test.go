package command

import (
	"bytes"
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
		ExpiresIn:               600,
	}

	presentation, err := createDeviceLoginPresentation(runtime, authorization)
	if err != nil {
		t.Fatalf("create device login presentation: %v", err)
	}
	t.Cleanup(func() { _ = removeDeviceLoginPresentation(presentation) })
	if !filepath.IsAbs(presentation.ImagePath) {
		t.Fatalf("image path must be absolute: %q", presentation.ImagePath)
	}
	if presentation.ImageChatSrc != localFileChatSrc(presentation.ImagePath) {
		t.Fatalf("unexpected chat image source: %q", presentation.ImageChatSrc)
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
	})
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
