//go:build windows

package pathidentity

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Anchor struct {
	handle windows.Handle
	id     string
}

type fileRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func OpenDirectory(path string) (*Anchor, error) {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathPointer,
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("path identity target is not a real directory")
	}
	fileIndex := uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)
	return &Anchor{
		handle: handle,
		id:     fmt.Sprintf("windows:%08x:%016x", info.VolumeSerialNumber, fileIndex),
	}, nil
}

func Directory(path string) (string, error) {
	anchor, err := OpenDirectory(path)
	if err != nil {
		return "", err
	}
	defer anchor.Close()
	return anchor.ID(), nil
}

func (anchor *Anchor) ID() string { return anchor.id }

func (anchor *Anchor) Close() error { return windows.CloseHandle(anchor.handle) }

func (*Anchor) Sync() error { return nil }

func (anchor *Anchor) RenameNoReplace(sourceName, targetName string) error {
	if err := validateBaseName(sourceName); err != nil {
		return err
	}
	if err := validateBaseName(targetName); err != nil {
		return err
	}
	source, err := windows.NewNTUnicodeString(sourceName)
	if err != nil {
		return err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		RootDirectory: anchor.handle,
		ObjectName:    source,
	}
	attributes.Length = uint32(unsafe.Sizeof(*attributes))
	var sourceHandle windows.Handle
	var status windows.IO_STATUS_BLOCK
	var allocationSize int64
	if err := windows.NtCreateFile(
		&sourceHandle,
		windows.DELETE|windows.SYNCHRONIZE,
		attributes,
		&status,
		&allocationSize,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		windows.FILE_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT,
		0,
		0,
	); err != nil {
		return err
	}
	defer windows.CloseHandle(sourceHandle)
	target, err := windows.UTF16FromString(targetName)
	if err != nil {
		return err
	}
	fileNameLength := (len(target) - 1) * 2
	var layout fileRenameInformation
	buffer := make([]byte, int(unsafe.Offsetof(layout.FileName))+fileNameLength)
	rename := (*fileRenameInformation)(unsafe.Pointer(&buffer[0]))
	rename.RootDirectory = anchor.handle
	rename.FileNameLength = uint32(fileNameLength)
	copy(
		(*[windows.MAX_LONG_PATH]uint16)(unsafe.Pointer(&rename.FileName[0]))[:fileNameLength/2:fileNameLength/2],
		target[:len(target)-1],
	)
	return windows.NtSetInformationFile(
		sourceHandle,
		&status,
		&buffer[0],
		uint32(len(buffer)),
		windows.FileRenameInformation,
	)
}
