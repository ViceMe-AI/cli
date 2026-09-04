package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

type deviceLoginResult struct {
	Authenticated              bool                            `json:"authenticated"`
	Profile                    string                          `json:"profile"`
	DistributionRegion         string                          `json:"distributionRegion"`
	UserID                     string                          `json:"userId,omitempty"`
	ExpiresAt                  *time.Time                      `json:"expiresAt,omitempty"`
	CreatorOnboardingSelection *api.CreatorOnboardingSelection `json:"creatorOnboardingSelection,omitempty"`
}

func newAuthCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "auth", Short: "Manage ViceMe CLI authentication"}
	command.AddCommand(newAuthLoginCommand(runtime))
	command.AddCommand(newAuthStatusCommand(runtime))
	command.AddCommand(newAuthLogoutCommand(runtime))
	return command
}

func newAuthLoginCommand(runtime *Runtime) *cobra.Command {
	var timeout time.Duration
	var purpose string
	command := &cobra.Command{
		Use:   "login",
		Short: "Start the ViceMe device login flow and wait for authorization",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if runtime.apiBaseURLFromEnv {
				return output.Policy("PROFILE_AUTHORITY_OVERRIDE_ACTIVE", "persistent login is disabled while VICEME_API_BASE_URL overrides the selected Profile").
					WithHint("add or select a Profile with the intended endpoint authority, then retry without VICEME_API_BASE_URL")
			}
			if _, source, _ := runtime.overrideCredential(); source != "" {
				return output.Policy("PROCESS_CREDENTIAL_ACTIVE", "device login is disabled while VICEME_ACCESS_TOKEN is active").WithHint("start a CLI process without VICEME_ACCESS_TOKEN to manage persistent login")
			}
			if timeout <= 0 {
				return output.Validation("timeout", "--timeout must be greater than zero")
			}
			devicePurpose := ""
			switch strings.TrimSpace(purpose) {
			case "", "default":
			case "creator-onboarding":
				devicePurpose = "CREATOR_ONBOARDING"
			default:
				return output.Validation("AUTH_LOGIN_PURPOSE_INVALID", "--purpose must be default or creator-onboarding")
			}
			if err := runtime.manager().PreflightSave(); err != nil {
				return err
			}
			client := runtime.client()
			authorization, err := client.StartDeviceAuthorization(
				command.Context(),
				api.DeviceAuthorizationRequest{
					ClientName: "viceme-cli",
					CLIVersion: buildinfo.Version,
					Purpose:    devicePurpose,
					Scopes: []string{
						"profile:read",
						"skill-publication:read",
						"skill-publication:write",
						"merchant-commerce:read",
						"merchant-commerce:write",
						"skill-use:read",
						"buyer-commerce:read",
						"buyer-commerce:write",
						"website-replica:read",
						"website-replica:write",
						"website-replica:purchase",
						"website-replica:analytics:read",
					},
				},
			)
			if err != nil {
				return err
			}
			if authorization.DeviceCode == "" || authorization.VerificationURIComplete == "" {
				return output.Internal("device_authorization_response", "ViceMe API returned an incomplete device authorization", nil)
			}
			writeHumanLoginStart(runtime.deps.ErrOut, authorization)
			interval := 2 * time.Second
			if authorization.Interval > 0 {
				interval = time.Duration(authorization.Interval) * time.Second
			}
			return finishDeviceLogin(command.Context(), runtime, client, authorization.DeviceCode, timeout, interval)
		},
	}
	command.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "maximum time to wait for browser authorization")
	command.Flags().StringVar(&purpose, "purpose", "default", "login purpose: default or creator-onboarding")
	return command
}

func writeHumanLoginStart(writer io.Writer, authorization api.DeviceAuthorization) {
	_, _ = fmt.Fprintln(writer, "Open this one-time URL in your browser to sign in to ViceMe:")
	_, _ = fmt.Fprintf(writer, "\n  %s\n\n", authorization.VerificationURIComplete)
	_, _ = fmt.Fprintln(writer, "ViceMe will authorize this CLI automatically after sign-in.")
	_, _ = fmt.Fprintln(writer, "Waiting for authorization...")
}

func finishDeviceLogin(ctx context.Context, runtime *Runtime, client *api.Client, deviceCode string, timeout, interval time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if interval < time.Second {
		interval = time.Second
	}
	for {
		token, err := client.ExchangeDeviceToken(ctx, deviceCode)
		if err == nil {
			if token.Status == "authorization_pending" {
				if token.Interval > 0 {
					interval = time.Duration(token.Interval) * time.Second
				}
				if sleepErr := runtime.deps.Sleep(ctx, interval); sleepErr != nil {
					return pendingLoginError(ctx, sleepErr)
				}
				continue
			}
			if token.Status != "authorized" || token.AccessToken == "" {
				return output.Internal("DEVICE_TOKEN_INVALID", "ViceMe API returned an invalid device token response", nil)
			}
			expiresAt, parseErr := time.Parse(time.RFC3339, token.ExpiresAt)
			if parseErr != nil {
				return output.Internal("DEVICE_TOKEN_INVALID", "ViceMe API returned an invalid token expiry", parseErr)
			}
			credential := credentialauth.Credential{
				AccessToken: token.AccessToken,
				TokenType:   token.TokenType,
				ExpiresAt:   expiresAt,
			}
			manager := runtime.manager()
			if err := manager.Save(credential); err != nil {
				revoked := client.Revoke(ctx, token.AccessToken) == nil
				return output.Authentication("credential_persistence_failed", "device authorization succeeded, but the issued credential could not be saved").
					WithHint("the one-time device authorization was consumed; fix the local credential store and start a new 'viceme auth login' flow").
					WithDetails(map[string]any{"authorizationConsumed": true, "issuedCredentialRevoked": revoked}).
					WithCause(err)
			}
			status, statusErr := runtime.client().AuthStatus(ctx)
			if statusErr != nil {
				_ = client.Revoke(ctx, token.AccessToken)
				_ = manager.Delete()
				return statusErr
			}
			credential.UserID = status.User.ID
			if err := manager.Save(credential); err != nil {
				_ = client.Revoke(ctx, token.AccessToken)
				_ = manager.Delete()
				return output.Authentication("CREDENTIAL_PERSISTENCE_FAILED", "login succeeded but the completed credential could not be saved").WithCause(err)
			}
			if err := runtime.recordProfileUserID(status.User.ID); err != nil {
				_ = client.Revoke(ctx, token.AccessToken)
				_ = manager.Delete()
				return err
			}
			result := deviceLoginResult{
				Authenticated:              true,
				Profile:                    runtime.profile.Name,
				DistributionRegion:         string(runtime.region),
				UserID:                     status.User.ID,
				ExpiresAt:                  &expiresAt,
				CreatorOnboardingSelection: token.CreatorOnboardingSelection,
			}
			return runtime.business(result)
		}
		return err
	}
}

func pendingLoginError(ctx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		pending := output.Authentication("AUTHORIZATION_PENDING", "device authorization is still pending")
		pending.Retryable = true
		pending.Hint = "run 'viceme auth login' again to start a fresh browser authorization"
		return pending
	}
	return err
}

func newAuthStatusCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show local ViceMe authentication status",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if _, source, persistent := runtime.overrideCredential(); source != "" {
				remote, err := runtime.client().AuthStatus(command.Context())
				if err != nil {
					return err
				}
				return runtime.business(map[string]any{
					"authenticated":      true,
					"source":             source,
					"persistent":         persistent,
					"profile":            runtime.profile.Name,
					"distributionRegion": runtime.region,
					"user":               remote.User,
					"scopes":             remote.Scopes,
					"expiresAt":          remote.ExpiresAt,
				})
			}
			status, err := runtime.manager().CurrentStatus()
			if err != nil {
				return err
			}
			if !status.Authenticated {
				return runtime.business(status)
			}
			remote, err := runtime.client().AuthStatus(command.Context())
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{
				"authenticated":      true,
				"profile":            runtime.profile.Name,
				"distributionRegion": runtime.region,
				"user":               remote.User,
				"scopes":             remote.Scopes,
				"expiresAt":          remote.ExpiresAt,
			})
		},
	}
}

func newAuthLogoutCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke and remove local ViceMe credentials",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if runtime.apiBaseURLFromEnv {
				return output.Policy("PROFILE_AUTHORITY_OVERRIDE_ACTIVE", "persistent logout is disabled while VICEME_API_BASE_URL overrides the selected Profile").
					WithHint("unset VICEME_API_BASE_URL and select the credential's Profile before logging out")
			}
			if _, source, _ := runtime.overrideCredential(); source != "" {
				return output.Policy("PROCESS_CREDENTIAL_ACTIVE", "logout cannot revoke a process credential").WithHint("stop passing VICEME_ACCESS_TOKEN to discard the process credential")
			}
			manager := runtime.manager()
			credential, err := manager.Load()
			if err != nil {
				var cliError *output.Error
				if errors.As(err, &cliError) && cliError.Subtype == "not_logged_in" {
					return runtime.business(map[string]any{"loggedOut": true, "alreadyLoggedOut": true, "profile": runtime.profile.Name, "distributionRegion": runtime.region})
				}
				return err
			}
			revokeErr := runtime.client().Revoke(command.Context(), credential.AccessToken)
			if revokeErr != nil && !credentialAlreadyRevoked(revokeErr) {
				return revokeErr
			}
			if err := manager.Delete(); err != nil {
				return err
			}
			return runtime.business(map[string]any{"loggedOut": true, "profile": runtime.profile.Name, "distributionRegion": runtime.region})
		},
	}
}

func credentialAlreadyRevoked(err error) bool {
	var cliError *output.Error
	return errors.As(err, &cliError) && cliError.Code == output.ExitAuthentication && cliError.Type == "authentication"
}
