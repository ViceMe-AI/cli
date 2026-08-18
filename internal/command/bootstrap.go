package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
)

const (
	bootstrapActivationJournalFilename = updatepkg.BootstrapActivationJournalFilename
	bootstrapActivationStagedFilename  = "bootstrap-activation-new"
	bootstrapActivationBackupFilename  = "bootstrap-activation-old"
)

type bootstrapActivationJournal struct {
	SchemaVersion int    `json:"schemaVersion"`
	Status        string `json:"status"`
	Destination   string `json:"destination"`
	Staged        string `json:"staged"`
	Backup        string `json:"backup"`
	HadExisting   bool   `json:"hadExisting"`
	PreviousHash  string `json:"previousHash,omitempty"`
	TargetHash    string `json:"targetHash"`
	TargetVersion string `json:"targetVersion,omitempty"`
}

type bootstrapActivationResult struct {
	Destination string                 `json:"destination"`
	Install     bootstrapInstallResult `json:"install"`
}

func newBootstrapCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "bootstrap", Hidden: true}
	command.AddCommand(newBootstrapActivateCommand(runtime))
	return command
}

func newBootstrapActivateCommand(runtime *Runtime) *cobra.Command {
	var destination string
	var agent string
	var region string
	command := &cobra.Command{
		Use: "activate", Hidden: true, Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := activateBootstrap(command, runtime, destination, agent, region)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&destination, "destination", "", "final ViceMe executable path")
	command.Flags().StringVar(&agent, "agent", "auto", "agent target")
	command.Flags().StringVar(&region, "region", "", "ViceMe region")
	_ = command.MarkFlagRequired("destination")
	return command
}

func activateBootstrap(command *cobra.Command, runtime *Runtime, destination, agent, region string) (bootstrapActivationResult, error) {
	absDestination, err := validBootstrapDestination(destination)
	if err != nil {
		return bootstrapActivationResult{}, output.Validation("BOOTSTRAP_DESTINATION_INVALID", "bootstrap destination is invalid")
	}
	if err := os.MkdirAll(runtime.configBase, 0o700); err != nil {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_CONFIG_DIR_FAILED", "could not create the bootstrap recovery directory", err)
	}
	activationContext := command.Context()
	if activationContext == nil {
		activationContext = context.Background()
	}
	activationLock := flock.New(filepath.Join(runtime.configBase, updatepkg.ActivationLockFilename))
	locked, err := activationLock.TryLockContext(activationContext, 50*time.Millisecond)
	if err != nil {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_LOCK_FAILED", "could not acquire the bootstrap activation lock", err)
	}
	if !locked {
		return bootstrapActivationResult{}, output.Validation("BOOTSTRAP_ACTIVE", "another ViceMe bootstrap or update is active")
	}
	defer activationLock.Unlock()
	outer, err := updatepkg.InspectOuterActivationJournals(runtime.configBase)
	if err != nil {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_JOURNAL_INSPECTION_FAILED", "could not inspect outer activation journals", err)
	}
	if outer.NPM {
		return bootstrapActivationResult{}, output.Policy("BOOTSTRAP_NPM_RECOVERY_REQUIRED", "an interrupted npm activation must be recovered before standalone bootstrap")
	}
	memberLock := flock.New(filepath.Join(runtime.configBase, updatepkg.ActivationMemberLockFilename))
	memberLocked, err := memberLock.TryLockContext(activationContext, 50*time.Millisecond)
	if err != nil {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_LOCK_FAILED", "could not inspect the activation member lock", err)
	}
	if !memberLocked {
		return bootstrapActivationResult{}, output.Validation("BOOTSTRAP_ACTIVE", "an activation child is still committing Skills and config")
	}
	defer memberLock.Unlock()
	if err := recoverBootstrapActivation(runtime.configBase, runtime.deps.Environment); err != nil {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_RECOVERY_FAILED", "could not recover the previous ViceMe activation", err)
	}
	outer, err = updatepkg.InspectOuterActivationJournals(runtime.configBase)
	if err != nil {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_JOURNAL_INSPECTION_FAILED", "could not inspect recovered activation journals", err)
	}
	if outer.Bootstrap || outer.NPM {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_RECOVERY_INCOMPLETE", "outer activation recovery did not retire its journal", nil)
	}

	executable, err := os.Executable()
	if err != nil {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_EXECUTABLE_FAILED", "could not resolve the staged ViceMe executable", err)
	}
	staged := filepath.Join(runtime.configBase, bootstrapActivationStagedFilename)
	backup := filepath.Join(runtime.configBase, bootstrapActivationBackupFilename)
	_ = os.Remove(staged)
	_ = os.Remove(backup)
	if err := copyBootstrapExecutable(executable, staged); err != nil {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_STAGE_FAILED", "could not preserve the staged ViceMe executable", err)
	}
	targetHash, err := bootstrapFileHash(staged)
	if err != nil {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_STAGE_FAILED", "could not verify the staged ViceMe executable", err)
	}
	targetVersion := buildinfo.CompatibilityVersion()
	targetGeneration, err := updatepkg.NewStandaloneGeneration(targetVersion, targetHash)
	if err != nil {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_VERSION_INVALID", "the staged ViceMe generation is invalid", err)
	}
	if err := updatepkg.ValidateActivationTarget(runtime.configBase, targetGeneration); err != nil {
		_ = os.Remove(staged)
		if errors.Is(err, updatepkg.ErrActivationMethodChange) {
			return bootstrapActivationResult{}, output.Policy("BOOTSTRAP_INSTALL_METHOD_CHANGE_REFUSED", "switching between standalone and npm installation is not supported in place")
		}
		return bootstrapActivationResult{}, output.Policy("BOOTSTRAP_DOWNGRADE_REFUSED", "the staged ViceMe release is older than or conflicts with the active generation")
	}
	journal := bootstrapActivationJournal{
		SchemaVersion: 1,
		Status:        "PREPARING",
		Destination:   absDestination,
		Staged:        staged,
		Backup:        backup,
		TargetHash:    targetHash,
		TargetVersion: targetVersion,
	}
	if _, err := os.Lstat(absDestination); err == nil {
		journal.HadExisting = true
		if err := copyBootstrapExecutable(absDestination, backup); err != nil {
			return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_BACKUP_FAILED", "could not preserve the previous ViceMe executable", err)
		}
		journal.PreviousHash, err = bootstrapFileHash(backup)
		if err != nil {
			return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_BACKUP_FAILED", "could not verify the previous ViceMe executable", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_DESTINATION_FAILED", "could not inspect the ViceMe executable destination", err)
	}
	journalPath := filepath.Join(runtime.configBase, bootstrapActivationJournalFilename)
	if err := writeBootstrapJournal(journalPath, journal); err != nil {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_JOURNAL_FAILED", "could not persist the ViceMe activation journal", err)
	}

	install, installErr := performInstall(command.Context(), runtime, agent, region, true, &installCommitAuthority{
		OuterJournalOwnsFailure: true,
		BeforeCommit: func() error {
			journal.Status = "COMMITTING"
			if err := writeBootstrapJournal(journalPath, journal); err != nil {
				return output.Internal("BOOTSTRAP_JOURNAL_FAILED", "could not persist the ViceMe activation commit point", err)
			}
			if err := activateBootstrapExecutable(journal.Staged, journal.Destination, journal.TargetHash); err != nil {
				return output.Internal("BOOTSTRAP_REPLACE_FAILED", "could not activate the ViceMe executable", err)
			}
			return nil
		},
	})
	if installErr != nil {
		recoveryErr := recoverBootstrapActivation(runtime.configBase, runtime.deps.Environment)
		if recoveryErr != nil {
			return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_RECOVERY_FAILED", "ViceMe activation failed and automatic recovery did not complete", errors.Join(installErr, recoveryErr))
		}
		return bootstrapActivationResult{}, installErr
	}
	journal.Status = "COMMITTED"
	if err := writeBootstrapJournal(journalPath, journal); err != nil {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_JOURNAL_FAILED", "ViceMe activated but its recovery journal could not be committed", err)
	}
	if err := recoverBootstrapActivation(runtime.configBase, runtime.deps.Environment); err != nil {
		return bootstrapActivationResult{}, output.Internal("BOOTSTRAP_RECOVERY_FAILED", "ViceMe activated but its recovery files could not be finalized", err)
	}
	return bootstrapActivationResult{Destination: absDestination, Install: install}, nil
}

func recoverBootstrapActivation(configDir string, environment skillcontent.Environment) error {
	journalPath := filepath.Join(configDir, bootstrapActivationJournalFilename)
	data, err := os.ReadFile(journalPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal bootstrapActivationJournal
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&journal); err != nil || journal.SchemaVersion != 1 {
		return errors.New("bootstrap activation journal is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("bootstrap activation journal contains trailing JSON")
	}
	validatedDestination, destinationErr := validBootstrapDestination(journal.Destination)
	if journal.Staged != filepath.Join(configDir, bootstrapActivationStagedFilename) || journal.Backup != filepath.Join(configDir, bootstrapActivationBackupFilename) || destinationErr != nil || validatedDestination != journal.Destination {
		return errors.New("bootstrap activation journal contains unsafe paths")
	}
	if journal.TargetVersion != "" {
		generation, err := updatepkg.NewStandaloneGeneration(journal.TargetVersion, journal.TargetHash)
		if err != nil {
			return err
		}
		if err := updatepkg.ValidateActivationTarget(configDir, generation); err != nil {
			return err
		}
	}
	switch journal.Status {
	case "PREPARING":
		if err := skillcontent.RecoverInstallTransaction(environment, false); err != nil {
			return err
		}
		if journal.HadExisting {
			if err := activateBootstrapExecutable(journal.Backup, journal.Destination, journal.PreviousHash); err != nil {
				return fmt.Errorf("restore previous ViceMe executable: %w", err)
			}
		} else if currentHash, hashErr := bootstrapFileHash(journal.Destination); hashErr == nil && currentHash == journal.TargetHash {
			if err := os.Remove(journal.Destination); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	case "COMMITTING", "COMMITTED":
		if currentHash, hashErr := bootstrapFileHash(journal.Destination); hashErr != nil || currentHash != journal.TargetHash {
			if err := activateBootstrapExecutable(journal.Staged, journal.Destination, journal.TargetHash); err != nil {
				return fmt.Errorf("complete ViceMe executable activation: %w", err)
			}
		}
		if err := skillcontent.RecoverInstallTransaction(environment, true); err != nil {
			return err
		}
		if journal.TargetVersion != "" {
			generation, err := updatepkg.NewStandaloneGeneration(journal.TargetVersion, journal.TargetHash)
			if err != nil {
				return err
			}
			if err := updatepkg.CommitActiveGeneration(configDir, generation); err != nil {
				return err
			}
		}
	default:
		return errors.New("bootstrap activation journal has an invalid status")
	}
	var cleanupErrors []error
	for _, path := range []string{journal.Staged, journal.Backup, journalPath} {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

func validBootstrapDestination(destination string) (string, error) {
	if destination == "" {
		return "", errors.New("destination is empty")
	}
	absDestination, err := filepath.Abs(destination)
	if err != nil || filepath.Clean(absDestination) != absDestination {
		return "", errors.New("destination is not canonical")
	}
	parent := filepath.Dir(absDestination)
	base := filepath.Base(absDestination)
	if parent == absDestination || base == "." || base == string(filepath.Separator) || base == "" {
		return "", errors.New("destination must name an executable file")
	}
	if info, err := os.Lstat(absDestination); err == nil && info.IsDir() {
		return "", errors.New("destination is a directory")
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	return absDestination, nil
}

func writeBootstrapJournal(filename string, journal bootstrapActivationJournal) error {
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".bootstrap-activation-*.tmp")
	if err != nil {
		return err
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
	return os.Rename(name, filename)
}

func activateBootstrapExecutable(source, destination, expectedHash string) error {
	if expectedHash == "" {
		return errors.New("expected executable digest is empty")
	}
	actual, err := bootstrapFileHash(source)
	if err != nil || actual != expectedHash {
		return errors.New("staged executable digest mismatch")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".viceme-activate-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	_ = temporary.Close()
	_ = os.Remove(name)
	if err := copyBootstrapExecutable(source, name); err != nil {
		return err
	}
	if err := os.Rename(name, destination); err == nil {
		return nil
	}
	if err := os.Remove(destination); err != nil && !errors.Is(err, fs.ErrNotExist) {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, destination); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

func copyBootstrapExecutable(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	outputFile, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()|0o500)
	if err != nil {
		return err
	}
	name := outputFile.Name()
	if _, err := io.Copy(outputFile, input); err != nil {
		_ = outputFile.Close()
		_ = os.Remove(name)
		return err
	}
	if err := outputFile.Sync(); err != nil {
		_ = outputFile.Close()
		_ = os.Remove(name)
		return err
	}
	if err := outputFile.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

func bootstrapFileHash(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(file, (100<<20)+1)); err != nil {
		return "", err
	}
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() > 100<<20 {
		return "", errors.New("executable exceeds bootstrap size limit")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
