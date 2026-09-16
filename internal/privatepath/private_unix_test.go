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

func TestPermissionMismatchClassifiesAsACLMismatch(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "wide")
	if err := os.WriteFile(file, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := RequirePrivateFile(file)
	if err == nil {
		t.Fatal("RequirePrivateFile accepted a group-readable file")
	}
	if !IsACLMismatch(err) {
		t.Fatalf("RequirePrivateFile error = %v, want an ACL mismatch", err)
	}
}

func TestSymlinkRejectionIsNotAnACLMismatch(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	err := RequirePrivateFile(link)
	if err == nil {
		t.Fatal("RequirePrivateFile accepted a symbolic link")
	}
	if IsACLMismatch(err) {
		t.Fatalf("symlink rejection must stay fail-closed, got ACL mismatch: %v", err)
	}
}
