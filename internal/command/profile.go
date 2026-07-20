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
	Active        bool          `json:"active"`
	UserID        string        `json:"user_id,omitempty"`
	Authenticated bool          `json:"authenticated"`
}

func newProfileCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "profile", Short: "Manage Viceme CLI profiles"}
	command.AddCommand(newProfileListCommand(runtime))
	command.AddCommand(newProfileAddCommand(runtime))
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
				scope, scopeErr := runtime.credentialScopeForRegion(profile.Region)
				if scopeErr != nil {
					return output.Validation("api_base_url", "Viceme API base URL must use HTTPS; HTTP is allowed only for localhost or loopback development")
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
				if status.UserID != "" {
					userID = status.UserID
				}
				items = append(items, profileListItem{
					Name:          profile.Name,
					Region:        profile.Region,
					Active:        profile.Name == runtime.config.CurrentProfile,
					UserID:        userID,
					Authenticated: status.Authenticated,
				})
			}
			return runtime.success(items)
		},
	}
}

func newProfileAddCommand(runtime *Runtime) *cobra.Command {
	var name string
	var region string
	var use bool
	command := &cobra.Command{
		Use:   "add",
		Short: "Add a new profile",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if region == "" {
				region = string(runtime.region)
			}
			resolvedRegion, err := config.ParseRegion(region)
			if err != nil {
				return output.Validation("region", err.Error())
			}
			profile, err := runtime.config.AddProfile(name, resolvedRegion)
			if err != nil {
				return output.Validation("profile", err.Error())
			}
			if use {
				runtime.config.PreviousProfile = runtime.config.CurrentProfile
				runtime.config.CurrentProfile = profile.Name
			}
			result, err := config.Save(runtime.configBase, runtime.config)
			if err != nil {
				return output.Internal("config_save", "could not save the new profile", err)
			}
			selected := runtime.config.CurrentProfile
			if err := runtime.reloadConfig(selected); err != nil {
				return err
			}
			return runtime.success(map[string]any{
				"name":   profile.Name,
				"region": profile.Region,
				"active": use,
				"config": result,
			})
		},
	}
	command.Flags().StringVar(&name, "name", "", "profile name (required)")
	command.Flags().StringVar(&region, "region", "", "Viceme region: cn or global (defaults to the selected profile region)")
	command.Flags().BoolVar(&use, "use", false, "switch to this profile after adding")
	_ = command.MarkFlagRequired("name")
	return command
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
			scope, scopeErr := runtime.credentialScopeForRegion(removed.Region)
			if scopeErr != nil {
				return output.Validation("api_base_url", "Viceme API base URL must use HTTPS; HTTP is allowed only for localhost or loopback development")
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
