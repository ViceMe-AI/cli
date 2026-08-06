package command

import (
	"errors"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/appmanifest"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newCapabilityCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "capability", Short: "Configure Creator App capabilities"}
	command.AddCommand(newCapabilityCatalogCommand(runtime))
	command.AddCommand(newCapabilityAddCommand(runtime))
	command.AddCommand(newCapabilityGetCommand(runtime))
	command.AddCommand(newCapabilityDoctorCommand(runtime))
	return command
}

func newCapabilityCatalogCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "catalog",
		Short: "List platform capabilities and availability",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			catalog, err := runtime.client().CapabilityCatalog(command.Context())
			if err != nil {
				return err
			}
			return runtime.business(catalog)
		},
	}
}

func newCapabilityAddCommand(runtime *Runtime) *cobra.Command {
	var directory string
	var appID string
	command := &cobra.Command{
		Use:   "add <name>",
		Short: "Enable one capability foundation for the linked App environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			projectDirectory, manifest, err := loadAppBinding(directory, appID)
			if err != nil {
				return err
			}
			name := strings.ToUpper(args[0])
			if name == "COMMERCE" && manifest.Environment != "TEST" {
				return output.Policy(
					"commerce_test_only",
					"Commerce capability is available only for the linked TEST environment in this release",
				)
			}
			capability, err := runtime.client().AddCreatorAppCapability(command.Context(), manifest.AppID, manifest.Environment, api.AddCapabilityRequest{Type: name, Config: map[string]any{}})
			if err != nil {
				return err
			}
			if manifest.Capabilities == nil {
				return output.Validation("app_manifest_invalid", "App manifest capabilities must be a JSON object")
			}
			manifest.Capabilities[strings.ToLower(capability.Type)] = capabilityBinding(capability)
			manifestPath, err := appmanifest.Save(projectDirectory, manifest)
			if err != nil {
				return output.Internal("app_manifest_save", "Capability was enabled remotely but the local manifest could not be updated", err).
					WithHint("fix local permissions and rerun the same capability add command; the remote operation is idempotent")
			}
			return runtime.business(map[string]any{"capability": capability, "manifest": manifestPath})
		},
	}
	addBindingFlags(command, &directory, &appID)
	return command
}

func newCapabilityGetCommand(runtime *Runtime) *cobra.Command {
	var directory string
	var appID string
	command := &cobra.Command{
		Use:   "get <name>",
		Short: "Get one capability from the linked App environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			_, manifest, err := loadAppBinding(directory, appID)
			if err != nil {
				return err
			}
			capability, err := runtime.client().GetCreatorAppCapability(command.Context(), manifest.AppID, manifest.Environment, strings.ToUpper(args[0]))
			if err != nil {
				return err
			}
			return runtime.business(capability)
		},
	}
	addBindingFlags(command, &directory, &appID)
	return command
}

func newCapabilityDoctorCommand(runtime *Runtime) *cobra.Command {
	var directory string
	var appID string
	command := &cobra.Command{
		Use:   "doctor <name>",
		Short: "Check one local capability binding against the control plane",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			_, manifest, err := loadAppBinding(directory, appID)
			if err != nil {
				return err
			}
			name := strings.ToLower(args[0])
			local, localOK := manifest.Capabilities[name]
			remote, remoteErr := runtime.client().GetCreatorAppCapability(command.Context(), manifest.AppID, manifest.Environment, strings.ToUpper(name))
			if remoteErr != nil {
				return remoteErr
			}
			healthy := localOK && local == capabilityBinding(remote) && remote.Status != "SUSPENDED"
			report := map[string]any{
				"healthy":     healthy,
				"app_id":      manifest.AppID,
				"environment": manifest.Environment,
				"capability":  strings.ToUpper(name),
				"local":       local,
			}
			report["remote"] = remote
			if !healthy {
				return output.Policy("capability_doctor_unhealthy", "capability binding is not healthy").WithDetails(report)
			}
			return runtime.business(report)
		},
	}
	addBindingFlags(command, &directory, &appID)
	return command
}

func addBindingFlags(command *cobra.Command, directory, appID *string) {
	command.Flags().StringVar(directory, "dir", ".", "project directory containing .viceme/app.json")
	command.Flags().StringVar(appID, "app", "", "assert the expected linked Creator App ID")
}

func loadAppBinding(directory, expectedAppID string) (string, appmanifest.Manifest, error) {
	projectDirectory, err := resolveProjectDirectory(directory)
	if err != nil {
		return "", appmanifest.Manifest{}, err
	}
	manifest, err := appmanifest.Load(projectDirectory)
	if errors.Is(err, appmanifest.ErrNotFound) {
		return "", appmanifest.Manifest{}, output.Validation("app_not_linked", "project is not linked; run 'viceme app link' first")
	}
	if err != nil {
		return "", appmanifest.Manifest{}, output.Validation("app_manifest_invalid", err.Error())
	}
	if expectedAppID != "" && manifest.AppID != expectedAppID {
		return "", appmanifest.Manifest{}, output.Validation("app_binding_conflict", "--app does not match the project manifest")
	}
	return projectDirectory, manifest, nil
}
