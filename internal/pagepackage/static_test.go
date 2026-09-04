package pagepackage

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildWebsiteWorkPageUsesExistingStaticOutputWithoutRunningBuilds(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "dist", "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("<!doctype html><title>Replica</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "assets", "app.js"), []byte("console.log('static')"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "package.json"), []byte(`{"scripts":{"build":"exit 99"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	pkg, found, err := BuildWebsiteWorkPage(project, "Replica page")
	if err != nil || !found {
		t.Fatalf("existing static output was not packaged: found=%v err=%v", found, err)
	}
	if pkg.Manifest.Kind != "WorkPage" || pkg.Manifest.Spec.Entry != "dist/index.html" || pkg.Artifact.FileName != "page.zip" || pkg.Artifact.Digest == "" {
		t.Fatalf("unexpected hosted page package: %#v", pkg)
	}
	reader, err := zip.NewReader(bytes.NewReader(pkg.Bytes), int64(len(pkg.Bytes)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]string{}
	for _, entry := range reader.File {
		opened, openErr := entry.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		data, readErr := io.ReadAll(opened)
		_ = opened.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		entries[entry.Name] = string(data)
	}
	if entries["dist/index.html"] == "" || entries["dist/assets/app.js"] == "" || entries["viceme-page.json"] == "" {
		t.Fatalf("page package omitted required static files: %#v", entries)
	}
	if _, leaked := entries["package.json"]; leaked {
		t.Fatal("source-only package metadata leaked into the hosted page archive")
	}
}

func TestBuildWebsiteWorkPageRequiresReplicaOnlyForUnverifiableExternalResources(t *testing.T) {
	for name, document := range map[string]string{
		"quoted":            `<script src="https://cdn.example/app.js"></script>`,
		"unquoted":          `<script src=https://cdn.example/app.js></script>`,
		"protocol-relative": `<img src=//cdn.example/image.png>`,
	} {
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			if err := os.MkdirAll(filepath.Join(project, "dist"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte(document), 0o600); err != nil {
				t.Fatal(err)
			}

			_, found, err := BuildWebsiteWorkPage(project, "Replica page")
			if !found || err == nil {
				t.Fatalf("unverifiable external resource did not block automatic hosting: found=%v err=%v", found, err)
			}
		})
	}
}

func TestBuildWebsiteWorkPageReturnsNoCandidateWithoutStaticOutput(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "index.html"), []byte("source entry"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, found, err := BuildWebsiteWorkPage(project, "Replica page")
	if err != nil || found {
		t.Fatalf("source tree was mistaken for an existing static output: found=%v err=%v", found, err)
	}
}
