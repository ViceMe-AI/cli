//go:build !windows

package atomicfile

import "os"

func Replace(source, target string) error {
	return os.Rename(source, target)
}

func SyncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
