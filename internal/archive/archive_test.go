package archive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestBuildDirectoryIsDeterministic(t *testing.T) {
	t.Parallel()
	first := t.TempDir()
	second := t.TempDir()
	writeFixture(t, first, false)
	writeFixture(t, second, true)

	one, err := BuildDirectory(context.Background(), first, DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	defer one.Cleanup()
	two, err := BuildDirectory(context.Background(), second, DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	defer two.Cleanup()

	if one.SHA256Digest != two.SHA256Digest {
		t.Fatalf("digest changed across metadata/order changes: %s != %s", one.SHA256Digest, two.SHA256Digest)
	}
	oneBytes, err := os.ReadFile(one.Path)
	if err != nil {
		t.Fatal(err)
	}
	twoBytes, err := os.ReadFile(two.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(oneBytes) != string(twoBytes) {
		t.Fatal("archive bytes are not deterministic")
	}

	entries := readArchive(t, one.Path)
	if _, ok := entries[".git/config"]; ok {
		t.Fatal(".git content must be ignored")
	}
	if _, ok := entries["node_modules/pkg/index.js"]; ok {
		t.Fatal("node_modules content must be ignored")
	}
	if got := entries["SKILL.md"]; got != "skill body\n" {
		t.Fatalf("unexpected SKILL.md: %q", got)
	}
}

func TestBuildDirectoryRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildDirectory(context.Background(), root, DefaultMaxBytes); err == nil {
		t.Fatal("expected symlink to fail")
	}
}

func TestFromFileEnforcesLimit(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "skill.zip")
	if err := os.WriteFile(file, []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := FromFile(file, 4); err == nil {
		t.Fatal("expected size limit error")
	}
	artifact, err := FromFile(file, 5)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Size != 5 || artifact.SHA256Digest == "" {
		t.Fatalf("unexpected artifact: %#v", artifact)
	}
}

func writeFixture(t *testing.T, root string, reverse bool) {
	t.Helper()
	paths := []struct {
		name string
		body string
	}{
		{"SKILL.md", "skill body\n"},
		{"references/commands.md", "commands\n"},
		{".git/config", "ignored\n"},
		{"node_modules/pkg/index.js", "ignored\n"},
	}
	if reverse {
		for left, right := 0, len(paths)-1; left < right; left, right = left+1, right-1 {
			paths[left], paths[right] = paths[right], paths[left]
		}
	}
	for _, item := range paths {
		full := filepath.Join(root, item.name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if reverse && item.name == "SKILL.md" {
			mode = 0o600
		}
		if err := os.WriteFile(full, []byte(item.body), mode); err != nil {
			t.Fatal(err)
		}
		stamp := time.Unix(100, 0)
		if reverse {
			stamp = time.Unix(9999, 0)
		}
		if err := os.Chtimes(full, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
}

func readArchive(t *testing.T, filename string) map[string]string {
	t.Helper()
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	result := make(map[string]string)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		result[header.Name] = string(data)
	}
	return result
}
