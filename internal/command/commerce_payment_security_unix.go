//go:build !windows

package command

import "os"

func secureCommercePaymentDirectory(path string) error {
	return os.Chmod(path, 0o700)
}

func secureCommercePaymentFile(path string) error {
	return os.Chmod(path, 0o600)
}

func commercePaymentPresentationIsPrivate(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular() && info.Mode().Perm() == 0o600, nil
}

func commercePaymentDirectoryIsPrivate(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir() && info.Mode().Perm() == 0o700, nil
}
