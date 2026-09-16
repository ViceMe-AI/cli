//go:build windows

package privatefile

import (
	"fmt"
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
