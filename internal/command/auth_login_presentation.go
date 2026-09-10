package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/privatepath"
)

const deviceLoginPresentationDirectory = "auth-presentations"

// deviceLoginPresentation is progress metadata for hosts that render the Shop-
// owned QR image and authorization link together before waiting for completion.
type deviceLoginPresentation struct {
	ImagePath        string `json:"imagePath,omitempty"`
	ImageChatSrc     string `json:"imageChatSrc,omitempty"`
	AltText          string `json:"altText,omitempty"`
	AuthorizationURL string `json:"authorizationUrl"`
}

func createDeviceLoginPresentation(runtime *Runtime, authorization api.DeviceAuthorization, sourceImage []byte) (deviceLoginPresentation, error) {
	authorizationURL := authorization.VerificationURIComplete
	imageURL := authorization.WechatMPQRCodeURL
	if authorization.LoginPresentation != nil {
		authorizationURL = authorization.LoginPresentation.AuthorizationURL
		imageURL = authorization.LoginPresentation.ImageURL
	}
	presentation := deviceLoginPresentation{
		AltText:          "ViceMe 登录二维码",
		AuthorizationURL: authorizationURL,
	}
	if imageURL == "" {
		return presentation, errors.New("device authorization did not include a direct WeChat QR image")
	}
	pngBytes, err := normalizeLoginQRCode(sourceImage)
	if err != nil {
		return presentation, err
	}
	directory := filepath.Join(runtime.configBase, deviceLoginPresentationDirectory)
	if _, err := privatepath.EnsureDirectory(directory); err != nil {
		return presentation, fmt.Errorf("create device login presentation directory: %w", err)
	}
	digest := sha256.Sum256([]byte(authorization.DeviceCode))
	filename := filepath.Join(directory, "login-"+hex.EncodeToString(digest[:16])+".png")
	if err := privatefile.Write(filename, pngBytes, ".login-qr-*.tmp"); err != nil {
		return presentation, fmt.Errorf("write device login QR image: %w", err)
	}
	absolutePath, err := filepath.Abs(filename)
	if err != nil {
		_ = os.Remove(filename)
		return presentation, fmt.Errorf("resolve device login presentation path: %w", err)
	}
	presentation.ImagePath = absolutePath
	// WorkBuddy accepts HTTPS images in Markdown, while its chat renderer does
	// not reliably load local-file:// URLs. The URL is owned by Shop; the CLI
	// validates the same bytes before asking the host to render it.
	presentation.ImageChatSrc = imageURL
	return presentation, nil
}

func normalizeLoginQRCode(source []byte) ([]byte, error) {
	if len(source) == 0 {
		return nil, errors.New("direct WeChat QR image is empty")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("decode login QR image metadata: %w", err)
	}
	if config.Width < 64 || config.Height < 64 || config.Width > 2048 || config.Height > 2048 {
		return nil, fmt.Errorf("login QR image dimensions are outside the supported range: %dx%d", config.Width, config.Height)
	}
	decoded, _, err := image.Decode(bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("decode login QR image: %w", err)
	}
	var normalized bytes.Buffer
	if err := png.Encode(&normalized, decoded); err != nil {
		return nil, fmt.Errorf("encode login QR image as PNG: %w", err)
	}
	return normalized.Bytes(), nil
}

func removeDeviceLoginPresentation(presentation deviceLoginPresentation) error {
	if presentation.ImagePath == "" {
		return nil
	}
	if err := os.Remove(presentation.ImagePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func writeHumanLoginStart(writer io.Writer, authorization api.DeviceAuthorization, presentation deviceLoginPresentation) {
	if presentation.AuthorizationURL == "" {
		presentation.AuthorizationURL = authorization.VerificationURIComplete
	}
	encoded, err := json.Marshal(presentation)
	if err == nil {
		_, _ = fmt.Fprintf(writer, "VICEME_LOGIN_QR_PRESENTATION=%s\n", encoded)
	}
	_, _ = fmt.Fprintln(writer, "Waiting for authorization...")
}
