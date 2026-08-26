package command

import (
	"os"
	"strings"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newMerchantOnboardingCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "onboarding", Short: "Apply for or claim Merchant ownership"}
	command.AddCommand(newMerchantOnboardingStatusCommand(runtime))
	command.AddCommand(newMerchantApplicationCommand(runtime))
	command.AddCommand(newMerchantGithubClaimCommand(runtime))
	command.AddCommand(newMerchantXiaohongshuClaimCommand(runtime))
	command.AddCommand(newMerchantOnboardingEvidenceCommand(runtime))
	command.AddCommand(newMerchantOnboardingSubmitCommand(runtime))
	return command
}

func newMerchantOnboardingStatusCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use: "status", Short: "Show the current Merchant application or claim", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			if strings.TrimSpace(merchantAccountID) != "" {
				result, err := runtime.client().GetMerchantTargetOnboarding(command.Context(), merchantAccountID)
				if err != nil {
					return err
				}
				return runtime.business(result)
			}
			result, err := runtime.client().GetMerchantOnboarding(command.Context())
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "optional pre-created Merchant account ID")
	return command
}

func newMerchantApplicationCommand(runtime *Runtime) *cobra.Command {
	var displayName, handle string
	command := &cobra.Command{
		Use: "apply", Short: "Submit a normal Merchant application and reserve its handle", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			result, err := runtime.client().CreateMerchantApplication(command.Context(), runtime.deps.NewID(), displayName, handle)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&displayName, "display-name", "", "Merchant display name")
	command.Flags().StringVar(&handle, "handle", "", "public creator handle to reserve")
	_ = command.MarkFlagRequired("display-name")
	_ = command.MarkFlagRequired("handle")
	return command
}

func newMerchantGithubClaimCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "claim-github <merchant-id>", Short: "Claim a pre-created Merchant through its configured GitHub identity", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			result, err := runtime.client().StartGithubMerchantClaim(command.Context(), args[0])
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
}

func newMerchantXiaohongshuClaimCommand(runtime *Runtime) *cobra.Command {
	var accountName, profileURL string
	command := &cobra.Command{
		Use: "claim-xiaohongshu <merchant-id>", Short: "Start an evidence-reviewed Xiaohongshu Merchant claim", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			var optionalURL *string
			if strings.TrimSpace(profileURL) != "" {
				optionalURL = &profileURL
			}
			result, err := runtime.client().StartXiaohongshuMerchantClaim(command.Context(), args[0], accountName, optionalURL)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&accountName, "account-name", "", "Xiaohongshu public account name")
	command.Flags().StringVar(&profileURL, "profile-url", "", "optional Xiaohongshu profile URL")
	_ = command.MarkFlagRequired("account-name")
	return command
}

func newMerchantOnboardingEvidenceCommand(runtime *Runtime) *cobra.Command {
	var filename string
	var lockVersion int
	command := &cobra.Command{
		Use: "evidence <onboarding-id>", Short: "Upload one Xiaohongshu account screenshot", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if lockVersion < 0 {
				return output.Validation("ONBOARDING_LOCK_VERSION_INVALID", "--lock-version must be non-negative")
			}
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			image, err := os.ReadFile(filename)
			if err != nil {
				return output.Validation("ONBOARDING_EVIDENCE_READ_FAILED", "could not read the evidence image").WithCause(err)
			}
			if len(image) == 0 || len(image) > 10<<20 {
				return output.Validation("ONBOARDING_EVIDENCE_SIZE_INVALID", "evidence image must be between 1 byte and 10 MiB")
			}
			result, err := runtime.client().UploadMerchantOnboardingEvidence(command.Context(), args[0], lockVersion, filename, image)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&filename, "path", "", "PNG, JPEG, or WebP account screenshot")
	command.Flags().IntVar(&lockVersion, "lock-version", -1, "current onboarding lock version")
	_ = command.MarkFlagRequired("path")
	_ = command.MarkFlagRequired("lock-version")
	return command
}

func newMerchantOnboardingSubmitCommand(runtime *Runtime) *cobra.Command {
	var lockVersion int
	command := &cobra.Command{
		Use: "submit <onboarding-id>", Short: "Submit an evidence-complete Xiaohongshu claim for review", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if lockVersion < 0 {
				return output.Validation("ONBOARDING_LOCK_VERSION_INVALID", "--lock-version must be non-negative")
			}
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			result, err := runtime.client().SubmitMerchantOnboarding(command.Context(), args[0], lockVersion)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().IntVar(&lockVersion, "lock-version", -1, "current onboarding lock version")
	_ = command.MarkFlagRequired("lock-version")
	return command
}

func newMerchantChannelCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "channel", Short: "Authorize a source channel for an owned Merchant"}
	command.AddCommand(&cobra.Command{
		Use: "github <merchant-id>", Short: "Authorize the Merchant's personal GitHub account", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			result, err := runtime.client().StartGithubChannel(command.Context(), args[0])
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	})
	command.AddCommand(newMerchantXiaohongshuChannelCommand(runtime))
	return command
}

func newMerchantXiaohongshuChannelCommand(runtime *Runtime) *cobra.Command {
	var subjectID, accountName, externalHandle, profileURL string
	command := &cobra.Command{
		Use: "xiaohongshu <merchant-id>", Short: "Start evidence review for an owned Merchant's Xiaohongshu channel", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			var handle, profile *string
			if strings.TrimSpace(externalHandle) != "" {
				value := strings.TrimSpace(externalHandle)
				handle = &value
			}
			if strings.TrimSpace(profileURL) != "" {
				value := strings.TrimSpace(profileURL)
				profile = &value
			}
			result, err := runtime.client().StartXiaohongshuChannelVerification(command.Context(), args[0], strings.TrimSpace(subjectID), strings.TrimSpace(accountName), handle, profile)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&subjectID, "subject-id", "", "stable Xiaohongshu account subject ID shown in the evidence")
	command.Flags().StringVar(&accountName, "account-name", "", "Xiaohongshu public account name")
	command.Flags().StringVar(&externalHandle, "handle", "", "optional Xiaohongshu display handle")
	command.Flags().StringVar(&profileURL, "profile-url", "", "optional Xiaohongshu profile URL")
	_ = command.MarkFlagRequired("subject-id")
	_ = command.MarkFlagRequired("account-name")
	return command
}
