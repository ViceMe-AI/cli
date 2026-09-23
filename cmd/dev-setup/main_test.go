package main

import (
	"strings"
	"testing"
)

func TestRejectUnsupportedRegionBeforeReadingPackage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("VICEME_INSTALL_METHOD", "")
	_, err := run([]string{"install", "--region", "global", "--package-dir", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "supports only the cn region") {
		t.Fatalf("unsupported dev region was not rejected before package access: %v", err)
	}
}
