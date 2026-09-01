package command

import (
	"context"
	"errors"

	"github.com/ViceMe-AI/cli/internal/output"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
	"github.com/spf13/cobra"
)

func newUpdateCommand(runtime *Runtime) *cobra.Command {
	var checkOnly bool
	var target string
	command := &cobra.Command{
		Use:   "update",
		Short: "Update the CLI release and reinstall matching official Skills",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(command.Context(), activationOperationTimeout)
			defer cancel()
			check, err := runtime.deps.Updater.Check(ctx)
			if err != nil {
				return updaterError(err, nil)
			}
			if checkOnly {
				return runtime.business(check)
			}
			if check.UpdateAvailable {
				if err := updatepkg.ProbeRenameCapability(runtime.configBase); err != nil {
					if errors.Is(err, updatepkg.ErrRenameDenied) {
						return output.Policy("UPDATE_ACTIVATION_SANDBOX_BLOCKED", "this environment denies the file renames needed to activate a new ViceMe CLI generation").
							WithHint("run 'viceme update' from a regular terminal outside the agent sandbox; the installed generation remains fully usable")
					}
					return output.Internal("UPDATE_ACTIVATION_PROBE_FAILED", "could not verify whether this environment can activate a new ViceMe CLI generation", err)
				}
			}
			result, err := runtime.deps.Updater.Apply(ctx, check, updatepkg.ApplyOptions{
				RefreshSkills: true,
				SkillTarget:   target,
			})
			if errors.Is(err, updatepkg.ErrNPMInstallRequired) {
				return output.Policy("update_install_method", "this CLI was not started through the npm launcher").WithHint("run 'npx --yes --registry=https://registry.npmjs.org --@viceme-ai:registry=https://registry.npmjs.org --package=@viceme-ai/cli@latest -- viceme install', then use the installed 'viceme' launcher")
			}
			if err != nil {
				return updaterError(err, result)
			}
			return runtime.business(result)
		},
	}
	command.Flags().BoolVar(&checkOnly, "check", false, "check the latest npm release without changing local state")
	command.Flags().StringVar(&target, "agent", "auto", "Agent target refreshed after update: auto, codex, claude, workbuddy, or agents")
	return command
}

func updaterError(err error, details any) *output.Error {
	var result *output.Error
	switch updatepkg.ErrorKindOf(err) {
	case updatepkg.ErrorRegistryNetwork:
		result = output.Network("update_registry_unavailable", "could not reach the npm registry", err)
	case updatepkg.ErrorRegistryResponse:
		result = output.Internal("update_registry_response", "npm registry returned an invalid release response", err)
	case updatepkg.ErrorNPMMissing:
		result = output.Policy("update_npm_missing", "npm is required to update this installation").WithHint("install npm and ensure it is available in PATH")
	case updatepkg.ErrorNPMPermission:
		result = output.Internal("update_npm_permission", "npm could not write the ViceMe cache or global installation directory", err).WithHint("ensure the ViceMe configuration directory and the npm global prefix are writable")
	case updatepkg.ErrorNPMCommand:
		result = output.Internal("update_npm_failed", "npm did not complete the CLI update", err).WithHint("run 'npm doctor' and verify the configured npm registry, proxy, and global prefix")
	case updatepkg.ErrorReleaseNetwork:
		result = output.Network("UPDATE_RELEASE_UNAVAILABLE", "could not reach the official ViceMe release store", err)
	case updatepkg.ErrorReleaseResponse:
		result = output.Internal("UPDATE_RELEASE_INVALID", "the official release response was invalid", err)
	case updatepkg.ErrorReleaseIntegrity:
		result = output.Policy("UPDATE_RELEASE_INTEGRITY_FAILED", "the downloaded ViceMe release failed checksum verification")
	case updatepkg.ErrorReleaseReplace:
		result = output.Internal("UPDATE_REPLACE_FAILED", "the ViceMe executable could not be replaced", err).WithHint("rerun the official bootstrap installer for the selected region")
	case updatepkg.ErrorReleaseSkillRefresh:
		result = output.Internal("UPDATE_SKILL_REFRESH_FAILED", "the CLI was updated but official Skills could not be refreshed", err).WithHint("run 'viceme install --agent auto' to repair the matching Skills")
	default:
		result = output.Internal("update_partial", "CLI update did not complete for every target", err)
	}
	if details != nil {
		result.WithDetails(details)
	}
	return result
}
