//go:build !darwin && !linux && !freebsd && !windows

package pathidentity

import "errors"

type Anchor struct{}

func OpenDirectory(string) (*Anchor, error) {
	return nil, errors.New("directory file identity is unsupported on this platform")
}

func Directory(path string) (string, error) {
	_, err := OpenDirectory(path)
	return "", err
}

func (*Anchor) ID() string { return "" }

func (*Anchor) Close() error { return nil }

func (*Anchor) Sync() error { return errors.New("directory sync is unsupported on this platform") }

func (*Anchor) RenameNoReplace(string, string) error {
	return errors.New("anchored atomic no-replace rename is unsupported on this platform")
}
