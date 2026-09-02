//go:build darwin

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
	return unix.RenameatxNp(directoryFD, sourceName, directoryFD, targetName, unix.RENAME_EXCL)
}
