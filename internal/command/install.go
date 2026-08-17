package command

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
)

var officialSkillNames = []string{
	"viceme-shared",
	"viceme-publish",
	"viceme-danmaku",
}

type installNextStep struct {
	Required bool   `json:"required"`
	Command  string `json:"command"`
	Reason   string `json:"reason"`
}

type bootstrapInstallResult struct {
	Launcher        updatepkg.TargetResult       `json:"launcher"`
	Skills          []skillcontent.InstallReport `json:"skills"`
	Config          config.EnsureResult          `json:"config"`
	Profile         string                       `json:"profile"`
	Region          config.Region                `json:"region"`
	Authenticated   bool                         `json:"authenticated"`
	AuthStatusKnown bool                         `json:"authStatusKnown"`
	Warnings        []string                     `json:"warnings,omitempty"`
	NextStep        installNextStep              `json:"nextStep"`
}

type installCommitAuthority struct {
	PrepareLauncher         func(context.Context) (updatepkg.TargetResult, error)
	BeforeCommit            func() error
	AfterCommit             func() error
	OuterJournalOwnsFailure bool
}

func newInstallCommand(runtime *Runtime) *cobra.Command {
	var agent string
	var region string
	var skipLauncherEnsure bool
	var activationChildNonce string
	var activationTarget string
	command := &cobra.Command{
		Use: "install", Short: "Install official ViceMe Skills for supported AI coding agents", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			internalFlagsPresent := skipLauncherEnsure || activationChildNonce != "" || activationTarget != ""
			if internalFlagsPresent {
				request := runtime.deps.activationChildRequest
				if !runtime.deps.coordinatedActivationChild || !skipLauncherEnsure ||
					activationChildNonce != request.Nonce || activationTarget != request.TargetVersion || agent != request.SkillTarget {
					return output.Policy("ACTIVATION_CHILD_INVALID", "internal activation flags require the exact committing parent journal")
				}
			}
			result, err := performAuthorizedInstall(command.Context(), runtime, agent, region)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&agent, "agent", "auto", "agent target: auto, codex, claude, workbuddy, or agents")
	command.Flags().StringVar(&region, "region", "", "ViceMe region: cn or global")
	command.Flags().BoolVar(&skipLauncherEnsure, "internal-skip-launcher-ensure", false, "skip launcher persistence inside a coordinated activation")
	command.Flags().StringVar(&activationChildNonce, "internal-activation-child", "", "run inside the outer activation coordinator")
	command.Flags().StringVar(&activationTarget, "internal-activation-target", "", "bind the activation child to an exact target")
	_ = command.Flags().MarkHidden("internal-skip-launcher-ensure")
	_ = command.Flags().MarkHidden("internal-activation-child")
	_ = command.Flags().MarkHidden("internal-activation-target")
	return command
}

func performAuthorizedInstall(ctx context.Context, runtime *Runtime, agent, region string) (bootstrapInstallResult, error) {
	if runtime.deps.coordinatedActivationChild {
		return performNPMChildInstall(ctx, runtime, agent, region)
	}
	return performOrdinaryInstall(ctx, runtime, agent, region)
}

func performOrdinaryInstall(ctx context.Context, runtime *Runtime, agent, region string) (bootstrapInstallResult, error) {
	if err := os.MkdirAll(runtime.configBase, 0o700); err != nil {
		return bootstrapInstallResult{}, output.Internal("INSTALL_AUTHORITY_FAILED", "could not create the activation directory", err)
	}
	activationLock := flock.New(filepath.Join(runtime.configBase, updatepkg.ActivationLockFilename))
	locked, err := activationLock.TryLock()
	if err != nil {
		return bootstrapInstallResult{}, output.Internal("INSTALL_AUTHORITY_FAILED", "could not acquire the activation lock", err)
	}
	if !locked {
		return bootstrapInstallResult{}, output.Validation("INSTALL_ACTIVE", "another ViceMe bootstrap or update is active")
	}
	defer activationLock.Unlock()
	memberLock := flock.New(filepath.Join(runtime.configBase, updatepkg.ActivationMemberLockFilename))
	memberLocked, err := memberLock.TryLock()
	if err != nil {
		return bootstrapInstallResult{}, output.Internal("INSTALL_AUTHORITY_FAILED", "could not inspect the activation member lock", err)
	}
	if !memberLocked {
		return bootstrapInstallResult{}, output.Validation("INSTALL_ACTIVE", "an activation child is still committing Skills and config")
	}
	defer memberLock.Unlock()

	expected, err := expectedRunningGeneration(runtime)
	if err != nil {
		return bootstrapInstallResult{}, output.Internal("INSTALL_AUTHORITY_FAILED", "could not identify the running CLI generation", err)
	}
	activeInitially, activeExists, err := validateInstallAuthority(runtime.configBase, expected, nil)
	if err != nil {
		return bootstrapInstallResult{}, output.Policy("INSTALL_GENERATION_CHANGED", "the active CLI generation changed; restart the command")
	}
	authority := &installCommitAuthority{BeforeCommit: func() error {
		active, exists, err := validateInstallAuthority(runtime.configBase, expected, &activeInitially)
		if err != nil || exists != activeExists || (exists && active != activeInitially) {
			return updatepkg.ErrActivationRestartNeeded
		}
		if !exists {
			return updatepkg.CommitActiveGeneration(runtime.configBase, expected)
		}
		return nil
	}}
	if service, ok := runtime.deps.Updater.(*updatepkg.NPMService); ok && service.InstallMethod == "npm" {
		var childNonce string
		authority.OuterJournalOwnsFailure = true
		authority.PrepareLauncher = func(ctx context.Context) (updatepkg.TargetResult, error) {
			launcher, nonce, err := service.PrepareCoordinatedInstallWhileLocked(ctx, agent)
			if err == nil {
				childNonce = nonce
			}
			return launcher, err
		}
		authority.BeforeCommit = func() error {
			outer, err := updatepkg.InspectOuterActivationJournals(runtime.configBase)
			if err != nil || outer.Bootstrap || !outer.NPM {
				return updatepkg.ErrActivationRestartNeeded
			}
			active, exists, err := updatepkg.ReadActiveGeneration(runtime.configBase)
			if err != nil || exists != activeExists || (exists && active != activeInitially) {
				return updatepkg.ErrActivationRestartNeeded
			}
			target, err := service.ValidateActivationChild(childNonce, expected.Version, agent)
			if err != nil || target != expected {
				return updatepkg.ErrActivationRestartNeeded
			}
			return nil
		}
		authority.AfterCommit = func() error {
			if err := service.ConfirmActivationChildCommitted(childNonce, expected.Version, agent); err != nil {
				return err
			}
			return service.RecoverActivationWhileLocked(ctx)
		}
	}
	return performInstall(ctx, runtime, agent, region, true, authority)
}

func performNPMChildInstall(ctx context.Context, runtime *Runtime, agent, region string) (bootstrapInstallResult, error) {
	memberLock := flock.New(filepath.Join(runtime.configBase, updatepkg.ActivationMemberLockFilename))
	memberLocked, err := memberLock.TryLock()
	if err != nil {
		return bootstrapInstallResult{}, output.Internal("ACTIVATION_CHILD_LOCK_FAILED", "could not acquire the activation child lock", err)
	}
	if !memberLocked {
		return bootstrapInstallResult{}, output.Validation("ACTIVATION_CHILD_ACTIVE", "another activation child is committing Skills and config")
	}
	defer memberLock.Unlock()
	request := runtime.deps.activationChildRequest
	service := npmRecoveryService(runtime.configBase, runtime.deps)
	validate := func() error {
		target, err := service.ValidateActivationChild(request.Nonce, request.TargetVersion, request.SkillTarget)
		if err != nil {
			return err
		}
		expected, err := expectedRunningGeneration(runtime)
		if err != nil {
			return err
		}
		if target != expected {
			return updatepkg.ErrActivationRestartNeeded
		}
		return nil
	}
	if err := validate(); err != nil {
		return bootstrapInstallResult{}, output.Policy("ACTIVATION_CHILD_INVALID", "the activation child no longer owns its parent journal")
	}
	authority := &installCommitAuthority{
		BeforeCommit: validate,
		AfterCommit: func() error {
			return service.ConfirmActivationChildCommitted(request.Nonce, request.TargetVersion, request.SkillTarget)
		},
	}
	return performInstall(ctx, runtime, agent, region, false, authority)
}

func expectedRunningGeneration(runtime *Runtime) (updatepkg.ActiveGeneration, error) {
	if runtime.deps.runningActivationGeneration != nil {
		return *runtime.deps.runningActivationGeneration, nil
	}
	return runningActivationGeneration(runtime.deps)
}

func validateInstallAuthority(configDir string, expected updatepkg.ActiveGeneration, initial *updatepkg.ActiveGeneration) (updatepkg.ActiveGeneration, bool, error) {
	outer, err := updatepkg.InspectOuterActivationJournals(configDir)
	if err != nil {
		return updatepkg.ActiveGeneration{}, false, err
	}
	if outer.Bootstrap || outer.NPM {
		return updatepkg.ActiveGeneration{}, false, errors.New("an outer activation journal is pending")
	}
	active, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil {
		return updatepkg.ActiveGeneration{}, false, err
	}
	if exists && active != expected {
		return active, true, updatepkg.ErrActivationRestartNeeded
	}
	if initial != nil && exists && active != *initial {
		return active, true, updatepkg.ErrActivationRestartNeeded
	}
	return active, exists, nil
}

func performInstall(ctx context.Context, runtime *Runtime, agent, region string, ensureLauncher bool, authority *installCommitAuthority) (bootstrapInstallResult, error) {
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
	if _, statErr := os.Stat(config.ConfigPath(runtime.configBase)); errors.Is(statErr, fs.ErrNotExist) {
		profile.APIBaseURL = config.APIBaseURL(resolvedRegion)
	} else if statErr != nil {
		return rollback(output.Internal("PROFILE_BACKUP_FAILED", "could not inspect the CLI configuration", statErr))
	}
	runtime.config.DistributionRegion = resolvedRegion
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
	authenticated, authStatusKnown, warnings := installAuthenticationStatus(runtime)
	if !network.Healthy {
		warnings = append(warnings, "the active profile API is unreachable; installation completed, configure the intended profile and run viceme doctor")
	}
	if err := transaction.MarkCommitting(); err != nil {
		return rollback(output.Internal("INSTALL_COMMIT_PREPARE_FAILED", "could not persist the verified installation commit point", err))
	}
	launcher := updatepkg.TargetResult{Target: "launcher", Status: "coordinated"}
	if ensureLauncher {
		if authority != nil && authority.PrepareLauncher != nil {
			launcher, err = authority.PrepareLauncher(installContext)
		} else {
			launcher, err = runtime.deps.Updater.EnsureLauncher(installContext)
		}
		if err != nil {
			return rollback(updaterError(err, launcher))
		}
	}
	if authority != nil && authority.BeforeCommit != nil {
		if err := authority.BeforeCommit(); err != nil {
			if authority.OuterJournalOwnsFailure {
				transaction.Abandon()
				return bootstrapInstallResult{}, err
			}
			return rollback(err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return bootstrapInstallResult{}, output.Internal("INSTALL_COMMIT_FAILED", "ViceMe installation was verified but could not commit its recovery journal", err)
	}
	if authority != nil && authority.AfterCommit != nil {
		if err := authority.AfterCommit(); err != nil {
			return bootstrapInstallResult{}, output.Internal("INSTALL_AUTHORITY_COMMIT_FAILED", "ViceMe installation committed but its activation authority could not be finalized", err)
		}
	}
	result := bootstrapInstallResult{
		Launcher:        launcher,
		Skills:          reports,
		Config:          configResult,
		Profile:         profile.Name,
		Region:          resolvedRegion,
		Authenticated:   authenticated,
		AuthStatusKnown: authStatusKnown,
		Warnings:        warnings,
	}
	if authenticated {
		result.NextStep = installNextStep{Command: "viceme skill publish --path <dir-or-zip>", Reason: "upload a private Draft and open its Owner Preview"}
	} else {
		result.NextStep = installNextStep{Required: true, Command: "viceme auth login", Reason: "sign in before publishing a Skill"}
	}
	return result, nil
}

func installAuthenticationStatus(runtime *Runtime) (authenticated, known bool, warnings []string) {
	if token, _, _ := runtime.overrideCredential(); token != "" {
		return true, true, nil
	}
	status, err := runtime.manager().CurrentStatus()
	if err != nil {
		return false, false, []string{"authentication status could not be read from the secure credential store"}
	}
	return status.Authenticated, true, nil
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
				"healthy": healthy, "profile": runtime.profile.Name, "distributionRegion": runtime.region,
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
