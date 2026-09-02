//go:build !windows

package privatepath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecurePathPermissions(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "directory")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	created, err := EnsureDirectory(directory)
	if err != nil || !created {
		t.Fatal(err)
	}
	if err := RequirePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	output, err := CreateExclusiveFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := output.WriteString("private"); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	if err := RequirePrivateFile(file); err != nil {
		t.Fatal(err)
	}
}
