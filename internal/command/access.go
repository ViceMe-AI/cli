package command

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	accessConfigSchemaVersion = 2
	defaultAccessConfigPath   = ".viceme/access.yaml"
	danmakuFeatureKey         = "danmaku"
)

var (
	accessWorkKeyPattern        = regexp.MustCompile(`^wrk_[A-Za-z0-9_-]{4,124}$`)
	accessWorkFeatureKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)
)

type accessConfig struct {
	SchemaVersion int                            `yaml:"schemaVersion"`
	APIBaseURL    string                         `yaml:"apiBaseUrl"`
	WebBaseURL    string                         `yaml:"webBaseUrl"`
	WorkKey       string                         `yaml:"workKey"`
	Region        string                         `yaml:"region"`
	DisplayName   string                         `yaml:"displayName"`
	Features      map[string]accessFeatureConfig `yaml:"features"`
	Status        string                         `yaml:"status"`
	ConfigVersion int                            `yaml:"configVersion"`
}

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

func newAccessCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "access", Short: "Configure website capabilities"}
	command.AddCommand(newAccessInitCommand(runtime))
	command.AddCommand(newAccessListCommand(runtime))
	command.AddCommand(newAccessDeleteCommand(runtime))
	command.AddCommand(newAccessInspectCommand(runtime))
	command.AddCommand(newAccessApplyCommand(runtime))
	return command
}

func newAccessInitCommand(runtime *Runtime) *cobra.Command {
	var filename string
	var displayName string
	var danmaku bool
	var workKey string
	var websitePath string
	var priceCents []string
	var followFeatures []string
	var purchaseFeatures []string
	command := &cobra.Command{
		Use:   "init",
		Short: "Publish the website when needed, then configure capabilities",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			authority, err := runtime.resolveProfileAuthority()
			if err != nil {
				return err
			}
			displayName = strings.TrimSpace(displayName)
			features, err := buildQuickAccessFeatures(followFeatures, purchaseFeatures, priceCents)
			if err != nil {
				return err
			}
			if danmaku {
				features[danmakuFeatureKey] = accessFeatureConfig{
					Title: "弹幕", Policy: accessFeaturePolicy{Type: "PUBLIC"}, Status: "ACTIVE",
				}
			}
			if len(features) == 0 {
				return output.Validation("ACCESS_FEATURE_REQUIRED", "configure --danmaku, --follow, or --purchase")
			}
			creatorAccess := len(followFeatures)+len(purchaseFeatures) > 0
			var publishedBinding websiteBinding
			if creatorAccess {
				publishedBinding, _, err = requirePublishedWebsiteBinding(websitePath)
				if err != nil {
					return err
				}
				if publishedBinding.Region != string(authority.Region) {
					return output.Validation("WEBSITE_REGION_MISMATCH", "website binding region does not match the active CLI profile")
				}
				if publishedBinding.SchemaVersion == 2 &&
					(publishedBinding.APIBaseURL != authority.APIBaseURL || publishedBinding.WebBaseURL != authority.WebBaseURL) {
					return output.Policy("WEBSITE_PROFILE_MISMATCH", "website binding belongs to a different Profile authority")
				}
				if displayName == "" {
					displayName = publishedBinding.DisplayName
				}
			}
			if displayName == "" {
				return output.Validation("ACCESS_NAME_REQUIRED", "--name is required")
			}
			workKey = strings.TrimSpace(workKey)
			if workKey != "" && !accessWorkKeyPattern.MatchString(workKey) {
				return output.Validation("WORK_KEY_INVALID", "--work-key must be a public wrk_ key")
			}
			if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
				return output.Internal("ACCESS_CONFIG_DIRECTORY_FAILED", "could not create access config directory", err)
			}
			lockPath, err := runtime.accessInitLockPath(filename, authority)
			if err != nil {
				return output.Internal("ACCESS_INIT_LOCK_FAILED", "could not scope the access initialization lock", err)
			}
			initLock := flock.New(lockPath)
			locked, err := initLock.TryLockContext(command.Context(), 25*time.Millisecond)
			if err != nil {
				return output.Internal("ACCESS_INIT_LOCK_FAILED", "could not acquire the access initialization lock", err)
			}
			if !locked {
				return output.Validation("ACCESS_INIT_LOCKED", "another process is initializing this Profile and access config")
			}
			defer initLock.Unlock()

			if _, statErr := os.Stat(filename); statErr == nil {
				local, readErr := readAccessConfig(filename)
				if readErr != nil {
					return readErr
				}
				if local.DisplayName != displayName || (workKey != "" && local.WorkKey != workKey) ||
					!accessFeatureConfigsEqual(local.Features, features) {
					return output.Validation("ACCESS_CONFIG_EXISTS", "access config already exists with a different binding").
						WithHint("use 'viceme access inspect' or choose another --config path")
				}
				if err := runtime.requireAccessAuthority(local); err != nil {
					return err
				}
				work, getErr := runtime.client().GetSdkWork(command.Context(), local.WorkKey)
				if getErr != nil {
					return getErr
				}
				if !sdkWorkMatchesAccessConfig(local, work) {
					return output.Validation("ACCESS_CONFIG_EXISTS", "access config exists but its remote Work has not committed the same configuration").
						WithHint("run 'viceme access inspect' before retrying or changing the binding")
				}
				if local.ConfigVersion != work.ConfigVersion {
					local.ConfigVersion = work.ConfigVersion
					if err := writeAccessConfigReplacing(filename, local); err != nil {
						return output.Internal("ACCESS_CONFIG_WRITE_FAILED", "the committed Work was found but configVersion could not be reconciled", err).
							WithDetails(map[string]any{"workKey": work.WorkKey, "configVersion": work.ConfigVersion})
					}
				}
				return runtime.business(accessCommandResult(local, work, filename))
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return output.Validation("ACCESS_CONFIG_UNAVAILABLE", "access config path is unavailable")
			}

			var work api.SdkWork
			if creatorAccess {
				if workKey != "" && workKey != publishedBinding.WorkKey {
					return output.Validation("WEBSITE_IDENTITY_CONFLICT", "--work-key does not match the published website binding")
				}
				work, err = runtime.client().GetSdkWork(command.Context(), publishedBinding.WorkKey)
			} else if workKey == "" {
				work, err = runtime.client().CreateSdkWork(command.Context(), api.CreateSdkWorkRequest{DisplayName: displayName})
				if err != nil {
					return sdkWorkCreationError(err)
				}
			} else {
				work, err = runtime.client().GetSdkWork(command.Context(), workKey)
				if err != nil {
					return err
				}
				if work.WorkKey != workKey || work.Status != "DRAFT" || work.ConfigVersion < 1 ||
					len(work.Features) != 0 || len(work.Capabilities) != 0 {
					return output.Policy(
						"SDK_WORK_REUSE_UNSAFE",
						"--work-key must select an owned, unconfigured DRAFT Work",
					).WithHint("run 'viceme access list', then select only the orphan created by the interrupted init or delete it explicitly")
				}
			}
			if err != nil {
				return err
			}
			local := accessConfig{
				SchemaVersion: accessConfigSchemaVersion,
				APIBaseURL:    authority.APIBaseURL,
				WebBaseURL:    authority.WebBaseURL,
				WorkKey:       work.WorkKey,
				Region:        string(authority.Region),
				DisplayName:   displayName,
				Features:      features,
				Status:        "ACTIVE",
				ConfigVersion: work.ConfigVersion,
			}
			if err := validateAccessConfig(local); err != nil {
				return err
			}
			if err := writeAccessConfig(filename, local); err != nil {
				return output.Internal("ACCESS_CONFIG_WRITE_FAILED", "SDK Work was created but the local access config could not be written", err).
					WithDetails(map[string]any{"workKey": work.WorkKey, "configVersion": work.ConfigVersion}).
					WithHint("run 'viceme access list', then remove this orphan with 'viceme access delete " + work.WorkKey + " --yes' before starting over")
			}

			work, err = runtime.client().ApplySdkWork(command.Context(), local.WorkKey, local.applyRequest())
			if err != nil {
				return err
			}
			local.ConfigVersion = work.ConfigVersion
			if err := writeAccessConfigReplacing(filename, local); err != nil {
				return output.Internal("ACCESS_CONFIG_WRITE_FAILED", "initial remote config was applied but configVersion could not be updated locally", err).
					WithDetails(map[string]any{"workKey": work.WorkKey, "configVersion": work.ConfigVersion})
			}
			return runtime.business(accessCommandResult(local, work, filename))
		},
	}
	command.Flags().StringVar(&filename, "config", defaultAccessConfigPath, "access config path")
	command.Flags().StringVar(&displayName, "name", "", "website display name")
	command.Flags().BoolVar(&danmaku, "danmaku", false, "activate the public hosted danmaku capability")
	command.Flags().StringVar(&workKey, "work-key", "", "explicit owned unconfigured DRAFT Work to recover")
	command.Flags().StringVar(&websitePath, "website", ".", "published website source directory")
	command.Flags().StringArrayVar(&priceCents, "price-minor", nil, "feature price in fen; repeat once per --purchase or provide one shared price")
	command.Flags().StringArrayVar(&followFeatures, "follow", nil, "activate FOLLOW_OWNER feature as key or key=title (repeatable)")
	command.Flags().StringArrayVar(&purchaseFeatures, "purchase", nil, "activate WORK_ENTITLEMENT feature as key or key=title (repeatable)")
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
			feature := accessFeatureConfig{
				Title: title, Policy: accessFeaturePolicy{Type: group.policy}, Status: "ACTIVE",
			}
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

func (runtime *Runtime) accessInitLockPath(filename string, authority profileAuthority) (string, error) {
	absoluteConfig, err := filepath.Abs(filename)
	if err != nil {
		return "", err
	}
	lockDirectory := filepath.Join(runtime.configBase, "locks", "access-init")
	if err := os.MkdirAll(lockDirectory, 0o700); err != nil {
		return "", err
	}
	scope := strings.Join([]string{absoluteConfig, authority.APIBaseURL, authority.WebBaseURL, string(authority.Region)}, "\x00")
	digest := sha256.Sum256([]byte(scope))
	return filepath.Join(lockDirectory, fmt.Sprintf("%x.lock", digest)), nil
}

func newAccessListCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List SDK Works owned by the authenticated account",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if _, err := runtime.resolveProfileAuthority(); err != nil {
				return err
			}
			works, err := runtime.client().ListSdkWorks(command.Context())
			if err != nil {
				return err
			}
			return runtime.business(works)
		},
	}
}

func newAccessDeleteCommand(runtime *Runtime) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:   "delete <work-key>",
		Short: "Delete an explicitly selected owned SDK Work",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			workKey := strings.TrimSpace(args[0])
			if !accessWorkKeyPattern.MatchString(workKey) {
				return output.Validation("WORK_KEY_INVALID", "work key must be a public wrk_ key")
			}
			if _, err := runtime.resolveProfileAuthority(); err != nil {
				return err
			}
			if !yes {
				return output.Confirmation(
					"SDK_WORK_DELETE_CONFIRMATION_REQUIRED",
					"deleting an SDK Work is permanent; pass --yes to confirm the exact work key",
				).WithDetails(map[string]any{"workKey": workKey})
			}
			work, err := runtime.client().DeleteSdkWork(command.Context(), workKey)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{"deleted": true, "work": work})
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "confirm permanent deletion of the selected SDK Work")
	return command
}

func sdkWorkCreationError(err error) error {
	cliError := output.AsError(err)
	unknown := cliError.Retryable
	if !unknown {
		switch cliError.Subtype {
		case "API_UNREACHABLE", "RESPONSE_READ_FAILED", "RESPONSE_TOO_LARGE", "RESPONSE_INVALID":
			unknown = true
		}
	}
	if !unknown {
		return err
	}
	recovery := output.NewError(
		output.ExitNetwork,
		"network",
		"SDK_WORK_CREATE_OUTCOME_UNKNOWN",
		"SDK Work creation outcome is unknown; do not retry access init blindly",
	).WithCause(err).WithDetails(map[string]any{"causeCode": cliError.Subtype})
	recovery.RequestID = cliError.RequestID
	return recovery.WithHint(
		"run 'viceme access list' first; delete an orphan with 'viceme access delete <work-key> --yes', or resume the selected unconfigured DRAFT with 'viceme access init --work-key <work-key> --name <name> --danmaku'",
	)
}

func newAccessInspectCommand(runtime *Runtime) *cobra.Command {
	var filename string
	command := &cobra.Command{
		Use:   "inspect",
		Short: "Show the local binding and authoritative remote Work",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			local, err := readAccessConfig(filename)
			if err != nil {
				return err
			}
			if err := runtime.requireAccessAuthority(local); err != nil {
				return err
			}
			work, err := runtime.client().GetSdkWork(command.Context(), local.WorkKey)
			if err != nil {
				return err
			}
			if sdkWorkMatchesAccessConfig(local, work) && local.ConfigVersion != work.ConfigVersion {
				if work.ConfigVersion < 1 {
					return output.Internal("SDK_WORK_RESPONSE_INVALID", "ViceMe API returned an invalid configVersion", nil)
				}
				local.ConfigVersion = work.ConfigVersion
				if err := writeAccessConfigReplacing(filename, local); err != nil {
					return output.Internal("ACCESS_CONFIG_WRITE_FAILED", "remote config matches locally, but configVersion could not be reconciled", err).
						WithDetails(map[string]any{"workKey": work.WorkKey, "configVersion": work.ConfigVersion})
				}
			}
			return runtime.business(accessCommandResult(local, work, filename))
		},
	}
	command.Flags().StringVar(&filename, "config", defaultAccessConfigPath, "access config path")
	return command
}

func newAccessApplyCommand(runtime *Runtime) *cobra.Command {
	var filename string
	command := &cobra.Command{
		Use:   "apply",
		Short: "Apply the local website capability config",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			local, err := readAccessConfig(filename)
			if err != nil {
				return err
			}
			if err := runtime.requireAccessAuthority(local); err != nil {
				return err
			}
			work, err := runtime.client().ApplySdkWork(command.Context(), local.WorkKey, local.applyRequest())
			if err != nil {
				return err
			}
			local.ConfigVersion = work.ConfigVersion
			if err := writeAccessConfigReplacing(filename, local); err != nil {
				return output.Internal("ACCESS_CONFIG_WRITE_FAILED", "remote config was applied but the local configVersion could not be updated", err).
					WithDetails(map[string]any{"workKey": work.WorkKey, "configVersion": work.ConfigVersion})
			}
			return runtime.business(accessCommandResult(local, work, filename))
		},
	}
	command.Flags().StringVar(&filename, "config", defaultAccessConfigPath, "access config path")
	return command
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
	return profileAuthority{
		APIBaseURL: apiBaseURL, WebBaseURL: webBaseURL, Region: marketRegion,
	}, nil
}

func (runtime *Runtime) requireAccessAuthority(local accessConfig) error {
	authority, err := runtime.resolveProfileAuthority()
	if err != nil {
		return err
	}
	if local.APIBaseURL != authority.APIBaseURL || local.WebBaseURL != authority.WebBaseURL ||
		local.Region != string(authority.Region) {
		return output.Policy(
			"ACCESS_PROFILE_MISMATCH",
			"access config belongs to a different Profile authority",
		).WithHint("select the original Profile; never reuse a Work binding across API, Web, or market environments")
	}
	return nil
}

func accessCommandResult(local accessConfig, work api.SdkWork, filename string) map[string]any {
	result := map[string]any{
		"work":                work,
		"workKey":             work.WorkKey,
		"configPath":          filename,
		"localConfigVersion":  local.ConfigVersion,
		"remoteConfigVersion": work.ConfigVersion,
	}
	if work.Status == "ACTIVE" && stringSliceContains(work.Capabilities, danmakuFeatureKey) {
		result["scriptUrl"] = strings.TrimRight(local.WebBaseURL, "/") + "/viceme-sdk/v1/viceme.min.js"
		result["embedSnippet"] = buildDanmakuEmbedSnippet(local.WebBaseURL, local.WorkKey, local.Region)
	}
	return result
}

func buildDanmakuEmbedSnippet(webBaseURL, workKey, region string) string {
	return fmt.Sprintf(
		`<script defer src="%s/viceme-sdk/v1/viceme.min.js" data-viceme-work="%s" data-viceme-region="%s" data-viceme-features="danmaku" data-viceme-target="body" data-viceme-theme="auto"></script>`,
		strings.TrimRight(webBaseURL, "/"), workKey, region,
	)
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sdkWorkMatchesAccessConfig(local accessConfig, work api.SdkWork) bool {
	expected := local.applyRequest()
	if work.WorkKey != local.WorkKey || work.DisplayName != expected.DisplayName || work.Status != expected.Status ||
		len(work.Features) != len(expected.Features) {
		return false
	}
	features := make(map[string]api.SdkWorkFeatureConfig, len(work.Features))
	for _, feature := range work.Features {
		if _, exists := features[feature.FeatureKey]; exists {
			return false
		}
		features[feature.FeatureKey] = feature
	}
	for _, feature := range expected.Features {
		remote, exists := features[feature.FeatureKey]
		if !exists || remote.Title != feature.Title || remote.Policy.Type != feature.Policy.Type ||
			remote.Status != feature.Status || !accessNullableIntsEqual(remote.PriceCents, feature.PriceCents) {
			return false
		}
	}
	return true
}

func (local accessConfig) applyRequest() api.ApplySdkWorkRequest {
	keys := make([]string, 0, len(local.Features))
	for key := range local.Features {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	features := make([]api.SdkWorkFeatureConfig, 0, len(keys))
	for _, key := range keys {
		feature := local.Features[key]
		status := feature.Status
		if status == "" {
			status = "ACTIVE"
		}
		features = append(features, api.SdkWorkFeatureConfig{
			FeatureKey: key,
			Title:      feature.Title,
			Policy:     api.SdkWorkFeaturePolicy{Type: feature.Policy.Type},
			PriceCents: feature.PriceCents,
			Status:     status,
		})
	}
	return api.ApplySdkWorkRequest{
		ExpectedConfigVersion: local.ConfigVersion,
		DisplayName:           local.DisplayName,
		Features:              features,
		Status:                local.Status,
	}
}

func readAccessConfig(filename string) (accessConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return accessConfig{}, output.Validation("ACCESS_CONFIG_READ_FAILED", "could not read access config")
	}
	var local accessConfig
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&local); err != nil {
		return accessConfig{}, output.Validation("ACCESS_CONFIG_INVALID", fmt.Sprintf("invalid access config: %v", err))
	}
	if err := validateAccessConfig(local); err != nil {
		return accessConfig{}, err
	}
	return local, nil
}

func validateAccessConfig(local accessConfig) error {
	if local.SchemaVersion != accessConfigSchemaVersion ||
		!accessWorkKeyPattern.MatchString(local.WorkKey) || local.ConfigVersion < 1 {
		return output.Validation("ACCESS_CONFIG_INVALID", "schemaVersion, workKey, or configVersion is invalid")
	}
	apiBaseURL, apiErr := config.NormalizeAPIBaseURL(local.APIBaseURL)
	webBaseURL, webErr := config.NormalizeWebBaseURL(local.WebBaseURL)
	if apiErr != nil || webErr != nil || apiBaseURL != local.APIBaseURL || webBaseURL != local.WebBaseURL {
		return output.Validation("ACCESS_CONFIG_INVALID", "apiBaseUrl and webBaseUrl must be canonical Profile authorities")
	}
	if local.Region != "cn" && local.Region != "global" {
		return output.Validation("ACCESS_CONFIG_INVALID", "region must be cn or global")
	}
	if err := config.ValidateProfileAuthority(config.Profile{
		APIBaseURL: local.APIBaseURL, WebBaseURL: local.WebBaseURL, MarketRegion: config.Region(local.Region),
	}); err != nil {
		return output.Validation("ACCESS_CONFIG_INVALID", "invalid Profile authority: "+err.Error())
	}
	if strings.TrimSpace(local.DisplayName) == "" || (local.Status != "DRAFT" && local.Status != "ACTIVE" && local.Status != "DISABLED") {
		return output.Validation("ACCESS_CONFIG_INVALID", "displayName or status is invalid")
	}
	if len(local.Features) == 0 {
		return output.Validation("ACCESS_CONFIG_INVALID", "at least one website capability is required")
	}
	for key, feature := range local.Features {
		if !accessWorkFeatureKeyPattern.MatchString(key) || strings.TrimSpace(feature.Title) == "" {
			return output.Validation("ACCESS_CONFIG_INVALID", "feature keys or titles are invalid")
		}
		switch feature.Policy.Type {
		case "PUBLIC":
			if key != danmakuFeatureKey || feature.PriceCents != nil {
				return output.Validation("POLICY_TYPE_UNSUPPORTED", "PUBLIC is reserved for the unpriced danmaku capability")
			}
		case "FOLLOW_OWNER":
			if key == danmakuFeatureKey {
				return output.Validation("POLICY_TYPE_UNSUPPORTED", "danmaku requires the PUBLIC policy")
			}
			if feature.PriceCents != nil {
				return output.Validation("ACCESS_CONFIG_INVALID", "follow policies must not define priceCents")
			}
		case "WORK_ENTITLEMENT":
			if key == danmakuFeatureKey {
				return output.Validation("POLICY_TYPE_UNSUPPORTED", "danmaku requires the PUBLIC policy")
			}
			if feature.PriceCents == nil || *feature.PriceCents <= 0 {
				return output.Validation("WORK_PRICE_REQUIRED", "purchase policies require a positive feature priceCents")
			}
		default:
			return output.Validation("POLICY_TYPE_UNSUPPORTED", "feature policy is not supported in this CLI version")
		}
		if feature.Status != "" && feature.Status != "ACTIVE" && feature.Status != "DISABLED" {
			return output.Validation("ACCESS_CONFIG_INVALID", "feature status must be ACTIVE or DISABLED")
		}
	}
	return nil
}

func accessFeatureConfigsEqual(left, right map[string]accessFeatureConfig) bool {
	if len(left) != len(right) {
		return false
	}
	for key, expected := range left {
		actual, exists := right[key]
		if !exists || actual.Title != expected.Title || actual.Policy != expected.Policy ||
			actual.Status != expected.Status || !accessNullableIntsEqual(actual.PriceCents, expected.PriceCents) {
			return false
		}
	}
	return true
}

func accessNullableIntsEqual(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func writeAccessConfig(filename string, local accessConfig) error {
	data, err := yaml.Marshal(local)
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

func writeAccessConfigReplacing(filename string, local accessConfig) error {
	data, err := yaml.Marshal(local)
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
