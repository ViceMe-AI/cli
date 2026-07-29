//go:build !windows

package config

import (
	"fmt"
	"os"
)

func securePrivateFile(filename string) error {
	return os.Chmod(filename, 0o600)
}

func requirePrivateFile(filename string) error {
	info, err := os.Stat(filename)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("config containing a local access token must have permissions 0600")
	}
	return nil
}
