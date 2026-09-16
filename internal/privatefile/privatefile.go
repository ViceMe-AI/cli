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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ViceMe-AI/cli/internal/atomicfile"
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

// ReplaceFile atomically replaces an existing target for fail-closed writes.
// It is replaced by tests to simulate activation failures.
var ReplaceFile = atomicfile.Replace

// renameAttempts bounds the retries given to a transient activation failure.
// Security software commonly holds a freshly written staging file for a scan
// and releases it within a few hundred milliseconds, so a short bounded retry
// absorbs that window before degrading or failing.
var renameAttempts = 3

// renameBackoff waits between rename attempts and is replaced by tests to
// remove real sleeps.
var renameBackoff = func(attempt int) {
	time.Sleep(time.Duration(attempt) * 150 * time.Millisecond)
}

// degradedWriteReporter holds the installed degradation callback. It is
// stored atomically because command instances are constructed concurrently in
// tests while writes may already be in flight.
var degradedWriteReporter atomic.Value // stores func(filename string, strictErr error)

// SetDegradedWriteReporter installs the callback notified after WriteTolerant
// completes a write whose hardened permission profile was refused by the
// environment. The command layer installs it per command instance; a later
// installation replaces the earlier one.
func SetDegradedWriteReporter(reporter func(filename string, strictErr error)) {
	degradedWriteReporter.Store(reporter)
}

func reportDegradedWrite(filename string, strictErr error) {
	reporter, ok := degradedWriteReporter.Load().(func(string, error))
	if ok && reporter != nil {
		reporter(filename, strictErr)
	}
}

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
	activateErr := retryRename(func() error { return RenameFile(staged, filename) })
	if activateErr == nil {
		return nil
	}
	if !shouldDirectWriteFallback(activateErr) {
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

// WriteTolerant behaves like Write but degrades one step further: when the
// strict write fails only because a permission profile check refused an
// otherwise writable file — the signature of security software rewriting
// access entries — it retries through a plain staging write and reports the
// degradation through DegradedWriteReporter. Reserve it for non-credential
// state; credential stores must keep failing closed through Write and
// WriteAtomic.
func WriteTolerant(filename string, data []byte, tempPattern string) error {
	strictErr := Write(filename, data, tempPattern)
	if strictErr == nil {
		return nil
	}
	if !privatepath.IsACLMismatch(strictErr) {
		return strictErr
	}
	if err := writeLenient(filename, data, tempPattern); err != nil {
		return errors.Join(strictErr, err)
	}
	reportDegradedWrite(filename, strictErr)
	return nil
}

// WriteAtomic is the fail-closed variant of Write. It never falls back to a
// direct target write when the activating rename is denied, so callers whose
// on-disk contract requires atomic replacement cannot expose a partial file.
func WriteAtomic(filename string, data []byte, tempPattern string) error {
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
	if err := retryRename(func() error { return ReplaceFile(staged, filename) }); err != nil {
		return fmt.Errorf("activate %s: %w", filename, err)
	}
	return nil
}

// retryRename gives a transient activation failure (a security-software scan
// hold surfaces as EACCES or a sharing violation) a bounded chance to settle
// before the caller degrades or fails.
func retryRename(activate func() error) error {
	err := activate()
	for attempt := 1; err != nil && isTransientRenameError(err) && attempt < renameAttempts; attempt++ {
		renameBackoff(attempt)
		err = activate()
	}
	return err
}

func isTransientRenameError(err error) bool {
	return isSharingContention(err) || isAccessDenied(err)
}

func shouldDirectWriteFallback(err error) bool {
	return IsPermissionDenial(err) || isSharingContention(err)
}

// writeLenient completes a write without the hardened permission profile
// after the environment refused it. The file still stages in the target
// directory and activates atomically when renames are available; only the
// owner-only ACL enforcement is dropped.
func writeLenient(filename string, data []byte, tempPattern string) error {
	directory := filepath.Dir(filename)
	sweepStaleStagingFiles(directory, tempPattern)
	file, err := os.CreateTemp(directory, tempPattern)
	if err != nil {
		return fmt.Errorf("create plain staging file: %w", err)
	}
	staged := file.Name()
	defer os.Remove(staged)
	_ = file.Chmod(PrivateMode)
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write plain staging file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync plain staging file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close plain staging file: %w", err)
	}
	activateErr := retryRename(func() error { return RenameFile(staged, filename) })
	if activateErr == nil {
		return nil
	}
	if !shouldDirectWriteFallback(activateErr) {
		return fmt.Errorf("activate %s: %w", filename, activateErr)
	}
	if info, statErr := os.Lstat(filename); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.Join(fmt.Errorf("activate %s: %w", filename, activateErr), errors.New("refuse to replace a symbolic link through the degraded path"))
	}
	if err := writeLenientDirect(filename, data); err != nil {
		return errors.Join(fmt.Errorf("activate %s: %w", filename, activateErr), err)
	}
	return nil
}

// writeLenientDirect replaces the target in place while keeping the durable
// write guarantee of the strict path: write, sync, close. The staged file's
// sync does not cover the target once the rename is refused.
func writeLenientDirect(filename string, data []byte) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, PrivateMode)
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
	// Best effort: the hardened profile was already refused by the
	// environment, so a denied chmod must not fail an otherwise durable write.
	_ = os.Chmod(filename, PrivateMode)
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
	if isAccessDenied(err) {
		return true
	}
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
