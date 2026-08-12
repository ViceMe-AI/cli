package paymentconfig

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const relativePath = ".viceme/payment.yaml"

type Config struct {
	SchemaVersion   int    `yaml:"schemaVersion" json:"schemaVersion"`
	CapabilitySpace string `yaml:"capabilitySpaceId" json:"capabilitySpaceId"`
	ApplicationID   string `yaml:"applicationId" json:"applicationId"`
	ApplicationSlug string `yaml:"applicationSlug" json:"applicationSlug"`
	Environment     string `yaml:"defaultEnvironment" json:"defaultEnvironment"`
	MarketRegion    string `yaml:"marketRegion" json:"marketRegion"`
	EnvironmentID   string `yaml:"environmentId" json:"environmentId"`
	InstallationID  string `yaml:"installationId" json:"installationId"`
	PaymentAPIKeyID string `yaml:"paymentApiKeyId,omitempty" json:"paymentApiKeyId,omitempty"`
}

func Path(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path is not a directory: %s", absolute)
	}
	return filepath.Join(absolute, filepath.FromSlash(relativePath)), nil
}

func Load(root string) (Config, string, error) {
	filename, err := Path(root)
	if err != nil {
		return Config{}, "", err
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return Config{}, filename, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var configured Config
	if err := decoder.Decode(&configured); err != nil {
		return Config{}, filename, err
	}
	if err := configured.Validate(); err != nil {
		return Config{}, filename, err
	}
	return configured, filename, nil
}

func Save(root string, configured Config) (string, error) {
	if err := configured.Validate(); err != nil {
		return "", err
	}
	filename, err := Path(root)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return "", err
	}
	data, err := yaml.Marshal(configured)
	if err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".payment-*.yaml")
	if err != nil {
		return "", err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return "", err
	}
	return filename, nil
}

func (configured Config) Validate() error {
	if configured.SchemaVersion != 1 {
		return errors.New("payment config schemaVersion must be 1")
	}
	for name, value := range map[string]string{
		"capabilitySpaceId": configured.CapabilitySpace,
		"applicationId":     configured.ApplicationID,
		"applicationSlug":   configured.ApplicationSlug,
		"environmentId":     configured.EnvironmentID,
		"installationId":    configured.InstallationID,
		"marketRegion":      configured.MarketRegion,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("payment config %s is required", name)
		}
	}
	if configured.Environment != "sandbox" {
		return errors.New("only the sandbox Payment environment is currently supported")
	}
	if configured.MarketRegion != "CN" && configured.MarketRegion != "GLOBAL" {
		return errors.New("payment config marketRegion must be CN or GLOBAL")
	}
	return nil
}
