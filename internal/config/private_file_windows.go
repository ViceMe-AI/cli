package config

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	accessAllowedObjectACE         = 0x5
	accessAllowedCallbackACE       = 0x9
	accessAllowedCallbackObjectACE = 0xb
	accessDeniedObjectACE          = 0x6
	accessDeniedCallbackACE        = 0xa
	accessDeniedCallbackObjectACE  = 0xc
)

func securePrivateFile(filename string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("resolve current Windows user: %w", err)
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"D:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;FA;;;SY)(A;;FA;;;BA)",
	)
	if err != nil {
		return fmt.Errorf("build private Windows ACL: %w", err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return fmt.Errorf("read private Windows ACL: %w", err)
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
		return fmt.Errorf("apply private Windows ACL: %w", err)
	}
	return nil
}

func requirePrivateFile(filename string) error {
	descriptor, err := windows.GetNamedSecurityInfo(
		filename,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("read Windows ACL: %w", err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("resolve current Windows user: %w", err)
	}
	return requirePrivateDescriptor(descriptor, user.User.Sid)
}

func requirePrivateDescriptor(descriptor *windows.SECURITY_DESCRIPTOR, user *windows.SID) error {
	owner, _, err := descriptor.Owner()
	if err != nil {
		return fmt.Errorf("read Windows config owner: %w", err)
	}
	system, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		return err
	}
	administrators, err := windows.StringToSid("S-1-5-32-544")
	if err != nil {
		return err
	}
	allowed := []*windows.SID{user, system, administrators}
	if !sidAllowed(owner, allowed) {
		return fmt.Errorf("config containing a local access token has an unsupported Windows owner")
	}

	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return fmt.Errorf("config containing a local access token must have a private Windows ACL")
	}
	for index := uint16(0); index < dacl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(index), &ace); err != nil {
			return fmt.Errorf("read Windows ACL entry: %w", err)
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		switch ace.Header.AceType {
		case windows.ACCESS_DENIED_ACE_TYPE, accessDeniedObjectACE, accessDeniedCallbackACE, accessDeniedCallbackObjectACE:
			continue
		case windows.ACCESS_ALLOWED_ACE_TYPE:
			sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			if !sidAllowed(sid, allowed) && ace.Mask != 0 {
				return fmt.Errorf("config containing a local access token grants access outside the current Windows user")
			}
		case accessAllowedObjectACE, accessAllowedCallbackACE, accessAllowedCallbackObjectACE:
			return fmt.Errorf("config containing a local access token uses an unsupported Windows ACL entry")
		default:
			return fmt.Errorf("config containing a local access token uses an unsupported Windows ACL entry")
		}
	}
	return nil
}

func sidAllowed(candidate *windows.SID, allowed []*windows.SID) bool {
	for _, sid := range allowed {
		if candidate.Equals(sid) {
			return true
		}
	}
	return false
}
