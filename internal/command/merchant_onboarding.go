package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
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
			var optionalDisplayName, optionalHandle *string
			if value := strings.TrimSpace(displayName); value != "" {
				optionalDisplayName = &value
			}
			if value := strings.TrimSpace(handle); value != "" {
				optionalHandle = &value
			}
			result, err := runtime.client().CreateMerchantApplication(command.Context(), runtime.deps.NewID(), optionalDisplayName, optionalHandle)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	// 两个参数均可选：服务端按用户资料派生；仅显式覆盖时发送。
	command.Flags().StringVar(&displayName, "display-name", "", "optional Merchant display name override")
	command.Flags().StringVar(&handle, "handle", "", "optional public creator handle override")
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
	var timeout time.Duration
	github := &cobra.Command{
		Use: "github <merchant-id>", Short: "Authorize the Merchant's personal GitHub account", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if timeout <= 0 {
				return output.Validation("timeout", "--timeout must be greater than zero")
			}
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			result, err := runtime.client().StartGithubChannel(command.Context(), args[0])
			if err != nil {
				return err
			}
			if result.Kind == "verified" {
				return runtime.business(result)
			}
			if result.Kind != "authorization" || result.AuthorizationURL == nil || strings.TrimSpace(*result.AuthorizationURL) == "" || result.AttemptID == nil || strings.TrimSpace(*result.AttemptID) == "" {
				return output.Internal("GITHUB_AUTHORIZATION_RESPONSE_INVALID", "ViceMe API returned an incomplete GitHub authorization", nil)
			}
			writeHumanGithubChannelStart(runtime.deps.ErrOut, *result.AuthorizationURL)
			return finishGithubChannelAuthorization(command.Context(), runtime, runtime.client(), args[0], *result.AttemptID, timeout, 2*time.Second)
		},
	}
	github.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "maximum time to wait for GitHub authorization")
	command.AddCommand(github)
	command.AddCommand(newMerchantXiaohongshuChannelCommand(runtime))
	return command
}

func writeHumanGithubChannelStart(writer io.Writer, authorizationURL string) {
	_, _ = fmt.Fprintln(writer, "Open this one-time URL in your browser to authorize GitHub:")
	_, _ = fmt.Fprintf(writer, "\n  %s\n\n", authorizationURL)
	_, _ = fmt.Fprintln(writer, "ViceMe will continue automatically after GitHub authorization.")
	_, _ = fmt.Fprintln(writer, "Waiting for authorization...")
}

func finishGithubChannelAuthorization(ctx context.Context, runtime *Runtime, client *api.Client, merchantAccountID, attemptID string, timeout, interval time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if interval < time.Second {
		interval = time.Second
	}
	for {
		status, err := client.GetGithubChannelStatus(ctx, merchantAccountID, attemptID)
		if err != nil {
			if contextErr := githubAuthorizationContextError(ctx); contextErr != nil {
				return contextErr
			}
			return err
		}
		switch status.Kind {
		case "verified":
			return runtime.business(api.GithubAuthorizationStart{Kind: "verified", AuthorizationURL: nil})
		case "pending":
			if sleepErr := runtime.deps.Sleep(ctx, interval); sleepErr != nil {
				if contextErr := githubAuthorizationContextError(ctx); contextErr != nil {
					return contextErr
				}
				return sleepErr
			}
		case "denied":
			return output.Authorization("GITHUB_AUTHORIZATION_DENIED", "GitHub authorization was denied")
		case "permissions":
			return output.Authorization("GITHUB_REPOSITORY_PERMISSION_REQUIRED", "GitHub repository access was not granted")
		case "conflict":
			return output.Policy("GITHUB_AUTHORIZATION_CONFLICT", "This GitHub account cannot be linked to the merchant")
		case "expired":
			expired := output.Authorization("GITHUB_AUTHORIZATION_EXPIRED", "GitHub authorization expired before it was completed")
			expired.Retryable = true
			expired.Hint = "run the same GitHub channel authorization command again to start a fresh one-time URL"
			return expired
		default:
			return output.Internal("GITHUB_AUTHORIZATION_STATUS_INVALID", "ViceMe API returned an invalid GitHub authorization status", nil)
		}
	}
}

func githubAuthorizationContextError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		pending := output.Authorization("GITHUB_AUTHORIZATION_PENDING", "GitHub authorization is still pending")
		pending.Retryable = true
		pending.Hint = "run the same GitHub channel authorization command again to start a fresh one-time URL"
		return pending
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return output.Authorization("GITHUB_AUTHORIZATION_CANCELLED", "GitHub authorization was cancelled")
	}
	return nil
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
