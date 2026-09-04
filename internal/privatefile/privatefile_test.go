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
