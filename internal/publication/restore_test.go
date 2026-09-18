package publication

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

const restoreAuthorBody = `---
name: restore-demo
description: Gated directory restore fixture.
---

# Restore Demo

Authored body.
`

// gatedFixture builds one channel-delivered directory from a pristine source
// directory: the gate entry replaces SKILL.md, the authored document moves to
// .viceme/trial-body.md, and the generated-file inventory covers every gate
// artifact that lives outside .viceme/.
func gatedFixture(t *testing.T, source string) string {
	t.Helper()
	gated := source + "-gated"
	if err := os.MkdirAll(filepath.Join(gated, ".viceme"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(gated, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(gated, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	gateEntry := `---
name: restore-demo
description: Gated directory restore fixture.
---
<!-- viceme-trial:v1 product=eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee -->

## 使用前必读

Generated gate section.

<!-- /viceme-trial:v1 -->
`
	files := map[string]string{
		"SKILL.md":                     gateEntry,
		".viceme/trial-body.md":        restoreAuthorBody,
		"references/viceme-runtime.md": "<!-- viceme-trial-runtime:v1 product=eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee -->\n",
		"scripts/run.sh":               "#!/bin/sh\necho demo\n",
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(gated, filepath.FromSlash(name)), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(gated, "scripts", "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	inventory := `{"schemaVersion":1,"productId":"eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee","kind":"trial","market":"global","files":{` +
		`"references/viceme-runtime.md":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",` +
		`".viceme/trial-body.md":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}`
	if err := os.WriteFile(filepath.Join(gated, ".viceme", "runtime.json"), []byte(inventory), 0o644); err != nil {
		t.Fatal(err)
	}
	return gated
}

func TestBuildRestoresGatedDirectoryToTheOriginalPackage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "skill")
	if err := os.MkdirAll(filepath.Join(source, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(restoreAuthorBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "scripts", "run.sh"), []byte("#!/bin/sh\necho demo\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pristine, err := Build(source)
	if err != nil {
		t.Fatal(err)
	}

	gated := gatedFixture(t, source)
	restored, err := Build(gated)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Artifact.Digest != pristine.Artifact.Digest {
		t.Fatalf("restored package does not equal the authored package: %s != %s", restored.Artifact.Digest, pristine.Artifact.Digest)
	}
	if restored.FileCount != pristine.FileCount {
		t.Fatalf("generated files leaked into the restored package: %d != %d", restored.FileCount, pristine.FileCount)
	}
	if restored.Manifest.Metadata.Title != pristine.Manifest.Metadata.Title {
		t.Fatalf("restored manifest lost the authored title: %#v", restored.Manifest.Metadata)
	}
	// The working directory itself stays untouched: the gate entry and the
	// generated reference are still on disk after the build.
	if skill, err := os.ReadFile(filepath.Join(gated, "SKILL.md")); err != nil || !strings.Contains(string(skill), skillcontent.TrialGateMarker) {
		t.Fatalf("build mutated the gated directory: %q %v", skill, err)
	}
	if _, err := os.Stat(filepath.Join(gated, skillcontent.TrialRuntimePath)); err != nil {
		t.Fatalf("build removed the generated runtime reference: %v", err)
	}
}

func TestBuildRefusesUnrecoverableGatedDirectories(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "skill")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(restoreAuthorBody), 0o644); err != nil {
		t.Fatal(err)
	}
	gated := gatedFixture(t, source)
	if err := os.Remove(filepath.Join(gated, ".viceme", "trial-body.md")); err != nil {
		t.Fatal(err)
	}
	_, err := Build(gated)
	if err == nil || output.AsError(err).Subtype != "SKILL_CHANNEL_RESTORE_INCOMPLETE" {
		t.Fatalf("gated directory without the trial body must fail closed: %v", err)
	}
}

func TestBuildRestoresDirectoryWhoseEntryStayedAuthored(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "skill")
	if err := os.MkdirAll(filepath.Join(source, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(restoreAuthorBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "scripts", "run.sh"), []byte("#!/bin/sh\necho demo\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pristine, err := Build(source)
	if err != nil {
		t.Fatal(err)
	}
	// Delivery stopped on a local conflict: the authored SKILL.md is still in
	// place while channel leftovers (runtime inventory + generated reference)
	// sit next to it. Publishing must still upload the authored package.
	gated := gatedFixture(t, source)
	if err := os.WriteFile(filepath.Join(gated, "SKILL.md"), []byte(restoreAuthorBody), 0o644); err != nil {
		t.Fatal(err)
	}
	restored, err := Build(gated)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Artifact.Digest != pristine.Artifact.Digest {
		t.Fatalf("authored entry with channel leftovers did not restore cleanly: %s != %s", restored.Artifact.Digest, pristine.Artifact.Digest)
	}
}

func TestBuildRejectsGatedArchive(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "skill")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(restoreAuthorBody), 0o644); err != nil {
		t.Fatal(err)
	}
	pristine, err := Build(source)
	if err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(root, "channel.zip")
	if err := os.WriteFile(zipPath, pristine.Bytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(zipPath); err != nil {
		t.Fatalf("pristine archive must build: %v", err)
	}
	// A channel ZIP carries the gate entry as SKILL.md; the archive reader
	// strips .viceme/, so publishing it would silently lose the authored
	// body. It must be refused with the directory workflow as guidance.
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(`---
name: restore-demo
description: Gated directory restore fixture.
---
<!-- viceme-trial:v1 product=eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee -->

## 使用前必读

Generated gate section.

<!-- /viceme-trial:v1 -->
`)); err != nil {
		t.Fatal(err)
	}
	script, err := writer.Create("scripts/run.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := script.Write([]byte("#!/bin/sh\necho demo\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	gatedZip := filepath.Join(root, "gated-channel.zip")
	if err := os.WriteFile(gatedZip, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Build(gatedZip)
	if err == nil || output.AsError(err).Subtype != "SKILL_CHANNEL_ARCHIVE_NOT_AUTHORABLE" {
		t.Fatalf("gated archive must be refused: %v", err)
	}
}
