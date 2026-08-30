package command

import (
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

const creatorSubscriptionPriceMinorMax = 10_000_000

// newSubscriptionCommand 暴露创作者订阅计划管理：发布完成后由 Agent 询问创作者
// 是否设置粉丝订阅，价格即单一月付档；show/off 供查看与下架。
func newSubscriptionCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "subscription", Short: "Manage the creator fan subscription plan"}
	command.AddCommand(newSubscriptionShowCommand(runtime))
	command.AddCommand(newSubscriptionSetCommand(runtime))
	command.AddCommand(newSubscriptionOffCommand(runtime))
	return command
}

func newSubscriptionShowCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "show",
		Short: "Show the current fan subscription plan",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantAccountID)
			if err != nil {
				return err
			}
			plan, err := runtime.client().GetCreatorSubscriptionPlan(command.Context(), merchant.ID)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{
				"creatorHandle":         plan.CreatorHandle,
				"displayName":           plan.DisplayName,
				"priceMinor":            plan.PriceMinor,
				"periodDays":            plan.PeriodDays,
				"status":                plan.Status,
				"activeSubscriberCount": plan.ActiveSubscriberCount,
				"updatedAt":             plan.UpdatedAt,
			})
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account id; defaults to the only active merchant")
	return command
}

func newSubscriptionSetCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	var priceMinor int
	command := &cobra.Command{
		Use:   "set",
		Short: "Create or update the fan subscription monthly price",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if priceMinor <= 0 || priceMinor > creatorSubscriptionPriceMinorMax {
				return output.Validation("CREATOR_SUBSCRIPTION_PRICE_INVALID", "--price-minor must be between 1 and 10000000 (CNY cents)")
			}
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantAccountID)
			if err != nil {
				return err
			}
			plan, err := runtime.client().SetCreatorSubscriptionPlan(command.Context(), merchant.ID, priceMinor)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{
				"creatorHandle":         plan.CreatorHandle,
				"displayName":           plan.DisplayName,
				"priceMinor":            plan.PriceMinor,
				"periodDays":            plan.PeriodDays,
				"status":                plan.Status,
				"activeSubscriberCount": plan.ActiveSubscriberCount,
				"updatedAt":             plan.UpdatedAt,
			})
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account id; defaults to the only active merchant")
	command.Flags().IntVar(&priceMinor, "price-minor", 0, "monthly price in CNY cents (1 = ¥0.01)")
	_ = command.MarkFlagRequired("price-minor")
	return command
}

func newSubscriptionOffCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "off",
		Short: "Disable the fan subscription plan; existing subscribers keep their paid period",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			merchant, err := resolveSkillPublicationMerchant(command.Context(), runtime, merchantAccountID)
			if err != nil {
				return err
			}
			plan, err := runtime.client().DisableCreatorSubscriptionPlan(command.Context(), merchant.ID)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{
				"creatorHandle":         plan.CreatorHandle,
				"displayName":           plan.DisplayName,
				"priceMinor":            plan.PriceMinor,
				"periodDays":            plan.PeriodDays,
				"status":                plan.Status,
				"activeSubscriberCount": plan.ActiveSubscriberCount,
				"updatedAt":             plan.UpdatedAt,
			})
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account id; defaults to the only active merchant")
	return command
}
