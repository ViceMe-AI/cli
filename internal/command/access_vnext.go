package command

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	accessVNextSchemaVersion = 3
	defaultAccessConfigPath  = ".viceme/access.yaml"
)

var (
	accessWorkKeyPattern        = regexp.MustCompile(`^wrk_[A-Za-z0-9_-]{4,124}$`)
	accessWorkFeatureKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)
)

type accessFeatureConfig struct {
	Title      string              `yaml:"title"`
	Policy     accessFeaturePolicy `yaml:"policy"`
	PriceCents *int                `yaml:"priceCents,omitempty"`
	Status     string              `yaml:"status,omitempty"`
}

type accessFeaturePolicy struct {
	Type string `yaml:"type"`
}

type profileAuthority struct {
	APIBaseURL string
	WebBaseURL string
	Region     config.Region
}

type accessVNextConfig struct {
	SchemaVersion     int                            `yaml:"schemaVersion"`
	APIBaseURL        string                         `yaml:"apiBaseUrl"`
	WebBaseURL        string                         `yaml:"webBaseUrl"`
	Region            string                         `yaml:"region"`
	MerchantAccountID string                         `yaml:"merchantAccountId"`
	WorkID            string                         `yaml:"workId"`
	WorkKey           string                         `yaml:"workKey"`
	HostedFeatures    []string                       `yaml:"hostedFeatures,omitempty"`
	Features          map[string]accessFeatureConfig `yaml:"features,omitempty"`
	ConfigVersion     int                            `yaml:"configVersion"`
}

func newAccessVNextCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "access", Short: "Configure access for a published website Work"}
	command.AddCommand(newAccessVNextInitCommand(runtime))
	command.AddCommand(newAccessVNextInspectCommand(runtime))
	command.AddCommand(newAccessVNextApplyCommand(runtime))
	command.AddCommand(newAccessVNextDisableCommand(runtime))
	command.AddCommand(newAccessVNextListCommand(runtime))
	return command
}

func newAccessVNextInitCommand(runtime *Runtime) *cobra.Command {
	var filename, workID, merchantID string
	var danmaku bool
	var prices, follow, purchase []string
	command := &cobra.Command{
		Use:   "init",
		Short: "Configure access for an already published website",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			authority, err := runtime.resolveProfileAuthority()
			if err != nil {
				return err
			}
			merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantID)
			if err != nil {
				return err
			}
			work, err := runtime.client().GetMerchantWork(command.Context(), workID, merchant.ID)
			if err != nil {
				return err
			}
			if work.Kind != "WEBSITE" || work.Status != "PUBLISHED" {
				return output.Policy("WEBSITE_PUBLICATION_REQUIRED", "access can only be attached to a published WEBSITE Work").WithDetails(map[string]any{"workId": workID, "status": work.Status})
			}
			quick, err := buildQuickAccessFeatures(follow, purchase, prices)
			if err != nil {
				return err
			}
			hosted := []string{}
			if danmaku {
				hosted = append(hosted, "danmaku")
			}
			if len(hosted) == 0 && len(quick) == 0 {
				return output.Validation("ACCESS_FEATURE_REQUIRED", "configure --danmaku, --follow, or --purchase")
			}
			if _, statErr := os.Stat(filename); statErr == nil {
				return output.Validation("ACCESS_CONFIG_EXISTS", "access config already exists").WithHint("use 'viceme access inspect' or 'viceme access apply'")
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return output.Validation("ACCESS_CONFIG_UNAVAILABLE", "access config path is unavailable")
			}
			request := api.CreateWorkSdkAccessRequest{
				MerchantAccountID: merchant.ID,
				Features:          hosted,
				AccessFeatures:    accessVNextFeatures(quick),
			}
			remote, err := runtime.client().CreateWorkSdkAccess(command.Context(), workID, request)
			if err != nil {
				return err
			}
			local := accessVNextConfig{
				SchemaVersion: accessVNextSchemaVersion, APIBaseURL: authority.APIBaseURL,
				WebBaseURL: authority.WebBaseURL, Region: string(authority.Region),
				MerchantAccountID: merchant.ID, WorkID: workID, WorkKey: remote.WorkKey,
				HostedFeatures: hosted, Features: quick, ConfigVersion: remote.ConfigVersion,
			}
			if err := writeAccessVNextConfig(filename, local); err != nil {
				return output.Internal("ACCESS_CONFIG_WRITE_FAILED", "remote access was created but its local config could not be written", err).WithDetails(map[string]any{"workId": workID, "workKey": remote.WorkKey})
			}
			return runtime.business(accessVNextResult(local, remote, filename))
		},
	}
	command.Flags().StringVar(&filename, "config", defaultAccessConfigPath, "access config path")
	command.Flags().StringVar(&workID, "work", "", "published Website Work ID")
	command.Flags().StringVar(&merchantID, "merchant", "", "merchant account ID when more than one is available")
	command.Flags().BoolVar(&danmaku, "danmaku", false, "activate hosted danmaku")
	command.Flags().StringArrayVar(&prices, "price-minor", nil, "price in fen; repeat once per --purchase or provide one shared price")
	command.Flags().StringArrayVar(&follow, "follow", nil, "FOLLOW_OWNER feature as key or key=title (repeatable)")
	command.Flags().StringArrayVar(&purchase, "purchase", nil, "WORK_ENTITLEMENT feature as key or key=title (repeatable)")
	_ = command.MarkFlagRequired("work")
	return command
}

func newAccessVNextInspectCommand(runtime *Runtime) *cobra.Command {
	var filename string
	command := &cobra.Command{Use: "inspect", Short: "Show local and authoritative remote access configuration", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		local, err := readAccessVNextConfig(filename)
		if err != nil {
			return err
		}
		if err := runtime.requireAccessVNextAuthority(local); err != nil {
			return err
		}
		remote, err := runtime.client().GetWorkSdkAccess(command.Context(), local.WorkID, local.MerchantAccountID)
		if err != nil {
			return err
		}
		return runtime.business(accessVNextResult(local, remote, filename))
	}}
	command.Flags().StringVar(&filename, "config", defaultAccessConfigPath, "access config path")
	return command
}

func newAccessVNextApplyCommand(runtime *Runtime) *cobra.Command {
	var filename string
	command := &cobra.Command{Use: "apply", Short: "Apply local website access configuration", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		local, err := readAccessVNextConfig(filename)
		if err != nil {
			return err
		}
		if err := runtime.requireAccessVNextAuthority(local); err != nil {
			return err
		}
		remote, err := runtime.client().UpdateWorkSdkAccess(command.Context(), local.WorkID, api.UpdateWorkSdkAccessRequest{
			MerchantAccountID: local.MerchantAccountID, ExpectedConfigVersion: local.ConfigVersion,
			Features: local.HostedFeatures, AccessFeatures: accessVNextFeatures(local.Features),
		})
		if err != nil {
			return err
		}
		local.ConfigVersion = remote.ConfigVersion
		local.WorkKey = remote.WorkKey
		if err := writeAccessVNextConfig(filename, local); err != nil {
			return output.Internal("ACCESS_CONFIG_WRITE_FAILED", "remote access was updated but configVersion could not be saved", err)
		}
		return runtime.business(accessVNextResult(local, remote, filename))
	}}
	command.Flags().StringVar(&filename, "config", defaultAccessConfigPath, "access config path")
	return command
}

func newAccessVNextDisableCommand(runtime *Runtime) *cobra.Command {
	var filename string
	var yes bool
	command := &cobra.Command{Use: "disable", Short: "Disable access without deleting the published Work", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		local, err := readAccessVNextConfig(filename)
		if err != nil {
			return err
		}
		if !yes {
			return output.Confirmation("ACCESS_DISABLE_CONFIRMATION_REQUIRED", "pass --yes to disable this website access configuration").WithDetails(map[string]any{"workId": local.WorkID, "workKey": local.WorkKey})
		}
		if err := runtime.requireAccessVNextAuthority(local); err != nil {
			return err
		}
		remote, err := runtime.client().DisableWorkSdkAccess(command.Context(), local.WorkID, local.MerchantAccountID)
		if err != nil {
			return err
		}
		local.ConfigVersion = remote.ConfigVersion
		local.WorkKey = remote.WorkKey
		if err := writeAccessVNextConfig(filename, local); err != nil {
			return output.Internal("ACCESS_CONFIG_WRITE_FAILED", "remote access was disabled but configVersion could not be saved", err)
		}
		return runtime.business(accessVNextResult(local, remote, filename))
	}}
	command.Flags().StringVar(&filename, "config", defaultAccessConfigPath, "access config path")
	command.Flags().BoolVar(&yes, "yes", false, "confirm disabling access")
	return command
}

func newAccessVNextListCommand(runtime *Runtime) *cobra.Command {
	var merchantID string
	command := &cobra.Command{Use: "list", Short: "List website access configurations", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		if _, err := runtime.resolveProfileAuthority(); err != nil {
			return err
		}
		merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantID)
		if err != nil {
			return err
		}
		items, err := runtime.client().ListWorkSdkAccesses(command.Context(), merchant.ID)
		if err != nil {
			return err
		}
		return runtime.business(items)
	}}
	command.Flags().StringVar(&merchantID, "merchant", "", "merchant account ID when more than one is available")
	return command
}

func buildQuickAccessFeatures(follow, purchase, rawPrices []string) (map[string]accessFeatureConfig, error) {
	prices, err := parsePurchasePrices(purchase, rawPrices)
	if err != nil {
		return nil, err
	}
	features := make(map[string]accessFeatureConfig, len(follow)+len(purchase))
	groups := []struct {
		values []string
		policy string
	}{
		{values: follow, policy: "FOLLOW_OWNER"},
		{values: purchase, policy: "WORK_ENTITLEMENT"},
	}
	for _, group := range groups {
		for index, value := range group.values {
			key, title, err := parseAccessFeatureSpec(value)
			if err != nil {
				return nil, err
			}
			if _, exists := features[key]; exists {
				return nil, output.Validation("ACCESS_FEATURE_DUPLICATE", fmt.Sprintf("feature %q is configured more than once", key))
			}
			feature := accessFeatureConfig{Title: title, Policy: accessFeaturePolicy{Type: group.policy}, Status: "ACTIVE"}
			if group.policy == "WORK_ENTITLEMENT" {
				price := prices[index]
				feature.PriceCents = &price
			}
			features[key] = feature
		}
	}
	return features, nil
}

func parsePurchasePrices(purchase, rawPrices []string) ([]int, error) {
	if len(purchase) == 0 {
		if len(rawPrices) > 0 {
			return nil, output.Validation("ACCESS_CONFIG_INVALID", "--price-minor requires at least one --purchase")
		}
		return nil, nil
	}
	if len(rawPrices) != 1 && len(rawPrices) != len(purchase) {
		return nil, output.Validation("WORK_PRICE_REQUIRED", "provide one shared --price-minor or one price for each --purchase")
	}
	prices := make([]int, len(purchase))
	for index := range purchase {
		raw := rawPrices[0]
		if len(rawPrices) > 1 {
			raw = rawPrices[index]
		}
		price, err := strconv.Atoi(raw)
		if err != nil || price <= 0 {
			return nil, output.Validation("WORK_PRICE_REQUIRED", "purchase feature prices must be positive integers")
		}
		prices[index] = price
	}
	return prices, nil
}

func parseAccessFeatureSpec(raw string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), "=", 2)
	key := strings.TrimSpace(parts[0])
	title := key
	if len(parts) == 2 {
		title = strings.TrimSpace(parts[1])
	}
	if !accessWorkFeatureKeyPattern.MatchString(key) || title == "" {
		return "", "", output.Validation("ACCESS_FEATURE_INVALID", "feature must use key or key=title")
	}
	return key, title, nil
}

func (runtime *Runtime) resolveProfileAuthority() (profileAuthority, error) {
	if runtime.apiBaseURLFromEnv {
		return profileAuthority{}, output.Policy(
			"PROFILE_AUTHORITY_OVERRIDE_ACTIVE",
			"website integration commands are disabled while VICEME_API_BASE_URL overrides the selected Profile",
		).WithHint("unset VICEME_API_BASE_URL and use a Profile containing apiBaseUrl, webBaseUrl, and marketRegion")
	}
	if err := config.ValidateProfileAuthority(runtime.profile); err != nil {
		return profileAuthority{}, output.Validation("PROFILE_AUTHORITY_INVALID", err.Error())
	}
	apiBaseURL, err := config.NormalizeAPIBaseURL(runtime.profile.ResolvedAPIBaseURL())
	if err != nil {
		return profileAuthority{}, output.Validation("PROFILE_API_BASE_URL_INVALID", err.Error())
	}
	activeAPIBaseURL, err := config.NormalizeAPIBaseURL(runtime.apiBaseURL)
	if err != nil {
		return profileAuthority{}, output.Validation("PROFILE_API_BASE_URL_INVALID", err.Error())
	}
	if activeAPIBaseURL != apiBaseURL {
		return profileAuthority{}, output.Policy(
			"PROFILE_AUTHORITY_OVERRIDE_ACTIVE",
			"website integration commands require the selected Profile's complete authority",
		).WithHint("remove the API endpoint override and use the selected Profile")
	}
	webBaseURL, err := config.NormalizeWebBaseURL(runtime.profile.ResolvedWebBaseURL())
	if err != nil {
		return profileAuthority{}, output.Validation("PROFILE_WEB_BASE_URL_REQUIRED", "the selected Profile has no valid Web address")
	}
	marketRegion, err := runtime.profile.ResolvedMarketRegion()
	if err != nil {
		return profileAuthority{}, output.Validation("PROFILE_MARKET_REGION_REQUIRED", err.Error())
	}
	return profileAuthority{APIBaseURL: apiBaseURL, WebBaseURL: webBaseURL, Region: marketRegion}, nil
}

func accessVNextFeatures(features map[string]accessFeatureConfig) []api.WorkAccessFeature {
	keys := make([]string, 0, len(features))
	for key := range features {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]api.WorkAccessFeature, 0, len(keys))
	for _, key := range keys {
		feature := features[key]
		var price *api.WorkAccessPrice
		if feature.PriceCents != nil {
			price = &api.WorkAccessPrice{Currency: "CNY", AmountCents: *feature.PriceCents}
		}
		status := feature.Status
		if status == "" {
			status = "ACTIVE"
		}
		result = append(result, api.WorkAccessFeature{FeatureKey: key, Title: feature.Title, PolicyType: feature.Policy.Type, Price: price, Status: status})
	}
	return result
}

func readAccessVNextConfig(filename string) (accessVNextConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return accessVNextConfig{}, output.Validation("ACCESS_CONFIG_READ_FAILED", "could not read access config")
	}
	var header struct {
		SchemaVersion int `yaml:"schemaVersion"`
	}
	if yaml.Unmarshal(data, &header) != nil || header.SchemaVersion != accessVNextSchemaVersion {
		return accessVNextConfig{}, output.Validation("LEGACY_ACCESS_CONFIG_UNSUPPORTED", "this access config uses the retired SDK Work model").WithHint("run 'viceme access init' after publishing the website with the current flow")
	}
	var local accessVNextConfig
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&local); err != nil {
		return accessVNextConfig{}, output.Validation("ACCESS_CONFIG_INVALID", fmt.Sprintf("invalid access config: %v", err))
	}
	if local.WorkID == "" || local.MerchantAccountID == "" || !accessWorkKeyPattern.MatchString(local.WorkKey) || local.ConfigVersion < 1 {
		return accessVNextConfig{}, output.Validation("ACCESS_CONFIG_INVALID", "workId, merchantAccountId, workKey, or configVersion is invalid")
	}
	if len(local.HostedFeatures) == 0 && len(local.Features) == 0 {
		return accessVNextConfig{}, output.Validation("ACCESS_CONFIG_INVALID", "at least one hosted or access feature is required")
	}
	for key, feature := range local.Features {
		if !accessWorkFeatureKeyPattern.MatchString(key) || strings.TrimSpace(feature.Title) == "" {
			return accessVNextConfig{}, output.Validation("ACCESS_CONFIG_INVALID", "feature keys or titles are invalid")
		}
		if (feature.Policy.Type == "WORK_ENTITLEMENT") != (feature.PriceCents != nil) {
			return accessVNextConfig{}, output.Validation("ACCESS_CONFIG_INVALID", "only WORK_ENTITLEMENT features require priceCents")
		}
		if feature.Policy.Type != "PUBLIC" && feature.Policy.Type != "FOLLOW_OWNER" && feature.Policy.Type != "WORK_ENTITLEMENT" {
			return accessVNextConfig{}, output.Validation("ACCESS_CONFIG_INVALID", "unsupported feature policy")
		}
	}
	return local, nil
}

func writeAccessVNextConfig(filename string, local accessVNextConfig) error {
	data, err := yaml.Marshal(local)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".access-*.yaml")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, filename)
}

func (runtime *Runtime) requireAccessVNextAuthority(local accessVNextConfig) error {
	authority, err := runtime.resolveProfileAuthority()
	if err != nil {
		return err
	}
	if local.APIBaseURL != authority.APIBaseURL || local.WebBaseURL != authority.WebBaseURL || local.Region != string(authority.Region) {
		return output.Policy("ACCESS_PROFILE_MISMATCH", "access config belongs to a different Profile authority")
	}
	return nil
}

func accessVNextResult(local accessVNextConfig, remote api.WorkSdkAccess, filename string) map[string]any {
	return map[string]any{
		"access": remote, "workId": remote.WorkID, "workKey": remote.WorkKey,
		"configPath": filename, "localConfigVersion": local.ConfigVersion,
		"remoteConfigVersion": remote.ConfigVersion,
		"scriptUrl":           strings.TrimRight(local.WebBaseURL, "/") + "/viceme-sdk/v1/viceme.min.js",
	}
}
