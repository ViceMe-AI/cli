package publication

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
)

const testSkillMarkdown = `---
name: Poster Skill
description: Create a poster from a short prompt.
---

# Poster Skill
`

func TestBuildIsDeterministicAcrossDirectoryAndZip(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "SKILL.md"), []byte(testSkillMarkdown), 0o644)
	writeTestFile(t, filepath.Join(directory, "scripts", "run.sh"), []byte("#!/bin/sh\necho ok\n"), 0o755)
	image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(directory, "assets", "cover.png"), image, 0o644)
	writeTestFile(t, filepath.Join(directory, ".git", "ignored"), []byte("ignored"), 0o644)

	first, err := Build(directory, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(directory, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Artifact.Digest != second.Artifact.Digest || !bytes.Equal(first.Bytes, second.Bytes) {
		t.Fatal("identical inputs did not produce an identical deterministic ZIP")
	}
	if first.Manifest.Spec.Sale.PriceMinor != 1 || first.Manifest.Spec.Sale.Currency != "CNY" {
		t.Fatalf("unexpected sale manifest: %#v", first.Manifest.Spec.Sale)
	}
	if len(first.Candidates) != 1 || first.Candidates[0].RelativePath != "assets/cover.png" {
		t.Fatalf("unexpected image candidates: %#v", first.Candidates)
	}

	zipPath := filepath.Join(t.TempDir(), "skill.zip")
	writeTestFile(t, zipPath, first.Bytes, 0o644)
	fromZip, err := Build(zipPath, 1)
	if err != nil {
		t.Fatal(err)
	}
	if fromZip.Artifact.Digest != first.Artifact.Digest || fromZip.Digest != first.Digest {
		t.Fatalf("directory and canonical ZIP disagree: directory=%#v zip=%#v", first.Artifact, fromZip.Artifact)
	}
}

func TestBuildRejectsForbiddenZipPathsAndSecrets(t *testing.T) {
	t.Parallel()
	forbidden := zipBytes(t, map[string][]byte{
		"SKILL.md":              []byte(testSkillMarkdown),
		"node_modules/pkg/a.js": []byte("module.exports = 1"),
	})
	forbiddenPath := filepath.Join(t.TempDir(), "forbidden.zip")
	writeTestFile(t, forbiddenPath, forbidden, 0o644)
	assertOutputCode(t, buildError(forbiddenPath), "SKILL_FORBIDDEN_PATH")

	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "SKILL.md"), []byte(testSkillMarkdown), 0o644)
	writeTestFile(t, filepath.Join(directory, "notes.txt"), []byte("AWS_ACCESS_KEY_ID=AKIA1234567890ABCDEF"), 0o644)
	assertOutputCode(t, buildError(directory), "SKILL_SECRET_DETECTED")
}

func TestBuildRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not reliably available on Windows test hosts")
	}
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "SKILL.md"), []byte(testSkillMarkdown), 0o644)
	if err := os.Symlink("SKILL.md", filepath.Join(directory, "copy.md")); err != nil {
		t.Fatal(err)
	}
	assertOutputCode(t, buildError(directory), "SKILL_SYMLINK_REJECTED")
}

func buildError(source string) error {
	_, err := Build(source, 1)
	return err
}

func assertOutputCode(t *testing.T, err error, expected string) {
	t.Helper()
	var cliError *output.Error
	if !errors.As(err, &cliError) || cliError.Subtype != expected {
		t.Fatalf("expected %s, got %T: %v", expected, err, err)
	}
}

func zipBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func writeTestFile(t *testing.T, filename string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, mode); err != nil {
		t.Fatal(err)
	}
}
