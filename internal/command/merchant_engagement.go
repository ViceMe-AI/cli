package command

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newMerchantWorkWebsiteVerificationCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "website-verification",
		Short: "Manage DNS ownership verification for Website Works",
	}
	command.AddCommand(newMerchantWorkWebsiteVerificationCreateCommand(runtime))
	command.AddCommand(newMerchantWorkWebsiteVerificationGetCommand(runtime))
	command.AddCommand(newMerchantWorkWebsiteVerificationVerifyCommand(runtime))
	command.AddCommand(newMerchantWorkWebsiteVerificationRevokeCommand(runtime))
	return command
}

func newMerchantWorkWebsiteVerificationCreateCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	var expectedRevision int
	command := &cobra.Command{
		Use:   "create <work-id>",
		Short: "Create a DNS ownership challenge for a Website Work",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if expectedRevision < 1 {
				return output.Validation("MERCHANT_WORK_REVISION_INVALID", "--expected-revision must be positive")
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			result, err := runtime.client().CreateWebsiteVerification(command.Context(), args[0], api.CreateWebsiteVerificationRequest{
				MerchantAccountID: merchantAccountID,
				ExpectedRevision:  expectedRevision,
			})
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	command.Flags().IntVar(&expectedRevision, "expected-revision", 0, "exact Work revision")
	_ = command.MarkFlagRequired("merchant")
	_ = command.MarkFlagRequired("expected-revision")
	return command
}

func newMerchantWorkWebsiteVerificationGetCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "get <work-id>",
		Short: "Get the latest DNS ownership challenge for a Website Work",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().GetLatestWebsiteVerification(command.Context(), args[0], merchantAccountID)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func newMerchantWorkWebsiteVerificationVerifyCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	var expectedVerificationVersion int
	command := &cobra.Command{
		Use:   "verify <work-id>",
		Short: "Verify the current DNS ownership challenge for a Website Work",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if expectedVerificationVersion < 1 {
				return output.Validation("WEBSITE_VERIFICATION_VERSION_INVALID", "--expected-verification-version must be positive")
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			result, err := runtime.client().VerifyWebsite(command.Context(), args[0], api.VerifyWebsiteRequest{
				MerchantAccountID:           merchantAccountID,
				ExpectedVerificationVersion: expectedVerificationVersion,
			})
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	command.Flags().IntVar(&expectedVerificationVersion, "expected-verification-version", 0, "exact Website verification version")
	_ = command.MarkFlagRequired("merchant")
	_ = command.MarkFlagRequired("expected-verification-version")
	return command
}

func newMerchantWorkWebsiteVerificationRevokeCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	var expectedRevision int
	command := &cobra.Command{
		Use:   "revoke <work-id>",
		Short: "Revoke verified ownership of a Website Work",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if expectedRevision < 1 {
				return output.Validation("MERCHANT_WORK_REVISION_INVALID", "--expected-revision must be positive")
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			result, err := runtime.client().RevokeWebsiteOwnership(command.Context(), args[0], api.RevokeWebsiteOwnershipRequest{
				MerchantAccountID: merchantAccountID,
				ExpectedRevision:  expectedRevision,
			})
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	command.Flags().IntVar(&expectedRevision, "expected-revision", 0, "exact Work revision")
	_ = command.MarkFlagRequired("merchant")
	_ = command.MarkFlagRequired("expected-revision")
	return command
}

func newMerchantWorkSdkAccessCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "sdk-access",
		Short: "Manage hosted, follow, and paid SDK features for merchant Works",
	}
	command.AddCommand(newMerchantWorkSdkAccessCreateCommand(runtime))
	command.AddCommand(newMerchantWorkSdkAccessGetCommand(runtime))
	command.AddCommand(newMerchantWorkSdkAccessListCommand(runtime))
	command.AddCommand(newMerchantWorkSdkAccessUpdateCommand(runtime))
	command.AddCommand(newMerchantWorkSdkAccessDisableCommand(runtime))
	return command
}

func newMerchantWorkSdkAccessCreateCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	var features, follows, purchases, prices []string
	command := &cobra.Command{
		Use:   "create <work-id>",
		Short: "Enable selected SDK features for a merchant Work",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			normalizedFeatures, err := normalizeWorkSdkFeatures(features)
			if err != nil {
				return err
			}
			accessFeatures, err := buildWorkAccessFeatures(follows, purchases, prices)
			if err != nil {
				return err
			}
			if len(normalizedFeatures) == 0 && len(accessFeatures) == 0 {
				return output.Validation("WORK_SDK_FEATURE_REQUIRED", "configure at least one hosted, follow, or purchase feature")
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			result, err := runtime.client().CreateWorkSdkAccess(command.Context(), args[0], api.CreateWorkSdkAccessRequest{
				MerchantAccountID: merchantAccountID,
				Features:          normalizedFeatures,
				AccessFeatures:    accessFeatures,
			})
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	command.Flags().StringArrayVar(&features, "feature", nil, "SDK feature to enable (danmaku or tip; repeatable)")
	addWorkAccessFeatureFlags(command, &follows, &purchases, &prices)
	_ = command.MarkFlagRequired("merchant")
	return command
}

func newMerchantWorkSdkAccessGetCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "get <work-id>",
		Short: "Get SDK access for a merchant Work",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().GetWorkSdkAccess(command.Context(), args[0], merchantAccountID)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func newMerchantWorkSdkAccessListCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "list",
		Short: "List Work SDK access owned by one merchant",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().ListWorkSdkAccesses(command.Context(), merchantAccountID)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func newMerchantWorkSdkAccessUpdateCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	var expectedConfigVersion int
	var features, follows, purchases, prices []string
	var clearHosted, clearAccess bool
	command := &cobra.Command{
		Use:   "update <work-id>",
		Short: "Replace all enabled SDK features for a merchant Work",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if clearHosted && command.Flags().Changed("feature") {
				return output.Validation("WORK_SDK_FEATURE_FLAGS_CONFLICT", "--clear-hosted cannot be combined with --feature")
			}
			if clearAccess && (command.Flags().Changed("follow") || command.Flags().Changed("purchase") || command.Flags().Changed("price-minor")) {
				return output.Validation("WORK_ACCESS_FEATURE_FLAGS_CONFLICT", "--clear-access cannot be combined with access feature flags")
			}
			if expectedConfigVersion < 1 {
				return output.Validation("WORK_SDK_CONFIG_VERSION_INVALID", "--expected-config-version must be positive")
			}
			var requestedHosted []string
			if command.Flags().Changed("feature") {
				var err error
				requestedHosted, err = normalizeWorkSdkFeatures(features)
				if err != nil {
					return err
				}
			}
			var requestedAccess []api.WorkAccessFeatureInput
			if command.Flags().Changed("follow") || command.Flags().Changed("purchase") || command.Flags().Changed("price-minor") {
				var err error
				requestedAccess, err = buildWorkAccessFeatures(follows, purchases, prices)
				if err != nil {
					return err
				}
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			current, err := runtime.client().GetWorkSdkAccess(command.Context(), args[0], merchantAccountID)
			if err != nil {
				return err
			}
			if current.ConfigVersion != expectedConfigVersion {
				return output.Policy("WORK_SDK_CONFIG_VERSION_CONFLICT", "Work SDK access config version changed").WithDetails(map[string]any{"expectedConfigVersion": expectedConfigVersion, "actualConfigVersion": current.ConfigVersion})
			}
			normalizedFeatures := current.Features
			if command.Flags().Changed("feature") {
				normalizedFeatures = requestedHosted
			} else if clearHosted {
				normalizedFeatures = []string{}
			}
			accessFeatures := workAccessFeatureInputs(current.AccessFeatures)
			if command.Flags().Changed("follow") || command.Flags().Changed("purchase") || command.Flags().Changed("price-minor") {
				accessFeatures = requestedAccess
			} else if clearAccess {
				accessFeatures = []api.WorkAccessFeatureInput{}
			}
			if len(normalizedFeatures) == 0 && len(accessFeatures) == 0 {
				return output.Validation("WORK_SDK_FEATURE_REQUIRED", "the Work SDK access must retain at least one feature")
			}
			result, err := runtime.client().UpdateWorkSdkAccess(command.Context(), args[0], api.UpdateWorkSdkAccessRequest{
				MerchantAccountID:     merchantAccountID,
				ExpectedConfigVersion: expectedConfigVersion,
				Features:              normalizedFeatures,
				AccessFeatures:        accessFeatures,
			})
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	command.Flags().IntVar(&expectedConfigVersion, "expected-config-version", 0, "exact Work SDK access config version")
	command.Flags().StringArrayVar(&features, "feature", nil, "replacement SDK feature (danmaku or tip; repeatable)")
	command.Flags().BoolVar(&clearHosted, "clear-hosted", false, "remove all danmaku and tip features")
	command.Flags().BoolVar(&clearAccess, "clear-access", false, "remove all follow and purchase features")
	addWorkAccessFeatureFlags(command, &follows, &purchases, &prices)
	_ = command.MarkFlagRequired("merchant")
	_ = command.MarkFlagRequired("expected-config-version")
	return command
}

func newMerchantWorkSdkAccessDisableCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "disable <work-id>",
		Short: "Disable SDK access for a merchant Work",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			result, err := runtime.client().DisableWorkSdkAccess(command.Context(), args[0], merchantAccountID)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func normalizeWorkSdkFeatures(features []string) ([]string, error) {
	var danmaku, tip bool
	for _, feature := range features {
		switch feature {
		case "danmaku":
			danmaku = true
		case "tip":
			tip = true
		default:
			return nil, output.Validation("WORK_SDK_FEATURE_INVALID", "--feature must be danmaku or tip")
		}
	}
	result := make([]string, 0, 2)
	if danmaku {
		result = append(result, "danmaku")
	}
	if tip {
		result = append(result, "tip")
	}
	return result, nil
}

var workAccessFeatureKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)

func addWorkAccessFeatureFlags(command *cobra.Command, follows, purchases, prices *[]string) {
	command.Flags().StringArrayVar(follows, "follow", nil, "FOLLOW_OWNER feature as key or key=title (repeatable)")
	command.Flags().StringArrayVar(purchases, "purchase", nil, "WORK_ENTITLEMENT feature as key or key=title (repeatable)")
	command.Flags().StringArrayVar(prices, "price-minor", nil, "price in fen; repeat once per --purchase or provide one shared price")
}

func buildWorkAccessFeatures(follows, purchases, rawPrices []string) ([]api.WorkAccessFeatureInput, error) {
	prices, err := parseWorkAccessPrices(purchases, rawPrices)
	if err != nil {
		return nil, err
	}
	features := make(map[string]api.WorkAccessFeatureInput, len(follows)+len(purchases))
	groups := []struct {
		values []string
		policy string
	}{
		{values: follows, policy: "FOLLOW_OWNER"},
		{values: purchases, policy: "WORK_ENTITLEMENT"},
	}
	for _, group := range groups {
		for index, value := range group.values {
			key, title, err := parseWorkAccessFeature(value)
			if err != nil {
				return nil, err
			}
			if _, exists := features[key]; exists {
				return nil, output.Validation("ACCESS_FEATURE_DUPLICATE", fmt.Sprintf("feature %q is configured more than once", key))
			}
			feature := api.WorkAccessFeatureInput{FeatureKey: key, Title: title, PolicyType: group.policy, Status: "ACTIVE"}
			if group.policy == "WORK_ENTITLEMENT" {
				feature.Price = &api.WorkAccessPrice{Currency: "CNY", AmountCents: prices[index]}
			}
			features[key] = feature
		}
	}
	keys := make([]string, 0, len(features))
	for key := range features {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]api.WorkAccessFeatureInput, 0, len(keys))
	for _, key := range keys {
		result = append(result, features[key])
	}
	return result, nil
}

func workAccessFeatureInputs(features []api.WorkAccessFeature) []api.WorkAccessFeatureInput {
	inputs := make([]api.WorkAccessFeatureInput, len(features))
	for index, feature := range features {
		inputs[index] = api.WorkAccessFeatureInput{
			FeatureKey: feature.FeatureKey,
			Title:      feature.Title,
			PolicyType: feature.PolicyType,
			Price:      feature.Price,
			Status:     feature.Status,
		}
	}
	return inputs
}

func parseWorkAccessPrices(purchases, rawPrices []string) ([]int, error) {
	if len(purchases) == 0 {
		if len(rawPrices) > 0 {
			return nil, output.Validation("ACCESS_CONFIG_INVALID", "--price-minor requires at least one --purchase")
		}
		return nil, nil
	}
	if len(rawPrices) != 1 && len(rawPrices) != len(purchases) {
		return nil, output.Validation("WORK_PRICE_REQUIRED", "provide one shared --price-minor or one price for each --purchase")
	}
	prices := make([]int, len(purchases))
	for index := range purchases {
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

func parseWorkAccessFeature(raw string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), "=", 2)
	key := strings.TrimSpace(parts[0])
	title := key
	if len(parts) == 2 {
		title = strings.TrimSpace(parts[1])
	}
	if !workAccessFeatureKeyPattern.MatchString(key) || title == "" {
		return "", "", output.Validation("ACCESS_FEATURE_INVALID", "feature must use key or key=title")
	}
	return key, title, nil
}

func newMerchantCommerceApplicationCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "commerce-application",
		Short: "Manage merchant Commerce Applications",
	}
	command.AddCommand(newMerchantCommerceApplicationCreateCommand(runtime))
	command.AddCommand(newMerchantCommerceApplicationListCommand(runtime))
	command.AddCommand(newMerchantCommerceApplicationGetCommand(runtime))
	command.AddCommand(newMerchantCommerceApplicationUpdateCommand(runtime))
	command.AddCommand(newMerchantCommerceApplicationLifecycleCommand(runtime, "activate"))
	command.AddCommand(newMerchantCommerceApplicationLifecycleCommand(runtime, "suspend"))
	return command
}

func newMerchantCommerceApplicationCreateCommand(runtime *Runtime) *cobra.Command {
	var inputFile string
	command := &cobra.Command{
		Use:   "create",
		Short: "Create a Commerce Application from a strict JSON request",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			request, err := readStrictJSONObject[api.CreateCommerceApplicationRequest](inputFile, "COMMERCE_APPLICATION_INPUT_INVALID")
			if err != nil {
				return err
			}
			result, err := runtime.client().CreateCommerceApplication(command.Context(), request)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&inputFile, "input", "", "strict create-Commerce-Application JSON file")
	_ = command.MarkFlagRequired("input")
	return command
}

func newMerchantCommerceApplicationListCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "list",
		Short: "List Commerce Applications owned by one merchant",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().ListCommerceApplications(command.Context(), merchantAccountID)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func newMerchantCommerceApplicationGetCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "get <application-id>",
		Short: "Get one merchant Commerce Application",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().GetCommerceApplication(command.Context(), args[0], merchantAccountID)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func newMerchantCommerceApplicationUpdateCommand(runtime *Runtime) *cobra.Command {
	var inputFile string
	command := &cobra.Command{
		Use:   "update <application-id>",
		Short: "Update a Commerce Application from a strict JSON request",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			request, err := readStrictJSONObject[api.UpdateCommerceApplicationRequest](inputFile, "COMMERCE_APPLICATION_INPUT_INVALID")
			if err != nil {
				return err
			}
			if request.ExpectedRevision < 1 {
				return output.Validation("COMMERCE_APPLICATION_REVISION_INVALID", "expectedRevision must be positive")
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			result, err := runtime.client().UpdateCommerceApplication(command.Context(), args[0], request)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&inputFile, "input", "", "strict update-Commerce-Application JSON file")
	_ = command.MarkFlagRequired("input")
	return command
}

func newMerchantCommerceApplicationLifecycleCommand(runtime *Runtime, action string) *cobra.Command {
	var merchantAccountID string
	var expectedRevision int
	command := &cobra.Command{
		Use:   action + " <application-id>",
		Short: action + " a merchant Commerce Application",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if expectedRevision < 1 {
				return output.Validation("COMMERCE_APPLICATION_REVISION_INVALID", "--expected-revision must be positive")
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			result, err := runtime.client().CommandCommerceApplication(command.Context(), args[0], action, api.CommerceApplicationCommand{
				MerchantAccountID: merchantAccountID,
				ExpectedRevision:  expectedRevision,
			})
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	command.Flags().IntVar(&expectedRevision, "expected-revision", 0, "exact Commerce Application revision")
	_ = command.MarkFlagRequired("merchant")
	_ = command.MarkFlagRequired("expected-revision")
	return command
}
