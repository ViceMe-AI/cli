package skillcontent

import (
	"crypto/rand"
	"encoding/hex"
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
const installTransactionSchemaVersion = 2
const managedSkillRegistryFilename = "managed-skills.json"
const legacyRetiredSkillMigrationVersion = 1

type Environment struct {
	Home               string
	CodexHome          string
	ClaudeConfigDir    string
	WorkBuddyConfigDir string
	AgentsSkillsDir    string
	ConfigDir          string
}

func DefaultEnvironment() Environment {
	home, _ := os.UserHomeDir()
	return Environment{
		Home:               home,
		CodexHome:          os.Getenv("CODEX_HOME"),
		ClaudeConfigDir:    os.Getenv("CLAUDE_CONFIG_DIR"),
		WorkBuddyConfigDir: os.Getenv("WORKBUDDY_CONFIG_DIR"),
		AgentsSkillsDir:    os.Getenv("VICEME_AGENTS_SKILLS_DIR"),
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

func resolveConfigDirectory(environment Environment) (string, error) {
	directory := environment.ConfigDir
	if directory == "" {
		directory = defaultConfigDir(environment.Home)
	}
	absolute, err := filepath.Abs(filepath.Clean(directory))
	if err != nil {
		return "", fmt.Errorf("normalize ViceMe config directory: %w", err)
	}
	resolved, err := resolveExistingPathPrefix(absolute)
	if err != nil {
		return "", fmt.Errorf("normalize ViceMe config directory: %w", err)
	}
	return resolved, nil
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
	ManagedOwnership      DoctorCheck `json:"managed_ownership"`
}

type installManifest struct {
	SchemaVersion         int    `json:"schema_version"`
	InstallID             string `json:"install_id,omitempty"`
	CLIVersion            string `json:"cli_version"`
	SkillVersion          string `json:"skill_version"`
	MinimumCLIVersion     string `json:"minimum_cli_version"`
	CLICompatibility      string `json:"cli_compatibility"`
	FullBundleDigest      string `json:"full_skill_bundle_digest"`
	EmbeddedContentDigest string `json:"embedded_content_digest"`
	ProductID             string `json:"product_id,omitempty"`
	ReleaseID             string `json:"release_id,omitempty"`
}

// SkillProvenance binds a marketplace Skill installation to the Product and
// Release it was downloaded from. It is persisted in the install manifest so a
// later install can tell a same-Product upgrade apart from an attempt to
// overwrite a foreign, official, or user-owned Skill directory.
type SkillProvenance struct {
	ProductID string
	ReleaseID string
}

func validateSkillProvenance(provenance SkillProvenance) error {
	if strings.TrimSpace(provenance.ProductID) == "" {
		return errors.New("marketplace Skill provenance requires a Product ID")
	}
	if strings.TrimSpace(provenance.ReleaseID) == "" {
		return errors.New("marketplace Skill provenance requires a Release ID")
	}
	return nil
}

type LegacyRetiredSkillIdentity struct {
	Name                  string
	SkillVersion          string
	MinimumCLIVersion     string
	CLICompatibility      string
	FullBundleDigest      string
	EmbeddedContentDigest string
	Provenance            string
}

type RetiredSkill struct {
	Name             string
	LegacyMigrations []LegacyRetiredSkillIdentity
}

type managedSkillRecord struct {
	SkillName string `json:"skill_name"`
	InstallID string `json:"install_id"`
	Target    string `json:"target"`
	Digest    string `json:"digest"`
}

type managedSkillRegistry struct {
	SchemaVersion          int                           `json:"schema_version"`
	LegacyMigrationVersion int                           `json:"legacy_migration_version,omitempty"`
	Installs               map[string]managedSkillRecord `json:"installs"`
}

func (b *Bundle) Install(name, target string, environment Environment) InstallReport {
	reports := b.InstallSet([]string{name}, target, environment)
	if len(reports) == 0 {
		return InstallReport{AllSucceeded: false, Results: []InstallResult{{Skill: name, Target: target, Status: "failed", Error: "installation transaction returned no report"}}}
	}
	return reports[0]
}

// InstallWithProvenance installs one marketplace Skill while refusing to
// replace any existing directory that is not already managed as the same
// Product: foreign, official, or user-owned directories fail closed, and only
// a same-Product upgrade or reinstall may replace managed content.
func (b *Bundle) InstallWithProvenance(name, target string, environment Environment, provenance SkillProvenance) InstallReport {
	transaction, reports, err := b.PrepareInstallSetWithProvenance([]string{name}, target, environment, provenance)
	if err != nil {
		return failAllInstallReports([]string{name}, target, err)[0]
	}
	if err := transaction.Commit(); err != nil {
		return failAllInstallReports([]string{name}, target, err)[0]
	}
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
	retirements []InstallResult
	closed      bool
}

func (transaction *InstallTransaction) RetirementResults() []InstallResult {
	if transaction == nil {
		return nil
	}
	return append([]InstallResult(nil), transaction.retirements...)
}

func (b *Bundle) PrepareInstallSet(names []string, target string, environment Environment) (*InstallTransaction, []InstallReport, error) {
	return b.PrepareInstallSetWithRetirements(names, nil, target, environment)
}

func (b *Bundle) PrepareInstallSetWithRetirements(names []string, retired []RetiredSkill, target string, environment Environment) (*InstallTransaction, []InstallReport, error) {
	return b.prepareInstallSet(names, retired, nil, target, environment)
}

// PrepareInstallSetWithProvenance prepares a marketplace Skill installation
// whose destinations are provenance-guarded: only a fresh destination or an
// existing same-Product installation may be replaced.
func (b *Bundle) PrepareInstallSetWithProvenance(names []string, target string, environment Environment, provenance SkillProvenance) (*InstallTransaction, []InstallReport, error) {
	if err := validateSkillProvenance(provenance); err != nil {
		return nil, nil, err
	}
	return b.prepareInstallSet(names, nil, &provenance, target, environment)
}

func (b *Bundle) prepareInstallSet(names []string, retired []RetiredSkill, provenance *SkillProvenance, target string, environment Environment) (*InstallTransaction, []InstallReport, error) {
	reports := make([]InstallReport, len(names))
	for index, name := range names {
		reports[index] = InstallReport{AllSucceeded: true}
		if err := b.Validate(name); err != nil {
			return nil, nil, err
		}
	}
	for _, skill := range retired {
		if err := validateRetiredSkill(skill); err != nil {
			return nil, nil, err
		}
	}
	configDirectory, err := resolveConfigDirectory(environment)
	if err != nil {
		return nil, nil, err
	}
	environment.ConfigDir = configDirectory
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
	registryPath := filepath.Join(environment.ConfigDir, managedSkillRegistryFilename)
	registry, err := readManagedSkillRegistry(registryPath)
	if err != nil {
		_ = transactionLock.Unlock()
		return nil, nil, err
	}
	transaction := &InstallTransaction{
		journalPath: journalPath,
		journal:     installTransaction{SchemaVersion: installTransactionSchemaVersion, Status: "PREPARING", TargetCLIVersion: buildinfo.Version},
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
	resolvedDestinations := make(map[string]string)
	registryDirty := false
	for index, name := range names {
		paths, resolveErr := resolveTargets(name, target, environment)
		if resolveErr != nil {
			return fail(resolveErr)
		}
		for _, resolved := range paths {
			managedPath := resolved.path
			if existing, duplicate := resolvedDestinations[managedPath]; duplicate {
				return fail(fmt.Errorf("Skill targets %s and %s resolve to the same managed path", existing, resolved.name))
			}
			resolvedDestinations[managedPath] = resolved.name
			operation := installOperation{
				Skill: name, Target: resolved.name, Destination: managedPath,
				ManagedPath: managedPath, ReportIndex: index,
			}
			var record *managedSkillRecord
			current, registered := registry.Installs[managedPath]
			if provenance == nil {
				if registered {
					if current.SkillName != name || current.Target != resolved.name {
						return fail(fmt.Errorf("managed Skill registry path %s belongs to %s for target %s", managedPath, current.SkillName, current.Target))
					}
					record = &current
					operation.InstallID = current.InstallID
				} else {
					operation.InstallID, err = newInstallID()
					if err != nil {
						return fail(err)
					}
				}
			} else if registered {
				return fail(fmt.Errorf("refuse to install marketplace Skill at %s: path is reserved by an official or legacy managed Skill %s for target %s", managedPath, current.SkillName, current.Target))
			}
			if b.installationCurrent(name, managedPath, resolved.name, record, provenance) {
				operation.Unchanged = true
			}
			if !operation.Unchanged {
				if provenance != nil {
					if guardErr := guardMarketplaceSkillDestination(managedPath, *provenance); guardErr != nil {
						return fail(guardErr)
					}
				} else if record == nil {
					if guardErr := guardOfficialSkillDestination(managedPath); guardErr != nil {
						return fail(guardErr)
					}
				} else if guardErr := guardRegisteredOfficialSkillDestination(managedPath, *record); guardErr != nil {
					return fail(guardErr)
				}
			}
			operations = append(operations, operation)
		}
	}
	legacyMigrationPending := provenance == nil && len(retired) > 0 && registry.LegacyMigrationVersion < legacyRetiredSkillMigrationVersion
	for _, skill := range retired {
		paths, resolveErr := resolveRetirementTargets(skill.Name, target, environment, registry, legacyMigrationPending)
		if resolveErr != nil {
			return fail(resolveErr)
		}
		for _, resolved := range paths {
			managedPath := resolved.path
			if activeTarget, active := resolvedDestinations[managedPath]; active {
				return fail(fmt.Errorf("active target %s and retired Skill %s resolve to the same managed path", activeTarget, skill.Name))
			}
			if !resolved.trusted {
				transaction.retirements = append(transaction.retirements, InstallResult{
					Skill: skill.Name, Target: resolved.name, Path: managedPath, Status: "preserved_unmanaged",
				})
				continue
			}
			status, ownership := inspectRetiredSkill(managedPath, managedPath, resolved.name, skill, registry, legacyMigrationPending)
			if status == "absent" {
				if record, exists := registry.Installs[managedPath]; exists && record.SkillName == skill.Name && record.Target == resolved.name {
					delete(registry.Installs, managedPath)
					registryDirty = true
				}
				continue
			}
			resultIndex := len(transaction.retirements)
			transaction.retirements = append(transaction.retirements, InstallResult{
				Skill: skill.Name, Target: resolved.name, Path: managedPath, Status: status,
			})
			if status != "retired" {
				continue
			}
			backup := managedPath + ".viceme-transaction-backup"
			if !pathDoesNotExist(backup) {
				transaction.retirements[resultIndex].Status = "preserved_unmanaged"
				continue
			}
			skillCopy := skill
			operations = append(operations, installOperation{
				Skill: skill.Name, Target: resolved.name, Destination: managedPath,
				ManagedPath: managedPath, ReportIndex: -1, HadExisting: true, Backup: backup,
				Retired: &skillCopy, Retirement: ownership, RetirementResultIndex: resultIndex,
			})
		}
	}
	if legacyMigrationPending {
		registry.LegacyMigrationVersion = legacyRetiredSkillMigrationVersion
		registryDirty = true
	}

	for index := range operations {
		operation := &operations[index]
		if operation.Unchanged {
			continue
		}
		if operation.Retired == nil {
			staged, expected, stageErr := b.stageInstallation(operation.Skill, operation.Destination, provenance, operation.InstallID)
			if stageErr != nil {
				cleanupStagedOperations(operations)
				return fail(stageErr)
			}
			operation.Stage = staged
			operation.Expected = expected
		}
		if operation.Backup == "" {
			operation.Backup = operation.Destination + ".viceme-transaction-backup"
		}
		if operation.Retired == nil && !pathDoesNotExist(operation.Backup) {
			cleanupStagedOperations(operations)
			return fail(errors.New("refuse to install while an unowned transaction backup exists"))
		}
		if operation.Retired != nil {
			operation.HadExisting = true
		} else if _, statErr := os.Lstat(operation.Destination); statErr == nil {
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
		journalIndex++
		if operation.Retired == nil && !pathDoesNotExist(operation.Backup) {
			return fail(errors.New("refuse to install while an unowned transaction backup exists"))
		}
		if operation.Retired != nil {
			status := verifyRetiredSkillOwnership(operation.Destination, operation.Target, *operation.Retired, operation.Retirement)
			if status != "retired" || !pathDoesNotExist(operation.Backup) {
				if status == "retired" {
					status = "preserved_unmanaged"
				}
				transaction.retirements[operation.RetirementResultIndex].Status = status
				entry.Backup = ""
				entry.HadExisting = false
				if err := writeInstallTransaction(journalPath, transaction.journal); err != nil {
					return fail(err)
				}
				continue
			}
		}
		entry.Activating = true
		if err := writeInstallTransaction(journalPath, transaction.journal); err != nil {
			return fail(err)
		}
		if operation.HadExisting {
			if err := os.Rename(operation.Destination, operation.Backup); err != nil {
				return fail(fmt.Errorf("stage existing Skill: %w", err))
			}
		}
		if operation.Retired != nil {
			if _, err := os.Lstat(operation.Destination); !errors.Is(err, fs.ErrNotExist) {
				return fail(errors.New("verify retired Skill removal: destination still exists"))
			}
			status := verifyRetiredSkillOwnership(operation.Backup, operation.Target, *operation.Retired, operation.Retirement)
			if status != "retired" {
				if err := os.Rename(operation.Backup, operation.Destination); err != nil {
					return fail(fmt.Errorf("restore preserved retired Skill: %w", err))
				}
				transaction.retirements[operation.RetirementResultIndex].Status = status
				entry.Activating = false
				entry.Backup = ""
				entry.HadExisting = false
				if err := writeInstallTransaction(journalPath, transaction.journal); err != nil {
					return fail(err)
				}
				continue
			}
			if operation.Retirement.Registry != nil {
				delete(registry.Installs, operation.ManagedPath)
				registryDirty = true
			}
			continue
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
		if provenance == nil {
			record := managedSkillRecord{
				SkillName: operation.Skill, InstallID: operation.InstallID,
				Target: operation.Target, Digest: operation.Expected.Full,
			}
			if previous, exists := registry.Installs[operation.ManagedPath]; !exists || previous != record {
				registry.Installs[operation.ManagedPath] = record
				registryDirty = true
			}
		}
	}
	if provenance == nil && registryDirty {
		if err := transaction.TrackPath(registryPath); err != nil {
			return fail(err)
		}
		if err := writeManagedSkillRegistry(registryPath, registry); err != nil {
			return fail(err)
		}
	}
	for _, operation := range operations {
		if operation.Retired != nil {
			continue
		}
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
	absDestination, err := normalizeTransactionPath(destination)
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
	if !pathDoesNotExist(backup) {
		return errors.New("refuse to track a path while an unowned transaction backup exists")
	}
	entry := installJournalEntry{Destination: absDestination, Backup: backup, HadExisting: hadExisting, Activating: true}
	transaction.journal.Entries = append(transaction.journal.Entries, entry)
	if err := writeInstallTransaction(transaction.journalPath, transaction.journal); err != nil {
		transaction.journal.Entries = transaction.journal.Entries[:len(transaction.journal.Entries)-1]
		return err
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
	configDirectory, err := resolveConfigDirectory(environment)
	if err != nil {
		return err
	}
	environment.ConfigDir = configDirectory
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
	if err := decodeStrictJSON(data, &journal); err != nil || validateInstallTransaction(journal) != nil {
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
	configDirectory, err := resolveConfigDirectory(environment)
	if err != nil {
		return err
	}
	environment.ConfigDir = configDirectory
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
	Skill                 string
	Target                string
	Destination           string
	ManagedPath           string
	InstallID             string
	Stage                 string
	Backup                string
	Expected              Digests
	ReportIndex           int
	RetirementResultIndex int
	HadExisting           bool
	Unchanged             bool
	Retired               *RetiredSkill
	Retirement            retiredSkillOwnership
}

type retiredSkillOwnership struct {
	Registry *managedSkillRecord
	Legacy   *LegacyRetiredSkillIdentity
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

func (b *Bundle) stageInstallation(name, destination string, provenance *SkillProvenance, installID string) (string, Digests, error) {
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
	if provenance != nil {
		manifest.ProductID = provenance.ProductID
		manifest.ReleaseID = provenance.ReleaseID
	} else {
		if !validInstallID(installID) {
			_ = os.RemoveAll(stageRoot)
			return "", Digests{}, errors.New("managed Skill installation requires a valid install ID")
		}
		manifest.SchemaVersion = 2
		manifest.InstallID = installID
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
	if err := validateInstallTransaction(journal); err != nil {
		return err
	}
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
	if err := decodeStrictJSON(data, &journal); err != nil || validateInstallTransaction(journal) != nil {
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

func validateInstallTransaction(journal installTransaction) error {
	if journal.SchemaVersion != 1 && journal.SchemaVersion != installTransactionSchemaVersion {
		return errors.New("install transaction has an unsupported schema version")
	}
	switch journal.Status {
	case "PREPARING", "COMMITTING", "COMMITTED":
	default:
		return errors.New("install transaction has an invalid status")
	}
	seen := make(map[string]bool, len(journal.Entries))
	for _, entry := range journal.Entries {
		if journal.SchemaVersion >= installTransactionSchemaVersion && (!validAbsoluteTransactionPath(entry.Destination) ||
			(entry.Stage != "" && !validAbsoluteTransactionPath(entry.Stage)) ||
			(entry.Backup != "" && !validAbsoluteTransactionPath(entry.Backup))) {
			return errors.New("install transaction contains an invalid path")
		}
		if seen[entry.Destination] {
			return errors.New("install transaction contains duplicate destinations")
		}
		seen[entry.Destination] = true
	}
	return nil
}

func validAbsoluteTransactionPath(filename string) bool {
	return filepath.IsAbs(filename) && filepath.Clean(filename) == filename
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

func (b *Bundle) Doctor(name, target string, environment Environment) DoctorReport {
	paths, err := resolveTargets(name, target, environment)
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
	configDirectory, err := resolveConfigDirectory(environment)
	if err != nil {
		return DoctorReport{Healthy: false, Results: []DoctorResult{{Target: target, Problem: err.Error()}}}
	}
	environment.ConfigDir = configDirectory
	registry, registryErr := readManagedSkillRegistry(filepath.Join(environment.ConfigDir, managedSkillRegistryFilename))
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
			var record *managedSkillRecord
			managedPath := resolved.path
			ownershipErr := registryErr
			if ownershipErr == nil {
				if current, exists := registry.Installs[managedPath]; exists {
					record = &current
				} else {
					ownershipErr = errors.New("managed Skill registry entry is missing")
				}
			}
			result.Checks = doctorChecks(packageMetadata, expected, actual, manifest, manifestErr, name, resolved.name, record, ownershipErr)
			result.Healthy, result.Problem = summarizeChecks(result.Checks)
			if !result.Healthy {
				report.Healthy = false
			}
		}
		report.Results = append(report.Results, result)
	}
	return report
}

func doctorChecks(packageMetadata PackageMetadata, expected, actual Digests, manifest installManifest, manifestErr error, name, target string, record *managedSkillRecord, ownershipErr error) DoctorChecks {
	checks := DoctorChecks{
		CLIVersion:            DoctorCheck{Expected: buildinfo.Version},
		SkillVersion:          DoctorCheck{Expected: packageMetadata.SkillVersion},
		FullBundleDigest:      DoctorCheck{Expected: expected.Full, Actual: actual.Full},
		EmbeddedContentDigest: DoctorCheck{Expected: expected.Embedded, Actual: actual.Embedded},
		Compatibility: DoctorCheck{
			Expected: fmt.Sprintf("minimum %s; %s", packageMetadata.MinimumCLIVersion, packageMetadata.CLICompatibility),
			Actual:   buildinfo.CompatibilityVersion(),
		},
		ManagedOwnership: DoctorCheck{Expected: "matching managed registry and manifest v2 install ID"},
	}
	if manifestErr != nil {
		problem := "install manifest is missing or invalid: " + manifestErr.Error()
		checks.CLIVersion.Problem = problem
		checks.SkillVersion.Problem = problem
		checks.FullBundleDigest.Problem = problem
		checks.EmbeddedContentDigest.Problem = problem
		checks.Compatibility.Problem = problem
		checks.ManagedOwnership.Problem = problem
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
	compatible, compatibilityErr := semver.Satisfies(buildinfo.CompatibilityVersion(), manifest.CLICompatibility)
	minimumComparison, minimumErr := semver.Compare(buildinfo.CompatibilityVersion(), manifest.MinimumCLIVersion)
	checks.Compatibility.Healthy = manifest.SchemaVersion == 2 &&
		manifest.MinimumCLIVersion == packageMetadata.MinimumCLIVersion &&
		manifest.CLICompatibility == packageMetadata.CLICompatibility &&
		compatibilityErr == nil && minimumErr == nil && compatible && minimumComparison >= 0
	if !checks.Compatibility.Healthy {
		checks.Compatibility.Problem = "current CLI does not satisfy the installed Skill compatibility contract"
	}
	checks.ManagedOwnership.Actual = manifest.InstallID
	if record != nil {
		checks.ManagedOwnership.Recorded = record.InstallID
	}
	checks.ManagedOwnership.Healthy = ownershipErr == nil && record != nil &&
		record.SkillName == name && record.Target == target && record.Digest == expected.Full &&
		manifest.InstallID == record.InstallID && validInstallID(record.InstallID) &&
		manifest.ProductID == "" && manifest.ReleaseID == "" && actual.Full == record.Digest
	if !checks.ManagedOwnership.Healthy {
		if ownershipErr != nil {
			checks.ManagedOwnership.Problem = ownershipErr.Error()
		} else {
			checks.ManagedOwnership.Problem = "registry, manifest, target, or installed digest does not identify the same managed Skill"
		}
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
		"managed_ownership":       checks.ManagedOwnership,
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

type retirementTarget struct {
	targetPath
	trusted bool
}

func resolveTargets(skillName, target string, environment Environment) ([]targetPath, error) {
	if target == "" {
		target = "auto"
	}
	known, err := resolveKnownTargets(skillName, environment)
	if err != nil {
		return nil, err
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

func resolveKnownTargets(skillName string, environment Environment) (map[string]targetPath, error) {
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
	agentsSkillsDir := environment.AgentsSkillsDir
	if agentsSkillsDir == "" {
		agentsSkillsDir = filepath.Join(environment.Home, ".agents", "skills")
	}
	raw := map[string]targetPath{
		"codex":     {name: "codex", path: filepath.Join(codexHome, "skills", skillName)},
		"claude":    {name: "claude", path: filepath.Join(claudeHome, "skills", skillName)},
		"workbuddy": {name: "workbuddy", path: filepath.Join(workBuddyHome, "skills", skillName)},
		"agents":    {name: "agents", path: filepath.Join(agentsSkillsDir, skillName)},
	}
	known := make(map[string]targetPath, len(raw))
	for name, resolved := range raw {
		normalized, err := normalizeManagedSkillPath(resolved.path)
		if err != nil {
			return nil, err
		}
		resolved.path = normalized
		known[name] = resolved
	}
	return known, nil
}

func resolveRetirementTargets(skillName, target string, environment Environment, registry managedSkillRegistry, includeLegacy bool) ([]retirementTarget, error) {
	selected, err := resolveTargets(skillName, target, environment)
	if err != nil {
		return nil, err
	}
	known, err := resolveKnownTargets(skillName, environment)
	if err != nil {
		return nil, err
	}
	candidates := make(map[string]retirementTarget)
	for _, resolved := range selected {
		candidates[resolved.path] = retirementTarget{targetPath: resolved, trusted: true}
	}
	if includeLegacy {
		for _, name := range []string{"agents", "codex", "claude", "workbuddy"} {
			resolved := known[name]
			if _, exists := candidates[resolved.path]; !exists {
				candidates[resolved.path] = retirementTarget{targetPath: resolved, trusted: true}
			}
		}
	}
	for managedPath, record := range registry.Installs {
		if record.SkillName != skillName {
			continue
		}
		expected, knownTarget := known[record.Target]
		trusted := knownTarget && expected.path == managedPath
		candidates[managedPath] = retirementTarget{
			targetPath: targetPath{name: record.Target, path: managedPath},
			trusted:    trusted,
		}
	}
	result := make([]retirementTarget, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].path == result[right].path {
			return result[left].name < result[right].name
		}
		return result[left].path < result[right].path
	})
	return result, nil
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

func (b *Bundle) installationCurrent(name, directory, target string, record *managedSkillRecord, provenance *SkillProvenance) bool {
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
	if provenance != nil {
		want.ProductID = provenance.ProductID
		want.ReleaseID = provenance.ReleaseID
	} else {
		if record == nil || record.SkillName != name || record.Target != target || record.Digest != expected.Full || !validInstallID(record.InstallID) {
			return false
		}
		want.SchemaVersion = 2
		want.InstallID = record.InstallID
	}
	installed, err := readInstallManifest(directory)
	return err == nil && installed == want
}

// ValidateLegacyRetiredSkillIdentity reports whether a one-time v1 migration
// identity pins a complete published bundle and its reproducible source.
func ValidateLegacyRetiredSkillIdentity(identity LegacyRetiredSkillIdentity) error {
	return validateLegacyRetiredSkillIdentity(identity)
}

func validateRetiredSkill(skill RetiredSkill) error {
	if err := validName(skill.Name); err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, identity := range skill.LegacyMigrations {
		if identity.Name != skill.Name {
			return fmt.Errorf("legacy migration for %s names a different retired Skill %s", skill.Name, identity.Name)
		}
		if err := validateLegacyRetiredSkillIdentity(identity); err != nil {
			return err
		}
		key := identity.SkillVersion + "\x00" + identity.FullBundleDigest
		if seen[key] {
			return fmt.Errorf("retired Skill %s has a duplicate legacy migration identity", skill.Name)
		}
		seen[key] = true
	}
	return nil
}

func validateLegacyRetiredSkillIdentity(identity LegacyRetiredSkillIdentity) error {
	if err := validName(identity.Name); err != nil {
		return err
	}
	if _, err := semver.Parse(identity.SkillVersion); err != nil {
		return fmt.Errorf("retired Skill %s has an invalid Skill version", identity.Name)
	}
	if _, err := semver.Parse(identity.MinimumCLIVersion); err != nil {
		return fmt.Errorf("retired Skill %s has an invalid minimum CLI version", identity.Name)
	}
	compatible, err := semver.Satisfies(identity.MinimumCLIVersion, identity.CLICompatibility)
	if err != nil || !compatible {
		return fmt.Errorf("retired Skill %s has an invalid CLI compatibility range", identity.Name)
	}
	for _, digest := range []string{identity.FullBundleDigest, identity.EmbeddedContentDigest} {
		if !validSHA256Digest(digest) {
			return fmt.Errorf("retired Skill %s has an invalid digest", identity.Name)
		}
	}
	provenance := strings.TrimSpace(identity.Provenance)
	if provenance != identity.Provenance || strings.ContainsAny(provenance, "\r\n") {
		return fmt.Errorf("retired Skill %s has invalid legacy provenance", identity.Name)
	}
	if strings.HasPrefix(provenance, "tag:") {
		if strings.TrimPrefix(provenance, "tag:") == "" {
			return fmt.Errorf("retired Skill %s has invalid legacy provenance", identity.Name)
		}
	} else if strings.HasPrefix(provenance, "commit:") {
		commit := strings.TrimPrefix(provenance, "commit:")
		if len(commit) != 40 {
			return fmt.Errorf("retired Skill %s has invalid legacy provenance", identity.Name)
		}
		if _, err := hex.DecodeString(commit); err != nil {
			return fmt.Errorf("retired Skill %s has invalid legacy provenance", identity.Name)
		}
	} else {
		return fmt.Errorf("retired Skill %s has invalid legacy provenance", identity.Name)
	}
	return nil
}

func guardOfficialSkillDestination(directory string) error {
	info, err := os.Lstat(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect existing Skill: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse to overwrite %s: existing path is not a managed official Skill directory", directory)
	}
	manifest, err := readInstallManifest(directory)
	if err != nil {
		return fmt.Errorf("refuse to overwrite %s: existing Skill has no managed ownership record", directory)
	}
	if manifest.ProductID != "" || manifest.ReleaseID != "" {
		return fmt.Errorf("refuse to overwrite %s: existing Skill is a marketplace installation", directory)
	}
	if manifest.SchemaVersion != 1 || manifest.InstallID != "" {
		return fmt.Errorf("refuse to overwrite %s: existing official manifest is not backed by the managed Skill registry", directory)
	}
	return nil
}

func guardRegisteredOfficialSkillDestination(directory string, record managedSkillRecord) error {
	info, err := os.Lstat(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect registered official Skill: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse to overwrite %s: registered official path no longer contains its managed directory", directory)
	}
	manifest, err := readInstallManifest(directory)
	if err != nil {
		return fmt.Errorf("refuse to overwrite %s: registered official path has no matching manifest v2", directory)
	}
	if manifest.ProductID != "" || manifest.ReleaseID != "" {
		return fmt.Errorf("refuse to overwrite %s: registered official path now contains a marketplace installation", directory)
	}
	if manifest.SchemaVersion != 2 || manifest.InstallID != record.InstallID {
		return fmt.Errorf("refuse to overwrite %s: registered official path has a different install ID", directory)
	}
	return nil
}

// guardMarketplaceSkillDestination fails closed when an existing destination
// cannot be proven to belong to the same Product: foreign or user-owned
// directories, directories managed as official or legacy Skills (no Product
// provenance), and directories owned by another Product are all refused.
func guardMarketplaceSkillDestination(directory string, provenance SkillProvenance) error {
	info, err := os.Lstat(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect existing Skill: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse to overwrite %s: existing path is not a ViceMe-managed Skill directory", directory)
	}
	manifest, err := readInstallManifest(directory)
	if err != nil {
		return fmt.Errorf("refuse to overwrite %s: existing Skill is not managed by a ViceMe installation", directory)
	}
	if manifest.ProductID == "" || manifest.ReleaseID == "" {
		return fmt.Errorf("refuse to overwrite %s: existing Skill is an official or legacy ViceMe Skill, not a marketplace installation", directory)
	}
	if manifest.ProductID != provenance.ProductID {
		return fmt.Errorf("refuse to overwrite %s: existing Skill belongs to a different Product (%s)", directory, manifest.ProductID)
	}
	return nil
}

func inspectRetiredSkill(directory, managedPath, target string, skill RetiredSkill, registry managedSkillRegistry, allowLegacy bool) (string, retiredSkillOwnership) {
	info, err := os.Lstat(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return "absent", retiredSkillOwnership{}
	}
	record, registered := registry.Installs[managedPath]
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		if registered && record.SkillName == skill.Name && record.Target == target {
			return "preserved_modified", retiredSkillOwnership{Registry: &record}
		}
		return "preserved_unmanaged", retiredSkillOwnership{}
	}
	if registered {
		if record.SkillName != skill.Name || record.Target != target {
			return "preserved_unmanaged", retiredSkillOwnership{}
		}
		ownership := retiredSkillOwnership{Registry: &record}
		return verifyRetiredSkillOwnership(directory, target, skill, ownership), ownership
	}
	if !allowLegacy {
		return "preserved_unmanaged", retiredSkillOwnership{}
	}
	manifest, err := readInstallManifest(directory)
	if err != nil {
		return "preserved_unmanaged", retiredSkillOwnership{}
	}
	for _, identity := range skill.LegacyMigrations {
		if !legacyManifestMatches(manifest, identity) {
			continue
		}
		identityCopy := identity
		ownership := retiredSkillOwnership{Legacy: &identityCopy}
		return verifyRetiredSkillOwnership(directory, target, skill, ownership), ownership
	}
	return "preserved_unmanaged", retiredSkillOwnership{}
}

func verifyRetiredSkillOwnership(directory, target string, skill RetiredSkill, ownership retiredSkillOwnership) string {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		if ownership.Registry != nil || ownership.Legacy != nil {
			return "preserved_modified"
		}
		return "preserved_unmanaged"
	}
	manifest, err := readInstallManifest(directory)
	if err != nil {
		return "preserved_modified"
	}
	actual, err := digestsInstalled(directory)
	if err != nil {
		return "preserved_modified"
	}
	if ownership.Registry != nil {
		record := *ownership.Registry
		if record.SkillName != skill.Name || record.Target != target || !validInstallID(record.InstallID) ||
			manifest.SchemaVersion != 2 || manifest.InstallID != record.InstallID ||
			manifest.ProductID != "" || manifest.ReleaseID != "" ||
			manifest.FullBundleDigest != record.Digest || actual.Full != record.Digest ||
			actual.Embedded != manifest.EmbeddedContentDigest {
			return "preserved_modified"
		}
		return "retired"
	}
	if ownership.Legacy != nil {
		identity := *ownership.Legacy
		if !legacyManifestMatches(manifest, identity) ||
			actual.Full != identity.FullBundleDigest || actual.Embedded != identity.EmbeddedContentDigest {
			return "preserved_modified"
		}
		return "retired"
	}
	return "preserved_unmanaged"
}

func legacyManifestMatches(manifest installManifest, identity LegacyRetiredSkillIdentity) bool {
	return manifest.SchemaVersion == 1 && manifest.InstallID == "" &&
		manifest.ProductID == "" && manifest.ReleaseID == "" &&
		strings.TrimSpace(manifest.CLIVersion) != "" &&
		manifest.SkillVersion == identity.SkillVersion &&
		manifest.MinimumCLIVersion == identity.MinimumCLIVersion &&
		manifest.CLICompatibility == identity.CLICompatibility &&
		manifest.FullBundleDigest == identity.FullBundleDigest &&
		manifest.EmbeddedContentDigest == identity.EmbeddedContentDigest
}

func normalizeManagedSkillPath(directory string) (string, error) {
	normalized, err := normalizeTransactionPath(directory)
	if err != nil {
		return "", fmt.Errorf("normalize managed Skill path: %w", err)
	}
	if runtime.GOOS == "windows" {
		normalized = strings.ToLower(normalized)
	}
	return normalized, nil
}

func normalizeTransactionPath(filename string) (string, error) {
	normalized, err := filepath.Abs(filepath.Clean(filename))
	if err != nil {
		return "", err
	}
	parent, err := resolveExistingPathPrefix(filepath.Dir(normalized))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(normalized)), nil
}

func resolveExistingPathPrefix(filename string) (string, error) {
	current := filepath.Clean(filename)
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func newInstallID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate managed Skill install ID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

func validInstallID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	decoded, err := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return err == nil && len(decoded) == 16
}

func validSHA256Digest(value string) bool {
	encoded := strings.TrimPrefix(value, "sha256:")
	if len(encoded) != 64 || len(encoded) == len(value) {
		return false
	}
	decoded, err := hex.DecodeString(encoded)
	return err == nil && len(decoded) == 32
}

func validManagedTarget(target string) bool {
	switch target {
	case "agents", "codex", "claude", "workbuddy":
		return true
	default:
		return false
	}
}

func readManagedSkillRegistry(filename string) (managedSkillRegistry, error) {
	registry := managedSkillRegistry{SchemaVersion: 1, Installs: make(map[string]managedSkillRecord)}
	data, err := os.ReadFile(filename)
	if errors.Is(err, fs.ErrNotExist) {
		return registry, nil
	}
	if err != nil {
		return managedSkillRegistry{}, fmt.Errorf("read managed Skill registry: %w", err)
	}
	if err := decodeStrictJSON(data, &registry); err != nil || registry.SchemaVersion != 1 ||
		registry.LegacyMigrationVersion < 0 || registry.LegacyMigrationVersion > legacyRetiredSkillMigrationVersion || registry.Installs == nil {
		return managedSkillRegistry{}, errors.New("managed Skill registry is invalid; refusing to reconcile installed Skills")
	}
	seen := make(map[string]string, len(registry.Installs))
	for managedPath, record := range registry.Installs {
		normalized, err := normalizeManagedSkillPath(managedPath)
		if err != nil || normalized != managedPath || validName(record.SkillName) != nil ||
			!validInstallID(record.InstallID) || !validManagedTarget(record.Target) || !validSHA256Digest(record.Digest) {
			return managedSkillRegistry{}, errors.New("managed Skill registry contains an invalid install record")
		}
		identity := normalized
		if runtime.GOOS == "windows" {
			identity = strings.ToLower(identity)
		}
		if previous, duplicate := seen[identity]; duplicate && previous != managedPath {
			return managedSkillRegistry{}, errors.New("managed Skill registry contains duplicate normalized paths")
		}
		seen[identity] = managedPath
	}
	return registry, nil
}

func writeManagedSkillRegistry(filename string, registry managedSkillRegistry) error {
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("encode managed Skill registry: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".managed-skills-*.tmp")
	if err != nil {
		return fmt.Errorf("create managed Skill registry: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
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
	if err := os.Rename(temporaryName, filename); err != nil {
		return fmt.Errorf("activate managed Skill registry: %w", err)
	}
	return nil
}

func pathDoesNotExist(filename string) bool {
	_, err := os.Lstat(filename)
	return errors.Is(err, fs.ErrNotExist)
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
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("copy Skill contains a link or special file: %s", name)
		}
		data, err := fs.ReadFile(source, name)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if info.Mode().Perm()&0o111 != 0 {
			mode = 0o755
		}
		return os.WriteFile(outPath, data, mode)
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
