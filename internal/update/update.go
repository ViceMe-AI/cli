// Package update implements the npm-backed ViceMe CLI update path. The npm
// launcher owns binary acquisition and checksum verification. CLI replacement
// and optional official Skill refresh are distinct operations so a Skill
// destination cannot roll back or block a completed CLI-only update.
package update

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/semver"
	"github.com/gofrs/flock"
)

const (
	PackageName             = "@viceme-ai/cli"
	RegistryURL             = "https://registry.npmjs.org"
	RegistryPackageURL      = RegistryURL + "/@viceme-ai%2fcli/latest"
	ScopeRegistryArg        = "--@viceme-ai:registry=" + RegistryURL
	updateStateFilename     = "update-state.json"
	releaseUpdateStateStem  = "release-update-state-"
	npmCacheDirectory       = "npm-cache"
	npmActivationFilename   = NPMActivationJournalFilename
	updateCacheTTL          = 5 * time.Minute
	maximumRegistryResponse = 256 << 10
	npmActivationNonceBytes = 32
)

var ErrNPMInstallRequired = errors.New("viceme update requires the npm-installed launcher")

type ErrorKind string

const (
	ErrorRegistryNetwork     ErrorKind = "registry_network"
	ErrorRegistryResponse    ErrorKind = "registry_response"
	ErrorNPMMissing          ErrorKind = "npm_missing"
	ErrorNPMPermission       ErrorKind = "npm_permission"
	ErrorPermission          ErrorKind = "permission"
	ErrorNPMCommand          ErrorKind = "npm_command"
	ErrorReleaseNetwork      ErrorKind = "release_network"
	ErrorReleaseResponse     ErrorKind = "release_response"
	ErrorReleaseIntegrity    ErrorKind = "release_integrity"
	ErrorReleaseReplace      ErrorKind = "release_replace"
	ErrorReleaseSkillRefresh ErrorKind = "release_skill_refresh"
)

type OperationError struct {
	Kind  ErrorKind
	Cause error
}

func (err *OperationError) Error() string {
	if err.Cause == nil {
		return string(err.Kind)
	}
	return err.Cause.Error()
}

func (err *OperationError) Unwrap() error { return err.Cause }

func ErrorKindOf(err error) ErrorKind {
	var operationError *OperationError
	if errors.As(err, &operationError) {
		return operationError.Kind
	}
	return ""
}

type CheckResult struct {
	CurrentVersion   string `json:"current_version"`
	AvailableVersion string `json:"available_version,omitempty"`
	UpdateAvailable  bool   `json:"update_available"`
	Method           string `json:"method"`
	Package          string `json:"package"`
	Source           string `json:"source"`
}

type TargetResult struct {
	Target string `json:"target"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type ApplyOptions struct {
	RefreshSkills bool
	SkillTarget   string
}

type ApplyResult struct {
	PreviousCLIVersion string         `json:"previous_cli_version"`
	CLIVersion         string         `json:"cli_version"`
	Targets            []TargetResult `json:"targets"`
}

type Service interface {
	EnsureLauncher(context.Context) (TargetResult, error)
	Check(context.Context) (CheckResult, error)
	Apply(context.Context, CheckResult, ApplyOptions) (ApplyResult, error)
}

// AutomaticChecker performs the cached release lookup used by the detached
// automatic-update worker. A fresh, validated local result avoids another
// network request; stale state is refreshed from the authoritative channel.
type AutomaticChecker interface {
	CheckAutomatic(context.Context) (CheckResult, error)
}

// LockedStartupRecoverer reconciles an outer launcher activation while the
// caller holds ActivationLockFilename. Command startup uses this narrower
// contract so npm and standalone journals share one coordinator and one lock.
type LockedStartupRecoverer interface {
	RecoverActivationWhileLocked(context.Context) error
}

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = withoutEnvironmentVariable(os.Environ(), "VICEME_ACCESS_TOKEN")
	return command.CombinedOutput()
}

func withoutEnvironmentVariable(environment []string, name string) []string {
	prefix := name + "="
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

type NPMService struct {
	CurrentVersion    string
	ComparableVersion string
	InstallMethod     string
	ConfigDir         string
	RegistryEndpoint  string
	HTTPClient        *http.Client
	Now               func() time.Time
	Runner            Runner
}

func NewNPMService(currentVersion, comparableVersion, installMethod string) *NPMService {
	return &NPMService{
		CurrentVersion:    currentVersion,
		ComparableVersion: comparableVersion,
		InstallMethod:     installMethod,
		RegistryEndpoint:  RegistryPackageURL,
		HTTPClient:        &http.Client{Timeout: 15 * time.Second},
		Now:               time.Now,
		Runner:            ExecRunner{},
	}
}

func (service *NPMService) EnsureLauncher(ctx context.Context) (TargetResult, error) {
	result := TargetResult{Target: "npm_global", Status: "skipped"}
	if service.InstallMethod != "npm" {
		return result, nil
	}
	if _, err := semver.Parse(service.ComparableVersion); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result, fmt.Errorf("refuse launcher install without an exact semantic version: %w", err)
	}
	target, err := NewNPMGeneration(service.ComparableVersion)
	if err != nil {
		return result, err
	}
	err = service.withNPMActivationLock(ctx, func() error {
		if err := service.recoverNPMActivation(ctx); err != nil {
			return err
		}
		if err := ValidateActivationTarget(service.ConfigDir, target); err != nil {
			return err
		}
		if err := service.probeNPMActivation(ctx); err != nil {
			return err
		}
		journal, err := service.newNPMActivationJournal(target, "auto", false)
		if err != nil {
			return err
		}
		if err := service.writeNPMActivation(journal); err != nil {
			return err
		}
		if err := service.applyNPMActivation(ctx, &journal); err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			return err
		}
		return service.finishNPMActivation(&journal)
	})
	if err != nil {
		return result, err
	}
	result.Status = "updated"
	return result, nil
}

// PrepareCoordinatedInstallWhileLocked persists and stages the npm member of a
// full CLI+Skills+config generation while the command layer holds both
// ActivationLockFilename and ActivationMemberLockFilename. The caller must
// commit its Skill/config transaction, consume the returned nonce with
// ConfirmActivationChildCommitted, and finish recovery before releasing the
// locks. If the process dies, the durable journal lets startup recovery finish
// the exact target generation through the normal bound child path.
func (service *NPMService) PrepareCoordinatedInstallWhileLocked(ctx context.Context, skillTarget string) (TargetResult, string, error) {
	result := TargetResult{Target: "npm_global", Status: "failed"}
	if service.InstallMethod != "npm" {
		return result, "", ErrNPMInstallRequired
	}
	target, err := NewNPMGeneration(service.ComparableVersion)
	if err != nil {
		return result, "", err
	}
	outer, err := InspectOuterActivationJournals(service.ConfigDir)
	if err != nil {
		return result, "", err
	}
	if outer.Bootstrap {
		return result, "", errors.New("a standalone bootstrap activation journal is pending")
	}
	if outer.NPM {
		return result, "", errors.New("an npm activation journal is already pending")
	}
	if err := ValidateActivationTarget(service.ConfigDir, target); err != nil {
		return result, "", err
	}
	if err := service.probeNPMActivation(ctx); err != nil {
		return result, "", err
	}
	journal, err := service.newNPMActivationJournal(target, skillTarget, true)
	if err != nil {
		return result, "", err
	}
	if err := service.writeNPMActivation(journal); err != nil {
		return result, "", err
	}
	if _, err := service.installExactPackage(ctx, journal.TargetVersion); err != nil {
		return result, journal.Nonce, err
	}
	journal.Status = "COMMITTING"
	if err := service.writeNPMActivation(journal); err != nil {
		return result, journal.Nonce, err
	}
	result.Status = "updated"
	return result, journal.Nonce, nil
}

func (service *NPMService) Check(ctx context.Context) (CheckResult, error) {
	result := CheckResult{
		CurrentVersion: service.CurrentVersion,
		Method:         "npm",
		Package:        PackageName,
	}
	available, source, err := service.latestVersion(ctx)
	if err != nil {
		return result, err
	}
	comparison, err := semver.Compare(available, service.ComparableVersion)
	if err != nil {
		return result, fmt.Errorf("compare npm release with current CLI: %w", err)
	}
	result.AvailableVersion = available
	result.UpdateAvailable = comparison > 0
	result.Source = source
	return result, nil
}

func (service *NPMService) CheckAutomatic(ctx context.Context) (CheckResult, error) {
	result := CheckResult{
		CurrentVersion: service.CurrentVersion,
		Method:         "npm",
		Package:        PackageName,
	}
	if service.InstallMethod != "npm" || automaticUpdateSuppressed(service.CurrentVersion, service.ComparableVersion) {
		result.Source = "disabled"
		return result, nil
	}
	if state, ok := service.loadUpdateState(); ok && service.updateStateIsFresh(state) {
		return service.checkForAvailableVersion(state.LatestVersion, "cache")
	}
	return service.Check(ctx)
}

func (service *NPMService) checkForAvailableVersion(available, source string) (CheckResult, error) {
	result := CheckResult{
		CurrentVersion:   service.CurrentVersion,
		AvailableVersion: available,
		Method:           "npm",
		Package:          PackageName,
		Source:           source,
	}
	comparison, err := semver.Compare(available, service.ComparableVersion)
	if err != nil {
		return result, fmt.Errorf("compare npm release with current CLI: %w", err)
	}
	result.UpdateAvailable = comparison > 0
	return result, nil
}

func (service *NPMService) Apply(ctx context.Context, check CheckResult, options ApplyOptions) (ApplyResult, error) {
	result := ApplyResult{PreviousCLIVersion: service.CurrentVersion, CLIVersion: service.CurrentVersion}
	if service.InstallMethod != "npm" {
		return result, ErrNPMInstallRequired
	}
	if _, err := semver.Parse(check.AvailableVersion); err != nil {
		return result, fmt.Errorf("refuse update without an exact semantic version: %w", err)
	}
	targetVersion := service.ComparableVersion
	if check.UpdateAvailable {
		targetVersion = check.AvailableVersion
	}
	if _, err := semver.Parse(targetVersion); err != nil {
		return result, fmt.Errorf("refuse update without an exact semantic version: %w", err)
	}
	target := options.SkillTarget
	if target == "" {
		target = "auto"
	}
	cliTarget := TargetResult{Target: "npm_global", Status: "updated"}
	skillTarget := TargetResult{Target: "agent_skill:" + target, Status: "updated"}
	generation, err := NewNPMGeneration(targetVersion)
	if err != nil {
		return result, err
	}
	alreadyActive := false
	preflightBlocked := false
	err = service.withNPMActivationLock(ctx, func() error {
		_, pending, inspectErr := service.readNPMActivation()
		if inspectErr != nil {
			return inspectErr
		}
		if err := service.recoverNPMActivation(ctx); err != nil {
			return err
		}
		if err := ValidateActivationTarget(service.ConfigDir, generation); err != nil {
			return err
		}
		active, exists, err := ReadActiveGeneration(service.ConfigDir)
		if err != nil {
			return err
		}
		if exists && active == generation {
			// A concurrent updater just activated this exact generation, so an
			// update-driven apply coalesces into its completed result. An
			// explicit repair without a newer release must still reinstall the
			// official Skills through the exact-version child.
			if check.UpdateAvailable || !options.RefreshSkills {
				alreadyActive = true
				return nil
			}
		}
		if err := service.probeNPMActivation(ctx); err != nil {
			// Only this pre-mutation refusal can keep the current command
			// running. Never infer safety from a missing journal after releasing
			// the lock: another process may have recovered a changed generation.
			running, generationErr := NewNPMGeneration(service.ComparableVersion)
			preflightBlocked = IsPermissionDenied(err) && !pending && generationErr == nil && exists && active == running
			return err
		}
		journal, err := service.newNPMActivationJournal(generation, target, options.RefreshSkills)
		if err != nil {
			return err
		}
		if err := service.writeNPMActivation(journal); err != nil {
			return err
		}
		if err := service.applyNPMActivation(ctx, &journal); err != nil {
			return err
		}
		return service.finishNPMActivation(&journal)
	})
	if err != nil {
		status := "recovery_pending"
		if preflightBlocked {
			status = "blocked"
		}
		cliTarget.Status = status
		result.Targets = append(result.Targets, cliTarget)
		if options.RefreshSkills {
			skillTarget.Status = status
			result.Targets = append(result.Targets, skillTarget)
			return result, fmt.Errorf("npm CLI and Skill activation did not complete: %w", err)
		}
		return result, fmt.Errorf("npm CLI activation did not complete: %w", err)
	}
	if !check.UpdateAvailable {
		cliTarget.Status = "unchanged"
	}
	if alreadyActive {
		cliTarget.Status = "unchanged"
		if options.RefreshSkills {
			skillTarget.Status = "unchanged"
		}
	}
	result.CLIVersion = targetVersion
	result.Targets = append(result.Targets, cliTarget)
	if options.RefreshSkills {
		result.Targets = append(result.Targets, skillTarget)
	}
	return result, nil
}

func (service *NPMService) installExactPackage(ctx context.Context, version string) ([]byte, error) {
	return service.runNPM(ctx, "install", "--registry="+RegistryURL, ScopeRegistryArg, "--global", "--ignore-scripts", "--no-audit", "--no-fund", PackageName+"@"+version)
}

type npmActivationJournal struct {
	SchemaVersion int               `json:"schemaVersion"`
	Status        string            `json:"status"`
	Nonce         string            `json:"nonce"`
	TargetVersion string            `json:"targetVersion"`
	Target        ActiveGeneration  `json:"target"`
	Previous      *ActiveGeneration `json:"previous,omitempty"`
	SkillTarget   string            `json:"skillTarget"`
	RefreshSkills bool              `json:"refreshSkills"`
}

func (service *NPMService) withNPMActivationLock(ctx context.Context, operation func() error) error {
	if service.ConfigDir == "" {
		return &OperationError{Kind: ErrorNPMPermission, Cause: errors.New("ViceMe config directory is required for recoverable npm activation")}
	}
	if err := os.MkdirAll(service.ConfigDir, 0o700); err != nil {
		return &OperationError{Kind: ErrorNPMPermission, Cause: errors.New("could not create the ViceMe config directory")}
	}
	activationLock := flock.New(filepath.Join(service.ConfigDir, ActivationLockFilename))
	locked, err := activationLock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return &OperationError{Kind: ErrorNPMPermission, Cause: errors.New("could not acquire the npm activation lock")}
	}
	if !locked {
		return &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("timed out waiting for another npm CLI and Skill activation")}
	}
	defer activationLock.Unlock()
	memberLock := flock.New(filepath.Join(service.ConfigDir, ActivationMemberLockFilename))
	memberAvailable, err := memberLock.TryLock()
	if err != nil {
		return &OperationError{Kind: ErrorNPMPermission, Cause: errors.New("could not inspect the activation member lock")}
	}
	if !memberAvailable {
		return &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("an activation child is still committing Skills and config")}
	}
	_ = memberLock.Unlock()
	outer, err := InspectOuterActivationJournals(service.ConfigDir)
	if err != nil {
		return &OperationError{Kind: ErrorNPMPermission, Cause: errors.New("could not inspect outer activation journals")}
	}
	if outer.Bootstrap {
		return &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("a standalone bootstrap activation journal must be recovered before npm activation")}
	}
	return operation()
}

func (service *NPMService) RecoverAtStartup(ctx context.Context) error {
	if err := service.withNPMActivationLock(ctx, func() error {
		return service.recoverNPMActivation(ctx)
	}); err != nil {
		return err
	}
	active, exists, err := ReadActiveGeneration(service.ConfigDir)
	if err != nil {
		return err
	}
	running, generationErr := NewNPMGeneration(service.ComparableVersion)
	if generationErr != nil {
		return generationErr
	}
	if exists && active != running {
		return ErrActivationRestartNeeded
	}
	return nil
}

// RecoverActivationWhileLocked is used by the command-level activation
// coordinator. It deliberately ignores the launcher's current install method:
// every launcher must inspect an interrupted npm journal before business logic.
func (service *NPMService) RecoverActivationWhileLocked(ctx context.Context) error {
	if service.ConfigDir == "" {
		return &OperationError{Kind: ErrorNPMPermission, Cause: errors.New("ViceMe config directory is required for recoverable npm activation")}
	}
	return service.recoverNPMActivation(ctx)
}

func (service *NPMService) recoverNPMActivation(ctx context.Context) error {
	journal, exists, err := service.readNPMActivation()
	if err != nil || !exists {
		return err
	}
	if journal.SchemaVersion < 3 || journal.Nonce == "" {
		journal.SchemaVersion = 3
		journal.Nonce, err = newNPMActivationNonce()
		if err != nil {
			return err
		}
		if err := service.writeNPMActivation(journal); err != nil {
			return err
		}
	}
	active, activeExists, err := ReadActiveGeneration(service.ConfigDir)
	if err != nil {
		return err
	}
	targetWasAlreadyActive := journal.Previous != nil && *journal.Previous == journal.Target
	if activeExists && active == journal.Target && (journal.Status == "COMMITTED" || !targetWasAlreadyActive) {
		// The active-generation record is the semantic commit point. A crash
		// after it was written may only retire the journal; it must never
		// perform another network mutation or roll back the committed target.
		return service.removeNPMActivation()
	}
	if journal.Status == "ROLLED_BACK" {
		if journal.Previous == nil {
			return errors.New("rolled-back npm activation has no previous generation")
		}
		if err := CommitActiveGeneration(service.ConfigDir, *journal.Previous); err != nil {
			return err
		}
		return service.removeNPMActivation()
	}
	if journal.Status == "CHILD_COMMITTED" {
		return service.finishNPMActivation(&journal)
	}
	if journal.Status == "ROLLING_BACK" {
		return service.rollbackNPMActivation(ctx, journal)
	}
	if err := ValidateActivationTarget(service.ConfigDir, journal.Target); err != nil {
		if errors.Is(err, ErrActivationDowngrade) {
			return service.removeNPMActivation()
		}
		return err
	}
	if journal.Status == "COMMITTED" {
		return service.finishNPMActivation(&journal)
	}
	if err := service.probeNPMActivation(ctx); err != nil {
		return err
	}
	if err := service.applyNPMActivation(ctx, &journal); err != nil {
		if IsPermissionDenied(err) {
			return err
		}
		if rollbackErr := service.rollbackNPMActivation(ctx, journal); rollbackErr != nil {
			return fmt.Errorf("recover interrupted npm CLI and Skill activation: %w", errors.Join(err, rollbackErr))
		}
		return nil
	}
	return service.finishNPMActivation(&journal)
}

func (service *NPMService) applyNPMActivation(ctx context.Context, journal *npmActivationJournal) error {
	if err := ValidateActivationTarget(service.ConfigDir, journal.Target); err != nil {
		return err
	}
	if journal.Status == "COMMITTED" || journal.Status == "CHILD_COMMITTED" {
		return nil
	}
	_, err := service.installExactPackage(ctx, journal.TargetVersion)
	if err != nil {
		return fmt.Errorf("install exact npm launcher %s: %w", journal.TargetVersion, err)
	}
	if journal.Status != "COMMITTING" {
		journal.Status = "COMMITTING"
		if err := service.writeNPMActivation(*journal); err != nil {
			return err
		}
	}
	if !journal.RefreshSkills {
		return nil
	}
	if err := service.installSkillsFromExactPackage(ctx, journal.TargetVersion, journal.SkillTarget, journal.Nonce); err != nil {
		return fmt.Errorf("install and verify exact official Skills: %w", err)
	}
	confirmed, exists, err := service.readNPMActivation()
	if err != nil || !exists || confirmed.Status != "CHILD_COMMITTED" || confirmed.Nonce != journal.Nonce || confirmed.Target != journal.Target {
		return errors.New("npm activation child did not commit the exact target generation")
	}
	*journal = confirmed
	return nil
}

func (service *NPMService) installSkillsFromExactPackage(ctx context.Context, version, target, nonce string) error {
	exactPackage := PackageName + "@" + version
	_, err := service.runNPM(
		ctx,
		"exec",
		"--registry="+RegistryURL,
		ScopeRegistryArg,
		"--yes",
		"--package="+exactPackage,
		"--",
		"viceme",
		"install",
		"--agent",
		target,
		"--internal-skip-launcher-ensure",
		"--internal-activation-child="+nonce,
		"--internal-activation-target="+version,
	)
	if err != nil {
		return err
	}
	return service.ConfirmActivationChildCommitted(nonce, version, target)
}

func (service *NPMService) rollbackNPMActivation(ctx context.Context, journal npmActivationJournal) error {
	active, exists, err := ReadActiveGeneration(service.ConfigDir)
	if err != nil {
		return err
	}
	if exists && active == journal.Target {
		if journal.Previous != nil && *journal.Previous == journal.Target {
			// A same-generation repair never changed the committed generation;
			// rolling it back only retires the journal.
			return service.removeNPMActivation()
		}
		return errors.New("npm activation target is already committed and cannot be rolled back")
	}
	if journal.Previous == nil {
		// The attempt began before any generation was committed. Rolling back
		// restores the no-generation state by retiring the journal; there is
		// no durable previous launcher or Skill set to reinstall. Retaining
		// the journal instead would make every later command fail recovery.
		return service.removeNPMActivation()
	}
	if journal.Previous.InstallMethod != "npm" {
		return errors.New("npm activation cannot restore a previous non-npm generation")
	}
	if exists && active != *journal.Previous {
		return errors.New("npm activation previous generation is no longer active")
	}
	if err := service.probeNPMActivation(ctx); err != nil {
		return err
	}
	nonce, err := newNPMActivationNonce()
	if err != nil {
		return err
	}
	journal.Status = "ROLLING_BACK"
	journal.Nonce = nonce
	if err := service.writeNPMActivation(journal); err != nil {
		return err
	}
	if _, err := service.installExactPackage(ctx, journal.Previous.Version); err != nil {
		return fmt.Errorf("restore previous npm launcher %s: %w", journal.Previous.Version, err)
	}
	if journal.RefreshSkills {
		if err := service.installSkillsFromExactPackage(ctx, journal.Previous.Version, journal.SkillTarget, journal.Nonce); err != nil {
			return fmt.Errorf("restore previous official Skills: %w", err)
		}
		confirmed, confirmedExists, err := service.readNPMActivation()
		if err != nil || !confirmedExists || confirmed.Status != "ROLLED_BACK" || confirmed.Nonce != journal.Nonce {
			return errors.New("npm rollback child did not commit the exact previous generation")
		}
		journal = confirmed
	} else {
		journal.Status = "ROLLED_BACK"
		if err := service.writeNPMActivation(journal); err != nil {
			return err
		}
	}
	if err := CommitActiveGeneration(service.ConfigDir, *journal.Previous); err != nil {
		return err
	}
	return service.removeNPMActivation()
}

func (service *NPMService) readNPMActivation() (npmActivationJournal, bool, error) {
	data, err := os.ReadFile(filepath.Join(service.ConfigDir, npmActivationFilename))
	if errors.Is(err, os.ErrNotExist) {
		return npmActivationJournal{}, false, nil
	}
	if err != nil {
		return npmActivationJournal{}, false, &OperationError{Kind: ErrorNPMPermission, Cause: errors.New("could not read the npm activation journal")}
	}
	var journal npmActivationJournal
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&journal); err != nil || (journal.SchemaVersion != 1 && journal.SchemaVersion != 2 && journal.SchemaVersion != 3) {
		return npmActivationJournal{}, false, &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("npm activation journal is invalid")}
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return npmActivationJournal{}, false, &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("npm activation journal contains trailing JSON")}
	}
	if journal.SchemaVersion == 1 {
		journal.SchemaVersion = 2
		journal.Status = "PREPARING"
		journal.Target, err = NewNPMGeneration(journal.TargetVersion)
		if err != nil {
			return npmActivationJournal{}, false, &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("npm activation journal contains invalid targets")}
		}
	}
	if err := validateReadableNPMActivationJournal(journal); err != nil {
		return npmActivationJournal{}, false, &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("npm activation journal contains invalid targets")}
	}
	return journal, true, nil
}

func (service *NPMService) writeNPMActivation(journal npmActivationJournal) error {
	journal.SchemaVersion = 3
	if err := validateNPMActivationJournal(journal); err != nil {
		return &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("refuse invalid npm activation journal")}
	}
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := privatefile.Write(filepath.Join(service.ConfigDir, npmActivationFilename), data, ".npm-activation-*.tmp"); err != nil {
		return &OperationError{Kind: ErrorNPMPermission, Cause: fmt.Errorf("could not write the npm recovery journal: %w", err)}
	}
	return nil
}

func (service *NPMService) finishNPMActivation(journal *npmActivationJournal) error {
	if journal.Status != "COMMITTED" {
		journal.Status = "COMMITTED"
		if err := service.writeNPMActivation(*journal); err != nil {
			return &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("could not persist the committed npm generation")}
		}
	}
	if err := CommitActiveGeneration(service.ConfigDir, journal.Target); err != nil {
		return &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("could not commit the active npm generation")}
	}
	return service.removeNPMActivation()
}

func (service *NPMService) removeNPMActivation() error {
	err := os.Remove(filepath.Join(service.ConfigDir, npmActivationFilename))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return &OperationError{Kind: ErrorNPMPermission, Cause: errors.New("could not commit the npm activation journal")}
	}
	return nil
}

func (service *NPMService) newNPMActivationJournal(target ActiveGeneration, skillTarget string, refreshSkills bool) (npmActivationJournal, error) {
	previous, exists, err := ReadActiveGeneration(service.ConfigDir)
	if err != nil {
		return npmActivationJournal{}, err
	}
	nonce, err := newNPMActivationNonce()
	if err != nil {
		return npmActivationJournal{}, err
	}
	journal := npmActivationJournal{
		SchemaVersion: 3,
		Status:        "PREPARING",
		Nonce:         nonce,
		TargetVersion: target.Version,
		Target:        target,
		SkillTarget:   skillTarget,
		RefreshSkills: refreshSkills,
	}
	if exists {
		if previous.InstallMethod != "npm" {
			return npmActivationJournal{}, ErrActivationMethodChange
		}
		journal.Previous = &previous
	}
	return journal, nil
}

func validateNPMActivationJournal(journal npmActivationJournal) error {
	if journal.SchemaVersion != 3 || (journal.Status != "PREPARING" && journal.Status != "COMMITTING" && journal.Status != "CHILD_COMMITTED" && journal.Status != "ROLLING_BACK" && journal.Status != "ROLLED_BACK" && journal.Status != "COMMITTED") {
		return errors.New("unsupported npm activation journal")
	}
	if len(journal.Nonce) != npmActivationNonceBytes*2 {
		return errors.New("npm activation nonce is invalid")
	}
	if _, err := hex.DecodeString(journal.Nonce); err != nil || journal.Nonce != strings.ToLower(journal.Nonce) {
		return errors.New("npm activation nonce is invalid")
	}
	if journal.TargetVersion != journal.Target.Version || !validSkillTarget(journal.SkillTarget) {
		return errors.New("npm activation targets do not match")
	}
	if err := validateActiveGeneration(journal.Target); err != nil || journal.Target.InstallMethod != "npm" {
		return errors.New("npm activation target is invalid")
	}
	if journal.Previous != nil {
		if err := validateActiveGeneration(*journal.Previous); err != nil || journal.Previous.InstallMethod != "npm" {
			return errors.New("npm activation previous generation is invalid")
		}
	}
	return nil
}

func validateReadableNPMActivationJournal(journal npmActivationJournal) error {
	if journal.SchemaVersion == 3 {
		return validateNPMActivationJournal(journal)
	}
	if journal.SchemaVersion != 2 || (journal.Status != "PREPARING" && journal.Status != "COMMITTING") {
		return errors.New("unsupported npm activation journal")
	}
	if journal.TargetVersion != journal.Target.Version || !validSkillTarget(journal.SkillTarget) {
		return errors.New("npm activation targets do not match")
	}
	if err := validateActiveGeneration(journal.Target); err != nil || journal.Target.InstallMethod != "npm" {
		return errors.New("npm activation target is invalid")
	}
	if journal.Previous != nil {
		if err := validateActiveGeneration(*journal.Previous); err != nil || journal.Previous.InstallMethod != "npm" {
			return errors.New("npm activation previous generation is invalid")
		}
	}
	return nil
}

func newNPMActivationNonce() (string, error) {
	value := make([]byte, npmActivationNonceBytes)
	if _, err := rand.Read(value); err != nil {
		return "", &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("could not create the npm activation child nonce")}
	}
	return hex.EncodeToString(value), nil
}

// ValidateActivationChild binds the lock-skipping install child to the exact
// durable journal created by its parent coordinator. Bare hidden flags cannot
// authorize a mutation.
func (service *NPMService) ValidateActivationChild(nonce, targetVersion, skillTarget string) (ActiveGeneration, error) {
	journal, exists, err := service.readNPMActivation()
	if err != nil {
		return ActiveGeneration{}, err
	}
	if !exists || journal.SchemaVersion != 3 || !journal.RefreshSkills {
		return ActiveGeneration{}, errors.New("npm activation child has no committing parent journal")
	}
	expected := journal.Target
	expectedVersion := journal.TargetVersion
	if journal.Status == "ROLLING_BACK" && journal.Previous != nil {
		expected = *journal.Previous
		expectedVersion = journal.Previous.Version
	} else if journal.Status != "COMMITTING" {
		return ActiveGeneration{}, errors.New("npm activation child has no committing parent journal")
	}
	if targetVersion != expectedVersion || skillTarget != journal.SkillTarget || len(nonce) != len(journal.Nonce) || subtle.ConstantTimeCompare([]byte(nonce), []byte(journal.Nonce)) != 1 {
		return ActiveGeneration{}, errors.New("npm activation child does not match its parent journal")
	}
	return expected, nil
}

// ConfirmActivationChildCommitted consumes a validated child nonce after the
// child has committed its Skill/config transaction while holding
// ActivationMemberLockFilename. Reusing the same child context is rejected.
func (service *NPMService) ConfirmActivationChildCommitted(nonce, targetVersion, skillTarget string) error {
	journal, exists, err := service.readNPMActivation()
	if err != nil {
		return err
	}
	if !exists || journal.SchemaVersion != 3 || !journal.RefreshSkills || journal.Nonce != nonce || journal.SkillTarget != skillTarget {
		return errors.New("npm activation child no longer owns the parent journal")
	}
	switch journal.Status {
	case "COMMITTING":
		if targetVersion != journal.TargetVersion {
			return errors.New("npm activation child target changed before commit")
		}
		journal.Status = "CHILD_COMMITTED"
	case "ROLLING_BACK":
		if journal.Previous == nil || targetVersion != journal.Previous.Version {
			return errors.New("npm rollback child target changed before commit")
		}
		journal.Status = "ROLLED_BACK"
	case "CHILD_COMMITTED":
		if targetVersion != journal.TargetVersion {
			return errors.New("npm activation child target changed before commit")
		}
		return nil
	case "ROLLED_BACK":
		if journal.Previous == nil || targetVersion != journal.Previous.Version {
			return errors.New("npm rollback child target changed before commit")
		}
		return nil
	default:
		return errors.New("npm activation child nonce was already consumed")
	}
	return service.writeNPMActivation(journal)
}

func validSkillTarget(target string) bool {
	switch target {
	case "auto", "codex", "claude", "workbuddy", "agents":
		return true
	default:
		return false
	}
}

func (service *NPMService) runner() Runner {
	if service.Runner == nil {
		return ExecRunner{}
	}
	return service.Runner
}

func (service *NPMService) latestVersion(ctx context.Context) (string, string, error) {
	version, err := service.fetchLatestVersion(ctx)
	if err == nil {
		service.saveUpdateState(version)
		return version, "registry", nil
	}
	if ErrorKindOf(err) == ErrorRegistryNetwork {
		if cached, ok := service.loadFreshUpdateState(); ok {
			return cached, "cache", nil
		}
	}
	return "", "", err
}

func automaticUpdateSuppressed(currentVersion, comparableVersion string) bool {
	if os.Getenv("CI") != "" {
		return true
	}
	if _, err := semver.Parse(currentVersion); err != nil {
		return true
	}
	_, err := semver.Parse(comparableVersion)
	return err != nil
}

func (service *NPMService) fetchLatestVersion(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, service.registryEndpoint(), nil)
	if err != nil {
		return "", &OperationError{Kind: ErrorRegistryResponse, Cause: fmt.Errorf("build npm registry request: %w", err)}
	}
	request.Header.Set("Accept", "application/json")
	response, err := service.httpClient().Do(request)
	if err != nil {
		return "", &OperationError{Kind: ErrorRegistryNetwork, Cause: fmt.Errorf("query npm registry: %w", err)}
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		kind := ErrorRegistryResponse
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError {
			kind = ErrorRegistryNetwork
		}
		return "", &OperationError{Kind: kind, Cause: fmt.Errorf("npm registry returned HTTP %d", response.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumRegistryResponse+1))
	if err != nil {
		return "", &OperationError{Kind: ErrorRegistryNetwork, Cause: fmt.Errorf("read npm registry response: %w", err)}
	}
	if len(body) > maximumRegistryResponse {
		return "", &OperationError{Kind: ErrorRegistryResponse, Cause: errors.New("npm registry response exceeded the size limit")}
	}
	var document struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return "", &OperationError{Kind: ErrorRegistryResponse, Cause: errors.New("npm registry returned invalid JSON")}
	}
	version := strings.TrimSpace(document.Version)
	if _, err := semver.Parse(version); err != nil {
		return "", &OperationError{Kind: ErrorRegistryResponse, Cause: fmt.Errorf("npm registry returned an invalid package version: %w", err)}
	}
	return version, nil
}

type updateState struct {
	LatestVersion string `json:"latest_version"`
	CheckedAt     int64  `json:"checked_at"`
}

func loadCachedUpdateState(filename string) (updateState, bool) {
	if filename == "" {
		return updateState{}, false
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return updateState{}, false
	}
	var state updateState
	if json.Unmarshal(data, &state) != nil || state.CheckedAt <= 0 {
		return updateState{}, false
	}
	if _, err := semver.Parse(state.LatestVersion); err != nil {
		return updateState{}, false
	}
	return state, true
}

func isCachedUpdateStateFresh(state updateState, now time.Time) bool {
	age := now.Sub(time.Unix(state.CheckedAt, 0))
	return age >= 0 && age <= updateCacheTTL
}

func saveCachedUpdateState(filename, directory, temporaryPattern, version string, now time.Time) {
	if filename == "" {
		return
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return
	}
	data, err := json.Marshal(updateState{LatestVersion: version, CheckedAt: now.Unix()})
	if err != nil {
		return
	}
	// The update cache is best effort: a sandbox that denies the activating
	// rename previously leaked one staging file per check, so route it through
	// the shared degraded write and ignore failures.
	_ = privatefile.Write(filename, data, temporaryPattern)
}

func (service *NPMService) loadUpdateState() (updateState, bool) {
	return loadCachedUpdateState(service.updateStatePath())
}

func (service *NPMService) updateStateIsFresh(state updateState) bool {
	return isCachedUpdateStateFresh(state, service.now())
}

func (service *NPMService) loadFreshUpdateState() (string, bool) {
	state, ok := service.loadUpdateState()
	if !ok || !service.updateStateIsFresh(state) {
		return "", false
	}
	return state.LatestVersion, true
}

func (service *NPMService) saveUpdateState(version string) {
	saveCachedUpdateState(service.updateStatePath(), service.ConfigDir, ".update-state-*", version, service.now())
}

func (service *NPMService) runNPM(ctx context.Context, args ...string) ([]byte, error) {
	if cacheArg := service.npmCacheArg(); cacheArg != "" {
		if err := os.MkdirAll(filepath.Join(service.ConfigDir, npmCacheDirectory), 0o700); err != nil {
			return nil, &OperationError{Kind: ErrorNPMPermission, Cause: errors.New("could not create the ViceMe npm cache directory")}
		}
		args = append([]string{cacheArg}, args...)
	}
	output, err := service.runner().Run(ctx, "npm", args...)
	if err == nil {
		return output, nil
	}
	return output, classifyNPMError(err, output)
}

func classifyNPMError(err error, output []byte) error {
	var executableError *exec.Error
	if errors.As(err, &executableError) || errors.Is(err, exec.ErrNotFound) {
		return &OperationError{Kind: ErrorNPMMissing, Cause: errors.New("npm is not available in PATH")}
	}
	normalized := strings.ToUpper(string(output))
	if errors.Is(err, os.ErrPermission) || strings.Contains(normalized, "EPERM") || strings.Contains(normalized, "EACCES") || strings.Contains(normalized, "EROFS") || strings.Contains(normalized, "CODEBUDDY_BROKER_DENY") || strings.Contains(normalized, "UPDATE_PERMISSION_REQUIRED") {
		return &OperationError{Kind: ErrorNPMPermission, Cause: errors.New("npm could not write its cache or global installation directory")}
	}
	return &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("npm command failed")}
}

func (service *NPMService) npmCacheArg() string {
	if service.ConfigDir == "" {
		return ""
	}
	return "--cache=" + filepath.Join(service.ConfigDir, npmCacheDirectory)
}

func (service *NPMService) updateStatePath() string {
	if service.ConfigDir == "" {
		return ""
	}
	return filepath.Join(service.ConfigDir, updateStateFilename)
}

func (service *NPMService) registryEndpoint() string {
	if service.RegistryEndpoint == "" {
		return RegistryPackageURL
	}
	return service.RegistryEndpoint
}

func (service *NPMService) httpClient() *http.Client {
	if service.HTTPClient == nil {
		return &http.Client{Timeout: 15 * time.Second}
	}
	return service.HTTPClient
}

func (service *NPMService) now() time.Time {
	if service.Now == nil {
		return time.Now()
	}
	return service.Now()
}

func commandError(err error, output []byte) string {
	// npm output can reflect registry configuration. Keep it out of stable JSON
	// rather than risking credential-bearing diagnostic text.
	_ = output
	return err.Error()
}
