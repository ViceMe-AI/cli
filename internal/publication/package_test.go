package publication

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	writeTestFile(t, filepath.Join(directory, ".viceme", "skill.json"), []byte(`{"binding":"local"}`), 0o600)
	writeTestFile(t, filepath.Join(directory, "node_modules", "pkg", "index.js"), []byte("ignored"), 0o644)
	writeTestFile(t, filepath.Join(directory, ".env.local"), []byte("TOKEN=ignored-locally"), 0o600)
	writeTestFile(t, filepath.Join(directory, ".vicemeignore"), []byte("generated/**\n"), 0o644)
	writeTestFile(t, filepath.Join(directory, "generated", "cache.json"), []byte("ignored"), 0o644)

	first, err := Build(directory)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(directory)
	if err != nil {
		t.Fatal(err)
	}
	if first.Artifact.Digest != second.Artifact.Digest || !bytes.Equal(first.Bytes, second.Bytes) {
		t.Fatal("identical inputs did not produce an identical deterministic ZIP")
	}
	if first.Manifest.Spec.Sale.PriceMinor != nil || first.Manifest.Spec.Sale.Currency != "CNY" {
		t.Fatalf("unexpected sale manifest: %#v", first.Manifest.Spec.Sale)
	}
	if len(first.Candidates) != 1 || first.Candidates[0].RelativePath != "assets/cover.png" {
		t.Fatalf("unexpected image candidates: %#v", first.Candidates)
	}
	if first.FileCount != 3 {
		t.Fatalf("workspace metadata, dependencies, environment, or ignored files entered the package: %#v", first)
	}

	zipPath := filepath.Join(t.TempDir(), "skill.zip")
	writeTestFile(t, zipPath, first.Bytes, 0o644)
	fromZip, err := Build(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if fromZip.Artifact.Digest != first.Artifact.Digest {
		t.Fatalf("directory and canonical ZIP disagree: directory=%#v zip=%#v", first.Artifact, fromZip.Artifact)
	}
	if first.Manifest.Spec.Source.Type != "WORKSPACE" || fromZip.Manifest.Spec.Source.Type != "ZIP" || first.Digest == fromZip.Digest {
		t.Fatalf("source provenance was not represented in the manifest: directory=%#v zip=%#v", first.Manifest.Spec.Source, fromZip.Manifest.Spec.Source)
	}
}

func TestBuildUnwrapsSingleZipRootDirectory(t *testing.T) {
	t.Parallel()
	image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "SKILL.md"), []byte(testSkillMarkdown), 0o644)
	writeTestFile(t, filepath.Join(directory, "scripts", "run.sh"), []byte("#!/bin/sh\necho ok\n"), 0o644)
	writeTestFile(t, filepath.Join(directory, "assets", "cover.png"), image, 0o644)

	canonical, err := Build(directory)
	if err != nil {
		t.Fatal(err)
	}

	wrapperZIP := zipBytes(t, map[string][]byte{
		"poster-skill-main/SKILL.md":         []byte(testSkillMarkdown),
		"poster-skill-main/assets/cover.png": image,
		"poster-skill-main/scripts/run.sh":   []byte("#!/bin/sh\necho ok\n"),
	})
	zipPath := filepath.Join(t.TempDir(), "github-download.zip")
	writeTestFile(t, zipPath, wrapperZIP, 0o644)

	wrapped, err := Build(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if wrapped.Artifact.Digest != canonical.Artifact.Digest {
		t.Fatalf("wrapped ZIP and canonical directory disagree: wrapped=%#v canonical=%#v", wrapped.Artifact, canonical.Artifact)
	}
	if len(wrapped.Candidates) != 1 || wrapped.Candidates[0].RelativePath != "assets/cover.png" {
		t.Fatalf("wrapped ZIP candidate path was not normalized: %#v", wrapped.Candidates)
	}

	reader, err := zip.NewReader(bytes.NewReader(wrapped.Bytes), int64(len(wrapped.Bytes)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, "poster-skill-main/") {
			t.Fatalf("deterministic artifact retained source archive wrapper: %s", file.Name)
		}
	}
}

func TestBuildDoesNotUnwrapAmbiguousOrNestedZipRoots(t *testing.T) {
	t.Parallel()
	tests := map[string]map[string][]byte{
		"multiple roots": {
			"poster-skill-main/SKILL.md": []byte(testSkillMarkdown),
			"other-root/readme.md":       []byte("other"),
		},
		"root file alongside wrapper": {
			"poster-skill-main/SKILL.md": []byte(testSkillMarkdown),
			"readme.md":                  []byte("root"),
		},
		"manifest below wrapper root": {
			"poster-skill-main/nested/SKILL.md": []byte(testSkillMarkdown),
		},
	}

	for name, files := range tests {
		name, files := name, files
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			zipPath := filepath.Join(t.TempDir(), "invalid.zip")
			writeTestFile(t, zipPath, zipBytes(t, files), 0o644)
			assertOutputCode(t, buildError(zipPath), "SKILL_MANIFEST_MISSING")
		})
	}
}

func TestBuildArchiveSubpathSelectsSkillInsideGithubWrapper(t *testing.T) {
	t.Parallel()
	archive := zipBytes(t, map[string][]byte{
		"repository-main/README.md":                      []byte("repository"),
		"repository-main/packages/poster/SKILL.md":       []byte(testSkillMarkdown),
		"repository-main/packages/poster/scripts/run.sh": []byte("#!/bin/sh\necho poster\n"),
		"repository-main/packages/other/notes.md":        []byte("not selected"),
	})
	archivePath := filepath.Join(t.TempDir(), "github.zip")
	writeTestFile(t, archivePath, archive, 0o644)

	result, err := BuildArchiveSubpath(archivePath, "packages/poster")
	if err != nil {
		t.Fatal(err)
	}
	if result.FileCount != 2 {
		t.Fatalf("GitHub subpath included repository siblings: %#v", result)
	}
	reader, err := zip.NewReader(bytes.NewReader(result.Bytes), int64(len(result.Bytes)))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	if strings.Join(names, ",") != "SKILL.md,scripts/run.sh" {
		t.Fatalf("unexpected selected archive paths: %v", names)
	}
	assertOutputCode(t, buildArchiveSubpathError(archivePath, "../poster"), "GITHUB_PATH_INVALID")
}

func TestBuildRemoteArchivePreservesReceiptBoundBytes(t *testing.T) {
	t.Parallel()
	archive := zipBytes(t, map[string][]byte{
		"SKILL.md":       []byte(testSkillMarkdown),
		"scripts/run.sh": []byte("#!/bin/sh\necho remote\n"),
	})
	archivePath := filepath.Join(t.TempDir(), "remote.zip")
	writeTestFile(t, archivePath, archive, 0o644)

	result, err := BuildRemoteArchive(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Bytes, archive) {
		t.Fatal("remote archive bytes were rewritten after receipt issuance")
	}
	if result.Artifact.Digest != sha256Hex(archive) {
		t.Fatalf("unexpected remote digest: %s", result.Artifact.Digest)
	}
}

func TestBuildExcludesRuntimePathsAndRejectsSecrets(t *testing.T) {
	t.Parallel()
	forbidden := zipBytes(t, map[string][]byte{
		"SKILL.md":              []byte(testSkillMarkdown),
		"node_modules/pkg/a.js": []byte("module.exports = 1"),
	})
	forbiddenPath := filepath.Join(t.TempDir(), "forbidden.zip")
	writeTestFile(t, forbiddenPath, forbidden, 0o644)
	result, err := Build(forbiddenPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.FileCount != 1 {
		t.Fatalf("runtime dependency paths entered the canonical package: %#v", result)
	}

	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "SKILL.md"), []byte(testSkillMarkdown), 0o644)
	writeTestFile(t, filepath.Join(directory, "notes.txt"), []byte("AWS_ACCESS_KEY_ID=AKIA1234567890ABCDEF"), 0o644)
	assertOutputCode(t, buildError(directory), "SKILL_SECRET_DETECTED")
}

func TestBuildSanitizesMacOSMetadataAndAllowsDocumentedCredentialPlaceholders(t *testing.T) {
	t.Parallel()
	original := zipBytes(t, map[string][]byte{
		"SKILL.md":            []byte(testSkillMarkdown),
		"README.md":           []byte("API_KEY=your-api-key-here\nCLIENT_SECRET=your-client-secret-here\n"),
		".DS_Store":           []byte("finder metadata"),
		"__MACOSX/._SKILL.md": []byte("apple double metadata"),
		"assets/._cover.png":  []byte("apple double metadata"),
	})
	zipPath := filepath.Join(t.TempDir(), "macos-skill.zip")
	writeTestFile(t, zipPath, original, 0o644)

	result, err := Build(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.FileCount != 2 {
		t.Fatalf("macOS metadata entered the canonical package: %#v", result)
	}
	sourceAfter, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sourceAfter, original) {
		t.Fatal("source ZIP was modified while producing the sanitized artifact")
	}
}

func TestBuildRejectsCredentialAssignmentsThatAreNotKnownPlaceholders(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "SKILL.md"), []byte(testSkillMarkdown), 0o644)
	writeTestFile(t, filepath.Join(directory, "config.md"), []byte("API_KEY=real-secret-value-1234567890"), 0o644)
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
	_, err := Build(source)
	return err
}

func buildArchiveSubpathError(source, subpath string) error {
	_, err := BuildArchiveSubpath(source, subpath)
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
