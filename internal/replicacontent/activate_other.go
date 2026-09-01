//go:build !darwin && !linux

package replicacontent

import (
	"errors"
	"io/fs"
	"os"
)

func activateNoReplace(source, target string) error {
	if _, err := os.Lstat(target); err == nil {
		return errors.New("target already exists")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.Rename(source, target)
}
