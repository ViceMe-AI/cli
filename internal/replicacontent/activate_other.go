//go:build !darwin && !linux && !windows

package replicacontent

import "errors"

func activateNoReplace(_, _ string) error {
	return errors.New("atomic no-replace directory activation is unsupported on this platform")
}
