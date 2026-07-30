package skillcontent

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsConfigDirKeepsExistingLegacyConfig(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, ".viceme-cli")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VICEME_CLI_CONFIG_DIR", "")
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "AppData", "Local"))
	if actual := defaultConfigDir(home); actual != legacy {
		t.Fatalf("default config dir=%q, want legacy %q", actual, legacy)
	}
}

func TestWindowsConfigDirKeepsUnreadableLegacyConfigForLoadDiagnostics(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, ".viceme-cli")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	setACL := func(sddl string) error {
		descriptor, err := windows.SecurityDescriptorFromString(sddl)
		if err != nil {
			return err
		}
		dacl, _, err := descriptor.DACL()
		if err != nil {
			return err
		}
		return windows.SetNamedSecurityInfo(
			legacy,
			windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
			nil,
			nil,
			dacl,
			nil,
		)
	}
	sid := user.User.Sid.String()
	if err := setACL("D:P(D;;FA;;;" + sid + ")(A;;FA;;;SY)(A;;FA;;;BA)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := setACL("D:P(A;;FA;;;" + sid + ")(A;;FA;;;SY)(A;;FA;;;BA)"); err != nil {
			t.Errorf("restore legacy config ACL: %v", err)
		}
	})

	t.Setenv("VICEME_CLI_CONFIG_DIR", "")
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "AppData", "Local"))
	if actual := defaultConfigDir(home); actual != legacy {
		t.Fatalf("default config dir=%q, want unreadable legacy %q", actual, legacy)
	}
}
