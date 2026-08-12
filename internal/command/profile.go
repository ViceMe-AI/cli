package command

import (
	"strings"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

type profileListItem struct {
	Name          string        `json:"name"`
	Region        config.Region `json:"region"`
	APIBaseURL    string        `json:"apiBaseUrl"`
	Active        bool          `json:"active"`
	UserID        string        `json:"userId,omitempty"`
	Authenticated bool          `json:"authenticated"`
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
					Store: runtime.deps.Store, Region: string(profile.Region),
					ProfileID: profile.ID, ProfileName: profile.Name, Scope: scope,
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
					Name: profile.Name, Region: profile.Region, APIBaseURL: profile.ResolvedAPIBaseURL(),
					Active: profile.Name == runtime.config.CurrentProfile,
					UserID: userID, Authenticated: status.Authenticated,
				})
			}
			return runtime.business(items)
		},
	}
}

func newProfileAddCommand(runtime *Runtime) *cobra.Command {
	var name string
	var region string
	var apiBaseURL string
	var use bool
	command := &cobra.Command{
		Use: "add", Short: "Add a regional ViceMe profile", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if region == "" {
				region = string(runtime.region)
			}
			resolved, err := config.ParseRegion(region)
			if err != nil {
				return output.Validation("PROFILE_REGION_INVALID", err.Error())
			}
			profile, err := runtime.config.AddProfile(name, resolved, apiBaseURL)
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
				"name": profile.Name, "region": profile.Region,
				"apiBaseUrl": profile.ResolvedAPIBaseURL(), "active": use, "config": result,
			})
		},
	}
	command.Flags().StringVar(&name, "name", "", "profile name")
	command.Flags().StringVar(&region, "region", "", "ViceMe region: cn or global")
	command.Flags().StringVar(&apiBaseURL, "api-base-url", "", "persist a custom API base URL for this profile")
	command.Flags().BoolVar(&use, "use", false, "switch to this profile")
	_ = command.MarkFlagRequired("name")
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
					"name": profile.Name, "region": profile.Region,
					"apiBaseUrl": profile.ResolvedAPIBaseURL(), "active": true, "unchanged": true,
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
				"name": profile.Name, "region": profile.Region,
				"apiBaseUrl": profile.ResolvedAPIBaseURL(), "active": true,
			})
		},
	}
}

func newProfileRemoveCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "remove <name>", Short: "Remove a profile and its local credential", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
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
				Store: runtime.deps.Store, Region: string(removed.Region),
				ProfileID: removed.ID, ProfileName: removed.Name, Scope: scope,
			}
			if err := manager.Delete(); err != nil {
				return err
			}
			runtime.config.Profiles = append(runtime.config.Profiles[:index], runtime.config.Profiles[index+1:]...)
			if runtime.config.CurrentProfile == removed.Name {
				runtime.config.CurrentProfile = runtime.config.Profiles[0].Name
			}
			if runtime.config.PreviousProfile == removed.Name {
				runtime.config.PreviousProfile = ""
			}
			if _, err := config.Save(runtime.configBase, runtime.config); err != nil {
				return output.Internal("PROFILE_SAVE_FAILED", "could not remove the profile", err)
			}
			if err := runtime.reloadConfig(runtime.config.CurrentProfile); err != nil {
				return err
			}
			return runtime.business(map[string]any{"removed": name, "active": runtime.config.CurrentProfile})
		},
	}
}
