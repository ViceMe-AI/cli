//go:build windows

package privatepath

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestPrivateWindowsPathsUseProtectedOwnerOnlyACLs(t *testing.T) {
	root := t.TempDir()
	directory, err := CreateTempDirectory(root, "private-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := RequirePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	file, err := CreateTempFile(directory, "private-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	filename := file.Name()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := RequirePrivateFile(filename); err != nil {
		t.Fatal(err)
	}

	worldDescriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := worldDescriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(
		filename,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		t.Fatal(err)
	}
	if err := RequirePrivateFile(filename); err == nil {
		t.Fatal("file granting Everyone access was accepted as private")
	}
}
