//go:build windows

package atomicfile

import "golang.org/x/sys/windows"

func Replace(source, target string) error {
	sourcePath, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	targetPath, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(
		sourcePath,
		targetPath,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}

// MoveFileEx with MOVEFILE_WRITE_THROUGH flushes the replacement. Windows does
// not expose a portable directory fsync equivalent for this call site.
func SyncDirectory(string) error { return nil }
