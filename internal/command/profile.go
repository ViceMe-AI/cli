package command

import (
	"errors"
	"fmt"
	"strings"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

type profileListItem struct {
	Name          string `json:"name"`
	APIBaseURL    string `json:"apiBaseUrl"`
	WebBaseURL    string `json:"webBaseUrl"`
	MarketRegion  string `json:"marketRegion"`
	Active        bool   `json:"active"`
	UserID        string `json:"userId,omitempty"`
	Authenticated bool   `json:"authenticated"`
}

func newProfileCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "profile", Short: "Manage ViceMe CLI profiles"}
	command.AddCommand(newProfileListCommand(runtime))
	command.AddCommand(newProfileAddCommand(runtime))
	command.AddCommand(newProfileUseCommand(runtime))
	command.AddCommand(newProfileRemoveCommand(runtime))
	return command
}

func newProfileListCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List all profiles", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			items := make([]profileListItem, 0, len(runtime.config.Profiles))
			for _, profile := range runtime.config.Profiles {
				scope, err := runtime.credentialScopeForProfile(profile)
				if err != nil {
					return output.Validation("PROFILE_API_BASE_URL_INVALID", err.Error())
				}
				manager := credentialauth.Manager{
					Store: runtime.deps.Store, Region: string(runtime.config.DistributionRegion),
					ProfileID: profile.ID, ProfileName: profile.Name, Scope: scope,
					LegacyRegion: legacyCredentialRegionForAPIBase(profile.ResolvedAPIBaseURL()),
				}
				status, err := manager.CurrentStatus()
				if err != nil {
					return err
				}
				userID := profile.UserID
				if status.UserID != "" {
					userID = status.UserID
				}
				items = append(items, profileListItem{
					Name: profile.Name, APIBaseURL: profile.ResolvedAPIBaseURL(),
					WebBaseURL:   profile.ResolvedWebBaseURL(),
					MarketRegion: string(profile.MarketRegion),
					Active:       profile.Name == runtime.config.CurrentProfile,
					UserID:       userID, Authenticated: status.Authenticated,
				})
			}
			return runtime.business(items)
		},
	}
}

func newProfileAddCommand(runtime *Runtime) *cobra.Command {
	var name string
	var apiBaseURL string
	var webBaseURL string
	var marketRegion string
	var use bool
	command := &cobra.Command{
		Use: "add", Short: "Add a ViceMe API profile", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			profile, err := runtime.config.AddProfile(name, apiBaseURL, webBaseURL, config.Region(marketRegion))
			if err != nil {
				return output.Validation("PROFILE_INVALID", err.Error())
			}
			if use {
				runtime.config.PreviousProfile = runtime.config.CurrentProfile
				runtime.config.CurrentProfile = profile.Name
			}
			result, err := config.Save(runtime.configBase, runtime.config)
			if err != nil {
				return output.Internal("PROFILE_SAVE_FAILED", "could not save the new profile", err)
			}
			if err := runtime.reloadConfig(runtime.config.CurrentProfile); err != nil {
				return err
			}
			return runtime.business(map[string]any{
				"name":       profile.Name,
				"apiBaseUrl": profile.ResolvedAPIBaseURL(), "webBaseUrl": profile.ResolvedWebBaseURL(),
				"marketRegion": profile.MarketRegion,
				"active":       use, "config": result,
			})
		},
	}
	command.Flags().StringVar(&name, "name", "", "profile name")
	command.Flags().StringVar(&apiBaseURL, "api-base-url", "", "API base URL for this profile")
	command.Flags().StringVar(&webBaseURL, "web-base-url", "", "matching Web base URL for this profile")
	command.Flags().StringVar(&marketRegion, "market-region", "", "product market region: cn or global")
	command.Flags().BoolVar(&use, "use", false, "switch to this profile")
	_ = command.MarkFlagRequired("name")
	_ = command.MarkFlagRequired("api-base-url")
	return command
}

func newProfileUseCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "use <name>", Short: "Switch to a profile", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			profile, err := runtime.config.Resolve(args[0])
			if err != nil {
				return output.Validation("PROFILE_NOT_FOUND", err.Error())
			}
			if runtime.config.CurrentProfile == profile.Name {
				return runtime.business(map[string]any{
					"name":       profile.Name,
					"apiBaseUrl": profile.ResolvedAPIBaseURL(), "webBaseUrl": profile.ResolvedWebBaseURL(),
					"marketRegion": profile.MarketRegion,
					"active":       true, "unchanged": true,
				})
			}
			runtime.config.PreviousProfile = runtime.config.CurrentProfile
			runtime.config.CurrentProfile = profile.Name
			if _, err := config.Save(runtime.configBase, runtime.config); err != nil {
				return output.Internal("PROFILE_SAVE_FAILED", "could not switch profiles", err)
			}
			if err := runtime.reloadConfig(profile.Name); err != nil {
				return err
			}
			return runtime.business(map[string]any{
				"name":       profile.Name,
				"apiBaseUrl": profile.ResolvedAPIBaseURL(), "webBaseUrl": profile.ResolvedWebBaseURL(),
				"marketRegion": profile.MarketRegion, "active": true,
			})
		},
	}
}

func newProfileRemoveCommand(runtime *Runtime) *cobra.Command {
	var all bool
	var yes bool
	command := &cobra.Command{
		Use: "remove [name]", Short: "Remove one profile or reset all profiles and credentials", Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if all {
				if len(args) != 0 {
					return output.Validation("PROFILE_REMOVE_FLAGS_CONFLICT", "a profile name cannot be combined with --all")
				}
				if !yes {
					return output.Confirmation(
						"PROFILE_REMOVE_ALL_CONFIRMATION_REQUIRED",
						"removing all profiles also removes every locally stored ViceMe credential; pass --yes to confirm",
					)
				}
				return removeAllProfiles(runtime)
			}
			if len(args) == 0 {
				return output.Validation("PROFILE_NAME_REQUIRED", "provide a profile name or use --all --yes")
			}
			if yes {
				return output.Validation("PROFILE_REMOVE_FLAGS_CONFLICT", "--yes is only valid with --all")
			}
			name := args[0]
			index := runtime.config.FindProfileIndex(name)
			if index < 0 {
				return output.Validation("PROFILE_NOT_FOUND", "profile not found; available profiles: "+strings.Join(runtime.config.ProfileNames(), ", "))
			}
			if len(runtime.config.Profiles) == 1 {
				return output.Validation("PROFILE_LAST", "cannot remove the only profile")
			}
			removed := runtime.config.Profiles[index]
			scope, err := runtime.credentialScopeForProfile(removed)
			if err != nil {
				return output.Validation("PROFILE_API_BASE_URL_INVALID", err.Error())
			}
			manager := credentialauth.Manager{
				Store: runtime.deps.Store, Region: string(runtime.config.DistributionRegion),
				ProfileID: removed.ID, ProfileName: removed.Name, Scope: scope,
				LegacyRegion: legacyCredentialRegionForAPIBase(removed.ResolvedAPIBaseURL()),
			}
			previous := cloneConfig(runtime.config)
			candidate := cloneConfig(runtime.config)
			candidate.Profiles = append(candidate.Profiles[:index], candidate.Profiles[index+1:]...)
			if candidate.CurrentProfile == removed.Name {
				candidate.CurrentProfile = candidate.Profiles[0].Name
			}
			if candidate.PreviousProfile == removed.Name {
				candidate.PreviousProfile = ""
			}
			if _, err := config.Save(runtime.configBase, candidate); err != nil {
				return output.Internal("PROFILE_SAVE_FAILED", "could not remove the profile", err)
			}
			if err := manager.Delete(); err != nil {
				return rollbackProfileConfig(runtime.configBase, previous, err)
			}
			runtime.config = candidate
			if err := runtime.reloadConfig(runtime.config.CurrentProfile); err != nil {
				return err
			}
			return runtime.business(map[string]any{"removed": name, "active": runtime.config.CurrentProfile})
		},
	}
	command.Flags().BoolVar(&all, "all", false, "remove every profile and local credential, then recreate an unauthenticated default profile")
	command.Flags().BoolVar(&yes, "yes", false, "confirm removal of every profile and local credential")
	return command
}

func removeAllProfiles(runtime *Runtime) error {
	if _, source, _ := runtime.overrideCredential(); source != "" {
		return output.Policy("PROCESS_CREDENTIAL_ACTIVE", "profile cleanup is disabled while VICEME_ACCESS_TOKEN is active").
			WithHint("start a CLI process without VICEME_ACCESS_TOKEN, then retry the same cleanup command")
	}
	removedProfiles := runtime.config.ProfileNames()
	credentialKeys, err := runtime.credentialStorageKeys()
	if err != nil {
		return output.Validation("PROFILE_API_BASE_URL_INVALID", err.Error())
	}
	previous := cloneConfig(runtime.config)
	reset := config.Default(runtime.region)
	result, err := config.Save(runtime.configBase, reset)
	if err != nil {
		return output.Internal("PROFILE_SAVE_FAILED", "could not save the clean default profile; no credentials were removed", err).
			WithHint("repair the ViceMe configuration directory, then retry 'viceme profile remove --all --yes'")
	}
	removedCredentials, err := credentialauth.DeleteStorageKeys(runtime.deps.Store, credentialKeys)
	if err != nil {
		return rollbackProfileConfig(runtime.configBase, previous, err)
	}
	runtime.config = reset
	if err := runtime.reloadConfig(reset.CurrentProfile); err != nil {
		return err
	}
	return runtime.business(map[string]any{
		"removedProfiles":    removedProfiles,
		"removedCredentials": removedCredentials,
		"active":             reset.CurrentProfile,
		"authenticated":      false,
		"config":             result,
	})
}

func cloneConfig(source config.Config) config.Config {
	cloned := source
	cloned.Profiles = append([]config.Profile(nil), source.Profiles...)
	return cloned
}

func rollbackProfileConfig(configBase string, previous config.Config, cause error) error {
	if _, rollbackErr := config.Save(configBase, previous); rollbackErr != nil {
		return output.Internal(
			"PROFILE_REMOVE_ROLLBACK_FAILED",
			"profile cleanup failed and the previous Profile configuration could not be restored",
			errors.Join(cause, fmt.Errorf("restore profile configuration: %w", rollbackErr)),
		).WithHint("repair the ViceMe configuration directory before retrying; locally stored credentials may require reconciliation")
	}
	return cause
}
