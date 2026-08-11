package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

type deviceLoginStartResult struct {
	api.DeviceAuthorization
	Profile string `json:"profile"`
	Region  string `json:"region"`
}

type deviceLoginResult struct {
	Authenticated bool       `json:"authenticated"`
	Profile       string     `json:"profile"`
	Region        string     `json:"region"`
	UserID        string     `json:"user_id,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

func newAuthCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "auth", Short: "Manage ViceMe CLI authentication"}
	command.AddCommand(newAuthLoginCommand(runtime))
	command.AddCommand(newAuthStatusCommand(runtime))
	command.AddCommand(newAuthLogoutCommand(runtime))
	return command
}

func newAuthLoginCommand(runtime *Runtime) *cobra.Command {
	var noWait bool
	var deviceCode string
	var timeout time.Duration
	command := &cobra.Command{
		Use:   "login",
		Short: "Start or continue the ViceMe device login flow",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if _, source, _ := runtime.overrideCredential(); source != "" {
				return output.Policy("PROCESS_CREDENTIAL_ACTIVE", "device login is disabled while VICEME_ACCESS_TOKEN is active").WithHint("start a CLI process without VICEME_ACCESS_TOKEN to manage persistent login")
			}
			if noWait && deviceCode != "" {
				return output.Validation("auth_flags", "--no-wait and --device-code cannot be used together")
			}
			if timeout <= 0 {
				return output.Validation("timeout", "--timeout must be greater than zero")
			}
			if err := runtime.manager().PreflightSave(); err != nil {
				return err
			}
			client := runtime.client()
			if deviceCode == "" {
				authorization, err := client.StartDeviceAuthorization(
					command.Context(),
					api.DeviceAuthorizationRequest{
						ClientName: "viceme-cli",
						CLIVersion: buildinfo.Version,
						Scopes: []string{
							"profile:read",
							"skill-publication:read",
							"skill-publication:write",
						},
					},
				)
				if err != nil {
					return err
				}
				if authorization.DeviceCode == "" || authorization.VerificationURIComplete == "" {
					return output.Internal("device_authorization_response", "ViceMe API returned an incomplete device authorization", nil)
				}
				if noWait {
					return runtime.business(deviceLoginStartResult{
						DeviceAuthorization: authorization,
						Profile:             runtime.profile.Name,
						Region:              string(runtime.region),
					})
				}
				writeHumanLoginStart(runtime.deps.ErrOut, authorization)
				deviceCode = authorization.DeviceCode
				interval := 2 * time.Second
				if authorization.Interval > 0 {
					interval = time.Duration(authorization.Interval) * time.Second
				}
				return finishDeviceLogin(command.Context(), runtime, client, deviceCode, timeout, interval, false)
			}
			return finishDeviceLogin(command.Context(), runtime, client, deviceCode, timeout, 2*time.Second, true)
		},
	}
	command.Flags().BoolVar(&noWait, "no-wait", false, "return device authorization immediately for an Agent split-flow")
	command.Flags().StringVar(&deviceCode, "device-code", "", "continue a previously started Agent authorization")
	command.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "maximum time to wait for browser authorization")
	return command
}

func writeHumanLoginStart(writer io.Writer, authorization api.DeviceAuthorization) {
	_, _ = fmt.Fprintln(writer, "Open this URL in your browser to sign in to ViceMe:")
	_, _ = fmt.Fprintf(writer, "\n  %s\n\n", authorization.VerificationURIComplete)
	if authorization.UserCode != "" {
		_, _ = fmt.Fprintf(writer, "If prompted, enter code: %s\n\n", authorization.UserCode)
	}
	_, _ = fmt.Fprintln(writer, "Waiting for authorization...")
}

func finishDeviceLogin(ctx context.Context, runtime *Runtime, client *api.Client, deviceCode string, timeout, interval time.Duration, jsonOutput bool) error {
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
					return pendingLoginError(ctx, sleepErr, jsonOutput)
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
					WithDetails(map[string]any{"authorization_consumed": true, "issued_credential_revoked": revoked}).
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
			result := deviceLoginResult{Authenticated: true, Profile: runtime.profile.Name, Region: string(runtime.region), UserID: status.User.ID, ExpiresAt: &expiresAt}
			return runtime.business(result)
		}
		return err
	}
}

func pendingLoginError(ctx context.Context, err error, splitFlow bool) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		pending := output.Authentication("AUTHORIZATION_PENDING", "device authorization is still pending")
		pending.Retryable = true
		if splitFlow {
			pending.Hint = "run 'viceme auth login --device-code <code>' again before the device code expires"
		} else {
			pending.Hint = "run 'viceme auth login' again"
		}
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
					"authenticated": true,
					"source":        source,
					"persistent":    persistent,
					"profile":       runtime.profile.Name,
					"region":        runtime.region,
					"user":          remote.User,
					"scopes":        remote.Scopes,
					"expiresAt":     remote.ExpiresAt,
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
				"authenticated": true,
				"profile":       runtime.profile.Name,
				"region":        runtime.region,
				"user":          remote.User,
				"scopes":        remote.Scopes,
				"expiresAt":     remote.ExpiresAt,
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
			if _, source, _ := runtime.overrideCredential(); source != "" {
				return output.Policy("PROCESS_CREDENTIAL_ACTIVE", "logout cannot revoke a process credential").WithHint("stop passing VICEME_ACCESS_TOKEN to discard the process credential")
			}
			manager := runtime.manager()
			credential, err := manager.Load()
			if err != nil {
				var cliError *output.Error
				if errors.As(err, &cliError) && cliError.Subtype == "not_logged_in" {
					return runtime.business(map[string]any{"logged_out": true, "already_logged_out": true, "profile": runtime.profile.Name, "region": runtime.region})
				}
				return err
			}
			revokeErr := runtime.client().Revoke(command.Context(), credential.AccessToken)
			deleteErr := manager.Delete()
			if deleteErr != nil {
				return deleteErr
			}
			if revokeErr != nil {
				return revokeErr
			}
			return runtime.business(map[string]any{"logged_out": true, "profile": runtime.profile.Name, "region": runtime.region})
		},
	}
}
