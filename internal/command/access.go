package command

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const defaultAccessConfigPath = ".viceme/access.yaml"

var accessFeatureKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)

type accessConfig struct {
	SchemaVersion int                            `yaml:"schemaVersion"`
	WorkKey       string                         `yaml:"workKey"`
	Region        string                         `yaml:"region"`
	DisplayName   string                         `yaml:"displayName"`
	ProductSlug   *string                        `yaml:"productSlug"`
	Origins       []string                       `yaml:"origins"`
	Features      map[string]accessFeatureConfig `yaml:"features"`
	Status        string                         `yaml:"status"`
	ConfigVersion int                            `yaml:"configVersion"`
}

type accessFeatureConfig struct {
	Title  string              `yaml:"title"`
	Policy accessFeaturePolicy `yaml:"policy"`
	Status string              `yaml:"status,omitempty"`
}

type accessFeaturePolicy struct {
	Type string `yaml:"type"`
}

func newAccessCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "access", Short: "Configure creator website access"}
	command.AddCommand(newAccessInitCommand(runtime))
	command.AddCommand(newAccessInspectCommand(runtime))
	command.AddCommand(newAccessApplyCommand(runtime))
	return command
}

func newAccessInitCommand(runtime *Runtime) *cobra.Command {
	var filename string
	var displayName string
	var productSlug string
	var origin string
	command := &cobra.Command{
		Use:   "init",
		Short: "Create a creator website workKey and local access config",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(displayName) == "" {
				return output.Validation("SDK_WORK_NAME_REQUIRED", "--name is required")
			}
			if _, err := os.Stat(filename); err == nil {
				return output.Validation("ACCESS_CONFIG_EXISTS", "access config already exists").WithHint("use 'viceme access inspect' or choose another --config path")
			} else if !errors.Is(err, os.ErrNotExist) {
				return output.Validation("ACCESS_CONFIG_UNAVAILABLE", "access config path is unavailable")
			}
			if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
				return output.Internal("ACCESS_CONFIG_DIRECTORY_FAILED", "could not create access config directory", err)
			}
			work, err := runtime.client().CreateSdkWork(command.Context(), api.CreateSdkWorkRequest{DisplayName: strings.TrimSpace(displayName)})
			if err != nil {
				return err
			}
			config := accessConfig{
				SchemaVersion: 1,
				WorkKey:       work.WorkKey,
				Region:        string(runtime.region),
				DisplayName:   work.DisplayName,
				Origins:       []string{},
				Features:      map[string]accessFeatureConfig{},
				Status:        "DRAFT",
				ConfigVersion: work.ConfigVersion,
			}
			if strings.TrimSpace(productSlug) != "" {
				value := strings.TrimSpace(productSlug)
				config.ProductSlug = &value
			}
			if strings.TrimSpace(origin) != "" {
				config.Origins = []string{strings.TrimSpace(origin)}
			}
			if err := validateAccessConfig(config); err != nil {
				return err
			}
			if err := writeAccessConfig(filename, config); err != nil {
				return output.Internal("ACCESS_CONFIG_WRITE_FAILED", "workKey was created but the local access config could not be written", err).WithDetails(map[string]any{"workKey": work.WorkKey})
			}
			if config.ProductSlug != nil || len(config.Origins) > 0 {
				work, err = runtime.client().ApplySdkWork(command.Context(), config.WorkKey, config.applyRequest())
				if err != nil {
					return err
				}
				config.ConfigVersion = work.ConfigVersion
				if err := writeAccessConfigReplacing(filename, config); err != nil {
					return output.Internal("ACCESS_CONFIG_WRITE_FAILED", "initial remote config was applied but configVersion could not be updated locally", err).WithDetails(map[string]any{"workKey": work.WorkKey, "configVersion": work.ConfigVersion})
				}
			}
			return runtime.business(map[string]any{"work": work, "configPath": filename})
		},
	}
	command.Flags().StringVar(&filename, "config", defaultAccessConfigPath, "access config path")
	command.Flags().StringVar(&displayName, "name", "", "creator website display name")
	command.Flags().StringVar(&productSlug, "product", "", "optional owned SkillProduct slug")
	command.Flags().StringVar(&origin, "origin", "", "optional initial website origin")
	return command
}

func newAccessInspectCommand(runtime *Runtime) *cobra.Command {
	var filename string
	command := &cobra.Command{
		Use:   "inspect",
		Short: "Show the authoritative work binding and capabilities",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			config, err := readAccessConfig(filename)
			if err != nil {
				return err
			}
			if config.Region != string(runtime.region) {
				return output.Validation("ACCESS_REGION_MISMATCH", "access config region does not match the active CLI profile")
			}
			work, err := runtime.client().GetSdkWork(command.Context(), config.WorkKey)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{"work": work, "configPath": filename})
		},
	}
	command.Flags().StringVar(&filename, "config", defaultAccessConfigPath, "access config path")
	return command
}

func newAccessApplyCommand(runtime *Runtime) *cobra.Command {
	var filename string
	command := &cobra.Command{
		Use:   "apply",
		Short: "Validate and apply the local access config",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			config, err := readAccessConfig(filename)
			if err != nil {
				return err
			}
			if config.Region != string(runtime.region) {
				return output.Validation("ACCESS_REGION_MISMATCH", "access config region does not match the active CLI profile")
			}
			work, err := runtime.client().ApplySdkWork(command.Context(), config.WorkKey, config.applyRequest())
			if err != nil {
				return err
			}
			config.ConfigVersion = work.ConfigVersion
			if err := writeAccessConfigReplacing(filename, config); err != nil {
				return output.Internal("ACCESS_CONFIG_WRITE_FAILED", "remote config was applied but the local configVersion could not be updated", err).WithDetails(map[string]any{"workKey": work.WorkKey, "configVersion": work.ConfigVersion})
			}
			return runtime.business(map[string]any{"work": work, "configPath": filename})
		},
	}
	command.Flags().StringVar(&filename, "config", defaultAccessConfigPath, "access config path")
	return command
}

func (config accessConfig) applyRequest() api.ApplySdkWorkRequest {
	keys := make([]string, 0, len(config.Features))
	for key := range config.Features {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	features := make([]api.SdkWorkFeatureConfig, 0, len(keys))
	for _, key := range keys {
		feature := config.Features[key]
		status := feature.Status
		if status == "" {
			status = "ACTIVE"
		}
		features = append(features, api.SdkWorkFeatureConfig{
			FeatureKey: key,
			Title:      feature.Title,
			Policy:     api.SdkWorkFeaturePolicy{Type: feature.Policy.Type},
			Status:     status,
		})
	}
	return api.ApplySdkWorkRequest{
		ExpectedConfigVersion: config.ConfigVersion,
		DisplayName:           config.DisplayName,
		ProductSlug:           config.ProductSlug,
		Origins:               config.Origins,
		Features:              features,
		Status:                config.Status,
	}
}

func readAccessConfig(filename string) (accessConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return accessConfig{}, output.Validation("ACCESS_CONFIG_READ_FAILED", "could not read access config")
	}
	var config accessConfig
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return accessConfig{}, output.Validation("ACCESS_CONFIG_INVALID", fmt.Sprintf("invalid access config: %v", err))
	}
	if err := validateAccessConfig(config); err != nil {
		return accessConfig{}, err
	}
	return config, nil
}

func validateAccessConfig(config accessConfig) error {
	if config.SchemaVersion != 1 || !regexp.MustCompile(`^wrk_[A-Za-z0-9_-]{4,124}$`).MatchString(config.WorkKey) || config.ConfigVersion < 1 {
		return output.Validation("ACCESS_CONFIG_INVALID", "schemaVersion, workKey, or configVersion is invalid")
	}
	if config.Region != "cn" && config.Region != "global" {
		return output.Validation("ACCESS_CONFIG_INVALID", "region must be cn or global")
	}
	if strings.TrimSpace(config.DisplayName) == "" || (config.Status != "DRAFT" && config.Status != "ACTIVE" && config.Status != "DISABLED") {
		return output.Validation("ACCESS_CONFIG_INVALID", "displayName or status is invalid")
	}
	for _, origin := range config.Origins {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return output.Validation("ACCESS_CONFIG_INVALID", "origins must be absolute origins without paths, query, or fragment")
		}
		if parsed.Path != "" && parsed.Path != "/" {
			return output.Validation("ACCESS_CONFIG_INVALID", "origins must not include a path")
		}
	}
	purchasePolicy := false
	for key, feature := range config.Features {
		if !accessFeatureKeyPattern.MatchString(key) || strings.TrimSpace(feature.Title) == "" {
			return output.Validation("ACCESS_CONFIG_INVALID", "feature keys or titles are invalid")
		}
		switch feature.Policy.Type {
		case "FOLLOW_OWNER":
		case "PURCHASE_BOUND_PRODUCT", "PURCHASE_ANY_OWNER_PRODUCT":
			purchasePolicy = true
		default:
			return output.Validation("POLICY_TYPE_UNSUPPORTED", "feature policy is not supported in this CLI version")
		}
		if feature.Status != "" && feature.Status != "ACTIVE" && feature.Status != "DISABLED" {
			return output.Validation("ACCESS_CONFIG_INVALID", "feature status must be ACTIVE or DISABLED")
		}
	}
	if purchasePolicy && (config.ProductSlug == nil || strings.TrimSpace(*config.ProductSlug) == "") {
		return output.Validation("WORK_PRODUCT_NOT_BOUND", "purchase policies require productSlug")
	}
	return nil
}

func writeAccessConfig(filename string, config accessConfig) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func writeAccessConfigReplacing(filename string, config accessConfig) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".access-*.yaml")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, filename)
}
