// Package privatefile writes private files that persist across CLI processes.
//
// Every write stages a temporary file in the target directory and activates it
// with a rename so a crashed process never leaves a partial file at the target
// path. Some agent sandboxes — observed with WorkBuddy's Seatbelt profile —
// allow plain file creation and writes outside the session workspace but deny
// rename and unlink. When the activating rename is denied with a permission
// error, the write degrades to a direct durable write at the target path so
// login, config, and publication flows keep working inside those sandboxes.
package privatefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ViceMe-AI/cli/internal/privatepath"
)

// PrivateMode is the on-disk permission of staged and target files on Unix.
const PrivateMode os.FileMode = 0o600

// staleStagingAge is how long an abandoned staging file must have been idle
// before a later write in the same directory removes it. In-flight writes from
// concurrent processes are seconds old, so the age gate keeps sweeps safe.
var staleStagingAge = time.Hour

// RenameFile activates staged writes and is replaced by tests to simulate
// sandbox denials. It must not be modified concurrently with Write.
var RenameFile = os.Rename

// Write durably writes data to filename as a private file, staging through a
// temporary file matching tempPattern in the same directory. Staging files
// abandoned by earlier writes are swept opportunistically.
func Write(filename string, data []byte, tempPattern string) error {
	directory := filepath.Dir(filename)
	sweepStaleStagingFiles(directory, tempPattern)
	file, err := privatepath.CreateTempFile(directory, tempPattern)
	if err != nil {
		return fmt.Errorf("create staging file: %w", err)
	}
	staged := file.Name()
	defer os.Remove(staged)
	if err := file.Chmod(PrivateMode); err != nil {
		_ = file.Close()
		return fmt.Errorf("secure staging file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write staging file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync staging file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close staging file: %w", err)
	}
	activateErr := RenameFile(staged, filename)
	if activateErr == nil {
		return nil
	}
	if !IsPermissionDenial(activateErr) {
		return fmt.Errorf("activate %s: %w", filename, activateErr)
	}
	// The sandbox denied the directory-entry mutation but still permits plain
	// writes, so replace the target directly. A crash mid-write can now leave
	// a partial target file; the caller treats the file as untrusted data.
	if directErr := writeDirect(filename, data); directErr != nil {
		return errors.Join(fmt.Errorf("activate %s: %w", filename, activateErr), directErr)
	}
	return nil
}

func writeDirect(filename string, data []byte) error {
	file, err := privatepath.CreateExclusiveFile(filename)
	if err != nil {
		createErr := err
		if err := privatepath.RequirePrivateFile(filename); err != nil {
			return errors.Join(fmt.Errorf("create target file: %w", createErr), fmt.Errorf("validate target file: %w", err))
		}
		file, err = os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC, PrivateMode)
	}
	if err != nil {
		return fmt.Errorf("create target file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write target file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync target file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close target file: %w", err)
	}
	// O_TRUNC keeps the previous mode of an existing file. Enforce the private
	// mode, tolerating a sandbox that denies chmod on existing files as long
	// as the file is already private.
	if err := os.Chmod(filename, PrivateMode); err != nil {
		info, statErr := os.Stat(filename)
		if statErr != nil || info.Mode().Perm() != PrivateMode {
			return fmt.Errorf("secure target file: %w", err)
		}
	}
	return nil
}

// IsPermissionDenial reports whether err wraps a sandbox-style permission
// denial (EPERM, EACCES, or a read-only filesystem) at any depth of its chain. Callers use it to
// classify a failed rename or remove and degrade to plain writes.
func IsPermissionDenial(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == syscall.EPERM || errno == syscall.EACCES || errno == syscall.EROFS
}

func sweepStaleStagingFiles(directory, tempPattern string) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-staleStagingAge)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matched, matchErr := filepath.Match(tempPattern, entry.Name())
		if matchErr != nil || !matched {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(directory, entry.Name()))
	}
}
