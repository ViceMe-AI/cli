package command

import (
	"context"
	"path/filepath"
	"time"

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
	Authenticated   bool                       `json:"authenticated"`
	AuthStatusKnown bool                       `json:"auth_status_known"`
	Warnings        []string                   `json:"warnings,omitempty"`
	NextStep        installNextStep            `json:"next_step"`
}

func newInstallCommand(runtime *Runtime) *cobra.Command {
	var target string
	command := &cobra.Command{
		Use:   "install",
		Short: "Persist the npm CLI, install its Viceme Skill, and initialize configuration",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
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
			configBase := runtime.deps.Environment.ConfigDir
			if configBase == "" {
				configBase = filepath.Join(runtime.deps.Environment.Home, ".config")
			}
			configResult, err := config.Ensure(configBase, config.Config{
				DefaultProfile: runtime.opts.Profile,
				APIBaseURL:     defaultAPIBaseURL,
				UpdateChannel:  "stable",
			})
			if err != nil {
				return output.Internal("bootstrap_config", "could not initialize non-sensitive CLI configuration", err).WithDetails(map[string]any{
					"skill":  report,
					"config": configResult,
				})
			}
			result := bootstrapInstallResult{
				CLI:    launcher,
				Skill:  report,
				Config: configResult,
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
					Command: "viceme skill inspect <source> --json",
					Reason:  "CLI, Skill, and authentication are ready",
				}
			} else {
				result.NextStep = installNextStep{
					Required: true,
					Command:  "viceme auth login --no-wait --json",
					Reason:   "complete device login before publishing a Skill Agent",
				}
			}
			return runtime.success(result)
		},
	}
	command.Flags().StringVar(&target, "target", "auto", "Skill target: auto, codex, or claude")
	return command
}
