//go:build !windows

package privatefile

import (
	"errors"
	"syscall"
)

// isAccessDenied reports whether err wraps EACCES. Non-Windows platforms use
// real POSIX errnos, so the constant comparison is exact.
func isAccessDenied(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == syscall.EACCES
}

// Sharing violations are a Windows file-locking concept; Unix renames do not
// fail because another process holds the file open.
func isSharingContention(error) bool {
	return false
}
