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
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

type deviceLoginStartResult struct {
	api.DeviceAuthorization
	Profile      string   `json:"profile"`
	Region       string   `json:"region"`
	ContinueArgs []string `json:"continue_args"`
}

type deviceLoginResult struct {
	Authenticated bool       `json:"authenticated"`
	Profile       string     `json:"profile"`
	Region        string     `json:"region"`
	UserID        string     `json:"user_id,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	Warnings      []string   `json:"warnings,omitempty"`
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
	var jsonOutput bool
	var deviceCode string
	var timeout time.Duration
	command := &cobra.Command{
		Use:   "login",
		Short: "Start or continue the ViceMe device login flow",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if noWait && deviceCode != "" {
				return output.Validation("auth_flags", "--no-wait and --device-code cannot be used together")
			}
			if (noWait || deviceCode != "") && !jsonOutput {
				return output.Validation("auth_json_required", "--no-wait and --device-code are Agent flows and require --json")
			}
			if jsonOutput && !noWait && deviceCode == "" {
				return output.Validation("auth_json_flow", "use --no-wait --json, then continue with --device-code <code> --json in a later turn")
			}
			if timeout <= 0 {
				return output.Validation("timeout", "--timeout must be greater than zero")
			}
			if err := runtime.manager().PreflightSave(); err != nil {
				return err
			}
			client := runtime.client()
			if deviceCode == "" {
				authorization, err := client.StartDeviceAuthorization(command.Context())
				if err != nil {
					return err
				}
				if authorization.DeviceCode == "" || authorization.VerificationURL == "" {
					return output.Internal("device_authorization_response", "ViceMe API returned an incomplete device authorization", nil)
				}
				if noWait {
					intervalSeconds := authorization.IntervalSeconds
					if intervalSeconds < 1 {
						intervalSeconds = 1
					}
					apiBaseURL := strings.TrimRight(runtime.apiBaseURL, "/")
					apiOrigin, originErr := api.NormalizeAPIOrigin(apiBaseURL)
					if originErr != nil {
						return output.Validation("api_base_url", "ViceMe API base URL is invalid")
					}
					pending := credentialauth.PendingDeviceLogin{
						SchemaVersion:   1,
						ProfileID:       runtime.profile.ID,
						ProfileName:     runtime.profile.Name,
						Region:          string(runtime.region),
						APIBaseURL:      apiBaseURL,
						APIOrigin:       apiOrigin,
						CredentialScope: runtime.credentialScope,
						IntervalSeconds: intervalSeconds,
						ExpiresAt:       authorization.ExpiresAt,
					}
					if err := credentialauth.SavePendingDeviceLogin(runtime.deps.Store, authorization.DeviceCode, pending); err != nil {
						return output.Authentication("device_authorization_context_save", "could not save the pending device authorization context").
							WithHint("fix the secure credential store and start a new login flow").
							WithCause(err)
					}
					return runtime.business(deviceLoginStartResult{
						DeviceAuthorization: authorization,
						Profile:             runtime.profile.Name,
						Region:              string(runtime.region),
						ContinueArgs:        []string{"--profile", runtime.profile.Name, "auth", "login", "--device-code", authorization.DeviceCode, "--json"},
					})
				}
				writeHumanLoginStart(runtime.deps.ErrOut, authorization)
				deviceCode = authorization.DeviceCode
				interval := 2 * time.Second
				if authorization.IntervalSeconds > 0 {
					interval = time.Duration(authorization.IntervalSeconds) * time.Second
				}
				return finishDeviceLogin(command.Context(), runtime, client, deviceCode, timeout, interval, false)
			}
			pending, err := loadAndValidatePendingDeviceLogin(runtime, deviceCode)
			if err != nil {
				return err
			}
			remaining := pending.ExpiresAt.Sub(runtime.deps.Now())
			if remaining <= 0 {
				_ = credentialauth.DeletePendingDeviceLogin(runtime.deps.Store, deviceCode)
				return output.Authentication("device_code_expired", "the pending device authorization has expired")
			}
			if timeout > remaining {
				timeout = remaining
			}
			return finishDeviceLogin(command.Context(), runtime, client, deviceCode, timeout, time.Duration(pending.IntervalSeconds)*time.Second, true)
		},
	}
	command.Flags().BoolVar(&noWait, "no-wait", false, "return device authorization immediately for an Agent split-flow (requires --json)")
	command.Flags().BoolVar(&jsonOutput, "json", false, "use structured JSON output for an Agent split-flow")
	command.Flags().StringVar(&deviceCode, "device-code", "", "continue a previously started Agent authorization (requires --json)")
	command.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "maximum time to wait for browser authorization")
	return command
}

func writeHumanLoginStart(writer io.Writer, authorization api.DeviceAuthorization) {
	verificationURL := authorization.VerificationURLComplete
	if verificationURL == "" {
		verificationURL = authorization.VerificationURL
	}
	_, _ = fmt.Fprintln(writer, "Open this URL in your browser to sign in to ViceMe:")
	_, _ = fmt.Fprintf(writer, "\n  %s\n\n", verificationURL)
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
			credential := credentialauth.Credential{
				AccessToken:      token.AccessToken,
				RefreshToken:     token.RefreshToken,
				TokenType:        token.TokenType,
				ExpiresAt:        token.ExpiresAt,
				RefreshExpiresAt: token.RefreshExpiresAt,
				UserID:           token.UserID,
				Scope:            token.Scope,
			}
			manager := runtime.manager()
			if err := manager.Save(credential); err != nil {
				if !jsonOutput {
					revoked := client.RevokeWithToken(ctx, token.AccessToken) == nil
					return output.Authentication("credential_persistence_failed", "device authorization succeeded, but the issued credential could not be saved").
						WithHint("fix the local credential store and start a new 'viceme auth login' flow").
						WithDetails(map[string]any{"issued_credential_revoked": revoked}).
						WithCause(err)
				}
				return output.Authentication("credential_persistence_failed", "device authorization succeeded, but the issued credential could not be saved").
					WithHint("fix the local credential store, then retry the exact continuation command before the device authorization expires").
					WithDetails(map[string]any{"authorization_recoverable": true}).
					WithCause(err)
			}
			result := deviceLoginResult{Authenticated: true, Profile: runtime.profile.Name, Region: string(runtime.region), UserID: token.UserID}
			if err := runtime.recordProfileUserID(token.UserID); err != nil {
				result.Warnings = append(result.Warnings, "authenticated successfully, but the profile user ID metadata could not be updated")
			}
			if jsonOutput {
				if err := credentialauth.DeletePendingDeviceLogin(runtime.deps.Store, deviceCode); err != nil {
					result.Warnings = append(result.Warnings, "the expired continuation metadata could not be removed from the secure store")
				}
			}
			if !token.ExpiresAt.IsZero() {
				expiresAt := token.ExpiresAt
				result.ExpiresAt = &expiresAt
			}
			if jsonOutput {
				return runtime.business(result)
			}
			_, _ = fmt.Fprintln(runtime.deps.ErrOut, "Authorization successful.")
			_, _ = fmt.Fprintf(runtime.deps.ErrOut, "Profile: %s\nRegion: %s\n", result.Profile, result.Region)
			return nil
		}
		if !api.IsSubtype(err, "authorization_pending") && !api.IsSubtype(err, "device_poll_too_fast") {
			if jsonOutput && isTerminalDeviceAuthorizationError(err) {
				_ = credentialauth.DeletePendingDeviceLogin(runtime.deps.Store, deviceCode)
			}
			return err
		}
		if api.IsSubtype(err, "device_poll_too_fast") {
			interval += time.Second
		}
		if err := runtime.deps.Sleep(ctx, interval); err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				pending := output.Authentication("authorization_pending", "device authorization is still pending")
				pending.Retryable = true
				if jsonOutput {
					pending.Hint = "run 'viceme auth login --device-code <code> --json' again with the same device code before it expires"
				} else {
					pending.Hint = "run 'viceme auth login' again"
				}
				return pending
			}
			return err
		}
	}
}

func newAuthStatusCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show local ViceMe authentication status",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			status, err := runtime.manager().CurrentStatus()
			if err != nil {
				return err
			}
			return runtime.business(status)
		},
	}
}

func newAuthLogoutCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke and remove local ViceMe credentials",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			manager := runtime.manager()
			credential, err := manager.Load()
			if err != nil {
				var cliError *output.Error
				if errors.As(err, &cliError) && cliError.Subtype == "not_logged_in" {
					return runtime.business(map[string]any{"logged_out": true, "already_logged_out": true, "profile": runtime.profile.Name, "region": runtime.region})
				}
				return err
			}
			client := runtime.client()
			accessToken := credential.AccessToken
			if credential.RefreshRequestID != "" {
				accessToken, err = client.Tokens.Token(command.Context())
				if err != nil {
					return err
				}
			}
			if err := client.RevokeWithToken(command.Context(), accessToken); err != nil {
				return err
			}
			if err := manager.Delete(); err != nil {
				return err
			}
			return runtime.business(map[string]any{"logged_out": true, "profile": runtime.profile.Name, "region": runtime.region})
		},
	}
}

func loadAndValidatePendingDeviceLogin(runtime *Runtime, deviceCode string) (credentialauth.PendingDeviceLogin, error) {
	pending, err := credentialauth.LoadPendingDeviceLogin(runtime.deps.Store, deviceCode)
	if err != nil {
		return credentialauth.PendingDeviceLogin{}, output.Authentication("device_authorization_context_missing", "the pending device authorization context is missing or invalid").
			WithHint("restart login with 'viceme auth login --no-wait --json'; do not continue a device code on another machine or profile").
			WithCause(err)
	}
	apiBaseURL := strings.TrimRight(runtime.apiBaseURL, "/")
	apiOrigin, err := api.NormalizeAPIOrigin(apiBaseURL)
	if err != nil {
		return credentialauth.PendingDeviceLogin{}, output.Validation("api_base_url", "ViceMe API base URL is invalid")
	}
	if pending.ProfileID != runtime.profile.ID ||
		pending.ProfileName != runtime.profile.Name ||
		pending.Region != string(runtime.region) ||
		pending.APIBaseURL != apiBaseURL ||
		pending.APIOrigin != apiOrigin ||
		pending.CredentialScope != runtime.credentialScope {
		return credentialauth.PendingDeviceLogin{}, output.Authentication("device_authorization_context_mismatch", "the device authorization must be continued with the original profile and API endpoint").
			WithHint("run the exact continue_args returned by 'viceme auth login --no-wait --json'")
	}
	return pending, nil
}

func isTerminalDeviceAuthorizationError(err error) bool {
	for _, subtype := range []string{"authorization_denied", "device_code_expired", "device_authorization_not_found"} {
		if api.IsSubtype(err, subtype) {
			return true
		}
	}
	return false
}
