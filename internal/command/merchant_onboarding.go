package command

import (
	"os"
	"strings"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

// 与 API 契约 MERCHANT_ONBOARDING_EVIDENCE_TEXT_MAX 保持一致。
const onboardingEvidenceTextMaxRunes = 2000

func newMerchantOnboardingCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "onboarding", Short: "Apply for Merchant access"}
	command.AddCommand(newMerchantOnboardingStatusCommand(runtime))
	command.AddCommand(newCreatorPageSetupCommand(runtime))
	command.AddCommand(newMerchantApplicationCommand(runtime))
	command.AddCommand(newMerchantOnboardingEvidenceCommand(runtime))
	command.AddCommand(newMerchantOnboardingSubmitCommand(runtime))
	return command
}

func newMerchantOnboardingStatusCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use: "status", Short: "Show the current Merchant application", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			result, err := runtime.client().GetMerchantOnboarding(command.Context())
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	return command
}

func newMerchantApplicationCommand(runtime *Runtime) *cobra.Command {
	var displayName, handle string
	command := &cobra.Command{
		Use: "apply", Short: "Submit a Merchant application with the creator username chosen by the author", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			var optionalDisplayName *string
			if value := strings.TrimSpace(displayName); value != "" {
				optionalDisplayName = &value
			}
			confirmedHandle := strings.TrimSpace(handle)
			if err := validateConfirmedCreatorHandle(confirmedHandle); err != nil {
				return err
			}
			result, err := runtime.client().CreateMerchantApplication(command.Context(), runtime.deps.NewID(), optionalDisplayName, confirmedHandle)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&displayName, "display-name", "", "optional Merchant display name override")
	command.Flags().StringVar(&handle, "handle", "", "author-confirmed creator username and permanent homepage handle")
	_ = command.MarkFlagRequired("handle")
	return command
}

func validateConfirmedCreatorHandle(handle string) error {
	_, reserved := pageReservedCreatorHandles[handle]
	if len(handle) < 2 || len(handle) > 32 || !pageCreatorHandlePattern.MatchString(handle) || reserved {
		return output.Validation("MERCHANT_APPLICATION_HANDLE_INVALID", "--handle must be 2-32 lowercase letters, digits, and single hyphens, start with a letter, and not be reserved")
	}
	return nil
}

func newMerchantOnboardingEvidenceCommand(runtime *Runtime) *cobra.Command {
	var filename string
	var statement string
	var lockVersion int
	command := &cobra.Command{
		Use: "evidence <onboarding-id>", Short: "Add one screenshot or text statement to an application", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if lockVersion < 0 {
				return output.Validation("ONBOARDING_LOCK_VERSION_INVALID", "--lock-version must be non-negative")
			}
			// 截图与文字说明二选一；与 API 端点的互斥校验一致。
			if (filename == "") == (statement == "") {
				return output.Validation("ONBOARDING_EVIDENCE_INPUT_INVALID", "provide exactly one of --path or --text")
			}
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			if statement != "" {
				text := strings.TrimSpace(statement)
				if text == "" {
					return output.Validation("ONBOARDING_EVIDENCE_TEXT_REQUIRED", "--text must not be blank")
				}
				if len([]rune(text)) > onboardingEvidenceTextMaxRunes {
					return output.Validation("ONBOARDING_EVIDENCE_TEXT_TOO_LONG", "--text must be at most 2000 characters")
				}
				result, err := runtime.client().UploadMerchantOnboardingEvidenceText(command.Context(), args[0], lockVersion, text)
				if err != nil {
					return err
				}
				return runtime.business(result)
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
	command.Flags().StringVar(&statement, "text", "", "text statement for this review round (at most one per round)")
	command.Flags().IntVar(&lockVersion, "lock-version", -1, "current onboarding lock version")
	_ = command.MarkFlagRequired("lock-version")
	return command
}

func newMerchantOnboardingSubmitCommand(runtime *Runtime) *cobra.Command {
	var lockVersion int
	command := &cobra.Command{
		Use: "submit <onboarding-id>", Short: "Submit a creator application for review", Args: cobra.ExactArgs(1),
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
