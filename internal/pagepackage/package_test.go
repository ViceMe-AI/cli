package pagepackage

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
)

func TestInspectValidatesAndNormalizesPagePackage(t *testing.T) {
	archive := writePageZIP(t, map[string]string{
		"viceme-page.json": `{"apiVersion":"page.viceme.ai/v1alpha1","kind":"WorkPage","metadata":{"name":" Work page "},"spec":{"entry":"dist/index.html","sdkVersion":"1","capabilities":["work.like","context.read","work.like"]}}`,
		"dist/index.html":  "<!doctype html><html><head></head><body>Page</body></html>",
	})

	pkg, err := Inspect(archive)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.Metadata.Name != "Work page" || strings.Join(pkg.Manifest.Spec.Capabilities, ",") != "context.read,work.like" {
		t.Fatalf("manifest was not normalized: %#v", pkg.Manifest)
	}
	if pkg.FileCount != 2 || pkg.Artifact.ContentType != "application/zip" || len(pkg.Artifact.Digest) != 64 {
		t.Fatalf("package metadata is incomplete: %#v", pkg)
	}
}

func TestInspectRejectsUnsafePagePackages(t *testing.T) {
	tests := []struct {
		name    string
		entries map[string]string
		code    string
	}{
		{
			name: "traversal",
			entries: map[string]string{
				"viceme-page.json": validManifest("CreatorPage"),
				"dist/index.html":  "Page",
				"../secret.txt":    "secret",
			},
			code: "PAGE_PACKAGE_PATH_INVALID",
		},
		{
			name: "unknown manifest field",
			entries: map[string]string{
				"viceme-page.json": strings.Replace(validManifest("CreatorPage"), `"metadata"`, `"unknown":true,"metadata"`, 1),
				"dist/index.html":  "Page",
			},
			code: "PAGE_MANIFEST_INVALID",
		},
		{
			name: "missing capabilities",
			entries: map[string]string{
				"viceme-page.json": strings.Replace(validManifest("CreatorPage"), `,"capabilities":["context.read"]`, "", 1),
				"dist/index.html":  "Page",
			},
			code: "PAGE_MANIFEST_INVALID",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := Inspect(writePageZIP(t, testCase.entries))
			if err == nil || output.AsError(err).Subtype != testCase.code {
				t.Fatalf("got error %v, want %s", err, testCase.code)
			}
		})
	}
}

func TestInspectAllowsOrdinaryBrowserFeaturesAndFlexibleLayouts(t *testing.T) {
	manifest := strings.Replace(validManifest("CreatorPage"), `dist/index.html`, `首页.html`, 1)
	archive := writePageZIP(t, map[string]string{
		"viceme-page.json":   manifest,
		"首页.html":            `<iframe src="https://example.com"></iframe><script src="https://cdn.example.com/app.js"></script>`,
		"legal/privacy.html": "<h1>Privacy</h1>",
		"assets/app.js":      `const access_token = "demo-value"; fetch("https://api.example.com/data")`,
		"assets/model.bin":   "binary-like-content",
		".env.example":       "PUBLIC_API_BASE=https://api.example.com",
	})

	pkg, err := Inspect(archive)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.Spec.Entry != "首页.html" || pkg.FileCount != 6 {
		t.Fatalf("flexible package was not preserved: %#v", pkg)
	}
}

func writePageZIP(t *testing.T, entries map[string]string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "page.zip")
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for name, value := range entries {
		entry, createErr := archive.Create(name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := entry.Write([]byte(value)); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return filename
}

func validManifest(kind string) string {
	return `{"apiVersion":"page.viceme.ai/v1alpha1","kind":"` + kind + `","metadata":{"name":"Page"},"spec":{"entry":"dist/index.html","sdkVersion":"1","capabilities":["context.read"]}}`
}
