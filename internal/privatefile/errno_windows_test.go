//go:build windows

package privatefile

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestIsSharingContentionCoversBothErrnoTypes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"syscall sharing violation", fmt.Errorf("rename: %w", syscall.Errno(windows.ERROR_SHARING_VIOLATION)), true},
		{"x/sys lock violation", fmt.Errorf("replace: %w", windows.ERROR_LOCK_VIOLATION), true},
		{"plain access denied", fmt.Errorf("rename: %w", syscall.EACCES), false},
		{"unrelated error", fmt.Errorf("rename: %w", syscall.ENOENT), false},
	}
	for _, tc := range cases {
		if got := isSharingContention(tc.err); got != tc.want {
			t.Errorf("%s: isSharingContention = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsAccessDeniedCoversNativeWindowsCode(t *testing.T) {
	// The real Windows denial is the native ERROR_ACCESS_DENIED (5); the
	// syscall package's EACCES is an invented value real APIs never return.
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"native syscall errno", &os.PathError{Op: "rename", Path: "a", Err: syscall.Errno(windows.ERROR_ACCESS_DENIED)}, true},
		{"native x/sys errno", fmt.Errorf("replace: %w", windows.ERROR_ACCESS_DENIED), true},
		{"invented EACCES", fmt.Errorf("rename: %w", syscall.EACCES), true},
		{"EPERM is not an access denial here", fmt.Errorf("rename: %w", syscall.EPERM), false},
		{"unrelated error", fmt.Errorf("rename: %w", syscall.ENOENT), false},
	}
	for _, tc := range cases {
		if got := isAccessDenied(tc.err); got != tc.want {
			t.Errorf("%s: isAccessDenied = %v, want %v", tc.name, got, tc.want)
		}
		if tc.want && !IsPermissionDenial(tc.err) {
			t.Errorf("%s: IsPermissionDenial must also classify the native denial", tc.name)
		}
	}
}

func TestWriteRetriesNativeAccessDenied(t *testing.T) {
	noRenameBackoff(t)
	attempts := 0
	original := RenameFile
	RenameFile = func(oldName, newName string) error {
		attempts++
		if attempts < 3 {
			return &os.PathError{Op: "rename", Path: oldName, Err: syscall.Errno(windows.ERROR_ACCESS_DENIED)}
		}
		return original(oldName, newName)
	}
	t.Cleanup(func() { RenameFile = original })

	directory := t.TempDir()
	filename := filepath.Join(directory, "state.json")
	if err := Write(filename, []byte("payload"), ".state-*.tmp"); err != nil {
		t.Fatalf("Write() with native access denials error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("rename attempts = %d, want 3", attempts)
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != "payload" {
		t.Fatalf("ReadFile() = %q, %v", data, err)
	}
}

func TestWriteFallsBackAfterNativeAccessDeniedRetries(t *testing.T) {
	noRenameBackoff(t)
	attempts := 0
	original := RenameFile
	RenameFile = func(oldName, _ string) error {
		attempts++
		return &os.PathError{Op: "rename", Path: oldName, Err: syscall.Errno(windows.ERROR_ACCESS_DENIED)}
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
}
