//go:build windows

package command

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func secureCommercePaymentDirectory(path string) error {
	return secureCommercePaymentWindowsPath(path, windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT)
}

func secureCommercePaymentFile(path string) error {
	return secureCommercePaymentWindowsPath(path, windows.NO_INHERITANCE)
}

func secureCommercePaymentWindowsPath(path string, inheritance uint32) error {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return fmt.Errorf("open current process token: %w", err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return fmt.Errorf("resolve current Windows user: %w", err)
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.SET_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
		},
	}}, nil)
	if err != nil {
		return fmt.Errorf("build owner-only Windows ACL: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	); err != nil {
		return fmt.Errorf("apply owner-only Windows ACL: %w", err)
	}
	return nil
}

func commercePaymentPresentationIsPrivate(path string) (bool, error) {
	return commercePaymentWindowsPathIsPrivate(path)
}

func commercePaymentDirectoryIsPrivate(path string) (bool, error) {
	return commercePaymentWindowsPathIsPrivate(path)
}

func commercePaymentWindowsPathIsPrivate(path string) (bool, error) {
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION,
	)
	if err != nil {
		return false, err
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return false, err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount != 1 {
		return false, err
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		return false, err
	}
	if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask&windows.GENERIC_ALL != windows.GENERIC_ALL {
		return false, nil
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil {
		return false, err
	}
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	return owner.Equals(aceSID), nil
}
