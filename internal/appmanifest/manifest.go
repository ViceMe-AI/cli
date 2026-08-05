package appmanifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/semver"
)

const SchemaVersion = 1

var (
	ErrNotFound     = errors.New("ViceMe App manifest not found")
	uuidPattern     = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	publishKeyRegex = regexp.MustCompile(`^app_pk_(test|live)_[A-Za-z0-9_-]{32,}$`)
)

type Capability struct {
	ContractVersion string `json:"contractVersion"`
	SDKPackage      string `json:"sdkPackage"`
	SDKVersion      string `json:"sdkVersion"`
}

type Manifest struct {
	SchemaVersion  int                   `json:"schemaVersion"`
	AppID          string                `json:"appId"`
	HostingMode    string                `json:"hostingMode"`
	Environment    string                `json:"environment"`
	PublishableKey string                `json:"publishableKey"`
	Origin         string                `json:"origin,omitempty"`
	Capabilities   map[string]Capability `json:"capabilities"`
}

func Path(directory string) string {
	return filepath.Join(directory, ".viceme", "app.json")
}

func Load(directory string) (Manifest, error) {
	filename := Path(directory)
	file, err := os.Open(filename)
	if errors.Is(err, os.ErrNotExist) {
		return Manifest{}, ErrNotFound
	}
	if err != nil {
		return Manifest{}, fmt.Errorf("open App manifest: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode App manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Manifest{}, errors.New("decode App manifest: trailing JSON data")
	}
	if err := validate(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func Save(directory string, manifest Manifest) (string, error) {
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = SchemaVersion
	}
	if err := validate(manifest); err != nil {
		return "", err
	}
	directory = filepath.Clean(directory)
	manifestDirectory := filepath.Join(directory, ".viceme")
	if err := os.MkdirAll(manifestDirectory, 0o755); err != nil {
		return "", fmt.Errorf("create App manifest directory: %w", err)
	}
	temporary, err := os.CreateTemp(manifestDirectory, ".app-*.json")
	if err != nil {
		return "", fmt.Errorf("create App manifest temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return "", fmt.Errorf("set App manifest permissions: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return "", fmt.Errorf("encode App manifest: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return "", fmt.Errorf("sync App manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close App manifest: %w", err)
	}
	filename := Path(directory)
	if err := os.Rename(temporaryName, filename); err != nil {
		return "", fmt.Errorf("replace App manifest: %w", err)
	}
	committed = true
	return filename, nil
}

func validate(manifest Manifest) error {
	if manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported App manifest schemaVersion %d", manifest.SchemaVersion)
	}
	if !uuidPattern.MatchString(strings.ToLower(manifest.AppID)) {
		return errors.New("App manifest contains an invalid appId")
	}
	if manifest.HostingMode != "EXTERNAL" && manifest.HostingMode != "VICEME_HOSTED" {
		return errors.New("App manifest contains an invalid hostingMode")
	}
	if manifest.Environment != "TEST" && manifest.Environment != "LIVE" {
		return errors.New("App manifest contains an invalid environment")
	}
	if !publishKeyRegex.MatchString(manifest.PublishableKey) {
		return errors.New("App manifest contains an invalid publishableKey")
	}
	expectedKeyPrefix := "app_pk_" + strings.ToLower(manifest.Environment) + "_"
	if !strings.HasPrefix(manifest.PublishableKey, expectedKeyPrefix) {
		return errors.New("App manifest publishableKey does not match its environment")
	}
	if manifest.Origin != "" {
		parsed, err := url.Parse(manifest.Origin)
		if err != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
			return errors.New("App manifest contains an invalid origin")
		}
		canonical, err := api.NormalizeAPIOrigin(manifest.Origin)
		if err != nil || canonical != manifest.Origin {
			return errors.New("App manifest origin must be a canonical HTTPS origin or loopback HTTP origin")
		}
	}
	if manifest.Capabilities == nil {
		return errors.New("App manifest capabilities must be a JSON object")
	}
	for name, capability := range manifest.Capabilities {
		if strings.TrimSpace(name) == "" || capability.SDKPackage != "@viceme/web-sdk" {
			return errors.New("App manifest contains an invalid capability binding")
		}
		if _, err := semver.Parse(capability.ContractVersion); err != nil {
			return errors.New("App manifest contains an invalid capability contractVersion")
		}
		if _, err := semver.Parse(capability.SDKVersion); err != nil {
			return errors.New("App manifest contains an invalid capability binding")
		}
	}
	return nil
}
