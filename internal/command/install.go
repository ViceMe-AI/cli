package command

import (
	"context"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
	"github.com/spf13/cobra"
)

type installNextStep struct {
	Required bool   `json:"required"`
	Command  string `json:"command"`
	Reason   string `json:"reason"`
}

type bootstrapInstallResult struct {
	CLI             updatepkg.TargetResult     `json:"cli"`
	Skill           skillcontent.InstallReport `json:"skill"`
	Config          config.EnsureResult        `json:"config"`
	Profile         string                     `json:"profile"`
	Region          config.Region              `json:"region"`
	Authenticated   bool                       `json:"authenticated"`
	AuthStatusKnown bool                       `json:"auth_status_known"`
	Warnings        []string                   `json:"warnings,omitempty"`
	NextStep        installNextStep            `json:"next_step"`
}

func newInstallCommand(runtime *Runtime) *cobra.Command {
	var target string
	var region string
	command := &cobra.Command{
		Use:   "install",
		Short: "Persist the npm CLI, install its Viceme Skill, and initialize configuration",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if region == "" {
				region = string(runtime.region)
			}
			resolvedRegion, err := config.ParseRegion(region)
			if err != nil {
				return output.Validation("region", err.Error())
			}
			if err := runtime.deps.Skills.Validate("viceme"); err != nil {
				return err
			}
			installContext, cancel := context.WithTimeout(command.Context(), 3*time.Minute)
			defer cancel()
			launcher, err := runtime.deps.Updater.EnsureLauncher(installContext)
			if err != nil {
				return output.Internal("bootstrap_cli_install", "could not install the persistent npm launcher", err).WithDetails(launcher)
			}
			report := runtime.deps.Skills.Install("viceme", target, runtime.deps.Environment)
			if !report.AllSucceeded {
				return output.Internal("bootstrap_install_partial", "one or more Skill targets could not be installed", nil).WithDetails(report)
			}
			activeProfile, err := runtime.config.Resolve(runtime.profile.Name)
			if err != nil {
				return output.Internal("bootstrap_config", "could not resolve the active CLI profile", err)
			}
			previousRegion := activeProfile.Region
			activeProfile.Region = resolvedRegion
			configResult, err := config.Save(runtime.configBase, runtime.config)
			if err != nil {
				return output.Internal("bootstrap_config", "could not initialize non-sensitive CLI configuration", err).WithDetails(map[string]any{
					"skill":  report,
					"config": configResult,
				})
			}
			var warnings []string
			if previousRegion != resolvedRegion {
				previousScope, scopeErr := runtime.credentialScopeForRegion(previousRegion)
				if scopeErr != nil {
					return output.Validation("api_base_url", "Viceme API base URL must use HTTPS; HTTP is allowed only for localhost or loopback development")
				}
				previousManager := credentialauth.Manager{
					Store:       runtime.deps.Store,
					Region:      string(previousRegion),
					ProfileID:   activeProfile.ID,
					ProfileName: activeProfile.Name,
					Scope:       previousScope,
				}
				if err := previousManager.Delete(); err != nil {
					warnings = append(warnings, "profile region changed but the previous local credential could not be removed from the operating system keychain")
				}
			}
			if err := runtime.setRegion(resolvedRegion); err != nil {
				return err
			}
			result := bootstrapInstallResult{
				CLI:      launcher,
				Skill:    report,
				Config:   configResult,
				Profile:  runtime.profile.Name,
				Region:   resolvedRegion,
				Warnings: warnings,
			}
			status, statusErr := runtime.manager().CurrentStatus()
			if statusErr == nil {
				result.AuthStatusKnown = true
				result.Authenticated = status.Authenticated
			} else {
				result.Warnings = append(result.Warnings, "authentication status could not be read from the operating system keychain")
			}
			if result.Authenticated {
				result.NextStep = installNextStep{
					Command: "viceme skill inspect <source>",
					Reason:  "CLI, Skill, and authentication are ready",
				}
			} else {
				result.NextStep = installNextStep{
					Required: true,
					Command:  "viceme auth login",
					Reason:   "complete device login before publishing a Skill Agent",
				}
			}
			return runtime.success(result)
		},
	}
	command.Flags().StringVar(&target, "target", "auto", "Skill target: auto, codex, or claude")
	command.Flags().StringVar(&region, "region", "", "Viceme region: cn or global (defaults to the selected profile region)")
	return command
}
