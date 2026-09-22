// Package devsetup owns opt-in test-package retirement. Production activation
// never imports this package and keeps its immutable-generation rules.
package devsetup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/update"
	"github.com/gofrs/flock"
)

type Retirement struct {
	SchemaVersion int                      `json:"schemaVersion"`
	Destination   string                   `json:"destination"`
	Digest        string                   `json:"digest"`
	Backup        string                   `json:"backup"`
	Generation    *update.ActiveGeneration `json:"generation,omitempty"`
}

// Resume finishes this tool's interrupted retirement for the requested path.
// Retire revalidates the complete journal, binary digest and authoritative state.
func Resume(configDir, destination string) error {
	data, err := os.ReadFile(filepath.Join(configDir, "dev-retirement.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var state Retirement
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	target, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if state.Destination != target {
		return errors.New("pending retirement belongs to a different installation path")
	}
	_, err = Retire(configDir, target, state.Digest)
	return err
}

// Retire removes only the verified executable and its matching generation.
// Config, credentials, owned Skills and their registry remain installed.
// An interrupted operation is resumed by the same explicit request; journals
// belonging to bootstrap/npm/Skill installation are never removed here.
func Retire(configDir, destination, digest string) (string, error) {
	destination, err := filepath.Abs(destination)
	if err != nil {
		return "", err
	}
	configDir, err = filepath.Abs(configDir)
	if err != nil {
		return "", err
	}
	raw, err := hex.DecodeString(digest)
	if err != nil || len(raw) != sha256.Size {
		return "", errors.New("expected SHA-256 is required")
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", err
	}
	var locks []*flock.Flock
	defer func() {
		for i := len(locks) - 1; i >= 0; i-- {
			_ = locks[i].Unlock()
		}
	}()
	for _, name := range []string{update.ActivationLockFilename, update.ActivationMemberLockFilename, "install.lock", "automatic-update.lock"} {
		lock := flock.New(filepath.Join(configDir, name))
		ok, err := lock.TryLock()
		if err != nil {
			return "", err
		}
		if !ok {
			return "", errors.New("CLI installation is busy; stop all CLI tasks and retry")
		}
		locks = append(locks, lock)
	}
	for _, name := range []string{update.BootstrapActivationJournalFilename, update.NPMActivationJournalFilename, "install-transaction.json"} {
		if _, err := os.Lstat(filepath.Join(configDir, name)); err == nil {
			return "", fmt.Errorf("unfinished %s: recover with its owning CLI before uninstalling", name)
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	journalPath := filepath.Join(configDir, "dev-retirement.json")
	backup := filepath.Join(filepath.Dir(destination), ".viceme-retired-"+digest)
	state := Retirement{SchemaVersion: 1, Destination: destination, Digest: digest, Backup: backup}
	if data, err := os.ReadFile(journalPath); err == nil {
		var pending Retirement
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&pending); err != nil || pending.SchemaVersion != 1 || pending.Destination != destination || pending.Digest != digest || pending.Backup != backup {
			return "", errors.New("another or invalid dev retirement is pending; preserve its journal")
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return "", errors.New("invalid trailing retirement data")
		}
		if pending.Generation != nil {
			expected, err := update.NewStandaloneGeneration(pending.Generation.Version, digest)
			if err != nil || expected != *pending.Generation {
				return "", errors.New("retirement generation does not match the verified executable")
			}
		}
		state = pending
	} else if !os.IsNotExist(err) {
		return "", err
	} else {
		if err := verifyFile(destination, digest); err != nil {
			return "", err
		}
		generation, exists, err := update.ReadActiveGeneration(configDir)
		if err != nil {
			return "", err
		}
		if exists {
			if generation.InstallMethod != "standalone" || generation.Identity != digest {
				return "", errors.New("binary is not the active standalone installation; npm must be removed with npm")
			}
			state.Generation = &generation
		}
		if _, err := os.Lstat(backup); err == nil {
			if err := verifyFile(backup, digest); err != nil {
				return "", err
			}
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if err := update.ProbeRenameCapability(filepath.Dir(destination)); err != nil {
			return "", err
		}
		if err := update.ProbeRenameCapability(configDir); err != nil {
			return "", err
		}
		if err := writeJSON(journalPath, state); err != nil {
			return "", err
		}
	}
	// Recheck the authoritative generation on every resume before any mutation.
	active, exists, err := update.ReadActiveGeneration(configDir)
	if err != nil {
		return "", err
	}
	if exists && (state.Generation == nil || active != *state.Generation) {
		return "", errors.New("active generation changed; retirement will not remove it")
	}
	if _, err := os.Lstat(destination); err == nil {
		if err := verifyFile(destination, digest); err != nil {
			return "", err
		}
		if _, err := os.Lstat(backup); err == nil {
			if err := verifyFile(backup, digest); err != nil {
				return "", err
			}
			if err := os.Remove(backup); err != nil {
				return "", err
			}
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if err := os.Rename(destination, backup); err != nil {
			return "", err
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := verifyFile(backup, digest); err != nil {
		return "", err
	}
	if exists {
		archiveDir := filepath.Join(configDir, "dev-retired")
		if err := os.MkdirAll(archiveDir, 0700); err != nil {
			return "", err
		}
		if err := writeJSON(filepath.Join(archiveDir, digest+".json"), state); err != nil {
			return "", err
		}
		if err := os.Remove(filepath.Join(configDir, "active-generation.json")); err != nil {
			return "", err
		}
	}
	if err := os.Remove(journalPath); err != nil {
		return "", err
	}
	return backup, nil
}

func verifyFile(path, digest string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("refuse symlink or non-regular executable")
	}
	actual, err := Digest(path)
	if err != nil {
		return err
	}
	if actual != digest {
		return errors.New("executable SHA-256 differs; preserve installation")
	}
	return nil
}
func Digest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return privatefile.Write(path, append(data, '\n'), ".dev-retirement-*.tmp")
}
