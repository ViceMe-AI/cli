//go:build !windows

package privatefile

// Sharing violations are a Windows file-locking concept; Unix renames do not
// fail because another process holds the file open.
func isSharingContention(error) bool {
	return false
}
