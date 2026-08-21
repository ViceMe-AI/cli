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
	"viceme-tip",
	"viceme-engagement",
}

var retiredOfficialSkills = []skillcontent.RetiredSkill{{
	Name: "viceme-access",
	Releases: []skillcontent.RetiredSkillRelease{
		{
			CLIVersions:       []string{"0.15.3-poc.1"},
			SkillVersion:      "0.15.0",
			MinimumCLIVersion: "0.15.0",
			CLICompatibility:  ">=0.15.0 <0.16.0",
			Digests: skillcontent.Digests{
				Full:     "sha256:f1e8b50be7e9cade06d178b48f1134ad627829eff64bd26df6153e23a72ff20d",
				Embedded: "sha256:795a211166273dc14c99e1a97df5aa82c7775ef8221af247a8fb3e36b0ab79a5",
			},
		},
		retiredAccessRelease(
			[]string{"0.16.0-beta.6"}, "0.16.0-beta.6",
			"sha256:a864116c3d17dffd7d430575bdaafd9e6c5908cea878d93fbd995af83bed5555",
			"sha256:70dc3615230f97362b9ee7ac419d5c1c2528fbaee1a4a44ea8f805c1b226e6ff",
		),
		retiredAccessRelease(
			[]string{"0.16.0-poc.1", "0.16.1-poc.1", "0.16.1-poc.2", "0.16.1-poc.3", "0.16.1-poc.4"}, "0.16.0",
			"sha256:9df092ffb8dcd9436ad35b82f2c6d79a1864f18f237cb4083a95563dfbb89aa1",
			"sha256:795a211166273dc14c99e1a97df5aa82c7775ef8221af247a8fb3e36b0ab79a5",
		),
		retiredAccessRelease(
			[]string{"0.16.0-poc.2", "0.16.0-poc.3", "0.16.0-poc.4", "0.16.0-poc.5", "0.16.0-poc.6"}, "0.16.0",
			"sha256:4be046e810d97716a1c89eb7c72e98e07712829a4a84b8dae06fa8284951aad5",
			"sha256:b0f3924886303f1f223e7153db056167da0eb8969ad8651b2063ca0114ed904e",
		),
		retiredAccessRelease(
			[]string{"0.16.0-poc.7"}, "0.16.0",
			"sha256:e1ed7c066cb7c3b620beb01e55791a71a134231d88a903eb46c85248f6ff854a",
			"sha256:dc3b70e927adfd496914389e645bf3c3a8a2fcca757780e4d5b04403f3e24521",
		),
		retiredAccessRelease(
			[]string{"0.16.0-poc.9"}, "0.16.0",
			"sha256:18bb0d8524f56cb7e9d95aa9deba5da2898ee877fff8ea0a5d0c8bb0c18938f8",
			"sha256:02daa9dbf25c0d45cd9af22dcd9cf6604911076d880a670a1f82d0159db08251",
		),
		retiredAccessRelease(
			[]string{"0.16.0-poc.10"}, "0.16.0",
			"sha256:12f92ff3193a65895482bc8baf22041fca0f900f2207062e82b7d7cd9aa3ee92",
			"sha256:e61977251318628e02ceacbf139ac66f5279d95f48bd9506f2db02d900043abc",
		),
		retiredAccessRelease(
			[]string{"0.16.0-poc.11", "0.16.0-poc.12", "0.16.1-poc.1", "0.16.1-poc.2", "0.16.1-poc.3", "0.16.1-poc.4"}, "0.16.0",
			"sha256:634eb95dc6d793c1dd7c2824ea275d84de504627267c86cfab45fb4aabe0cf47",
			"sha256:70dc3615230f97362b9ee7ac419d5c1c2528fbaee1a4a44ea8f805c1b226e6ff",
		),
		retiredAccessRelease(
			[]string{"0.16.1-poc.5"}, "0.16.0",
			"sha256:f62e61379ef7256a6155fbbbfe97a0225912836d90da6072ee80a78dad4b193a",
			"sha256:ad6e09ba31e5709058c1d8b21cd16e207885eee7886d5f2337c6dea4d7e70ffc",
		),
		retiredAccessRelease(
			[]string{"0.16.1-poc.6"}, "0.16.1",
			"sha256:9d45686b3b055dc05a02c0c50a64bad12b443826f489eddfd3aefd1ed4fab046",
			"sha256:52ded3d728a229ab49a0900ef4e002aa0e87ea94b5763b54bf683a23d1c13a2c",
		),
	},
}}

func retiredAccessRelease(cliVersions []string, skillVersion, fullDigest, embeddedDigest string) skillcontent.RetiredSkillRelease {
	return skillcontent.RetiredSkillRelease{
		CLIVersions:  cliVersions,
		SkillVersion: skillVersion, MinimumCLIVersion: skillVersion,
		CLICompatibility: ">=" + skillVersion + " <0.17.0",
		Digests:          skillcontent.Digests{Full: fullDigest, Embedded: embeddedDigest},
	}
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

type installSourceOverride struct {
	APIBaseURL     string
	WebBaseURL     string
	ReleaseChannel string
	ReleaseBaseURL string
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
	return performInstall(ctx, runtime, agent, region, true, authority, nil)
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
	return performInstall(ctx, runtime, agent, region, false, authority, nil)
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

func performInstall(ctx context.Context, runtime *Runtime, agent, region string, ensureLauncher bool, authority *installCommitAuthority, sourceOverride *installSourceOverride) (bootstrapInstallResult, error) {
	if region == "" {
		region = string(runtime.region)
	}
	resolvedRegion, err := config.ParseRegion(region)
	if err != nil {
		return bootstrapInstallResult{}, output.Validation("REGION_INVALID", err.Error())
	}
	installContext, cancel := context.WithTimeout(ctx, activationOperationTimeout)
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
	for _, retired := range retiredOfficialSkills {
		if err := transaction.RetireSkill(retired, agent, runtime.deps.Environment); err != nil {
			return rollback(output.Validation("RETIRED_SKILL_CONFLICT", "a retired official Skill was modified and could not be removed safely").WithHint("move or remove the conflicting retired Skill directory, then rerun the installation"))
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
	if sourceOverride != nil {
		if sourceOverride.APIBaseURL != "" {
			profile.APIBaseURL, err = config.NormalizeAPIBaseURL(sourceOverride.APIBaseURL)
			if err != nil {
				return rollback(output.Validation("API_BASE_URL_INVALID", err.Error()))
			}
		}
		if sourceOverride.WebBaseURL != "" {
			profile.WebBaseURL, err = config.NormalizeWebBaseURL(sourceOverride.WebBaseURL)
			if err != nil {
				return rollback(output.Validation("WEB_BASE_URL_INVALID", err.Error()))
			}
		}
		switch sourceOverride.ReleaseChannel {
		case config.ReleaseChannelStable, config.ReleaseChannelPOC:
			runtime.config.ReleaseChannel = sourceOverride.ReleaseChannel
		default:
			return rollback(output.Validation("RELEASE_CHANNEL_INVALID", "release channel must be stable or poc"))
		}
		runtime.config.ReleaseBaseURL, err = config.NormalizeReleaseBaseURL(sourceOverride.ReleaseBaseURL)
		if err != nil {
			return rollback(output.Validation("RELEASE_BASE_URL_INVALID", err.Error()))
		}
	} else if _, statErr := os.Stat(config.ConfigPath(runtime.configBase)); errors.Is(statErr, fs.ErrNotExist) {
		profile.APIBaseURL = config.APIBaseURL(resolvedRegion)
	} else if statErr != nil {
		return rollback(output.Internal("PROFILE_BACKUP_FAILED", "could not inspect the CLI configuration", statErr))
	}
	runtime.config.DistributionRegion = resolvedRegion
	if sourceOverride == nil && runtime.config.ReleaseChannel == config.ReleaseChannelStable {
		runtime.config.ReleaseBaseURL = config.StableReleaseBaseURL(resolvedRegion)
	}
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
		result.NextStep = installNextStep{Required: true, Command: "viceme auth login", Reason: "sign in before creating website integrations or publishing Skills"}
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
