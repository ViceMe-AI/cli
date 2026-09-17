//go:build windows

package privatefile

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// On Windows the syscall package's EACCES/EPERM are invented values that real
// API calls never return: a denied operation surfaces as the native
// ERROR_ACCESS_DENIED (5). Two rename paths also surface different errno
// types — os.Rename wraps syscall.Errno while atomicfile.Replace wraps
// x/sys/windows.Errno — so every classifier here unwraps both.

// isAccessDenied reports whether err is a Windows access denial: the native
// ERROR_ACCESS_DENIED (5) in either errno representation, or the invented
// syscall.EACCES that tests and translated errors may carry.
func isAccessDenied(err error) bool {
	var sysErrno syscall.Errno
	if errors.As(err, &sysErrno) &&
		(sysErrno == syscall.EACCES || uint32(sysErrno) == uint32(windows.ERROR_ACCESS_DENIED)) {
		return true
	}
	var winErrno windows.Errno
	if errors.As(err, &winErrno) && winErrno == windows.ERROR_ACCESS_DENIED {
		return true
	}
	return false
}

// isSharingContention reports whether err is a Windows sharing or lock
// violation — the signature of security software briefly holding a freshly
// written file for scanning.
func isSharingContention(err error) bool {
	var sysErrno syscall.Errno
	if errors.As(err, &sysErrno) && isSharingErrno(uint32(sysErrno)) {
		return true
	}
	var winErrno windows.Errno
	if errors.As(err, &winErrno) && isSharingErrno(uint32(winErrno)) {
		return true
	}
	return false
}

func isSharingErrno(value uint32) bool {
	return value == uint32(windows.ERROR_SHARING_VIOLATION) || value == uint32(windows.ERROR_LOCK_VIOLATION)
}
