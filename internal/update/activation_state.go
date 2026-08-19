package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ViceMe-AI/cli/internal/semver"
)

const (
	// ActivationLockFilename is shared by standalone and npm activation. A
	// process may download a release before taking the lock, but it must recover
	// and compare the durable active generation while holding this lock before it
	// changes the launcher, Skills, or config.
	ActivationLockFilename = "activation.lock"
	// ActivationMemberLockFilename protects Skills/config while the outer npm
	// coordinator temporarily delegates that part of a generation commit to an
	// exact child process. Every other activation path probes or holds this lock
	// before it mutates generation state.
	ActivationMemberLockFilename = "activation-member.lock"
	// BootstrapActivationJournalFilename and NPMActivationJournalFilename are
	// the two mutually exclusive outer recovery protocols. Every mutation entry
	// must inspect both while holding ActivationLockFilename before it stages a
	// launcher or creates a new journal.
	BootstrapActivationJournalFilename = "bootstrap-activation.json"
	NPMActivationJournalFilename       = "npm-activation.json"
	activeGenerationFile               = "active-generation.json"
)

var (
	ErrActivationDowngrade     = errors.New("activation target is older than the active generation")
	ErrActivationMethodChange  = errors.New("changing the CLI installation method is not supported")
	ErrActivationRestartNeeded = errors.New("activation recovered a different CLI generation; restart the command")
)

type ActiveGeneration struct {
	SchemaVersion int    `json:"schemaVersion"`
	Version       string `json:"version"`
	InstallMethod string `json:"installMethod"`
	Identity      string `json:"identity"`
}

type OuterActivationJournals struct {
	Bootstrap bool
	NPM       bool
}

func InspectOuterActivationJournals(configDir string) (OuterActivationJournals, error) {
	bootstrap, err := activationFileExists(filepath.Join(configDir, BootstrapActivationJournalFilename))
	if err != nil {
		return OuterActivationJournals{}, err
	}
	npm, err := activationFileExists(filepath.Join(configDir, NPMActivationJournalFilename))
	if err != nil {
		return OuterActivationJournals{}, err
	}
	return OuterActivationJournals{Bootstrap: bootstrap, NPM: npm}, nil
}

func activationFileExists(filename string) (bool, error) {
	_, err := os.Stat(filename)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func NewNPMGeneration(version string) (ActiveGeneration, error) {
	identity := sha256.Sum256([]byte("npm\x00" + PackageName + "@" + version))
	return newActiveGeneration(version, "npm", hex.EncodeToString(identity[:]))
}

func NewStandaloneGeneration(version, binaryDigest string) (ActiveGeneration, error) {
	return newActiveGeneration(version, "standalone", strings.ToLower(binaryDigest))
}

func newActiveGeneration(version, installMethod, identity string) (ActiveGeneration, error) {
	generation := ActiveGeneration{
		SchemaVersion: 1,
		Version:       version,
		InstallMethod: installMethod,
		Identity:      identity,
	}
	if err := validateActiveGeneration(generation); err != nil {
		return ActiveGeneration{}, err
	}
	return generation, nil
}

func ReadActiveGeneration(configDir string) (ActiveGeneration, bool, error) {
	data, err := os.ReadFile(filepath.Join(configDir, activeGenerationFile))
	if errors.Is(err, os.ErrNotExist) {
		return ActiveGeneration{}, false, nil
	}
	if err != nil {
		return ActiveGeneration{}, false, fmt.Errorf("read active generation: %w", err)
	}
	var generation ActiveGeneration
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&generation); err != nil {
		return ActiveGeneration{}, false, errors.New("active generation is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ActiveGeneration{}, false, errors.New("active generation contains trailing JSON")
	}
	if err := validateActiveGeneration(generation); err != nil {
		return ActiveGeneration{}, false, fmt.Errorf("active generation is invalid: %w", err)
	}
	return generation, true, nil
}

// ValidateActivationTarget must be called while ActivationLockFilename is
// held. Equal immutable generations may be verified or repaired. Reusing a
// semantic version for different bytes/package identity fails closed.
func ValidateActivationTarget(configDir string, target ActiveGeneration) error {
	if err := validateActiveGeneration(target); err != nil {
		return err
	}
	active, exists, err := ReadActiveGeneration(configDir)
	if err != nil || !exists {
		return err
	}
	if target.InstallMethod != active.InstallMethod {
		return fmt.Errorf(
			"%w: target %s, active %s",
			ErrActivationMethodChange,
			target.InstallMethod,
			active.InstallMethod,
		)
	}
	comparison, err := semver.Compare(target.Version, active.Version)
	if err != nil {
		return err
	}
	if comparison < 0 {
		return fmt.Errorf("%w: target %s, active %s", ErrActivationDowngrade, target.Version, active.Version)
	}
	if comparison == 0 && (target.InstallMethod != active.InstallMethod || target.Identity != active.Identity) {
		return errors.New("activation target reuses the active version with a different immutable identity")
	}
	return nil
}

// AdoptExternalUpgrade adopts the running build as the active generation for
// out-of-band upgrades (direct `npm install -g`, reinstalls, or a migration
// from the standalone installer) where no activation journal exists. It only
// permits a strict version increase: rollbacks and same-version identity or
// install-method changes still fail closed. The caller must hold
// ActivationLockFilename.
func AdoptExternalUpgrade(configDir string, target ActiveGeneration) error {
	if err := validateActiveGeneration(target); err != nil {
		return err
	}
	active, exists, err := ReadActiveGeneration(configDir)
	if err != nil {
		return err
	}
	if exists {
		comparison, err := semver.Compare(target.Version, active.Version)
		if err != nil {
			return err
		}
		if comparison <= 0 {
			return fmt.Errorf("%w: target %s, active %s", ErrActivationDowngrade, target.Version, active.Version)
		}
	}
	return writeActiveGeneration(configDir, target)
}

// CommitActiveGeneration writes the semantic commit point for launcher,
// Skills, and config. The caller must still hold ActivationLockFilename.
func CommitActiveGeneration(configDir string, target ActiveGeneration) error {
	if err := ValidateActivationTarget(configDir, target); err != nil {
		return err
	}
	return writeActiveGeneration(configDir, target)
}

func writeActiveGeneration(configDir string, target ActiveGeneration) error {
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(target, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(configDir, ".active-generation-*.tmp")
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
	return os.Rename(name, filepath.Join(configDir, activeGenerationFile))
}

func validateActiveGeneration(generation ActiveGeneration) error {
	if generation.SchemaVersion != 1 {
		return errors.New("unsupported active generation schema")
	}
	if _, err := semver.Parse(generation.Version); err != nil {
		return err
	}
	if generation.InstallMethod != "npm" && generation.InstallMethod != "standalone" {
		return errors.New("unsupported activation install method")
	}
	if len(generation.Identity) != 64 {
		return errors.New("activation identity must be a SHA-256 digest")
	}
	if _, err := hex.DecodeString(generation.Identity); err != nil {
		return errors.New("activation identity must be lowercase hexadecimal")
	}
	if generation.Identity != strings.ToLower(generation.Identity) {
		return errors.New("activation identity must be lowercase hexadecimal")
	}
	return nil
}
