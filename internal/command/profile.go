package command

import (
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

type profileListItem struct {
	Name             string        `json:"name"`
	Region           config.Region `json:"region"`
	APIBaseURL       string        `json:"api_base_url,omitempty"`
	CredentialSource string        `json:"credential_source,omitempty"`
	Active           bool          `json:"active"`
	UserID           string        `json:"user_id,omitempty"`
	Authenticated    bool          `json:"authenticated"`
}

func newProfileCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "profile", Short: "Manage ViceMe CLI profiles"}
	command.AddCommand(newProfileListCommand(runtime))
	command.AddCommand(newProfileAddCommand(runtime))
	command.AddCommand(newProfileConfigureCommand(runtime))
	command.AddCommand(newProfileUseCommand(runtime))
	command.AddCommand(newProfileRenameCommand(runtime))
	command.AddCommand(newProfileRemoveCommand(runtime))
	return command
}

func newProfileListCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all profiles",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			items := make([]profileListItem, 0, len(runtime.config.Profiles))
			for _, profile := range runtime.config.Profiles {
				scope, scopeErr := runtime.credentialScopeForProfile(profile)
				if scopeErr != nil {
					return output.Validation("api_base_url", "ViceMe API base URL must use HTTPS; HTTP is allowed only for localhost or loopback development")
				}
				if profile.AccessToken != "" {
					items = append(items, profileListItem{
						Name:             profile.Name,
						Region:           profile.Region,
						APIBaseURL:       profile.APIBaseURL,
						CredentialSource: "local_profile",
						Active:           profile.Name == runtime.config.CurrentProfile,
						UserID:           profile.UserID,
						Authenticated:    true,
					})
					continue
				}
				manager := credentialauth.Manager{
					Store:       runtime.deps.Store,
					Region:      string(profile.Region),
					ProfileID:   profile.ID,
					ProfileName: profile.Name,
					Scope:       scope,
				}
				status, err := manager.CurrentStatus()
				if err != nil {
					return err
				}
				userID := profile.UserID
				credentialSource := ""
				if status.UserID != "" {
					userID = status.UserID
				}
				if status.Authenticated {
					credentialSource = "keychain"
				}
				items = append(items, profileListItem{
					Name:             profile.Name,
					Region:           profile.Region,
					APIBaseURL:       profile.APIBaseURL,
					CredentialSource: credentialSource,
					Active:           profile.Name == runtime.config.CurrentProfile,
					UserID:           userID,
					Authenticated:    status.Authenticated,
				})
			}
			return runtime.success(items)
		},
	}
}

func newProfileAddCommand(runtime *Runtime) *cobra.Command {
	var name string
	var region string
	var apiBaseURL string
	var accessToken string
	var use bool
	command := &cobra.Command{
		Use:   "add",
		Short: "Add a new profile",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if region == "" {
				region = string(runtime.region)
			}
			resolvedRegion, err := config.ParseRegion(region)
			if err != nil {
				return output.Validation("region", err.Error())
			}
			resolvedAPIBaseURL, err := validateProfileAPIBaseURL(apiBaseURL)
			if err != nil {
				return err
			}
			if command.Flags().Changed("access-token") {
				if err := validateLocalProfileAccessToken(accessToken); err != nil {
					return err
				}
			}
			profile, err := runtime.config.AddProfile(name, resolvedRegion)
			if err != nil {
				return output.Validation("profile", err.Error())
			}
			profile.APIBaseURL = resolvedAPIBaseURL
			profile.AccessToken = accessToken
			if profile.AccessToken != "" && profile.APIBaseURL == "" {
				return output.Validation("profile_access_token", "an explicit local access token requires --api-base-url")
			}
			if use {
				runtime.config.PreviousProfile = runtime.config.CurrentProfile
				runtime.config.CurrentProfile = profile.Name
			}
			result, err := config.Save(runtime.configBase, runtime.config)
			if err != nil {
				return output.Internal("config_save", "could not save the new profile", err)
			}
			responseName := profile.Name
			responseRegion := profile.Region
			responseAPIBaseURL := profile.APIBaseURL
			responseAccessTokenConfigured := profile.AccessToken != ""
			selected := runtime.config.CurrentProfile
			if err := runtime.reloadConfig(selected); err != nil {
				return err
			}
			return runtime.success(map[string]any{
				"name":                    responseName,
				"region":                  responseRegion,
				"api_base_url":            responseAPIBaseURL,
				"access_token_configured": responseAccessTokenConfigured,
				"active":                  use,
				"config":                  result,
			})
		},
	}
	command.Flags().StringVar(&name, "name", "", "profile name (required)")
	command.Flags().StringVar(&region, "region", "", "ViceMe region: cn or global (defaults to the selected profile region)")
	command.Flags().StringVar(&apiBaseURL, "api-base-url", "", "persist an API base URL for this profile")
	command.Flags().StringVar(&accessToken, "access-token", "", "persist an explicit local access token")
	command.Flags().BoolVar(&use, "use", false, "switch to this profile after adding")
	_ = command.MarkFlagRequired("name")
	return command
}

func newProfileConfigureCommand(runtime *Runtime) *cobra.Command {
	var apiBaseURL string
	var clearAPIBaseURL bool
	var accessToken string
	var clearAccessToken bool
	command := &cobra.Command{
		Use:   "configure <name>",
		Short: "Configure explicit local overrides for a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if command.Flags().Changed("api-base-url") && clearAPIBaseURL {
				return output.Validation("profile_config", "--api-base-url and --clear-api-base-url cannot be used together")
			}
			if command.Flags().Changed("access-token") && clearAccessToken {
				return output.Validation("profile_config", "--access-token and --clear-access-token cannot be used together")
			}
			if !command.Flags().Changed("api-base-url") && !clearAPIBaseURL && !command.Flags().Changed("access-token") && !clearAccessToken {
				return output.Validation("profile_config", "provide an API base URL or access token change")
			}
			index := runtime.config.FindProfileIndex(args[0])
			if index < 0 {
				return output.Validation("profile_not_found", "profile not found; available profiles: "+strings.Join(runtime.config.ProfileNames(), ", "))
			}
			profile := &runtime.config.Profiles[index]
			previousProfile := *profile
			if command.Flags().Changed("api-base-url") {
				resolved, err := validateProfileAPIBaseURL(apiBaseURL)
				if err != nil {
					return err
				}
				profile.APIBaseURL = resolved
			} else if clearAPIBaseURL {
				profile.APIBaseURL = ""
			}
			if command.Flags().Changed("access-token") {
				if err := validateLocalProfileAccessToken(accessToken); err != nil {
					return err
				}
				profile.AccessToken = accessToken
			} else if clearAccessToken {
				profile.AccessToken = ""
			}
			if profile.AccessToken != "" && profile.APIBaseURL == "" {
				return output.Validation("profile_access_token", "an explicit local access token requires an explicit profile API base URL")
			}
			if previousProfile.AccessToken != "" && !command.Flags().Changed("access-token") && !clearAccessToken &&
				!sameAPIOrigin(previousProfile.APIBaseURL, profile.APIBaseURL) {
				return output.Validation("profile_access_token_scope", "changing the API origin requires replacing or clearing the explicit local access token in the same command")
			}
			result, err := config.Save(runtime.configBase, runtime.config)
			if err != nil {
				return output.Internal("config_save", "could not configure the profile", err)
			}
			var warnings []string
			previousScope, previousScopeErr := credentialScopeForStoredProfile(previousProfile)
			currentScope, currentScopeErr := credentialScopeForStoredProfile(*profile)
			if previousScopeErr != nil || currentScopeErr != nil {
				return output.Validation("api_base_url", "ViceMe API base URL must use HTTPS; HTTP is allowed only for localhost or loopback development")
			}
			if credentialNamespace(previousProfile.Region, previousScope) != credentialNamespace(profile.Region, currentScope) {
				manager := credentialauth.Manager{
					Store:       runtime.deps.Store,
					Region:      string(previousProfile.Region),
					ProfileID:   previousProfile.ID,
					ProfileName: previousProfile.Name,
					Scope:       previousScope,
				}
				if err := manager.Delete(); err != nil {
					warnings = append(warnings, "profile API endpoint changed but its previous Keychain credential could not be removed")
				}
			}
			responseName := profile.Name
			responseRegion := profile.Region
			responseAPIBaseURL := profile.APIBaseURL
			responseAccessTokenConfigured := profile.AccessToken != ""
			if err := runtime.reloadConfig(runtime.config.CurrentProfile); err != nil {
				return err
			}
			response := map[string]any{
				"name":                    responseName,
				"region":                  responseRegion,
				"api_base_url":            responseAPIBaseURL,
				"access_token_configured": responseAccessTokenConfigured,
				"config":                  result,
			}
			if len(warnings) > 0 {
				response["warnings"] = warnings
			}
			return runtime.success(response)
		},
	}
	command.Flags().StringVar(&apiBaseURL, "api-base-url", "", "persist an API base URL for this profile")
	command.Flags().BoolVar(&clearAPIBaseURL, "clear-api-base-url", false, "clear the persisted API base URL")
	command.Flags().StringVar(&accessToken, "access-token", "", "replace the explicit local access token")
	command.Flags().BoolVar(&clearAccessToken, "clear-access-token", false, "clear the explicit local access token")
	return command
}

func validateProfileAPIBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if _, err := api.NormalizeAPIOrigin(value); err != nil {
		return "", output.Validation("api_base_url", "ViceMe API base URL must use HTTPS; HTTP is allowed only for localhost or loopback development")
	}
	return strings.TrimRight(value, "/"), nil
}

func credentialScopeForStoredProfile(profile config.Profile) (string, error) {
	apiBaseURL := profile.APIBaseURL
	if apiBaseURL == "" {
		apiBaseURL = config.APIBaseURL(profile.Region)
	}
	return credentialScopeForAPIBase(apiBaseURL, profile.Region)
}

func credentialNamespace(region config.Region, scope string) string {
	if scope != "" {
		return scope
	}
	return string(region)
}

func validateLocalProfileAccessToken(value string) error {
	if value == "" {
		return output.Validation("profile_access_token", "local profile access token cannot be empty")
	}
	if err := config.ValidateLocalAccessToken(value); err != nil {
		return output.Validation("profile_access_token", "invalid local profile access token: "+err.Error())
	}
	return nil
}

func newProfileUseCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Switch to a profile (use '-' to toggle back)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			if name == "-" {
				if runtime.config.PreviousProfile == "" {
					return output.Validation("profile_previous", "no previous profile to switch back to")
				}
				name = runtime.config.PreviousProfile
			}
			profile, err := runtime.config.Resolve(name)
			if err != nil {
				return output.Validation("profile_not_found", err.Error())
			}
			if runtime.config.CurrentProfile == profile.Name {
				return runtime.success(map[string]any{"name": profile.Name, "region": profile.Region, "active": true, "unchanged": true})
			}
			runtime.config.PreviousProfile = runtime.config.CurrentProfile
			runtime.config.CurrentProfile = profile.Name
			if _, err := config.Save(runtime.configBase, runtime.config); err != nil {
				return output.Internal("config_save", "could not switch profiles", err)
			}
			if err := runtime.reloadConfig(profile.Name); err != nil {
				return err
			}
			return runtime.success(map[string]any{"name": profile.Name, "region": profile.Region, "active": true})
		},
	}
}

func newProfileRenameCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename a profile",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			oldName, newName := args[0], args[1]
			if err := config.ValidateProfileName(newName); err != nil {
				return output.Validation("profile", err.Error())
			}
			index := runtime.config.FindProfileIndex(oldName)
			if index < 0 {
				return output.Validation("profile_not_found", "profile not found; available profiles: "+strings.Join(runtime.config.ProfileNames(), ", "))
			}
			if oldName == newName {
				profile := runtime.config.Profiles[index]
				return runtime.success(map[string]any{"old_name": oldName, "name": newName, "region": profile.Region, "unchanged": true})
			}
			if runtime.config.FindProfileIndex(newName) >= 0 {
				return output.Validation("profile_exists", "profile already exists")
			}
			runtime.config.Profiles[index].Name = newName
			if runtime.config.CurrentProfile == oldName {
				runtime.config.CurrentProfile = newName
			}
			if runtime.config.PreviousProfile == oldName {
				runtime.config.PreviousProfile = newName
			}
			if _, err := config.Save(runtime.configBase, runtime.config); err != nil {
				return output.Internal("config_save", "could not rename the profile", err)
			}
			if err := runtime.reloadConfig(newName); err != nil {
				return err
			}
			return runtime.success(map[string]any{"old_name": oldName, "name": newName})
		},
	}
}

func newProfileRemoveCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a profile and its local credentials",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			index := runtime.config.FindProfileIndex(name)
			if index < 0 {
				return output.Validation("profile_not_found", "profile not found; available profiles: "+strings.Join(runtime.config.ProfileNames(), ", "))
			}
			if len(runtime.config.Profiles) == 1 {
				return output.Validation("profile_last", "cannot remove the only profile")
			}
			removed := runtime.config.Profiles[index]
			runtime.config.Profiles = append(runtime.config.Profiles[:index], runtime.config.Profiles[index+1:]...)
			if runtime.config.CurrentProfile == removed.Name {
				runtime.config.CurrentProfile = runtime.config.Profiles[0].Name
			}
			if runtime.config.PreviousProfile == removed.Name {
				runtime.config.PreviousProfile = ""
			}
			if _, err := config.Save(runtime.configBase, runtime.config); err != nil {
				return output.Internal("config_save", "could not remove the profile", err)
			}
			scope, scopeErr := runtime.credentialScopeForProfile(removed)
			if scopeErr != nil {
				return output.Validation("api_base_url", "ViceMe API base URL must use HTTPS; HTTP is allowed only for localhost or loopback development")
			}
			manager := credentialauth.Manager{
				Store:       runtime.deps.Store,
				Region:      string(removed.Region),
				ProfileID:   removed.ID,
				ProfileName: removed.Name,
				Scope:       scope,
			}
			var warnings []string
			if err := manager.Delete(); err != nil {
				warnings = append(warnings, "profile was removed but its local credential could not be removed from the operating system keychain")
			}
			if err := runtime.reloadConfig(runtime.config.CurrentProfile); err != nil {
				return err
			}
			result := map[string]any{"removed": true, "name": removed.Name}
			if len(warnings) > 0 {
				result["warnings"] = warnings
			}
			return runtime.success(result)
		},
	}
}
