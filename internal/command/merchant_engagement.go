package command

import (
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
		Short: "Manage the danmaku and tip SDK features for merchant Works",
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
	var features []string
	command := &cobra.Command{
		Use:   "create <work-id>",
		Short: "Enable selected SDK features for a merchant Work",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			normalizedFeatures, err := normalizeWorkSdkFeatures(features)
			if err != nil {
				return err
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			result, err := runtime.client().CreateWorkSdkAccess(command.Context(), args[0], api.CreateWorkSdkAccessRequest{
				MerchantAccountID: merchantAccountID,
				Features:          normalizedFeatures,
			})
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	command.Flags().StringArrayVar(&features, "feature", nil, "SDK feature to enable (danmaku or tip; repeatable)")
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
	var features []string
	command := &cobra.Command{
		Use:   "update <work-id>",
		Short: "Replace all enabled SDK features for a merchant Work",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if expectedConfigVersion < 1 {
				return output.Validation("WORK_SDK_CONFIG_VERSION_INVALID", "--expected-config-version must be positive")
			}
			normalizedFeatures, err := normalizeWorkSdkFeatures(features)
			if err != nil {
				return err
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			result, err := runtime.client().UpdateWorkSdkAccess(command.Context(), args[0], api.UpdateWorkSdkAccessRequest{
				MerchantAccountID:     merchantAccountID,
				ExpectedConfigVersion: expectedConfigVersion,
				Features:              normalizedFeatures,
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
	if len(features) == 0 {
		return nil, output.Validation("WORK_SDK_FEATURE_REQUIRED", "at least one --feature is required")
	}
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
