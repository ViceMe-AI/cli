//go:build windows

package command

import (
	"errors"
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
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		user.User.Sid,
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
	if err != nil {
		return false, err
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return false, fmt.Errorf("Windows payment DACL is not protected: control=%#x", control)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return false, err
	}
	if dacl == nil || dacl.AceCount == 0 {
		return false, errors.New("Windows payment DACL has no access entries")
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil {
		return false, err
	}
	// Windows may map GENERIC_ALL to file-object-specific rights and may split
	// inheritable directory permissions into multiple ACEs. Privacy depends on
	// every effective entry granting access only to the owner, not on a specific
	// generic mask bit or ACE count.
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return false, err
		}
		if ace == nil {
			return false, fmt.Errorf("Windows payment DACL ACE %d is unavailable", index)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask == 0 {
			return false, fmt.Errorf(
				"Windows payment DACL ACE %d is not an effective allow entry: type=%d flags=%#x mask=%#x",
				index,
				ace.Header.AceType,
				ace.Header.AceFlags,
				ace.Mask,
			)
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !owner.Equals(aceSID) {
			return false, fmt.Errorf(
				"Windows payment DACL ACE %d trustee is not the owner: type=%d flags=%#x mask=%#x",
				index,
				ace.Header.AceType,
				ace.Header.AceFlags,
				ace.Mask,
			)
		}
	}
	return true, nil
}
