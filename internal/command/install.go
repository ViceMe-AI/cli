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
	command := &cobra.Command{
		Use: "install", Short: "Install official ViceMe Skills for supported AI coding agents", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if region == "" {
				region = string(runtime.region)
			}
			resolvedRegion, err := config.ParseRegion(region)
			if err != nil {
				return output.Validation("REGION_INVALID", err.Error())
			}
			ctx, cancel := context.WithTimeout(command.Context(), 3*time.Minute)
			defer cancel()
			launcher, err := runtime.deps.Updater.EnsureLauncher(ctx)
			if err != nil {
				return updaterError(err, launcher)
			}
			for _, name := range officialSkillNames {
				if err := runtime.deps.Skills.Validate(name); err != nil {
					return err
				}
			}
			reports := runtime.deps.Skills.InstallSet(officialSkillNames, agent, runtime.deps.Environment)
			if len(reports) != len(officialSkillNames) {
				return output.Internal("SKILL_INSTALL_TRANSACTION_INVALID", "official Skill installation returned an incomplete report", nil)
			}
			for _, report := range reports {
				if !report.AllSucceeded {
					return output.Internal("SKILL_INSTALL_PARTIAL", "official Skills were not activated", nil).WithDetails(reports)
				}
			}
			doctorResults := make([]doctorSkillResult, 0, len(officialSkillNames))
			for _, name := range officialSkillNames {
				report := runtime.deps.Skills.Doctor(name, agent, runtime.deps.Environment)
				doctorResults = append(doctorResults, doctorSkillResult{Name: name, Report: report})
				if !report.Healthy {
					return output.Validation("SKILL_INSTALL_VERIFICATION_FAILED", "installed official Skills did not pass Doctor").WithDetails(doctorResults)
				}
			}
			profile, err := runtime.config.Resolve(runtime.profile.Name)
			if err != nil {
				return output.Internal("PROFILE_INVALID", "could not resolve the active profile", err)
			}
			if profile.Region != resolvedRegion {
				previous := credentialauth.Manager{Store: runtime.deps.Store, Region: string(profile.Region), ProfileID: profile.ID, ProfileName: profile.Name}
				if err := previous.Delete(); err != nil {
					return err
				}
				profile.Region = resolvedRegion
			}
			configResult, err := config.Save(runtime.configBase, runtime.config)
			if err != nil {
				return output.Internal("PROFILE_SAVE_FAILED", "could not initialize CLI configuration", err)
			}
			if err := runtime.reloadConfig(profile.Name); err != nil {
				return err
			}
			status, err := runtime.manager().CurrentStatus()
			if err != nil {
				return err
			}
			result := bootstrapInstallResult{Launcher: launcher, Skills: reports, Config: configResult, Profile: profile.Name, Region: resolvedRegion, Authenticated: status.Authenticated}
			if status.Authenticated {
				result.NextStep = installNextStep{Command: "viceme skill inspect --path <dir-or-zip>", Reason: "CLI and official Skills are ready"}
			} else {
				result.NextStep = installNextStep{Required: true, Command: "viceme auth login", Reason: "sign in before publishing a Skill"}
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&agent, "agent", "auto", "agent target: auto, codex, claude, workbuddy, or agents")
	command.Flags().StringVar(&region, "region", "", "ViceMe region: cn or global")
	return command
}

type doctorSkillResult struct {
	Name   string                    `json:"name"`
	Report skillcontent.DoctorReport `json:"report"`
}

func newDoctorCommand(runtime *Runtime) *cobra.Command {
	var agent string
	command := &cobra.Command{
		Use: "doctor", Short: "Check the CLI, profile, credentials, and official Skills", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
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
			result := map[string]any{
				"healthy": healthy, "profile": runtime.profile.Name, "region": runtime.region,
				"authenticated": status.Authenticated, "skills": results,
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
