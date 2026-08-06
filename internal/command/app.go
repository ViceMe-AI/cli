package command

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/appmanifest"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

type appDoctorCheck struct {
	Healthy bool   `json:"healthy"`
	Detail  string `json:"detail"`
}

type appDoctorReport struct {
	Healthy      bool                      `json:"healthy"`
	ManifestPath string                    `json:"manifest_path"`
	AppID        string                    `json:"app_id,omitempty"`
	Checks       map[string]appDoctorCheck `json:"checks"`
}

func newAppCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "app", Short: "Bind and diagnose ViceMe Creator Apps"}
	command.AddCommand(newAppLinkCommand(runtime))
	command.AddCommand(newAppGetCommand(runtime))
	command.AddCommand(newAppListCommand(runtime))
	command.AddCommand(newAppDoctorCommand(runtime))
	return command
}

func newAppLinkCommand(runtime *Runtime) *cobra.Command {
	var directory string
	var appID string
	var name string
	var environment string
	var origin string
	command := &cobra.Command{
		Use:   "link",
		Short: "Bind the current project to a TEST or LIVE Creator App",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			projectDirectory, err := resolveProjectDirectory(directory)
			if err != nil {
				return err
			}
			environment, err = normalizeEnvironment(environment)
			if err != nil {
				return err
			}
			if origin != "" {
				origin, err = canonicalAppOrigin(origin)
				if err != nil {
					return err
				}
			}

			linkErr := appmanifest.WithLinkLock(
				command.Context(),
				runtime.processLockRoot,
				projectDirectory,
				func() error {
					return executeAppLink(command, runtime, projectDirectory, appID, name, environment, origin)
				},
			)
			if errors.Is(linkErr, appmanifest.ErrLinkLock) {
				return output.Internal("app_link_lock", "the project App binding lock is unavailable", linkErr).
					WithHint("wait for the other 'viceme app link' command to finish, then retry")
			}
			return linkErr
		},
	}
	command.Flags().StringVar(&directory, "dir", ".", "project directory")
	command.Flags().StringVar(&appID, "app", "", "bind an existing Creator App instead of creating one")
	command.Flags().StringVar(&name, "name", "", "name for a newly created Creator App (defaults to directory name)")
	command.Flags().StringVar(&environment, "environment", "TEST", "Creator App environment: TEST or LIVE")
	command.Flags().StringVar(&origin, "origin", "", "canonical browser origin to register, for example http://localhost:3000")
	return command
}

func executeAppLink(command *cobra.Command, runtime *Runtime, projectDirectory, appID, name, environment, origin string) error {
	existing, loadErr := appmanifest.Load(projectDirectory)
	if loadErr != nil && !errors.Is(loadErr, appmanifest.ErrNotFound) {
		return output.Validation("app_manifest_invalid", loadErr.Error())
	}
	if appID == "" && loadErr == nil {
		appID = existing.AppID
	}
	if appID != "" && loadErr == nil && existing.AppID != appID {
		return output.Validation("app_binding_conflict", "the project is already bound to a different Creator App")
	}
	if appID != "" && loadErr == nil {
		if !command.Flags().Changed("environment") {
			environment = existing.Environment
		}
		if !command.Flags().Changed("origin") {
			origin = existing.Origin
		}
	}

	client := runtime.client()
	var app api.CreatorApp
	var err error
	if appID == "" {
		if name == "" {
			name = filepath.Base(projectDirectory)
		}
		intent, intentErr := appmanifest.LoadOrCreateLinkIntent(projectDirectory, name, "EXTERNAL", runtime.deps.NewID)
		if intentErr != nil {
			return output.Validation("app_link_intent", intentErr.Error())
		}
		app, err = client.CreateCreatorApp(command.Context(), api.CreateCreatorAppRequest{
			ClientRequestID: intent.ClientRequestID,
			Name:            name,
			HostingMode:     "EXTERNAL",
		})
	} else {
		app, err = client.GetCreatorApp(command.Context(), appID)
	}
	if err != nil {
		return err
	}
	target, err := selectAppEnvironment(app, environment)
	if err != nil {
		return err
	}
	if origin != "" {
		if _, err := client.AddCreatorAppOrigin(command.Context(), app.ID, target.Type, origin); err != nil {
			return err
		}
	}
	manifest := manifestFromApp(app, target, origin)
	manifestPath, err := appmanifest.Save(projectDirectory, manifest)
	if err != nil {
		return output.Internal("app_manifest_save", "Creator App was resolved but the local manifest could not be saved", err).
			WithHint("fix local project permissions and rerun the same 'viceme app link' command; creation is idempotent").
			WithDetails(map[string]any{"app_id": app.ID})
	}
	if err := appmanifest.RemoveLinkIntent(projectDirectory); err != nil {
		return output.Internal("app_link_intent_cleanup", "the App manifest was saved, but its pending link intent could not be removed", err).
			WithDetails(map[string]any{"app_id": app.ID, "manifest": manifestPath})
	}
	return runtime.business(map[string]any{
		"app":          app,
		"binding":      manifest,
		"manifest":     manifestPath,
		"api_base_url": strings.TrimRight(runtime.apiBaseURL, "/") + "/v1",
	})
}

func newAppGetCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "get <app-id>",
		Short: "Get one owned Creator App",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			app, err := runtime.client().GetCreatorApp(command.Context(), args[0])
			if err != nil {
				return err
			}
			return runtime.business(app)
		},
	}
}

func newAppListCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List owned Creator Apps",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			apps, err := runtime.client().ListCreatorApps(command.Context())
			if err != nil {
				return err
			}
			return runtime.business(apps)
		},
	}
}

func newAppDoctorCommand(runtime *Runtime) *cobra.Command {
	var directory string
	command := &cobra.Command{
		Use:   "doctor",
		Short: "Check the local App binding against the ViceMe control plane",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			report, err := diagnoseApp(command, runtime, directory)
			if err != nil {
				return err
			}
			if !report.Healthy {
				return output.Policy("app_doctor_unhealthy", "Creator App binding is not healthy").WithDetails(report)
			}
			return runtime.business(report)
		},
	}
	command.Flags().StringVar(&directory, "dir", ".", "project directory")
	return command
}

func diagnoseApp(command *cobra.Command, runtime *Runtime, directory string) (appDoctorReport, error) {
	report := appDoctorReport{Checks: map[string]appDoctorCheck{}}
	projectDirectory, err := resolveProjectDirectory(directory)
	if err != nil {
		report.Checks["project"] = appDoctorCheck{Detail: err.Error()}
		return report, nil
	}
	report.ManifestPath = appmanifest.Path(projectDirectory)
	manifest, err := appmanifest.Load(projectDirectory)
	if err != nil {
		report.Checks["manifest"] = appDoctorCheck{Detail: err.Error()}
		return report, nil
	}
	report.AppID = manifest.AppID
	report.Checks["manifest"] = appDoctorCheck{Healthy: true, Detail: "schema and safe fields are valid"}

	app, err := runtime.client().GetCreatorApp(command.Context(), manifest.AppID)
	if err != nil {
		return report, err
	}
	report.Checks["ownership"] = appDoctorCheck{Healthy: true, Detail: "App belongs to the authenticated user"}
	target, err := selectAppEnvironment(app, manifest.Environment)
	if err != nil {
		report.Checks["environment"] = appDoctorCheck{Detail: err.Error()}
		return report, nil
	}
	keyHealthy := target.PublishableKey == manifest.PublishableKey
	report.Checks["publishable_key"] = appDoctorCheck{Healthy: keyHealthy, Detail: ternary(keyHealthy, "local key matches the control plane", "local key differs from the control plane")}
	originHealthy := manifest.Origin != "" && contains(target.AllowedOrigins, manifest.Origin)
	report.Checks["origin"] = appDoctorCheck{Healthy: originHealthy, Detail: ternary(originHealthy, "origin is registered", "manifest origin is missing or not registered")}
	capabilitiesHealthy := sameCapabilityBindings(manifest.Capabilities, target.Capabilities)
	report.Checks["capabilities"] = appDoctorCheck{
		Healthy: capabilitiesHealthy,
		Detail:  ternary(capabilitiesHealthy, "local capability protocol versions match the control plane", "local capability protocol versions differ from the control plane"),
	}
	if originHealthy && keyHealthy {
		context, contextErr := runtime.client().GetPublicAppContext(command.Context(), manifest.PublishableKey, manifest.Origin)
		if contextErr != nil {
			return report, contextErr
		}
		contextHealthy := contextErr == nil && context.Environment == manifest.Environment && context.App.Name == app.Name
		detail := "public Widget context resolves to the bound App"
		if !contextHealthy {
			detail = "public Widget context does not match the local binding"
		}
		report.Checks["widget_context"] = appDoctorCheck{Healthy: contextHealthy, Detail: detail}
	} else {
		report.Checks["widget_context"] = appDoctorCheck{Detail: "publishable key and origin must be healthy first"}
	}
	report.Healthy = allAppChecksHealthy(report.Checks)
	return report, nil
}

func resolveProjectDirectory(directory string) (string, error) {
	abs, err := filepath.Abs(directory)
	if err != nil {
		return "", output.Validation("project_directory", "could not resolve the project directory")
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", output.Validation("project_directory", "project directory does not exist or is not a directory")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", output.Validation("project_directory", "could not resolve project directory symlinks")
	}
	return resolved, nil
}

func normalizeEnvironment(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value != "TEST" && value != "LIVE" {
		return "", output.Validation("app_environment", "environment must be TEST or LIVE")
	}
	return value, nil
}

func canonicalAppOrigin(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
		return "", output.Validation("app_origin", "origin must be a canonical http(s) origin without a path")
	}
	origin, err := api.NormalizeAPIOrigin(value)
	if err != nil {
		return "", output.Validation("app_origin", "origin must use HTTPS; HTTP is allowed only for loopback development")
	}
	if origin != value {
		return "", output.Validation("app_origin", fmt.Sprintf("origin is not canonical; use %s", origin))
	}
	return origin, nil
}

func selectAppEnvironment(app api.CreatorApp, environment string) (api.CreatorAppEnvironment, error) {
	for _, candidate := range app.Environments {
		if candidate.Type == environment {
			return candidate, nil
		}
	}
	return api.CreatorAppEnvironment{}, output.Validation("app_environment_missing", "Creator App does not contain the requested environment")
}

func manifestFromApp(app api.CreatorApp, environment api.CreatorAppEnvironment, origin string) appmanifest.Manifest {
	capabilities := make(map[string]appmanifest.Capability, len(environment.Capabilities))
	for _, capability := range environment.Capabilities {
		capabilities[strings.ToLower(capability.Type)] = capabilityBinding(capability)
	}
	return appmanifest.Manifest{
		SchemaVersion:  appmanifest.SchemaVersion,
		AppID:          app.ID,
		HostingMode:    app.HostingMode,
		Environment:    environment.Type,
		PublishableKey: environment.PublishableKey,
		Origin:         origin,
		Capabilities:   capabilities,
	}
}

func capabilityBinding(capability api.CreatorAppCapability) appmanifest.Capability {
	return appmanifest.Capability{
		ContractVersion: capability.ContractVersion,
		SDKPackage:      capability.SDKPackage,
		SDKVersion:      capability.SDKVersion,
	}
}

func sameCapabilityBindings(local map[string]appmanifest.Capability, remote []api.CreatorAppCapability) bool {
	if local == nil || len(local) != len(remote) {
		return false
	}
	for _, capability := range remote {
		binding, exists := local[strings.ToLower(capability.Type)]
		if !exists || binding != capabilityBinding(capability) {
			return false
		}
	}
	return true
}

func allAppChecksHealthy(checks map[string]appDoctorCheck) bool {
	if len(checks) == 0 {
		return false
	}
	for _, check := range checks {
		if !check.Healthy {
			return false
		}
	}
	return true
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func ternary(condition bool, yes, no string) string {
	if condition {
		return yes
	}
	return no
}
