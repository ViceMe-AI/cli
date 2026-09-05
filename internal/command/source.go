package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newSourceCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "source", Short: "Manage private repository access for the current user"}
	var timeout time.Duration
	github := &cobra.Command{
		Use: "github <merchant-id>", Short: "Authorize your personal GitHub account for private repositories", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if timeout <= 0 {
				return output.Validation("timeout", "--timeout must be greater than zero")
			}
			if err := runtime.requireSkillPublicationAuthentication(command.Context()); err != nil {
				return err
			}
			if err := ensureGithubSourceAuthorization(command.Context(), runtime, args[0], timeout); err != nil {
				return err
			}
			return runtime.business(api.GithubAuthorizationStart{Kind: "authorized"})
		},
	}
	github.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "maximum time to wait for GitHub authorization")
	command.AddCommand(github)
	return command
}

func writeHumanGithubSourceStart(writer io.Writer, authorizationURL string) {
	_, _ = fmt.Fprintln(writer, "Open this one-time URL in your browser to authorize GitHub:")
	_, _ = fmt.Fprintf(writer, "\n  %s\n\n", authorizationURL)
	_, _ = fmt.Fprintln(writer, "ViceMe will continue automatically after GitHub authorization.")
	_, _ = fmt.Fprintln(writer, "Waiting for authorization...")
}

func waitGithubSourceAuthorization(ctx context.Context, runtime *Runtime, client *api.Client, merchantAccountID, attemptID string, timeout, interval time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if interval < time.Second {
		interval = time.Second
	}
	for {
		status, err := client.GetGithubSourceStatus(ctx, merchantAccountID, attemptID)
		if err != nil {
			if contextErr := githubAuthorizationContextError(ctx); contextErr != nil {
				return contextErr
			}
			return err
		}
		switch status.Kind {
		case "authorized":
			return nil
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
			return output.Policy("GITHUB_AUTHORIZATION_CONFLICT", "This private repository authorization changed; start again")
		case "expired":
			expired := output.Authorization("GITHUB_AUTHORIZATION_EXPIRED", "GitHub authorization expired before it was completed")
			expired.Retryable = true
			expired.Hint = "run the same GitHub private repository authorization command again to start a fresh one-time URL"
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
		pending.Hint = "run the same GitHub private repository authorization command again to start a fresh one-time URL"
		return pending
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return output.Authorization("GITHUB_AUTHORIZATION_CANCELLED", "GitHub authorization was cancelled")
	}
	return nil
}

func ensureGithubSourceAuthorization(ctx context.Context, runtime *Runtime, merchantID string, timeout time.Duration) error {
	result, err := runtime.client().StartGithubSource(ctx, merchantID)
	if err != nil {
		return err
	}
	if result.Kind == "authorized" {
		return nil
	}
	if result.Kind != "authorization" || result.AuthorizationURL == nil || strings.TrimSpace(*result.AuthorizationURL) == "" || result.AttemptID == nil || strings.TrimSpace(*result.AttemptID) == "" {
		return output.Internal("GITHUB_AUTHORIZATION_RESPONSE_INVALID", "ViceMe API returned an incomplete GitHub authorization", nil)
	}
	writeHumanGithubSourceStart(runtime.deps.ErrOut, *result.AuthorizationURL)
	return waitGithubSourceAuthorization(ctx, runtime, runtime.client(), merchantID, *result.AttemptID, timeout, 2*time.Second)
}
