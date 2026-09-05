package pagepackage

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticOutputExcludesPublicationRecovery(t *testing.T) {
	directory := t.TempDir()
	recovery := filepath.Join(directory, "nested", ".viceme", "publications")
	if err := os.MkdirAll(recovery, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recovery, "pending.json"), []byte("private recovery data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("<h1>Public</h1>"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := archiveStaticDirectory(directory, "Site")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if strings.Contains(file.Name, ".viceme") {
			t.Fatal("recovery file leaked into hosted page")
		}
	}
}
