//go:build windows

package privatepath

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func EnsureDirectory(path string) (bool, error) {
	descriptor, attributes, err := privateSecurityAttributes(true)
	if err != nil {
		return false, err
	}
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	err = windows.CreateDirectory(pathPointer, attributes)
	runtime.KeepAlive(descriptor)
	if err == nil {
		return true, RequirePrivateDirectory(path)
	}
	if !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return false, err
	}
	return false, RequirePrivateDirectory(path)
}

func CreateTempDirectory(directory, pattern string) (string, error) {
	for range 100 {
		name, err := privateTempName(pattern)
		if err != nil {
			return "", err
		}
		path := filepath.Join(directory, name)
		created, err := EnsureDirectory(path)
		if err == nil && created {
			return path, nil
		}
		if err != nil && !errors.Is(err, fs.ErrExist) && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return "", err
		}
	}
	return "", errors.New("could not create a unique private temporary directory")
}

func CreateExclusiveFile(path string) (*os.File, error) {
	descriptor, attributes, err := privateSecurityAttributes(false)
	if err != nil {
		return nil, err
	}
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathPointer,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		attributes,
		windows.CREATE_NEW,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	runtime.KeepAlive(descriptor)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		_ = os.Remove(path)
		return nil, errors.New("could not wrap private Windows file handle")
	}
	if err := RequirePrivateFile(path); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return file, nil
}

func CreateTempFile(directory, pattern string) (*os.File, error) {
	for range 100 {
		name, err := privateTempName(pattern)
		if err != nil {
			return nil, err
		}
		file, err := CreateExclusiveFile(filepath.Join(directory, name))
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, fs.ErrExist) && !errors.Is(err, windows.ERROR_FILE_EXISTS) && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return nil, err
		}
	}
	return nil, errors.New("could not create a unique private temporary file")
}

func EnsureFile(path string) (bool, error) {
	file, err := CreateExclusiveFile(path)
	if err == nil {
		if closeErr := file.Close(); closeErr != nil {
			_ = os.Remove(path)
			return false, closeErr
		}
		return true, nil
	}
	if !errors.Is(err, fs.ErrExist) && !errors.Is(err, windows.ERROR_FILE_EXISTS) && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return false, err
	}
	return false, RequirePrivateFile(path)
}

func RequirePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("path is not a real directory")
	}
	return requirePrivateWindowsACL(path)
}

func RequirePrivateFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("path is not a regular file")
	}
	return requirePrivateWindowsACL(path)
}

func privateSecurityAttributes(directory bool) (*windows.SECURITY_DESCRIPTOR, *windows.SecurityAttributes, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve current Windows user: %w", err)
	}
	inheritance := ""
	if directory {
		inheritance = "OICI"
	}
	sid := user.User.Sid.String()
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:" + sid + "D:P(A;" + inheritance + ";FA;;;" + sid + ")",
	)
	if err != nil {
		return nil, nil, fmt.Errorf("build private Windows security descriptor: %w", err)
	}
	attributes := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: descriptor,
	}
	return descriptor, attributes, nil
}

func requirePrivateWindowsACL(path string) error {
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("read private Windows security descriptor: %w", err)
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("private Windows DACL is not protected")
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil {
		return errors.New("private Windows path owner is unavailable")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("resolve current Windows user: %w", err)
	}
	if !owner.Equals(user.User.Sid) {
		return errors.New("private Windows path is not owned by the current user")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount == 0 {
		return errors.New("private Windows path has no DACL entries")
	}
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil || ace == nil {
			return fmt.Errorf("read private Windows DACL entry %d: %w", index, err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask == 0 {
			return fmt.Errorf("private Windows DACL entry %d is unsupported", index)
		}
		trustee := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !trustee.Equals(user.User.Sid) {
			return fmt.Errorf("private Windows DACL entry %d grants another principal access", index)
		}
	}
	return nil
}

func privateTempName(pattern string) (string, error) {
	if pattern == "" || filepath.Base(pattern) != pattern {
		return "", errors.New("private temporary pattern must be a base name")
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	suffix := hex.EncodeToString(random)
	if index := strings.LastIndexByte(pattern, '*'); index >= 0 {
		return pattern[:index] + suffix + pattern[index+1:], nil
	}
	return pattern + suffix, nil
}
