package privatefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/privatepath"
)

func denyRename(t *testing.T, errno syscall.Errno) {
	t.Helper()
	original := RenameFile
	RenameFile = func(oldName, _ string) error {
		return fmt.Errorf("rename %s: %w", oldName, errno)
	}
	t.Cleanup(func() { RenameFile = original })
}

func denyReplace(t *testing.T, errno syscall.Errno) {
	t.Helper()
	original := ReplaceFile
	ReplaceFile = func(oldName, _ string) error {
		return fmt.Errorf("replace %s: %w", oldName, errno)
	}
	t.Cleanup(func() { ReplaceFile = original })
}

func requirePrivateMode(t *testing.T, filename string) {
	t.Helper()
	if err := privatepath.RequirePrivateFile(filename); err != nil {
		t.Fatalf("%s is not private: %v", filename, err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("stat %s: %v", filename, err)
	}
	if info.Mode().Perm() != PrivateMode {
		t.Fatalf("%s mode = %o, want %o", filename, info.Mode().Perm(), PrivateMode)
	}
}

func stagingFileNames(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read %s: %v", directory, err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

func TestWriteActivatesThroughRename(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "state.json")
	if err := Write(filename, []byte("payload"), ".state-*.tmp"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != "payload" {
		t.Fatalf("ReadFile() = %q, %v", data, err)
	}
	requirePrivateMode(t, filename)
	if names := stagingFileNames(t, directory); len(names) != 1 || names[0] != "state.json" {
		t.Fatalf("staging debris left behind: %v", names)
	}
}

func TestWriteReplacesExistingContentThroughRename(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "state.json")
	if err := Write(filename, []byte("first"), ".state-*.tmp"); err != nil {
		t.Fatalf("first Write() error = %v", err)
	}
	if err := Write(filename, []byte("second"), ".state-*.tmp"); err != nil {
		t.Fatalf("second Write() error = %v", err)
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != "second" {
		t.Fatalf("ReadFile() = %q, %v; want second", data, err)
	}
}

func TestWriteFallsBackWhenRenameIsDenied(t *testing.T) {
	denyRename(t, syscall.EPERM)
	directory := t.TempDir()
	filename := filepath.Join(directory, "state.json")
	if err := Write(filename, []byte("sandbox-payload"), ".state-*.tmp"); err != nil {
		t.Fatalf("Write() under denied rename error = %v", err)
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != "sandbox-payload" {
		t.Fatalf("ReadFile() = %q, %v", data, err)
	}
	requirePrivateMode(t, filename)

	// A second write must replace the previous direct-write target.
	if err := Write(filename, []byte("sandbox-payload-2"), ".state-*.tmp"); err != nil {
		t.Fatalf("second Write() under denied rename error = %v", err)
	}
	data, err = os.ReadFile(filename)
	if err != nil || string(data) != "sandbox-payload-2" {
		t.Fatalf("ReadFile() = %q, %v; want sandbox-payload-2", data, err)
	}
	requirePrivateMode(t, filename)
}

func TestWriteAtomicFailsClosedWhenRenameIsDenied(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "binding.json")
	if err := os.WriteFile(filename, []byte("existing"), PrivateMode); err != nil {
		t.Fatal(err)
	}
	denyReplace(t, syscall.EPERM)

	err := WriteAtomic(filename, []byte("replacement"), ".binding-*.tmp")
	if err == nil || !errors.Is(err, syscall.EPERM) {
		t.Fatalf("WriteAtomic() error = %v, want EPERM", err)
	}
	data, readErr := os.ReadFile(filename)
	if readErr != nil || string(data) != "existing" {
		t.Fatalf("atomic write changed its target after a denied rename: data=%q err=%v", data, readErr)
	}
	if names := stagingFileNames(t, directory); len(names) != 1 || names[0] != "binding.json" {
		t.Fatalf("atomic write left staging debris: %v", names)
	}
}

func TestWriteReportsBothErrorsWhenFallbackAlsoFails(t *testing.T) {
	denyRename(t, syscall.EACCES)
	directory := t.TempDir()
	// The target path is a directory, so the direct write cannot succeed.
	filename := filepath.Join(directory, "state.json")
	if err := os.Mkdir(filename, 0o700); err != nil {
		t.Fatal(err)
	}
	err := Write(filename, []byte("payload"), ".state-*.tmp")
	if err == nil {
		t.Fatal("Write() unexpectedly succeeded when both paths failed")
	}
	if !errors.Is(err, syscall.EACCES) {
		t.Fatalf("Write() error = %v, want EACCES from the denied rename", err)
	}
}

func TestWriteDoesNotFallBackOnNonPermissionRenameErrors(t *testing.T) {
	denyRename(t, syscall.ENOENT)
	directory := t.TempDir()
	filename := filepath.Join(directory, "state.json")
	err := Write(filename, []byte("payload"), ".state-*.tmp")
	if err == nil {
		t.Fatal("Write() unexpectedly succeeded with a non-permission rename error")
	}
	if !errors.Is(err, syscall.ENOENT) {
		t.Fatalf("Write() error = %v, want ENOENT", err)
	}
	if _, statErr := os.Stat(filename); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("target unexpectedly written through the fallback: %v", statErr)
	}
}

func TestWriteSweepsStaleStagingFiles(t *testing.T) {
	directory := t.TempDir()
	stale := filepath.Join(directory, ".state-stale.tmp")
	fresh := filepath.Join(directory, ".state-fresh.tmp")
	for _, name := range []string{stale, fresh} {
		if err := os.WriteFile(name, []byte("orphan"), PrivateMode); err != nil {
			t.Fatal(err)
		}
	}
	staleTime := time.Now().Add(-2 * staleStagingAge)
	if err := os.Chtimes(stale, staleTime, staleTime); err != nil {
		t.Fatal(err)
	}

	filename := filepath.Join(directory, "state.json")
	if err := Write(filename, []byte("payload"), ".state-*.tmp"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale staging file survived the sweep: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh staging file removed by the sweep: %v", err)
	}
}

func noRenameBackoff(t *testing.T) {
	t.Helper()
	original := renameBackoff
	renameBackoff = func(int) {}
	t.Cleanup(func() { renameBackoff = original })
}

func TestWriteRetriesTransientRenameFailure(t *testing.T) {
	noRenameBackoff(t)
	attempts := 0
	original := RenameFile
	RenameFile = func(oldName, newName string) error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("rename %s: %w", oldName, syscall.EACCES)
		}
		return original(oldName, newName)
	}
	t.Cleanup(func() { RenameFile = original })

	directory := t.TempDir()
	filename := filepath.Join(directory, "state.json")
	if err := Write(filename, []byte("payload"), ".state-*.tmp"); err != nil {
		t.Fatalf("Write() with transient rename denials error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("rename attempts = %d, want 3", attempts)
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != "payload" {
		t.Fatalf("ReadFile() = %q, %v", data, err)
	}
	requirePrivateMode(t, filename)
}

func TestWriteFallsBackAfterTransientRetriesExhausted(t *testing.T) {
	noRenameBackoff(t)
	attempts := 0
	original := RenameFile
	RenameFile = func(oldName, _ string) error {
		attempts++
		return fmt.Errorf("rename %s: %w", oldName, syscall.EACCES)
	}
	t.Cleanup(func() { RenameFile = original })

	directory := t.TempDir()
	filename := filepath.Join(directory, "state.json")
	if err := Write(filename, []byte("payload"), ".state-*.tmp"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if attempts != renameAttempts {
		t.Fatalf("rename attempts = %d, want %d", attempts, renameAttempts)
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != "payload" {
		t.Fatalf("ReadFile() = %q, %v", data, err)
	}
	requirePrivateMode(t, filename)
}

func TestWriteDoesNotRetryDeterministicDenials(t *testing.T) {
	noRenameBackoff(t)
	attempts := 0
	original := RenameFile
	RenameFile = func(oldName, _ string) error {
		attempts++
		return fmt.Errorf("rename %s: %w", oldName, syscall.EPERM)
	}
	t.Cleanup(func() { RenameFile = original })

	directory := t.TempDir()
	filename := filepath.Join(directory, "state.json")
	if err := Write(filename, []byte("payload"), ".state-*.tmp"); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if attempts != 1 {
		t.Fatalf("rename attempts = %d, want 1 (sandbox EPERM is deterministic)", attempts)
	}
}

func TestWriteAtomicRetriesTransientReplaceFailure(t *testing.T) {
	noRenameBackoff(t)
	attempts := 0
	original := ReplaceFile
	ReplaceFile = func(oldName, newName string) error {
		attempts++
		if attempts == 1 {
			return fmt.Errorf("replace %s: %w", oldName, syscall.EACCES)
		}
		return original(oldName, newName)
	}
	t.Cleanup(func() { ReplaceFile = original })

	directory := t.TempDir()
	filename := filepath.Join(directory, "binding.json")
	if err := WriteAtomic(filename, []byte("payload"), ".binding-*.tmp"); err != nil {
		t.Fatalf("WriteAtomic() with transient replace denial error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("replace attempts = %d, want 2", attempts)
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != "payload" {
		t.Fatalf("ReadFile() = %q, %v", data, err)
	}
}

func TestWriteTolerantDegradesOnPermissionProfileMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the staged-file ACL mismatch simulated here is produced through the Unix mode bits")
	}
	noRenameBackoff(t)
	denyRename(t, syscall.EPERM)

	directory := t.TempDir()
	filename := filepath.Join(directory, "state.json")
	// A pre-existing target with group/other permissions makes the strict
	// direct-write validation refuse it as an ACL mismatch.
	if err := os.WriteFile(filename, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	var reported string
	reportedErr := error(nil)
	reporter := func(name string, strictErr error) { reported, reportedErr = name, strictErr }

	if err := WriteTolerant(filename, []byte("payload"), ".state-*.tmp", reporter); err != nil {
		t.Fatalf("WriteTolerant() error = %v", err)
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != "payload" {
		t.Fatalf("ReadFile() = %q, %v", data, err)
	}
	if reported != filename {
		t.Fatalf("degraded write reported %q, want %q", reported, filename)
	}
	if !privatepath.IsACLMismatch(reportedErr) {
		t.Fatalf("reported strict error = %v, want an ACL mismatch", reportedErr)
	}
}

func TestWriteTolerantPropagatesNonMismatchFailures(t *testing.T) {
	noRenameBackoff(t)
	denyRename(t, syscall.EPERM)

	directory := t.TempDir()
	// The target path is a directory: the direct-write fallback fails with a
	// plain validation error, never an ACL mismatch.
	filename := filepath.Join(directory, "state.json")
	if err := os.Mkdir(filename, 0o700); err != nil {
		t.Fatal(err)
	}

	reported := false
	reporter := func(string, error) { reported = true }

	err := WriteTolerant(filename, []byte("payload"), ".state-*.tmp", reporter)
	if err == nil {
		t.Fatal("WriteTolerant() unexpectedly succeeded")
	}
	if privatepath.IsACLMismatch(err) {
		t.Fatalf("WriteTolerant() error = %v, want a non-mismatch failure", err)
	}
	if reported {
		t.Fatal("degraded write reporter fired for a non-degraded failure")
	}
}

func TestWriteTolerantReportsOnlyToTheDegradedCall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the staged-file ACL mismatch simulated here is produced through the Unix mode bits")
	}
	noRenameBackoff(t)
	denyRename(t, syscall.EPERM)

	directory := t.TempDir()
	filename := filepath.Join(directory, "state.json")
	// A pre-existing target with group/other permissions makes the strict
	// direct-write validation refuse it as an ACL mismatch, so every call
	// here degrades and reports.
	if err := os.WriteFile(filename, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	var firstHeard []string
	firstReporter := func(name string, _ error) { firstHeard = append(firstHeard, name) }
	if err := WriteTolerant(filename, []byte("first"), ".state-*.tmp", firstReporter); err != nil {
		t.Fatalf("first WriteTolerant() error = %v", err)
	}
	if len(firstHeard) != 1 || firstHeard[0] != filename {
		t.Fatalf("first call's reporter heard %v, want exactly [%s]", firstHeard, filename)
	}

	// The first degraded write healed the target's permissions, so widen them
	// again to force the second call down the degraded path too.
	if err := os.Chmod(filename, 0o644); err != nil {
		t.Fatal(err)
	}

	var secondHeard []string
	secondReporter := func(name string, _ error) { secondHeard = append(secondHeard, name) }
	if err := WriteTolerant(filename, []byte("second"), ".state-*.tmp", secondReporter); err != nil {
		t.Fatalf("second WriteTolerant() error = %v", err)
	}
	if len(secondHeard) != 1 || secondHeard[0] != filename {
		t.Fatalf("second call's reporter heard %v, want exactly [%s]", secondHeard, filename)
	}
	if len(firstHeard) != 1 {
		t.Fatalf("first call's reporter heard another call's warning: %v", firstHeard)
	}

	// A call without a reporter stays silent.
	if err := os.Chmod(filename, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteTolerant(filename, []byte("third"), ".state-*.tmp", nil); err != nil {
		t.Fatalf("reporter-less WriteTolerant() error = %v", err)
	}
	if len(firstHeard) != 1 || len(secondHeard) != 1 {
		t.Fatalf("reporter-less call leaked a warning: first=%v second=%v", firstHeard, secondHeard)
	}
}

func TestWriteTolerantUsesStrictPathWhenHealthy(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "state.json")
	reported := false
	reporter := func(string, error) { reported = true }

	if err := WriteTolerant(filename, []byte("payload"), ".state-*.tmp", reporter); err != nil {
		t.Fatalf("WriteTolerant() error = %v", err)
	}
	if reported {
		t.Fatal("degraded write reporter fired for a healthy strict write")
	}
	requirePrivateMode(t, filename)
}
