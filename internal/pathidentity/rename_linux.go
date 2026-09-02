//go:build linux

package pathidentity

import "golang.org/x/sys/unix"

func (anchor *Anchor) RenameNoReplace(sourceName, targetName string) error {
	if err := validateBaseName(sourceName); err != nil {
		return err
	}
	if err := validateBaseName(targetName); err != nil {
		return err
	}
	directoryFD := int(anchor.directory.Fd())
	return unix.Renameat2(directoryFD, sourceName, directoryFD, targetName, unix.RENAME_NOREPLACE)
}
