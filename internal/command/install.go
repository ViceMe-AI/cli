package command

import (
	"context"
	"errors"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
	"github.com/spf13/cobra"
)

var officialSkillNames = []string{"viceme-shared", "viceme-publish"}

type installNextStep struct {
	Required bool   `json:"required"`
	Command  string `json:"command"`
	Reason   string `json:"reason"`
}

type bootstrapInstallResult struct {
	Launcher      updatepkg.TargetResult       `json:"launcher"`
	Skills        []skillcontent.InstallReport `json:"skills"`
	Config        config.EnsureResult          `json:"config"`
	Profile       string                       `json:"profile"`
	Region        config.Region                `json:"region"`
	Authenticated bool                         `json:"authenticated"`
	NextStep      installNextStep              `json:"nextStep"`
}

func newInstallCommand(runtime *Runtime) *cobra.Command {
	var agent string
	var region string
	var skipLauncherEnsure bool
	var activationChild bool
	command := &cobra.Command{
		Use: "install", Short: "Install official ViceMe Skills for supported AI coding agents", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := performInstall(command.Context(), runtime, agent, region, !skipLauncherEnsure, nil)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&agent, "agent", "auto", "agent target: auto, codex, claude, workbuddy, or agents")
	command.Flags().StringVar(&region, "region", "", "ViceMe region: cn or global")
	command.Flags().BoolVar(&skipLauncherEnsure, "internal-skip-launcher-ensure", false, "skip launcher persistence inside a coordinated activation")
	command.Flags().BoolVar(&activationChild, "internal-activation-child", false, "run inside the outer activation coordinator")
	_ = command.Flags().MarkHidden("internal-skip-launcher-ensure")
	_ = command.Flags().MarkHidden("internal-activation-child")
	return command
}

func performInstall(ctx context.Context, runtime *Runtime, agent, region string, ensureLauncher bool, beforeCommit func() error) (bootstrapInstallResult, error) {
	if region == "" {
		region = string(runtime.region)
	}
	resolvedRegion, err := config.ParseRegion(region)
	if err != nil {
		return bootstrapInstallResult{}, output.Validation("REGION_INVALID", err.Error())
	}
	installContext, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	for _, name := range officialSkillNames {
		if err := runtime.deps.Skills.Validate(name); err != nil {
			return bootstrapInstallResult{}, err
		}
	}
	transaction, reports, err := runtime.deps.Skills.PrepareInstallSet(officialSkillNames, agent, runtime.deps.Environment)
	if err != nil {
		return bootstrapInstallResult{}, output.Internal("SKILL_INSTALL_PREPARE_FAILED", "official Skills could not be prepared as one transaction", err)
	}
	rollback := func(cause error) (bootstrapInstallResult, error) {
		if rollbackErr := transaction.Rollback(); rollbackErr != nil {
			return bootstrapInstallResult{}, output.Internal("INSTALL_ROLLBACK_FAILED", "ViceMe installation failed and the previous generation could not be fully restored", errors.Join(cause, rollbackErr))
		}
		return bootstrapInstallResult{}, cause
	}
	if len(reports) != len(officialSkillNames) {
		return rollback(output.Internal("SKILL_INSTALL_TRANSACTION_INVALID", "official Skill installation returned an incomplete report", nil))
	}
	for _, report := range reports {
		if !report.AllSucceeded {
			return rollback(output.Internal("SKILL_INSTALL_PARTIAL", "official Skills were not activated", nil).WithDetails(reports))
		}
	}
	doctorResults := make([]doctorSkillResult, 0, len(officialSkillNames))
	for _, name := range officialSkillNames {
		report := runtime.deps.Skills.Doctor(name, agent, runtime.deps.Environment)
		doctorResults = append(doctorResults, doctorSkillResult{Name: name, Report: report})
		if !report.Healthy {
			return rollback(output.Validation("SKILL_INSTALL_VERIFICATION_FAILED", "installed official Skills did not pass Doctor").WithDetails(doctorResults))
		}
	}
	profile, err := runtime.config.Resolve(runtime.profile.Name)
	if err != nil {
		return rollback(output.Internal("PROFILE_INVALID", "could not resolve the active profile", err))
	}
	profile.Region = resolvedRegion
	if err := transaction.TrackPath(config.ConfigPath(runtime.configBase)); err != nil {
		return rollback(output.Internal("PROFILE_BACKUP_FAILED", "could not preserve the previous CLI configuration", err))
	}
	configResult, err := config.Save(runtime.configBase, runtime.config)
	if err != nil {
		return rollback(output.Internal("PROFILE_SAVE_FAILED", "could not initialize CLI configuration", err))
	}
	if err := runtime.reloadConfig(profile.Name); err != nil {
		return rollback(err)
	}
	network := checkDoctorNetwork(ctx, runtime)
	if !network.Healthy {
		return rollback(output.Network("DOCTOR_API_UNREACHABLE", "ViceMe API did not pass the installation readiness check", nil).WithDetails(network))
	}
	status, err := runtime.manager().CurrentStatus()
	if err != nil {
		return rollback(err)
	}
	if err := transaction.MarkCommitting(); err != nil {
		return rollback(output.Internal("INSTALL_COMMIT_PREPARE_FAILED", "could not persist the verified installation commit point", err))
	}
	launcher := updatepkg.TargetResult{Target: "launcher", Status: "coordinated"}
	if ensureLauncher {
		launcher, err = runtime.deps.Updater.EnsureLauncher(installContext)
		if err != nil {
			return rollback(updaterError(err, launcher))
		}
	}
	if beforeCommit != nil {
		if err := beforeCommit(); err != nil {
			transaction.Abandon()
			return bootstrapInstallResult{}, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return bootstrapInstallResult{}, output.Internal("INSTALL_COMMIT_FAILED", "ViceMe installation was verified but could not commit its recovery journal", err)
	}
	result := bootstrapInstallResult{Launcher: launcher, Skills: reports, Config: configResult, Profile: profile.Name, Region: resolvedRegion, Authenticated: status.Authenticated}
	if status.Authenticated {
		result.NextStep = installNextStep{Command: "viceme skill inspect --path <dir-or-zip>", Reason: "CLI and official Skills are ready"}
	} else {
		result.NextStep = installNextStep{Required: true, Command: "viceme auth login", Reason: "sign in before publishing a Skill"}
	}
	return result, nil
}

type doctorSkillResult struct {
	Name   string                    `json:"name"`
	Report skillcontent.DoctorReport `json:"report"`
}

type doctorNetworkResult struct {
	Healthy bool   `json:"healthy"`
	Code    string `json:"code,omitempty"`
	Problem string `json:"problem,omitempty"`
}

func checkDoctorNetwork(ctx context.Context, runtime *Runtime) doctorNetworkResult {
	probeContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := runtime.client().HealthReady(probeContext); err != nil {
		cliError := output.AsError(err)
		return doctorNetworkResult{Healthy: false, Code: cliError.Subtype, Problem: cliError.Message}
	}
	return doctorNetworkResult{Healthy: true}
}

func newDoctorCommand(runtime *Runtime) *cobra.Command {
	var agent string
	command := &cobra.Command{
		Use: "doctor", Short: "Check the CLI, profile, credentials, and official Skills", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			results := make([]doctorSkillResult, 0, len(officialSkillNames))
			healthy := true
			for _, name := range officialSkillNames {
				report := runtime.deps.Skills.Doctor(name, agent, runtime.deps.Environment)
				results = append(results, doctorSkillResult{Name: name, Report: report})
				healthy = healthy && report.Healthy
			}
			status, err := runtime.manager().CurrentStatus()
			if err != nil {
				return err
			}
			network := checkDoctorNetwork(command.Context(), runtime)
			healthy = healthy && network.Healthy
			result := map[string]any{
				"healthy": healthy, "profile": runtime.profile.Name, "region": runtime.region,
				"authenticated": status.Authenticated, "network": network, "skills": results,
			}
			if !healthy {
				return output.Validation("DOCTOR_UNHEALTHY", "ViceMe CLI or official Skill installation is unhealthy").WithDetails(result)
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&agent, "agent", "auto", "agent target: auto, codex, claude, workbuddy, or agents")
	return command
}
