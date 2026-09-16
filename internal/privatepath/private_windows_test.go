//go:build windows

package privatepath

import (
	"path/filepath"
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

func setDACL(t *testing.T, path, sddl string) {
	t.Helper()
	descriptor, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		t.Fatal(err)
	}
}

func currentUserSID(t *testing.T) string {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	return user.User.Sid.String()
}

func TestRequirePrivateFileToleratesSystemPrincipalACEs(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "state.json")
	file, err := CreateExclusiveFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	user := currentUserSID(t)
	// Security software commonly injects read entries for SYSTEM or the local
	// Administrators group; both already hold full machine access.
	setDACL(t, filename, "D:P(A;;FA;;;"+user+")(A;;0x1200a9;;;S-1-5-18)(A;;0x1200a9;;;S-1-5-32-544)")
	if err := RequirePrivateFile(filename); err != nil {
		t.Fatalf("system principal ACEs must be tolerated: %v", err)
	}
}

func TestRequirePrivateFileRejectsUnknownPrincipalAsACLMismatch(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "state.json")
	file, err := CreateExclusiveFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	user := currentUserSID(t)
	// S-1-5-21-1000-1000-1000-1001 stands in for an arbitrary other account.
	setDACL(t, filename, "D:P(A;;FA;;;"+user+")(A;;0x1200a9;;;S-1-5-21-1000-1000-1000-1001)")
	err = RequirePrivateFile(filename)
	if err == nil {
		t.Fatal("file granting an unknown principal access was accepted as private")
	}
	if !IsACLMismatch(err) {
		t.Fatalf("unknown principal rejection = %v, want an ACL mismatch", err)
	}
}

func TestRequirePrivateFileToleratesAdministratorsOwner(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "state.json")
	file, err := CreateExclusiveFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	// Elevated creators produce Administrators-owned files by default. Setting
	// the owner needs administrative privilege, so skip where unavailable.
	ownerSID, err := windows.StringToSid("S-1-5-32-544")
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(
		filename,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
		ownerSID,
		nil,
		nil,
		nil,
	); err != nil {
		t.Skipf("cannot reassign ownership in this environment: %v", err)
	}
	if err := RequirePrivateFile(filename); err != nil {
		t.Fatalf("an Administrators-owned file must be tolerated: %v", err)
	}
}
