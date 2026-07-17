package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newAuthCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "auth", Short: "Manage Viceme CLI authentication"}
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
		Short: "Start or continue the Viceme device login flow",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if noWait && deviceCode != "" {
				return output.Validation("auth_flags", "--no-wait and --device-code cannot be used together")
			}
			if runtime.opts.JSON && !noWait && deviceCode == "" {
				return output.Validation("non_interactive_login", "use --no-wait with --json, then continue with --device-code in a later turn")
			}
			if timeout <= 0 {
				return output.Validation("timeout", "--timeout must be greater than zero")
			}
			client := runtime.client()
			if deviceCode == "" {
				authorization, err := client.StartDeviceAuthorization(command.Context())
				if err != nil {
					return err
				}
				if authorization.DeviceCode == "" || authorization.VerificationURL == "" {
					return output.Internal("device_authorization_response", "Viceme API returned an incomplete device authorization", nil)
				}
				if noWait {
					return runtime.success(authorization)
				}
				_, _ = fmt.Fprintf(runtime.deps.ErrOut, "Open %s and authorize code %s\n", authorization.VerificationURL, authorization.UserCode)
				deviceCode = authorization.DeviceCode
				if authorization.IntervalSeconds > 0 {
					return finishDeviceLogin(command.Context(), runtime, client, deviceCode, timeout, time.Duration(authorization.IntervalSeconds)*time.Second)
				}
			}
			return finishDeviceLogin(command.Context(), runtime, client, deviceCode, timeout, 2*time.Second)
		},
	}
	command.Flags().BoolVar(&noWait, "no-wait", false, "return the device authorization immediately")
	command.Flags().StringVar(&deviceCode, "device-code", "", "continue a previously started device authorization")
	command.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "maximum time to poll for authorization")
	return command
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
			credential := credentialauth.Credential{
				AccessToken:  token.AccessToken,
				RefreshToken: token.RefreshToken,
				TokenType:    token.TokenType,
				ExpiresAt:    token.ExpiresAt,
				UserID:       token.UserID,
			}
			if err := runtime.manager().Save(credential); err != nil {
				return err
			}
			result := map[string]any{"authenticated": true, "profile": runtime.opts.Profile}
			if token.UserID != "" {
				result["user_id"] = token.UserID
			}
			if !token.ExpiresAt.IsZero() {
				result["expires_at"] = token.ExpiresAt
			}
			return runtime.success(result)
		}
		if !api.IsSubtype(err, "authorization_pending") && !api.IsSubtype(err, "slow_down") {
			return err
		}
		if api.IsSubtype(err, "slow_down") {
			interval += time.Second
		}
		if err := runtime.deps.Sleep(ctx, interval); err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				pending := output.Authentication("authorization_pending", "device authorization is still pending")
				pending.Retryable = true
				pending.Hint = "run auth login again with the same --device-code before it expires"
				return pending
			}
			return err
		}
	}
}

func newAuthStatusCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show local Viceme authentication status",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			status, err := runtime.manager().CurrentStatus()
			if err != nil {
				return err
			}
			return runtime.success(status)
		},
	}
}

func newAuthLogoutCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke and remove local Viceme credentials",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			manager := runtime.manager()
			credential, err := manager.Load()
			if err != nil {
				var cliError *output.Error
				if errors.As(err, &cliError) && cliError.Subtype == "not_logged_in" {
					return runtime.success(map[string]any{"logged_out": true, "already_logged_out": true, "profile": runtime.opts.Profile})
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
			return runtime.success(map[string]any{"logged_out": true, "profile": runtime.opts.Profile})
		},
	}
}
