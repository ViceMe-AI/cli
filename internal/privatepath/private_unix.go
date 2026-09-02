//go:build !windows

package privatepath

import (
	"errors"
	"io/fs"
	"os"
)

func EnsureDirectory(path string) (bool, error) {
	if err := os.Mkdir(path, 0o700); err == nil {
		return true, RequirePrivateDirectory(path)
	} else if !errors.Is(err, fs.ErrExist) {
		return false, err
	}
	return false, RequirePrivateDirectory(path)
}

func CreateTempDirectory(directory, pattern string) (string, error) {
	path, err := os.MkdirTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	if err := RequirePrivateDirectory(path); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func CreateExclusiveFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if err := RequirePrivateFile(path); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return file, nil
}

func CreateTempFile(directory, pattern string) (*os.File, error) {
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return nil, err
	}
	if err := RequirePrivateFile(file.Name()); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, err
	}
	return file, nil
}

func EnsureFile(path string) (bool, error) {
	file, err := CreateExclusiveFile(path)
	if err == nil {
		if closeErr := file.Close(); closeErr != nil {
			_ = os.Remove(path)
			return false, closeErr
		}
		return true, nil
	}
	if !errors.Is(err, fs.ErrExist) {
		return false, err
	}
	return false, RequirePrivateFile(path)
}

func RequirePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return errors.New("path is not a private directory")
	}
	return nil
}

func RequirePrivateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return errors.New("path is not a private regular file")
	}
	return nil
}
