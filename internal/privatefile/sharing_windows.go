//go:build windows

package privatefile

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// isSharingContention reports whether err is a Windows sharing or lock
// violation — the signature of security software briefly holding a freshly
// written file for scanning. The two rename paths surface different errno
// types (os.Rename wraps syscall.Errno, atomicfile.Replace wraps
// x/sys/windows.Errno), so both are unwrapped here.
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
