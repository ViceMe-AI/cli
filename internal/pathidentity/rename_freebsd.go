//go:build freebsd

package pathidentity

import "errors"

func (*Anchor) RenameNoReplace(string, string) error {
	return errors.New("anchored atomic no-replace rename is unsupported on this platform")
}
