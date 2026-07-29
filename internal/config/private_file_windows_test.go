package config

import (
	"errors"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestLocalAccessTokenUsesWindowsACLInUnicodeConfigPath(t *testing.T) {
	base := filepath.Join(t.TempDir(), "谢忻彤", "AppData", "Local", "ViceMe", "Config")
	configured := Default(RegionCN)
	configured.Profiles[0].APIBaseURL = "http://localhost:8090"
	configured.Profiles[0].AccessToken = "local-operator-secret"
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
