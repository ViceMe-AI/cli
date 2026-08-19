package skillcontent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/semver"
	"github.com/gofrs/flock"
)

const installManifestPath = ".viceme/install-manifest.json"
const installTransactionFilename = "install-transaction.json"

type Environment struct {
	Home               string
	CodexHome          string
	ClaudeConfigDir    string
	WorkBuddyConfigDir string
	ConfigDir          string
}

func DefaultEnvironment() Environment {
	home, _ := os.UserHomeDir()
	return Environment{
		Home:               home,
		CodexHome:          os.Getenv("CODEX_HOME"),
		ClaudeConfigDir:    os.Getenv("CLAUDE_CONFIG_DIR"),
		WorkBuddyConfigDir: os.Getenv("WORKBUDDY_CONFIG_DIR"),
		ConfigDir:          defaultConfigDir(home),
	}
}

func defaultConfigDir(home string) string {
	if directory := os.Getenv("VICEME_CLI_CONFIG_DIR"); directory != "" {
		return directory
	}
	if runtime.GOOS == "windows" {
		if directory := os.Getenv("LOCALAPPDATA"); directory != "" {
			preferred := filepath.Join(directory, "ViceMe", "Config")
			legacy := filepath.Join(home, ".viceme-cli")
			if _, err := os.Stat(filepath.Join(preferred, "config.json")); err == nil || !errors.Is(err, fs.ErrNotExist) {
				return preferred
			}
			if _, err := os.Stat(filepath.Join(legacy, "config.json")); err == nil || !errors.Is(err, fs.ErrNotExist) {
				return legacy
			}
			return preferred
		}
	}
	return filepath.Join(home, ".viceme-cli")
}

type InstallResult struct {
	Skill  string `json:"skill"`
	Target string `json:"target"`
	Path   string `json:"path"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type InstallReport struct {
	AllSucceeded bool            `json:"all_succeeded"`
	Results      []InstallResult `json:"results"`
}

type DoctorResult struct {
	Target                 string       `json:"target"`
	Path                   string       `json:"path"`
	Installed              bool         `json:"installed"`
	Healthy                bool         `json:"healthy"`
	ExpectedDigest         string       `json:"expected_digest"`
	ActualDigest           string       `json:"actual_digest,omitempty"`
	ExpectedEmbeddedDigest string       `json:"expected_embedded_digest"`
	ActualEmbeddedDigest   string       `json:"actual_embedded_digest,omitempty"`
	ManifestPath           string       `json:"manifest_path"`
	Checks                 DoctorChecks `json:"checks"`
	Problem                string       `json:"problem,omitempty"`
}

type DoctorReport struct {
	Healthy bool           `json:"healthy"`
	Results []DoctorResult `json:"results"`
}

type DoctorCheck struct {
	Healthy  bool   `json:"healthy"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Recorded string `json:"recorded,omitempty"`
	Problem  string `json:"problem,omitempty"`
}

type DoctorChecks struct {
	CLIVersion            DoctorCheck `json:"cli_version"`
	SkillVersion          DoctorCheck `json:"skill_version"`
	FullBundleDigest      DoctorCheck `json:"full_bundle_digest"`
	EmbeddedContentDigest DoctorCheck `json:"embedded_content_digest"`
	Compatibility         DoctorCheck `json:"compatibility"`
}

type installManifest struct {
	SchemaVersion         int    `json:"schema_version"`
	CLIVersion            string `json:"cli_version"`
	SkillVersion          string `json:"skill_version"`
	MinimumCLIVersion     string `json:"minimum_cli_version"`
	CLICompatibility      string `json:"cli_compatibility"`
	FullBundleDigest      string `json:"full_skill_bundle_digest"`
	EmbeddedContentDigest string `json:"embedded_content_digest"`
}

func (b *Bundle) Install(name, target string, environment Environment) InstallReport {
	reports := b.InstallSet([]string{name}, target, environment)
	if len(reports) == 0 {
		return InstallReport{AllSucceeded: false, Results: []InstallResult{{Skill: name, Target: target, Status: "failed", Error: "installation transaction returned no report"}}}
	}
	return reports[0]
}

// InstallSet activates every requested Skill and target as one recoverable
// local transaction. A process crash leaves a private journal and backups that
// the next invocation rolls back before starting new work.
func (b *Bundle) InstallSet(names []string, target string, environment Environment) []InstallReport {
	transaction, reports, err := b.PrepareInstallSet(names, target, environment)
	if err != nil {
		return failAllInstallReports(names, target, err)
	}
	if err := transaction.Commit(); err != nil {
		return failAllInstallReports(names, target, err)
	}
	return reports
}

// InstallTransaction keeps every previous Skill/config path and the process
// advisory lock alive until the caller has completed its final Doctor checks.
// A crash releases the lock but leaves the journal, so the next invocation can
// restore the complete previous generation before doing any new work.
type InstallTransaction struct {
	journalPath string
	journal     installTransaction
	lock        *flock.Flock
	closed      bool
}

func (b *Bundle) PrepareInstallSet(names []string, target string, environment Environment) (*InstallTransaction, []InstallReport, error) {
	reports := make([]InstallReport, len(names))
	for index, name := range names {
		reports[index] = InstallReport{AllSucceeded: true}
		if err := b.Validate(name); err != nil {
			return nil, nil, err
		}
	}
	if environment.ConfigDir == "" {
		environment.ConfigDir = defaultConfigDir(environment.Home)
	}
	if err := os.MkdirAll(environment.ConfigDir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create ViceMe config directory: %w", err)
	}
	transactionLock := flock.New(filepath.Join(environment.ConfigDir, "install.lock"))
	locked, err := transactionLock.TryLock()
	if err != nil {
		return nil, nil, fmt.Errorf("acquire install transaction lock: %w", err)
	}
	if !locked {
		return nil, nil, errors.New("another ViceMe install transaction is active")
	}
	journalPath := filepath.Join(environment.ConfigDir, installTransactionFilename)
	if err := recoverInstallTransaction(journalPath); err != nil {
		_ = transactionLock.Unlock()
		return nil, nil, err
	}
	transaction := &InstallTransaction{
		journalPath: journalPath,
		journal:     installTransaction{SchemaVersion: 1, Status: "PREPARING", TargetCLIVersion: buildinfo.Version},
		lock:        transactionLock,
	}
	fail := func(cause error) (*InstallTransaction, []InstallReport, error) {
		rollbackErr := transaction.Rollback()
		if rollbackErr != nil {
			return nil, nil, errors.Join(cause, rollbackErr)
		}
		return nil, nil, cause
	}

	operations := make([]installOperation, 0)
	for index, name := range names {
		paths, resolveErr := resolveTargets(name, target, environment, true)
		if resolveErr != nil {
			return fail(resolveErr)
		}
		for _, resolved := range paths {
			operation := installOperation{Skill: name, Target: resolved.name, Destination: resolved.path, ReportIndex: index}
			if b.installationCurrent(name, resolved.path) {
				operation.Unchanged = true
			}
			operations = append(operations, operation)
		}
	}

	for index := range operations {
		operation := &operations[index]
		if operation.Unchanged {
			continue
		}
		staged, expected, stageErr := b.stageInstallation(operation.Skill, operation.Destination)
		if stageErr != nil {
			cleanupStagedOperations(operations)
			return fail(stageErr)
		}
		operation.Stage = staged
		operation.Expected = expected
		operation.Backup = operation.Destination + ".viceme-transaction-backup"
		if _, statErr := os.Lstat(operation.Destination); statErr == nil {
			operation.HadExisting = true
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			cleanupStagedOperations(operations)
			return fail(fmt.Errorf("inspect existing Skill: %w", statErr))
		}
		transaction.journal.Entries = append(transaction.journal.Entries, installJournalEntry{
			Destination: operation.Destination, Stage: operation.Stage, Backup: operation.Backup, HadExisting: operation.HadExisting,
		})
	}
	if len(transaction.journal.Entries) > 0 {
		if err := writeInstallTransaction(journalPath, transaction.journal); err != nil {
			cleanupStagedOperations(operations)
			return fail(err)
		}
	}

	journalIndex := 0
	for index := range operations {
		operation := &operations[index]
		if operation.Unchanged {
			continue
		}
		entry := &transaction.journal.Entries[journalIndex]
		entry.Activating = true
		if err := writeInstallTransaction(journalPath, transaction.journal); err != nil {
			return fail(err)
		}
		_ = os.RemoveAll(operation.Backup)
		if operation.HadExisting {
			if err := os.Rename(operation.Destination, operation.Backup); err != nil {
				return fail(fmt.Errorf("stage existing Skill: %w", err))
			}
		}
		if err := os.Rename(operation.Stage, operation.Destination); err != nil {
			return fail(fmt.Errorf("activate staged Skill: %w", err))
		}
		actual, err := digestsInstalled(operation.Destination)
		if err != nil || actual != operation.Expected {
			if err != nil {
				return fail(fmt.Errorf("verify installed Skill: %w", err))
			}
			return fail(errors.New("verify installed Skill: digest mismatch"))
		}
		journalIndex++
	}
	for _, operation := range operations {
		status := "updated"
		if operation.Unchanged {
			status = "unchanged"
		}
		report := &reports[operation.ReportIndex]
		report.Results = append(report.Results, InstallResult{Skill: operation.Skill, Target: operation.Target, Path: operation.Destination, Status: status})
	}
	return transaction, reports, nil
}

// TrackPath adds a non-Skill file (currently the CLI profile config) to the
// same durable rollback journal before the caller replaces it.
func (transaction *InstallTransaction) TrackPath(destination string) error {
	if transaction == nil || transaction.closed {
		return errors.New("install transaction is not active")
	}
	absDestination, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve install transaction path: %w", err)
	}
	for _, entry := range transaction.journal.Entries {
		if entry.Destination == absDestination {
			return nil
		}
	}
	hadExisting := false
	if _, err := os.Lstat(absDestination); err == nil {
		hadExisting = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect install transaction path: %w", err)
	}
	backup := absDestination + ".viceme-transaction-backup"
	entry := installJournalEntry{Destination: absDestination, Backup: backup, HadExisting: hadExisting, Activating: true}
	transaction.journal.Entries = append(transaction.journal.Entries, entry)
	if err := writeInstallTransaction(transaction.journalPath, transaction.journal); err != nil {
		transaction.journal.Entries = transaction.journal.Entries[:len(transaction.journal.Entries)-1]
		return err
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove stale install backup: %w", err)
	}
	if hadExisting {
		if err := os.Rename(absDestination, backup); err != nil {
			return fmt.Errorf("preserve install transaction path: %w", err)
		}
	}
	return nil
}

func (transaction *InstallTransaction) Commit() error {
	if transaction == nil || transaction.closed {
		return errors.New("install transaction is not active")
	}
	transaction.journal.Status = "COMMITTED"
	if len(transaction.journal.Entries) > 0 {
		if err := writeInstallTransaction(transaction.journalPath, transaction.journal); err != nil {
			transaction.close()
			return err
		}
		if err := recoverInstallTransaction(transaction.journalPath); err != nil {
			transaction.close()
			return err
		}
	}
	transaction.close()
	return nil
}

func (transaction *InstallTransaction) MarkCommitting() error {
	if transaction == nil || transaction.closed {
		return errors.New("install transaction is not active")
	}
	transaction.journal.Status = "COMMITTING"
	if len(transaction.journal.Entries) == 0 {
		return nil
	}
	return writeInstallTransaction(transaction.journalPath, transaction.journal)
}

// RecoverInstallTransaction is used by the staged bootstrap coordinator after
// a process crash. The outer bootstrap journal is authoritative: false forces
// rollback before its commit point, while true forces roll-forward after the
// launcher is durable. This deliberately ignores the inner target CLI version.
func RecoverInstallTransaction(environment Environment, commit bool) error {
	if environment.ConfigDir == "" {
		environment.ConfigDir = defaultConfigDir(environment.Home)
	}
	if err := os.MkdirAll(environment.ConfigDir, 0o700); err != nil {
		return fmt.Errorf("create ViceMe config directory: %w", err)
	}
	transactionLock := flock.New(filepath.Join(environment.ConfigDir, "install.lock"))
	if err := transactionLock.Lock(); err != nil {
		return fmt.Errorf("acquire install recovery lock: %w", err)
	}
	defer transactionLock.Unlock()
	journalPath := filepath.Join(environment.ConfigDir, installTransactionFilename)
	data, err := os.ReadFile(journalPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read install recovery journal: %w", err)
	}
	var journal installTransaction
	if err := decodeStrictJSON(data, &journal); err != nil || journal.SchemaVersion != 1 {
		return errors.New("install recovery journal is invalid; refusing to reconcile installed Skills")
	}
	if !commit {
		return rollbackInstallTransaction(journalPath, journal)
	}
	journal.Status = "COMMITTED"
	if err := writeInstallTransaction(journalPath, journal); err != nil {
		return err
	}
	return recoverInstallTransaction(journalPath)
}

// RecoverInstallTransactionAuto reconciles a standalone install transaction
// when no outer bootstrap journal exists. A matching COMMITTING generation is
// safe to finish because the current CLI already is that generation; all other
// incomplete generations roll back.
func RecoverInstallTransactionAuto(environment Environment) error {
	if environment.ConfigDir == "" {
		environment.ConfigDir = defaultConfigDir(environment.Home)
	}
	if err := os.MkdirAll(environment.ConfigDir, 0o700); err != nil {
		return fmt.Errorf("create ViceMe config directory: %w", err)
	}
	transactionLock := flock.New(filepath.Join(environment.ConfigDir, "install.lock"))
	if err := transactionLock.Lock(); err != nil {
		return fmt.Errorf("acquire install recovery lock: %w", err)
	}
	defer transactionLock.Unlock()
	return recoverInstallTransaction(filepath.Join(environment.ConfigDir, installTransactionFilename))
}

func (transaction *InstallTransaction) Rollback() error {
	if transaction == nil || transaction.closed {
		return nil
	}
	err := rollbackInstallTransaction(transaction.journalPath, transaction.journal)
	transaction.close()
	return err
}

// Abandon releases only the process lock. The durable journal remains for an
// outer bootstrap coordinator that has already persisted its own commit point.
func (transaction *InstallTransaction) Abandon() {
	if transaction != nil {
		transaction.close()
	}
}

func (transaction *InstallTransaction) close() {
	if transaction.closed {
		return
	}
	transaction.closed = true
	if transaction.lock != nil {
		_ = transaction.lock.Unlock()
	}
}

type installOperation struct {
	Skill       string
	Target      string
	Destination string
	Stage       string
	Backup      string
	Expected    Digests
	ReportIndex int
	HadExisting bool
	Unchanged   bool
}

type installTransaction struct {
	SchemaVersion    int                   `json:"schema_version"`
	Status           string                `json:"status"`
	TargetCLIVersion string                `json:"target_cli_version,omitempty"`
	Entries          []installJournalEntry `json:"entries"`
}

type installJournalEntry struct {
	Destination string `json:"destination"`
	Stage       string `json:"stage"`
	Backup      string `json:"backup"`
	HadExisting bool   `json:"had_existing"`
	Activating  bool   `json:"activating"`
}

func (b *Bundle) stageInstallation(name, destination string) (string, Digests, error) {
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", Digests{}, fmt.Errorf("create Skill parent: %w", err)
	}
	expected, err := b.Digests(name)
	if err != nil {
		return "", Digests{}, err
	}
	stageRoot, err := os.MkdirTemp(parent, ".viceme-stage-")
	if err != nil {
		return "", Digests{}, fmt.Errorf("create Skill staging directory: %w", err)
	}
	stagedSkill := filepath.Join(stageRoot, name)
	if err := os.MkdirAll(stagedSkill, 0o755); err != nil {
		_ = os.RemoveAll(stageRoot)
		return "", Digests{}, err
	}
	if err := copyTree(b.FS, name, stagedSkill); err != nil {
		_ = os.RemoveAll(stageRoot)
		return "", Digests{}, err
	}
	manifest, err := b.installManifest(name)
	if err != nil {
		_ = os.RemoveAll(stageRoot)
		return "", Digests{}, err
	}
	if err := writeInstallManifest(stagedSkill, manifest); err != nil {
		_ = os.RemoveAll(stageRoot)
		return "", Digests{}, err
	}
	stagedBundle := New(os.DirFS(stageRoot))
	if err := stagedBundle.Validate(name); err != nil {
		_ = os.RemoveAll(stageRoot)
		return "", Digests{}, fmt.Errorf("validate staged Skill: %w", err)
	}
	return stagedSkill, expected, nil
}

func writeInstallTransaction(filename string, journal installTransaction) error {
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install transaction: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".install-transaction-*.tmp")
	if err != nil {
		return fmt.Errorf("create install transaction: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, filename); err != nil {
		return fmt.Errorf("activate install transaction: %w", err)
	}
	return nil
}

func recoverInstallTransaction(filename string) error {
	data, err := os.ReadFile(filename)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read install recovery journal: %w", err)
	}
	var journal installTransaction
	if err := decodeStrictJSON(data, &journal); err != nil || journal.SchemaVersion != 1 {
		return errors.New("install recovery journal is invalid; refusing to overwrite installed Skills")
	}
	if journal.Status == "COMMITTING" && journal.TargetCLIVersion == buildinfo.Version {
		journal.Status = "COMMITTED"
		if err := writeInstallTransaction(filename, journal); err != nil {
			return err
		}
	}
	if journal.Status != "COMMITTED" {
		return rollbackInstallTransaction(filename, journal)
	}
	var cleanupErrors []error
	for _, entry := range journal.Entries {
		if entry.Stage != "" {
			if err := os.RemoveAll(filepath.Dir(entry.Stage)); err != nil {
				cleanupErrors = append(cleanupErrors, err)
			}
		}
		if entry.Backup != "" {
			if err := os.RemoveAll(entry.Backup); err != nil {
				cleanupErrors = append(cleanupErrors, err)
			}
		}
	}
	if len(cleanupErrors) > 0 {
		return fmt.Errorf("complete committed Skill installation: %w", errors.Join(cleanupErrors...))
	}
	if err := os.Remove(filename); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("complete install recovery: %w", err)
	}
	return nil
}

func rollbackInstallTransaction(filename string, journal installTransaction) error {
	var rollbackErrors []error
	for index := len(journal.Entries) - 1; index >= 0; index-- {
		entry := journal.Entries[index]
		if entry.Activating {
			if _, err := os.Lstat(entry.Backup); err == nil {
				if removeErr := os.RemoveAll(entry.Destination); removeErr != nil {
					rollbackErrors = append(rollbackErrors, removeErr)
				} else if renameErr := os.Rename(entry.Backup, entry.Destination); renameErr != nil {
					rollbackErrors = append(rollbackErrors, renameErr)
				}
			} else if !entry.HadExisting {
				if removeErr := os.RemoveAll(entry.Destination); removeErr != nil {
					rollbackErrors = append(rollbackErrors, removeErr)
				}
			} else if _, destinationErr := os.Lstat(entry.Destination); destinationErr != nil {
				rollbackErrors = append(rollbackErrors, errors.New("install rollback lost both the previous backup and active destination"))
			}
		}
		if entry.Stage != "" {
			if err := os.RemoveAll(filepath.Dir(entry.Stage)); err != nil {
				rollbackErrors = append(rollbackErrors, err)
			}
		}
	}
	if len(rollbackErrors) == 0 {
		if err := os.Remove(filename); err != nil && !errors.Is(err, fs.ErrNotExist) {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if len(rollbackErrors) > 0 {
		return fmt.Errorf("roll back incomplete Skill installation: %w", errors.Join(rollbackErrors...))
	}
	return nil
}

func cleanupStagedOperations(operations []installOperation) {
	for _, operation := range operations {
		if operation.Stage != "" {
			_ = os.RemoveAll(filepath.Dir(operation.Stage))
		}
	}
}

func failedInstallReport(name, target string, err error) InstallReport {
	return InstallReport{AllSucceeded: false, Results: []InstallResult{{Skill: name, Target: target, Status: "failed", Error: err.Error()}}}
}

func failAllInstallReports(names []string, target string, err error) []InstallReport {
	reports := make([]InstallReport, len(names))
	for index, name := range names {
		reports[index] = failedInstallReport(name, target, err)
	}
	return reports
}

func (b *Bundle) installLegacy(name, target string, environment Environment) InstallReport {
	paths, err := resolveTargets(name, target, environment, true)
	if err != nil {
		return InstallReport{AllSucceeded: false, Results: []InstallResult{{Target: target, Status: "failed", Error: err.Error()}}}
	}
	report := InstallReport{AllSucceeded: true}
	for _, resolved := range paths {
		result := InstallResult{Skill: name, Target: resolved.name, Path: resolved.path, Status: "updated"}
		if b.installationCurrent(name, resolved.path) {
			result.Status = "unchanged"
			report.Results = append(report.Results, result)
			continue
		}
		if err := b.installOne(name, resolved.path); err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			report.AllSucceeded = false
		}
		report.Results = append(report.Results, result)
	}
	return report
}

func (b *Bundle) Doctor(name, target string, environment Environment) DoctorReport {
	paths, err := resolveTargets(name, target, environment, false)
	if err != nil {
		return DoctorReport{Healthy: false, Results: []DoctorResult{{Target: target, Problem: err.Error()}}}
	}
	expected, err := b.Digests(name)
	if err != nil {
		return DoctorReport{Healthy: false, Results: []DoctorResult{{Target: target, Problem: err.Error()}}}
	}
	packageMetadata, err := b.Package(name)
	if err != nil {
		return DoctorReport{Healthy: false, Results: []DoctorResult{{Target: target, Problem: err.Error()}}}
	}
	report := DoctorReport{Healthy: true}
	for _, resolved := range paths {
		result := DoctorResult{
			Target:                 resolved.name,
			Path:                   resolved.path,
			ExpectedDigest:         expected.Full,
			ExpectedEmbeddedDigest: expected.Embedded,
			ManifestPath:           filepath.Join(resolved.path, filepath.FromSlash(installManifestPath)),
		}
		actual, err := digestsInstalled(resolved.path)
		if errors.Is(err, fs.ErrNotExist) {
			result.Problem = "not installed"
			report.Healthy = false
		} else if err != nil {
			result.Problem = err.Error()
			report.Healthy = false
		} else {
			result.Installed = true
			result.ActualDigest = actual.Full
			result.ActualEmbeddedDigest = actual.Embedded
			manifest, manifestErr := readInstallManifest(resolved.path)
			result.Checks = doctorChecks(packageMetadata, expected, actual, manifest, manifestErr)
			result.Healthy, result.Problem = summarizeChecks(result.Checks)
			if !result.Healthy {
				report.Healthy = false
			}
		}
		report.Results = append(report.Results, result)
	}
	return report
}

func doctorChecks(packageMetadata PackageMetadata, expected, actual Digests, manifest installManifest, manifestErr error) DoctorChecks {
	checks := DoctorChecks{
		CLIVersion:            DoctorCheck{Expected: buildinfo.Version},
		SkillVersion:          DoctorCheck{Expected: packageMetadata.SkillVersion},
		FullBundleDigest:      DoctorCheck{Expected: expected.Full, Actual: actual.Full},
		EmbeddedContentDigest: DoctorCheck{Expected: expected.Embedded, Actual: actual.Embedded},
		Compatibility: DoctorCheck{
			Expected: fmt.Sprintf("minimum %s; %s", packageMetadata.MinimumCLIVersion, packageMetadata.CLICompatibility),
			Actual:   buildinfo.CompatibilityVersion(),
		},
	}
	if manifestErr != nil {
		problem := "install manifest is missing or invalid: " + manifestErr.Error()
		checks.CLIVersion.Problem = problem
		checks.SkillVersion.Problem = problem
		checks.FullBundleDigest.Problem = problem
		checks.EmbeddedContentDigest.Problem = problem
		checks.Compatibility.Problem = problem
		return checks
	}
	checks.CLIVersion.Actual = manifest.CLIVersion
	checks.CLIVersion.Healthy = manifest.CLIVersion == buildinfo.Version
	if !checks.CLIVersion.Healthy {
		checks.CLIVersion.Problem = "installed Skill was written by a different CLI version"
	}
	checks.SkillVersion.Actual = manifest.SkillVersion
	checks.SkillVersion.Healthy = manifest.SkillVersion == packageMetadata.SkillVersion
	if !checks.SkillVersion.Healthy {
		checks.SkillVersion.Problem = "installed Skill version differs from the bundled Skill"
	}
	checks.FullBundleDigest.Recorded = manifest.FullBundleDigest
	checks.FullBundleDigest.Healthy = actual.Full == expected.Full && manifest.FullBundleDigest == expected.Full
	if !checks.FullBundleDigest.Healthy {
		checks.FullBundleDigest.Problem = "installed files or recorded full bundle digest differ from this CLI release"
	}
	checks.EmbeddedContentDigest.Recorded = manifest.EmbeddedContentDigest
	checks.EmbeddedContentDigest.Healthy = actual.Embedded == expected.Embedded && manifest.EmbeddedContentDigest == expected.Embedded
	if !checks.EmbeddedContentDigest.Healthy {
		checks.EmbeddedContentDigest.Problem = "agent-readable files or recorded embedded digest differ from this CLI release"
	}
	checks.Compatibility.Recorded = fmt.Sprintf("minimum %s; %s", manifest.MinimumCLIVersion, manifest.CLICompatibility)
	compatibilityVersion := buildinfo.SkillCompatibilityVersion()
	compatible, compatibilityErr := semver.Satisfies(compatibilityVersion, manifest.CLICompatibility)
	minimumComparison, minimumErr := semver.Compare(compatibilityVersion, manifest.MinimumCLIVersion)
	checks.Compatibility.Healthy = manifest.SchemaVersion == 1 &&
		manifest.MinimumCLIVersion == packageMetadata.MinimumCLIVersion &&
		manifest.CLICompatibility == packageMetadata.CLICompatibility &&
		compatibilityErr == nil && minimumErr == nil && compatible && minimumComparison >= 0
	if !checks.Compatibility.Healthy {
		checks.Compatibility.Problem = "current CLI does not satisfy the installed Skill compatibility contract"
	}
	return checks
}

func summarizeChecks(checks DoctorChecks) (bool, string) {
	var problems []string
	for name, check := range map[string]DoctorCheck{
		"cli_version":             checks.CLIVersion,
		"skill_version":           checks.SkillVersion,
		"full_bundle_digest":      checks.FullBundleDigest,
		"embedded_content_digest": checks.EmbeddedContentDigest,
		"compatibility":           checks.Compatibility,
	} {
		if !check.Healthy {
			problems = append(problems, name+": "+check.Problem)
		}
	}
	sort.Strings(problems)
	return len(problems) == 0, strings.Join(problems, "; ")
}

type targetPath struct {
	name string
	path string
}

func resolveTargets(skillName, target string, environment Environment, _ bool) ([]targetPath, error) {
	if target == "" {
		target = "auto"
	}
	codexHome := environment.CodexHome
	if codexHome == "" {
		codexHome = filepath.Join(environment.Home, ".codex")
	}
	claudeHome := environment.ClaudeConfigDir
	if claudeHome == "" {
		claudeHome = filepath.Join(environment.Home, ".claude")
	}
	workBuddyHome := environment.WorkBuddyConfigDir
	if workBuddyHome == "" {
		workBuddyHome = filepath.Join(environment.Home, ".workbuddy")
	}
	known := map[string]targetPath{
		"codex":     {name: "codex", path: filepath.Join(codexHome, "skills", skillName)},
		"claude":    {name: "claude", path: filepath.Join(claudeHome, "skills", skillName)},
		"workbuddy": {name: "workbuddy", path: filepath.Join(workBuddyHome, "skills", skillName)},
		"agents":    {name: "agents", path: filepath.Join(environment.Home, ".agents", "skills", skillName)},
	}
	if target != "auto" {
		resolved, ok := known[target]
		if !ok {
			return nil, fmt.Errorf("unsupported Skill target %q; use auto, codex, claude, workbuddy, or agents", target)
		}
		if target == "agents" {
			return []targetPath{resolved}, nil
		}
		return []targetPath{resolved, known["agents"]}, nil
	}
	result := []targetPath{known["agents"]}
	for _, name := range []string{"codex", "claude", "workbuddy"} {
		resolved := known[name]
		base := filepath.Dir(filepath.Dir(resolved.path))
		if _, err := os.Stat(base); err == nil {
			result = append(result, resolved)
		}
	}
	return result, nil
}

func (b *Bundle) installOne(name, destination string) error {
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create Skill parent: %w", err)
	}
	lockPath := destination + ".viceme-install-lock"
	installLock := flock.New(lockPath)
	locked, err := installLock.TryLock()
	if err != nil {
		return fmt.Errorf("acquire Skill install lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("another Skill install is already updating %s", destination)
	}
	defer installLock.Unlock()
	expected, err := b.Digests(name)
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".viceme-stage-")
	if err != nil {
		return fmt.Errorf("create Skill staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	stagedSkill := filepath.Join(stage, name)
	if err := os.MkdirAll(stagedSkill, 0o755); err != nil {
		return fmt.Errorf("create staged Skill directory: %w", err)
	}
	if err := copyTree(b.FS, name, stagedSkill); err != nil {
		return err
	}
	manifest, err := b.installManifest(name)
	if err != nil {
		return err
	}
	if err := writeInstallManifest(stagedSkill, manifest); err != nil {
		return err
	}
	stagedBundle := New(os.DirFS(stage))
	if err := stagedBundle.Validate(name); err != nil {
		return fmt.Errorf("validate staged Skill: %w", err)
	}
	backup := destination + ".viceme-backup"
	_ = os.RemoveAll(backup)
	hadExisting := false
	if _, err := os.Lstat(destination); err == nil {
		hadExisting = true
		if err := os.Rename(destination, backup); err != nil {
			return fmt.Errorf("stage existing Skill: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect existing Skill: %w", err)
	}
	if err := os.Rename(stagedSkill, destination); err != nil {
		if hadExisting {
			_ = os.Rename(backup, destination)
		}
		return fmt.Errorf("activate staged Skill: %w", err)
	}
	actualDigests, err := digestsInstalled(destination)
	if err != nil || actualDigests != expected {
		_ = os.RemoveAll(destination)
		if hadExisting {
			_ = os.Rename(backup, destination)
		}
		if err != nil {
			return fmt.Errorf("verify installed Skill: %w", err)
		}
		return fmt.Errorf("verify installed Skill: digest mismatch")
	}
	if hadExisting {
		_ = os.RemoveAll(backup)
	}
	return nil
}

func (b *Bundle) installManifest(name string) (installManifest, error) {
	metadata, err := b.Package(name)
	if err != nil {
		return installManifest{}, err
	}
	digests, err := b.Digests(name)
	if err != nil {
		return installManifest{}, err
	}
	return installManifest{
		SchemaVersion:         1,
		CLIVersion:            buildinfo.Version,
		SkillVersion:          metadata.SkillVersion,
		MinimumCLIVersion:     metadata.MinimumCLIVersion,
		CLICompatibility:      metadata.CLICompatibility,
		FullBundleDigest:      digests.Full,
		EmbeddedContentDigest: digests.Embedded,
	}, nil
}

func writeInstallManifest(directory string, manifest installManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Skill install manifest: %w", err)
	}
	manifestFile := filepath.Join(directory, filepath.FromSlash(installManifestPath))
	if err := os.MkdirAll(filepath.Dir(manifestFile), 0o755); err != nil {
		return fmt.Errorf("create Skill manifest directory: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(manifestFile, data, 0o644); err != nil {
		return fmt.Errorf("write Skill install manifest: %w", err)
	}
	return nil
}

func readInstallManifest(directory string) (installManifest, error) {
	data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(installManifestPath)))
	if err != nil {
		return installManifest{}, err
	}
	var manifest installManifest
	if err := decodeStrictJSON(data, &manifest); err != nil {
		return installManifest{}, err
	}
	return manifest, nil
}

func (b *Bundle) installationCurrent(name, directory string) bool {
	expected, err := b.Digests(name)
	if err != nil {
		return false
	}
	actual, err := digestsInstalled(directory)
	if err != nil || actual != expected {
		return false
	}
	want, err := b.installManifest(name)
	if err != nil {
		return false
	}
	installed, err := readInstallManifest(directory)
	return err == nil && installed == want
}

func copyTree(source fs.FS, root, destination string) error {
	return fs.WalkDir(source, root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(name, root), "/")
		if rel == "" {
			return nil
		}
		outPath := filepath.Join(destination, filepath.FromSlash(rel))
		if entry.IsDir() {
			return os.MkdirAll(outPath, 0o755)
		}
		data, err := fs.ReadFile(source, name)
		if err != nil {
			return err
		}
		return os.WriteFile(outPath, data, 0o644)
	})
}

func digestsInstalled(directory string) (Digests, error) {
	if _, err := os.Stat(directory); err != nil {
		return Digests{}, err
	}
	fsys := os.DirFS(directory)
	full, err := digestFS(fsys, ".", func(relative string) bool { return relative != installManifestPath })
	if err != nil {
		return Digests{}, err
	}
	embedded, err := digestFS(fsys, ".", func(relative string) bool {
		return relative == "SKILL.md" || strings.HasPrefix(relative, "references/")
	})
	if err != nil {
		return Digests{}, err
	}
	return Digests{Full: full, Embedded: embedded}, nil
}

func InstalledFiles(directory string) ([]string, error) {
	var result []string
	err := filepath.WalkDir(directory, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(directory, name)
		if err != nil {
			return err
		}
		result = append(result, path.Clean(filepath.ToSlash(rel)))
		return nil
	})
	sort.Strings(result)
	return result, err
}
