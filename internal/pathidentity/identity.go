package pathidentity

import (
	"errors"
	"path/filepath"
	"strings"
)

func validateBaseName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
		return errors.New("anchored path must be a single base name")
	}
	return nil
}
