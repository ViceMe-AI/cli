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
	"github.com/ViceMe-AI/cli/internal/config"
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
	var jsonOutput bool
	var deviceCode string
	var timeout time.Duration
	command := &cobra.Command{
		Use:   "login",
		Short: "Start or continue the ViceMe device login flow",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if _, source, _ := runtime.overrideCredential(); source != "" {
				if source == "process" {
					return output.Policy("process_credential_active", "device login is disabled while a process credential is active").WithHint("start a CLI process without VICEME_ACCESS_TOKEN to manage persistent login")
				}
				return output.Policy("local_profile_credential_active", "device login is disabled while the selected profile has an explicit local access token").WithHint("clear the local profile access token before managing persistent device login")
			}
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
					return runtime.business(deviceLoginStartResult{
						DeviceAuthorization: authorization,
						Profile:             runtime.profile.Name,
						Region:              string(runtime.region),
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
			return finishDeviceLogin(command.Context(), runtime, client, deviceCode, timeout, 2*time.Second, true)
		},
	}
	command.Flags().BoolVar(&noWait, "no-wait", false, "return device authorization immediately for an Agent split-flow (requires --json)")
	command.Flags().BoolVar(&jsonOutput, "json", false, "use structured JSON output for an Agent split-flow")
	command.Flags().StringVar(&deviceCode, "device-code", "", "continue a previously started Agent authorization (requires --json)")
	command.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "maximum time to wait for browser authorization")
	return command
}

func writeHumanLoginStart(writer io.Writer, authorization api.DeviceAuthorization) {
	_, _ = fmt.Fprintln(writer, "Open this URL in your browser to sign in to ViceMe:")
	_, _ = fmt.Fprintf(writer, "\n  %s\n\n", authorization.VerificationURL)
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
				AccessToken:  token.AccessToken,
				RefreshToken: token.RefreshToken,
				TokenType:    token.TokenType,
				ExpiresAt:    token.ExpiresAt,
				UserID:       token.UserID,
			}
			manager := runtime.manager()
			if err := manager.Save(credential); err != nil {
				revoked := client.Revoke(ctx, token.AccessToken) == nil
				return output.Authentication("credential_persistence_failed", "device authorization succeeded, but the issued credential could not be saved").
					WithHint("the one-time device authorization was consumed; fix the local credential store and start a new 'viceme auth login' flow").
					WithDetails(map[string]any{"authorization_consumed": true, "issued_credential_revoked": revoked}).
					WithCause(err)
			}
			if err := runtime.recordProfileUserID(token.UserID); err != nil {
				_ = manager.Delete()
				return err
			}
			result := deviceLoginResult{Authenticated: true, Profile: runtime.profile.Name, Region: string(runtime.region), UserID: token.UserID}
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
	var verify bool
	command := &cobra.Command{
		Use:   "status",
		Short: "Show local ViceMe authentication status",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			credential := runtime.overrideCredentialDetails()
			if credential.source != "" {
				result := runtime.overrideAuthStatus(credential)
				if !verify || result["authenticated"] == false {
					return runtime.business(result)
				}
				remote, err := runtime.client().CredentialStatus(command.Context())
				if err != nil {
					return runtime.business(runtime.failedCredentialVerification(result, credential, err))
				}
				if credential.source == "local_profile" {
					if err := runtime.cacheProfileCredentialMetadata(remote.CredentialStatus, remote.ExpiresAt); err != nil {
						return err
					}
				}
				mergeCredentialStatus(result, remote)
				result["verified"] = true
				return runtime.business(result)
			}
			status, err := runtime.manager().CurrentStatus()
			if err != nil {
				return err
			}
			if !verify || !status.Authenticated {
				return runtime.business(status)
			}
			result := map[string]any{
				"authenticated": status.Authenticated,
				"profile":       status.Profile,
				"region":        status.Region,
			}
			if status.UserID != "" {
				result["user_id"] = status.UserID
			}
			if status.ExpiresAt != nil {
				result["expires_at"] = status.ExpiresAt
			}
			remote, err := runtime.client().CredentialStatus(command.Context())
			if err != nil {
				return runtime.business(runtime.failedCredentialVerification(result, credential, err))
			}
			mergeCredentialStatus(result, remote)
			result["verified"] = true
			return runtime.business(result)
		},
	}
	command.Flags().BoolVar(&verify, "verify", false, "verify the active credential against the ViceMe API")
	return command
}

const publicationCredentialExpiryWarning = 5 * time.Minute

func localPublicationCredentialStatus(now time.Time, profile config.Profile) (bool, string) {
	status := strings.ToUpper(profile.AccessTokenStatus)
	if status == "REVOKED" || status == "EXPIRED" || status == "INVALID" {
		return false, strings.ToLower(status)
	}
	if profile.AccessTokenExpiresAt == nil {
		if status == "" {
			return true, "unknown"
		}
		return true, strings.ToLower(status)
	}
	if !now.Before(*profile.AccessTokenExpiresAt) {
		return false, "expired"
	}
	if !now.Add(publicationCredentialExpiryWarning).Before(*profile.AccessTokenExpiresAt) {
		return true, "expiring"
	}
	if status == "" {
		return true, "ready"
	}
	return true, strings.ToLower(status)
}

func (r *Runtime) overrideAuthStatus(credential overrideCredential) map[string]any {
	authenticated, status := true, "unknown"
	if credential.source == "local_profile" {
		authenticated, status = localPublicationCredentialStatus(r.deps.Now(), r.profile)
	}
	result := map[string]any{
		"authenticated":     authenticated,
		"source":            credential.source,
		"persistent":        credential.persistent,
		"profile":           r.profile.Name,
		"region":            r.region,
		"credential_status": status,
	}
	if credential.expiresAt != nil {
		result["expires_at"] = credential.expiresAt
	}
	return result
}

func (r *Runtime) failedCredentialVerification(result map[string]any, credential overrideCredential, err error) map[string]any {
	result["verified"] = false
	result["verify_error"] = err.Error()
	var cliError *output.Error
	if !errors.As(err, &cliError) {
		result["credential_status"] = "verify_failed"
		return result
	}
	switch cliError.Subtype {
	case "publication_credential_expired":
		result["authenticated"] = false
		result["credential_status"] = "expired"
		if credential.source == "local_profile" {
			_ = r.cacheProfileCredentialMetadata("EXPIRED", credential.expiresAt)
		}
	case "publication_credential_revoked":
		result["authenticated"] = false
		result["credential_status"] = "revoked"
		if credential.source == "local_profile" {
			_ = r.cacheProfileCredentialMetadata("REVOKED", credential.expiresAt)
		}
	default:
		if cliError.Type == "authentication" || cliError.Type == "authorization" {
			result["authenticated"] = false
			result["credential_status"] = "invalid"
			if credential.source == "local_profile" {
				_ = r.cacheProfileCredentialMetadata("INVALID", credential.expiresAt)
			}
		} else {
			result["credential_status"] = "verify_failed"
		}
	}
	if cliError.Hint != "" {
		result["hint"] = cliError.Hint
	}
	return result
}

func mergeCredentialStatus(result map[string]any, status api.CredentialStatus) {
	result["authenticated"] = status.Authenticated
	result["credential_type"] = status.CredentialType
	result["credential_status"] = strings.ToLower(status.CredentialStatus)
	result["new_publication_allowed"] = status.NewPublicationAllowed
	result["actor_user_id"] = status.ActorUserID
	result["effective_user_id"] = status.EffectiveUserID
	if status.ExpiresAt != nil {
		result["expires_at"] = status.ExpiresAt
	}
	if status.AuthorizationKind != nil {
		result["authorization_kind"] = *status.AuthorizationKind
	}
	if status.AuthorizationID != nil {
		result["authorization_id"] = *status.AuthorizationID
	}
	if status.PublicationID != nil {
		result["publication_id"] = *status.PublicationID
	}
}

func (r *Runtime) cacheProfileCredentialMetadata(status string, expiresAt *time.Time) error {
	index := r.config.FindProfileIndex(r.profile.Name)
	if index < 0 || r.config.Profiles[index].AccessToken != r.profile.AccessToken {
		return output.Internal("config_profile", "could not update publication credential metadata", nil)
	}
	profile := &r.config.Profiles[index]
	profile.AccessTokenStatus = strings.ToUpper(status)
	profile.AccessTokenExpiresAt = expiresAt
	if _, err := config.Save(r.configBase, r.config); err != nil {
		return output.Internal("config_save", "could not save publication credential metadata", err)
	}
	r.profile = *profile
	return nil
}

func newAuthLogoutCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke and remove local ViceMe credentials",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if _, source, _ := runtime.overrideCredential(); source != "" {
				if source == "process" {
					return output.Policy("process_credential_active", "logout cannot revoke or delete a process credential").WithHint("stop passing VICEME_ACCESS_TOKEN to discard the process credential")
				}
				return output.Policy("local_profile_credential_active", "logout cannot revoke or delete an explicit local profile credential").WithHint("run 'viceme profile configure <name> --clear-access-token' to remove the local override")
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
