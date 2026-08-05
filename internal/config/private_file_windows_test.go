package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestConfigUsesWindowsACLInUnicodeConfigPath(t *testing.T) {
	base := filepath.Join(t.TempDir(), "谢忻彤", "AppData", "Local", "ViceMe", "Config")
	configured := Default(RegionCN)
	result, err := Save(base, configured)
	if err != nil {
		t.Fatal(err)
	}
	if err := requirePrivateFile(result.Path); err != nil {
		t.Fatalf("saved config is not private: %v", err)
	}
	if _, err := LoadOrDefault(base); err != nil {
		t.Fatalf("private Windows config did not load: %v", err)
	}

	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(
		result.Path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		t.Fatal(err)
	}
	_, err = LoadOrDefault(base)
	var loadErr *LoadError
	if !errors.As(err, &loadErr) || loadErr.Stage != "permissions" {
		t.Fatalf("broad Windows ACL was accepted: %v", err)
	}
}

func TestLoadHardensSafeUnprotectedWindowsConfig(t *testing.T) {
	base := filepath.Join(t.TempDir(), "ViceMe", "Config")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sid := user.User.Sid.String()
	parentDescriptor, err := windows.SecurityDescriptorFromString(
		"D:P(A;OICI;FA;;;" + sid + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)",
	)
	if err != nil {
		t.Fatal(err)
	}
	parentDACL, _, err := parentDescriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(
		base,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		parentDACL,
		nil,
	); err != nil {
		t.Fatal(err)
	}

	configured := Default(RegionCN)
	result, err := Save(base, configured)
	if err != nil {
		t.Fatal(err)
	}
	legacyDescriptor, err := windows.SecurityDescriptorFromString(
		"D:(A;;FA;;;" + sid + ")(A;;FA;;;SY)(A;;FA;;;BA)",
	)
	if err != nil {
		t.Fatal(err)
	}
	legacyDACL, _, err := legacyDescriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(
		result.Path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		legacyDACL,
		nil,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadOrDefault(base); err != nil {
		t.Fatalf("safe legacy config was not hardened: %v", err)
	}
	hardened, err := windows.GetNamedSecurityInfo(
		result.Path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := hardened.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("legacy config DACL remained unprotected")
	}
}

func TestForeignWindowsConfigOwnerIsRejected(t *testing.T) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:WDD:P(A;;FA;;;" + user.User.Sid.String() + ")",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := requirePrivateDescriptor(descriptor, user.User.Sid); err == nil {
		t.Fatal("config owned outside the current user was accepted")
	}
}
