package command

import (
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const defaultAccessConfigPath = ".viceme/access.yaml"

var accessWorkKeyPattern = regexp.MustCompile(`^wrk_[A-Za-z0-9_-]{4,124}$`)

type accessConfig struct {
	SchemaVersion int                            `yaml:"schemaVersion"`
	WorkKey       string                         `yaml:"workKey"`
	ProfileID     string                         `yaml:"profileId"`
	Region        string                         `yaml:"region"`
	DisplayName   string                         `yaml:"displayName"`
	Features      map[string]accessFeatureConfig `yaml:"features"`
	Status        string                         `yaml:"status"`
	ConfigVersion int                            `yaml:"configVersion"`
}

type legacyAccessConfig struct {
	SchemaVersion int                            `yaml:"schemaVersion"`
	WorkKey       string                         `yaml:"workKey"`
	Region        string                         `yaml:"region"`
	DisplayName   string                         `yaml:"displayName"`
	PriceCents    *int                           `yaml:"priceCents"`
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
	command := &cobra.Command{Use: "access", Short: "Configure hosted danmaku access"}
	command.AddCommand(newAccessInitCommand(runtime))
	command.AddCommand(newAccessInspectCommand(runtime))
	command.AddCommand(newAccessApplyCommand(runtime))
	return command
}

func newAccessInitCommand(runtime *Runtime) *cobra.Command {
	var filename string
	var displayName string
	var danmaku bool
	var websitePath string
	var creatorDisplayName string
	command := &cobra.Command{
		Use:   "init",
		Short: "Publish a website and activate hosted danmaku",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(displayName) == "" {
				return output.Validation("SDK_WORK_NAME_REQUIRED", "--name is required")
			}
			if !danmaku {
				return output.Validation("DANMAKU_FLAG_REQUIRED", "--danmaku is required")
			}
			if _, err := os.Stat(filename); err == nil {
				return output.Validation("ACCESS_CONFIG_EXISTS", "access config already exists").WithHint("use 'viceme access inspect' or choose another --config path")
			} else if !errors.Is(err, os.ErrNotExist) {
				return output.Validation("ACCESS_CONFIG_UNAVAILABLE", "access config path is unavailable")
			}
			if _, err := danmakuScriptURL(runtime); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
				return output.Internal("ACCESS_CONFIG_DIRECTORY_FAILED", "could not create access config directory", err)
			}
			work, _, err := publishWebsite(command.Context(), runtime, publishWebsiteInput{
				SourcePath: websitePath, DisplayName: displayName,
				CreatorDisplayName: creatorDisplayName,
			})
			if err != nil {
				return err
			}
			if err := validateSdkWorkBinding(work, work.WorkKey); err != nil {
				return err
			}
			features := map[string]accessFeatureConfig{
				"danmaku": {
					Title:  "弹幕",
					Policy: accessFeaturePolicy{Type: "PUBLIC"},
				},
			}
			config := accessConfig{
				SchemaVersion: 1,
				WorkKey:       work.WorkKey,
				ProfileID:     runtime.profile.ID,
				Region:        string(runtime.region),
				DisplayName:   work.DisplayName,
				Features:      features,
				Status:        "ACTIVE",
				ConfigVersion: work.ConfigVersion,
			}
			if err := validateAccessConfig(config); err != nil {
				return err
			}
			if err := writeAccessConfig(filename, config); err != nil {
				return output.Internal("ACCESS_CONFIG_WRITE_FAILED", "website was published but the local access config could not be written", err).WithDetails(map[string]any{"workKey": work.WorkKey})
			}
			work, err = runtime.client().ApplySdkWork(command.Context(), config.WorkKey, config.applyRequest())
			if err != nil {
				return err
			}
			if err := validateSdkWorkBinding(work, config.WorkKey); err != nil {
				return err
			}
			config.ConfigVersion = work.ConfigVersion
			if err := writeAccessConfigReplacing(filename, config); err != nil {
				return output.Internal("ACCESS_CONFIG_WRITE_FAILED", "initial remote config was applied but configVersion could not be updated locally", err).WithDetails(map[string]any{"workKey": work.WorkKey, "configVersion": work.ConfigVersion})
			}
			result, err := buildAccessResult(runtime, work, filename, config.WorkKey)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&filename, "config", defaultAccessConfigPath, "access config path")
	command.Flags().StringVar(&websitePath, "website", ".", "website source directory")
	command.Flags().StringVar(&displayName, "name", "", "website display name")
	command.Flags().StringVar(&creatorDisplayName, "creator-display-name", "", "creator display name used for a first website publication")
	command.Flags().BoolVar(&danmaku, "danmaku", false, "activate the public hosted danmaku capability")
	return command
}

func newAccessInspectCommand(runtime *Runtime) *cobra.Command {
	var filename string
	command := &cobra.Command{
		Use:   "inspect",
		Short: "Show the authoritative work binding and capabilities",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			config, legacy, err := readAccessConfig(filename)
			if err != nil {
				return err
			}
			if err := validateAccessProfile(runtime, config, legacy); err != nil {
				return err
			}
			if accessConfigHasActiveDanmaku(config) {
				if _, err := danmakuScriptURL(runtime); err != nil {
					return err
				}
			}
			work, err := runtime.client().GetSdkWork(command.Context(), config.WorkKey)
			if err != nil {
				return err
			}
			result, err := buildAccessResult(runtime, work, filename, config.WorkKey)
			if err != nil {
				return err
			}
			return runtime.business(result)
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
			config, legacy, err := readAccessConfig(filename)
			if err != nil {
				return err
			}
			if err := validateAccessProfile(runtime, config, legacy); err != nil {
				return err
			}
			if accessConfigHasActiveDanmaku(config) {
				if _, err := danmakuScriptURL(runtime); err != nil {
					return err
				}
			}
			work, err := runtime.client().ApplySdkWork(command.Context(), config.WorkKey, config.applyRequest())
			if err != nil {
				return err
			}
			if err := validateSdkWorkBinding(work, config.WorkKey); err != nil {
				return err
			}
			if legacy {
				config.ProfileID = runtime.profile.ID
			}
			config.ConfigVersion = work.ConfigVersion
			if err := writeAccessConfigReplacing(filename, config); err != nil {
				return output.Internal("ACCESS_CONFIG_WRITE_FAILED", "remote config was applied but the local configVersion could not be updated", err).WithDetails(map[string]any{"workKey": work.WorkKey, "configVersion": work.ConfigVersion})
			}
			result, err := buildAccessResult(runtime, work, filename, config.WorkKey)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&filename, "config", defaultAccessConfigPath, "access config path")
	return command
}

func buildAccessResult(runtime *Runtime, work api.SdkWork, configPath, expectedWorkKey string) (map[string]any, error) {
	if err := validateSdkWorkBinding(work, expectedWorkKey); err != nil {
		return nil, err
	}
	result := map[string]any{"work": work, "workKey": work.WorkKey, "configPath": configPath}
	if !sdkWorkHasDanmaku(work) {
		return result, nil
	}
	scriptURL, err := danmakuScriptURL(runtime)
	if err != nil {
		return nil, err
	}
	result["scriptUrl"] = scriptURL
	result["embedSnippet"] = fmt.Sprintf(
		"<script\n  defer src=\"%s\" data-viceme-work=\"%s\" data-viceme-region=\"%s\"\n  data-viceme-features=\"danmaku\" data-viceme-target=\"body\"\n  data-viceme-theme=\"auto\"></script>",
		html.EscapeString(scriptURL),
		html.EscapeString(work.WorkKey),
		html.EscapeString(string(runtime.region)),
	)
	return result, nil
}

func validateSdkWorkBinding(work api.SdkWork, expectedWorkKey string) error {
	if accessWorkKeyPattern.MatchString(work.WorkKey) && work.WorkKey == expectedWorkKey {
		return nil
	}
	return output.Internal(
		"SDK_WORK_RESPONSE_INVALID",
		"ViceMe returned an invalid work binding",
		fmt.Errorf("unexpected workKey in SDK Work response"),
	)
}

func danmakuScriptURL(runtime *Runtime) (string, error) {
	if runtime.apiBaseURLFromEnv {
		return "", output.Validation(
			"profile_api_base_url_conflict",
			"hosted snippets require the selected Profile to own both API and Web addresses",
		).WithHint("unset VICEME_API_BASE_URL and use a Profile with matching API and Web addresses")
	}
	webBaseURL := strings.TrimRight(runtime.profile.ResolvedWebBaseURL(), "/")
	if webBaseURL == "" {
		return "", output.Validation(
			"PROFILE_WEB_BASE_URL_REQUIRED",
			"the selected Profile has no Web address; recreate it with `viceme profile add --web-base-url`",
		)
	}
	return webBaseURL + "/viceme-sdk/v1/viceme.min.js", nil
}

func sdkWorkHasDanmaku(work api.SdkWork) bool {
	if work.Status != "ACTIVE" {
		return false
	}
	for _, capability := range work.Capabilities {
		if capability == "danmaku" {
			return true
		}
	}
	for _, feature := range work.Features {
		if feature.FeatureKey == "danmaku" && feature.Status == "ACTIVE" && feature.Policy.Type == "PUBLIC" {
			return true
		}
	}
	return false
}

func accessConfigHasActiveDanmaku(config accessConfig) bool {
	feature, exists := config.Features["danmaku"]
	return exists && config.Status == "ACTIVE" && feature.Policy.Type == "PUBLIC" &&
		(feature.Status == "" || feature.Status == "ACTIVE")
}

func validateAccessProfile(runtime *Runtime, config accessConfig, legacy bool) error {
	if config.Region != string(runtime.region) || (!legacy && config.ProfileID != runtime.profile.ID) {
		return output.Validation(
			"ACCESS_PROFILE_MISMATCH",
			"access config does not belong to the selected CLI Profile",
		).WithHint("rerun the command with the Profile that created this access config")
	}
	return nil
}

func (config accessConfig) applyRequest() api.ApplySdkWorkRequest {
	feature := config.Features["danmaku"]
	status := feature.Status
	if status == "" {
		status = "ACTIVE"
	}
	return api.ApplySdkWorkRequest{
		ExpectedConfigVersion: config.ConfigVersion,
		DisplayName:           config.DisplayName,
		PriceCents:            nil,
		Features: []api.SdkWorkFeatureConfig{{
			FeatureKey: "danmaku",
			Title:      feature.Title,
			Policy:     api.SdkWorkFeaturePolicy{Type: feature.Policy.Type},
			Status:     status,
		}},
		Status: config.Status,
	}
}

func readAccessConfig(filename string) (accessConfig, bool, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return accessConfig{}, false, output.Validation("ACCESS_CONFIG_READ_FAILED", "could not read access config")
	}
	var config accessConfig
	decodeErr := decodeAccessConfig(data, &config)
	var validationErr error
	if decodeErr == nil {
		validationErr = validateAccessConfig(config)
		if validationErr == nil {
			return config, false, nil
		}
	}
	var legacy legacyAccessConfig
	if legacyErr := decodeAccessConfig(data, &legacy); legacyErr == nil {
		if err := validateLegacyAccessConfig(legacy); err != nil {
			return accessConfig{}, false, err
		}
		return legacy.current(), true, nil
	}
	if decodeErr != nil {
		return accessConfig{}, false, output.Validation("ACCESS_CONFIG_INVALID", fmt.Sprintf("invalid access config: %v", decodeErr))
	}
	return accessConfig{}, false, validationErr
}

func decodeAccessConfig(data []byte, destination any) error {
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	return decoder.Decode(destination)
}

func (config legacyAccessConfig) current() accessConfig {
	return accessConfig{
		SchemaVersion: config.SchemaVersion,
		WorkKey:       config.WorkKey,
		Region:        config.Region,
		DisplayName:   config.DisplayName,
		Features:      config.Features,
		Status:        config.Status,
		ConfigVersion: config.ConfigVersion,
	}
}

func validateLegacyAccessConfig(config legacyAccessConfig) error {
	danmaku, hasDanmaku := config.Features["danmaku"]
	if len(config.Features) != 1 || !hasDanmaku || danmaku.Policy.Type != "PUBLIC" || config.PriceCents != nil {
		return output.Validation("ACCESS_CONFIG_MIGRATION_UNSUPPORTED", "this CLI release can only migrate a legacy access config containing one public danmaku feature and no price")
	}
	current := config.current()
	current.ProfileID = "legacy"
	return validateAccessConfig(current)
}

func validateAccessConfig(config accessConfig) error {
	if config.SchemaVersion != 1 || !accessWorkKeyPattern.MatchString(config.WorkKey) || strings.TrimSpace(config.ProfileID) == "" || config.ConfigVersion < 1 {
		return output.Validation("ACCESS_CONFIG_INVALID", "schemaVersion, workKey, profileId, or configVersion is invalid")
	}
	if config.Region != "cn" && config.Region != "global" {
		return output.Validation("ACCESS_CONFIG_INVALID", "region must be cn or global")
	}
	if strings.TrimSpace(config.DisplayName) == "" || (config.Status != "DRAFT" && config.Status != "ACTIVE" && config.Status != "DISABLED") {
		return output.Validation("ACCESS_CONFIG_INVALID", "displayName or status is invalid")
	}
	feature, exists := config.Features["danmaku"]
	if len(config.Features) != 1 || !exists || strings.TrimSpace(feature.Title) == "" {
		return output.Validation("ACCESS_CONFIG_INVALID", "exactly one danmaku feature is required")
	}
	if feature.Policy.Type != "PUBLIC" {
		return output.Validation("POLICY_TYPE_UNSUPPORTED", "only the public danmaku policy is supported")
	}
	if feature.Status != "" && feature.Status != "ACTIVE" && feature.Status != "DISABLED" {
		return output.Validation("ACCESS_CONFIG_INVALID", "feature status must be ACTIVE or DISABLED")
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
