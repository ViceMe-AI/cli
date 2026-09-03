//go:build darwin || linux || freebsd

package pathidentity

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

type Anchor struct {
	directory *os.File
	id        string
}

func OpenDirectory(path string) (*Anchor, error) {
	directory, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := directory.Stat()
	if err != nil {
		_ = directory.Close()
		return nil, err
	}
	if !info.IsDir() {
		_ = directory.Close()
		return nil, errors.New("path identity target is not a directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		_ = directory.Close()
		return nil, errors.New("directory file identity is unavailable")
	}
	return &Anchor{directory: directory, id: fmt.Sprintf("unix:%d:%d", stat.Dev, stat.Ino)}, nil
}

func Directory(path string) (string, error) {
	anchor, err := OpenDirectory(path)
	if err != nil {
		return "", err
	}
	defer anchor.Close()
	return anchor.ID(), nil
}

func (anchor *Anchor) ID() string { return anchor.id }

func (anchor *Anchor) Close() error { return anchor.directory.Close() }

func (anchor *Anchor) Sync() error { return anchor.directory.Sync() }
