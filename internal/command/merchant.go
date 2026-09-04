package command

import (
	"context"
	"strings"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newMerchantCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "merchant",
		Short: "Author and operate products for an approved ViceMe merchant",
	}
	command.AddCommand(newMerchantAccountsCommand(runtime))
	command.AddCommand(newMerchantOnboardingCommand(runtime))
	command.AddCommand(newMerchantChannelCommand(runtime))
	command.AddCommand(newMerchantWorkCommand(runtime))
	command.AddCommand(newMerchantPageCommand(runtime))
	command.AddCommand(newMerchantCommerceApplicationCommand(runtime))
	command.AddCommand(newMerchantProductCommand(runtime))
	return command
}

func newMerchantAccountsCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "accounts",
		Short: "List merchant accounts owned by the current login",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().ListMerchantAccounts(command.Context())
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
}

func newMerchantWorkCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "work", Short: "Manage merchant Works"}
	command.AddCommand(newMerchantWorkCreateCommand(runtime))
	command.AddCommand(newMerchantWorkListCommand(runtime))
	command.AddCommand(newMerchantWorkGetCommand(runtime))
	command.AddCommand(newMerchantWorkUpdateCommand(runtime))
	command.AddCommand(newMerchantWorkWebsiteVerificationCommand(runtime))
	command.AddCommand(newMerchantWorkSdkAccessCommand(runtime))
	return command
}

func newMerchantWorkCreateCommand(runtime *Runtime) *cobra.Command {
	var inputFile string
	command := &cobra.Command{
		Use:   "create",
		Short: "Create a merchant Work from a strict JSON request",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			input, err := readJSONObject(inputFile, "MERCHANT_WORK_INPUT_INVALID")
			if err != nil {
				return err
			}
			result, err := runtime.client().CreateMerchantWork(command.Context(), input)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&inputFile, "input", "", "strict create-Work JSON file")
	_ = command.MarkFlagRequired("input")
	return command
}

func newMerchantWorkListCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "list",
		Short: "List Works owned by one merchant",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().ListMerchantWorks(command.Context(), merchantAccountID)
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

func newMerchantWorkGetCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "get <work-id>",
		Short: "Get one merchant Work",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().GetMerchantWork(command.Context(), args[0], merchantAccountID)
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

func newMerchantWorkUpdateCommand(runtime *Runtime) *cobra.Command {
	var inputFile string
	command := &cobra.Command{
		Use:   "update <work-id>",
		Short: "Update a merchant Work from a strict JSON request",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			input, err := readJSONObject(inputFile, "MERCHANT_WORK_INPUT_INVALID")
			if err != nil {
				return err
			}
			result, err := runtime.client().UpdateMerchantWork(command.Context(), args[0], input)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&inputFile, "input", "", "strict update-Work JSON file")
	_ = command.MarkFlagRequired("input")
	return command
}

func newMerchantProductCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "product", Short: "Manage merchant Products and generated purchase Skills"}
	command.AddCommand(newMerchantProductLifecycleCommand(runtime, "suspend"))
	command.AddCommand(newMerchantProductLifecycleCommand(runtime, "archive"))
	command.AddCommand(newMerchantProductListCommand(runtime))
	return command
}

func newMerchantProductLifecycleCommand(runtime *Runtime, action string) *cobra.Command {
	var merchantAccountID string
	var expectedRevision int
	command := &cobra.Command{
		Use:   action + " <product-id>",
		Short: strings.ToUpper(action[:1]) + action[1:] + " a merchant Product",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if expectedRevision < 1 {
				return output.Validation("MERCHANT_PRODUCT_REVISION_INVALID", "--expected-revision must be positive")
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			result, err := runtime.client().CommandMerchantProduct(command.Context(), args[0], action, merchantAccountID, expectedRevision)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	command.Flags().IntVar(&expectedRevision, "expected-revision", 0, "exact Product revision")
	_ = command.MarkFlagRequired("merchant")
	_ = command.MarkFlagRequired("expected-revision")
	return command
}

func newMerchantProductListCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	var status string
	var cursor string
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List Products owned by one merchant",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if limit < 1 || limit > 100 {
				return output.Validation("MERCHANT_PRODUCT_LIMIT_INVALID", "--limit must be between 1 and 100")
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().ListMerchantProducts(command.Context(), merchantAccountID, strings.ToUpper(status), cursor, limit)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	command.Flags().StringVar(&status, "status", "", "optional DRAFT, ACTIVE, SUSPENDED, or ARCHIVED filter")
	command.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	command.Flags().IntVar(&limit, "limit", 20, "page size")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func (runtime *Runtime) requireMerchantCommerceAuthentication(ctx context.Context, write bool) error {
	status, err := runtime.client().AuthStatus(ctx)
	if err != nil {
		return err
	}
	required := []string{"merchant-commerce:read"}
	if write {
		required = append(required, "merchant-commerce:write")
	}
	available := make(map[string]struct{}, len(status.Scopes))
	for _, scope := range status.Scopes {
		available[scope] = struct{}{}
	}
	var missing []string
	for _, scope := range required {
		if _, ok := available[scope]; !ok {
			missing = append(missing, scope)
		}
	}
	if len(missing) > 0 {
		return output.Authorization("MERCHANT_COMMERCE_SCOPE_REQUIRED", "the current login is not authorized to manage merchant commerce").
			WithHint("run 'viceme auth login' again for the current profile to grant merchant commerce access").
			WithDetails(map[string]any{"profile": runtime.profile.Name, "missingScopes": missing})
	}
	return nil
}
