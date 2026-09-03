//go:build darwin || linux || windows

package pathidentity

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAnchorRenameStaysWithTheOpenedDirectory(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(parent, "stage"), 0o700); err != nil {
		t.Fatal(err)
	}
	anchor, err := OpenDirectory(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer anchor.Close()
	moved := filepath.Join(root, "moved-parent")
	if err := os.Rename(parent, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(parent, "stage"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := anchor.RenameNoReplace("stage", "target"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(filepath.Join(moved, "target")); err != nil || !info.IsDir() {
		t.Fatalf("anchored target was not activated in the opened directory: info=%v err=%v", info, err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "target")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("anchored rename activated in the replacement directory: %v", err)
	}
	if info, err := os.Lstat(filepath.Join(parent, "stage")); err != nil || !info.IsDir() {
		t.Fatalf("replacement directory stage was modified: info=%v err=%v", info, err)
	}
}
