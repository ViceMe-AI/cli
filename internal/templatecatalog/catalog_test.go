package templatecatalog

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/commerceartifact"
)

func TestLoadSourceCatalogRejectsDevMockAndDuplicateVersion(t *testing.T) {
	t.Parallel()

	_, err := LoadSourceCatalog(strings.NewReader(`{
  "schema_version": 1,
  "templates": [
    {
      "id": "bonjour-card",
      "status": "production",
      "version": "1.0.0",
      "name": "Bonjour Card",
      "scenario": "作品、资料与公开联系方式",
      "description": "个人名片",
      "source_dir": "a",
      "preview_file": "b",
      "license": "ViceMe template license"
    },
    {
      "id": "bonjour-card",
      "status": "dev_mock",
      "version": "1.0.0",
      "name": "Bonjour Card",
      "scenario": "作品、资料与公开联系方式",
      "description": "个人名片",
      "source_dir": "a",
      "preview_file": "b",
      "license": "ViceMe template license"
    }
  ]
}`))
	if !errors.Is(err, ErrInvalidSourceCatalog) {
		t.Fatalf("LoadSourceCatalog() error = %v, want ErrInvalidSourceCatalog", err)
	}
}

func TestSafeRelativePathRejectsURLSyntax(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"https://example.com/source.zip",
		"//example.com/source.zip",
		"releases/source.zip?download=1",
		"releases/source.zip#fragment",
		"releases/%2e%2e/source.zip",
		`..\outside\source.zip`,
	} {
		if safeRelativePath(value) {
			t.Fatalf("safeRelativePath(%q) = true", value)
		}
	}
	if !safeRelativePath("releases/bonjour-card/1.0.0/source.zip") {
		t.Fatal("expected canonical release path to be accepted")
	}
}

func TestBuildRejectsCatalogSymlinksOutsideSourceRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	external := t.TempDir()
	if err := os.MkdirAll(filepath.Join(external, "source"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "source", "index.html"), []byte("external source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "preview.html"), []byte("external preview"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "local-source"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "local-source", "index.html"), []byte("local source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "local-preview.html"), []byte("local preview"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(external, "preview.html"), filepath.Join(root, "linked-preview.html")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(external, filepath.Join(root, "linked-parent")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer := Signer{KeyID: "test-v1", PrivateKey: privateKey}
	base := SourceTemplate{
		ID: "bonjour-card", Status: "production", Version: "1.0.0", Name: "Bonjour Card",
		Scenario: "works", Description: "profile", SourceDir: "local-source", PreviewFile: "local-preview.html", License: "ViceMe template license",
	}
	for _, test := range []struct {
		name        string
		sourceDir   string
		previewDir  string
		previewFile string
	}{
		{name: "preview file", sourceDir: base.SourceDir, previewFile: "linked-preview.html"},
		{name: "preview directory parent", sourceDir: base.SourceDir, previewDir: "linked-parent/source"},
		{name: "source parent", sourceDir: "linked-parent/source", previewFile: base.PreviewFile},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := base
			entry.SourceDir, entry.PreviewDir, entry.PreviewFile = test.sourceDir, test.previewDir, test.previewFile
			_, err := Build(root, SourceCatalog{SchemaVersion: 1, Templates: []SourceTemplate{entry}}, filepath.Join(root, "output-"+strings.ReplaceAll(test.name, " ", "-")), "https://s3.viceme.cn/templates", signer)
			if !errors.Is(err, ErrBuild) {
				t.Fatalf("Build() error = %v, want ErrBuild", err)
			}
		})
	}
}

func TestBuildWritesDeterministicZipAndManifestDigest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "source", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "src", "main.js"), []byte("export default 'bonjour'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "preview.html"), []byte("<h1>Bonjour</h1>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	catalog := SourceCatalog{SchemaVersion: 1, Templates: []SourceTemplate{{
		ID: "bonjour-card", Status: "production", Version: "1.0.0", Name: "Bonjour Card",
		Scenario: "作品", Description: "个人名片", SourceDir: "source", PreviewFile: "preview.html", License: "ViceMe template license",
	}}}

	first, err := Build(root, catalog, filepath.Join(root, "first"), "https://s3.viceme.cn/templates", Signer{KeyID: "test-v1", PrivateKey: privateKey})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(root, catalog, filepath.Join(root, "second"), "https://s3.viceme.cn/templates", Signer{KeyID: "test-v1", PrivateKey: privateKey})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.SourceZIP, second.SourceZIP) {
		t.Fatal("source ZIP bytes changed between identical builds")
	}
	wantDigest := sha256.Sum256(first.SourceZIP)
	if got := first.Manifest.Templates[0].SourceSHA256; got != "sha256:"+hex.EncodeToString(wantDigest[:]) {
		t.Fatalf("source digest = %q", got)
	}
	for _, artifact := range []string{
		"index.html",
		"manifest.json",
		"manifest.sig",
		"releases/bonjour-card/1.0.0/preview/index.html",
		"releases/bonjour-card/1.0.0/source.zip",
	} {
		if _, err := os.Stat(filepath.Join(root, "first", artifact)); err != nil {
			t.Fatalf("missing artifact %s: %v", artifact, err)
		}
	}
}

func TestBuildPublishesCompletePreviewDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "source"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "index.html"), []byte("<h1>Source</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "source", "node_modules", "vite"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "node_modules", "vite", "package.json"), []byte(`{"private":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "source", "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "dist", "index.html"), []byte("<h1>Stale build</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "preview", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "preview", "index.html"), []byte(`<script src="./assets/main.js"></script>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "preview", "assets", "main.js"), []byte("console.log('bonjour')"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	catalog := SourceCatalog{SchemaVersion: 1, Templates: []SourceTemplate{{
		ID: "bonjour-card", Status: "production", Version: "1.0.1", Name: "Bonjour Card",
		Scenario: "作品", Description: "个人名片", SourceDir: "source", PreviewDir: "preview", License: "ViceMe template license",
	}}}

	output := filepath.Join(root, "output")
	if _, err := Build(root, catalog, output, "https://s3.viceme.cn/templates", Signer{KeyID: "test-v1", PrivateKey: privateKey}); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []string{
		"releases/bonjour-card/1.0.1/preview/index.html",
		"releases/bonjour-card/1.0.1/preview/assets/main.js",
	} {
		body, err := os.ReadFile(filepath.Join(output, artifact))
		if err != nil {
			t.Fatalf("missing preview artifact %s: %v", artifact, err)
		}
		if len(body) == 0 {
			t.Fatalf("preview artifact %s is empty", artifact)
		}
	}
	reader, err := zip.OpenReader(filepath.Join(output, "releases", "bonjour-card", "1.0.1", "source.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if strings.Contains(entry.Name, "/node_modules/") || strings.Contains(entry.Name, "/dist/") {
			t.Fatalf("source ZIP includes transient build artifact %q", entry.Name)
		}
	}
}

func TestBuildIsByteIdenticalAcrossPublicOrigins(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "source"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "index.html"), []byte("<h1>Bonjour</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "preview.html"), []byte("<h1>Preview</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	catalog := SourceCatalog{SchemaVersion: 1, Templates: []SourceTemplate{{
		ID: "bonjour-card", Status: "production", Version: "1.0.0", Name: "Bonjour Card",
		Scenario: "作品", Description: "个人名片", SourceDir: "source", PreviewFile: "preview.html", License: "ViceMe template license",
	}}}
	cn := filepath.Join(root, "cn")
	global := filepath.Join(root, "global")
	if _, err := Build(root, catalog, cn, "https://s3.viceme.cn/templates", Signer{KeyID: "test-v1", PrivateKey: privateKey}); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(root, catalog, global, "https://s3.viceme.ai/templates", Signer{KeyID: "test-v1", PrivateKey: privateKey}); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []string{"index.html", "manifest.json", "manifest.sig", "releases/bonjour-card/1.0.0/preview/index.html", "releases/bonjour-card/1.0.0/source.zip"} {
		cnBody, err := os.ReadFile(filepath.Join(cn, artifact))
		if err != nil {
			t.Fatal(err)
		}
		globalBody, err := os.ReadFile(filepath.Join(global, artifact))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(cnBody, globalBody) {
			t.Fatalf("%s differs between CN and Global builds", artifact)
		}
	}
}

func TestBuildSignsCanonicalManifest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "source"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "index.html"), []byte("<h1>Bonjour</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "preview.html"), []byte("<h1>Preview</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	catalog := SourceCatalog{SchemaVersion: 1, Templates: []SourceTemplate{{
		ID: "bonjour-card", Status: "production", Version: "1.0.0", Name: "Bonjour Card",
		Scenario: "作品", Description: "个人名片", SourceDir: "source", PreviewFile: "preview.html", License: "ViceMe template license",
	}}}
	output := filepath.Join(root, "output")
	if _, err := Build(root, catalog, output, "https://s3.viceme.cn/templates", Signer{KeyID: "test-v1", PrivateKey: privateKey}); err != nil {
		t.Fatal(err)
	}
	manifestBody, err := os.ReadFile(filepath.Join(output, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		t.Fatal(err)
	}
	signatureBody, err := os.ReadFile(filepath.Join(output, "manifest.sig"))
	if err != nil {
		t.Fatal(err)
	}
	var signature Signature
	if err := json.Unmarshal(signatureBody, &signature); err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := commerceartifact.VerifyDetachedDocument(manifest, base64.RawURLEncoding.EncodeToString(publicDER), signature.Signature); err != nil {
		t.Fatalf("catalog signature did not verify: %v", err)
	}
}

func TestBuildForLocalDemoAllowsOnlyLoopbackHTTPOrigin(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "source"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "index.html"), []byte("<h1>Bonjour</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "preview.html"), []byte("<h1>Preview</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	catalog := SourceCatalog{SchemaVersion: 1, Templates: []SourceTemplate{{
		ID: "bonjour-card", Status: "production", Version: "1.0.0", Name: "Bonjour Card",
		Scenario: "作品", Description: "个人名片", SourceDir: "source", PreviewFile: "preview.html", License: "ViceMe template license",
	}}}
	if _, err := BuildForLocalDemo(root, catalog, filepath.Join(root, "local"), "http://127.0.0.1:8765/templates", Signer{KeyID: "test-v1", PrivateKey: privateKey}); err != nil {
		t.Fatalf("loopback local demo build failed: %v", err)
	}
	if _, err := BuildForLocalDemo(root, catalog, filepath.Join(root, "unsafe"), "http://example.com/templates", Signer{KeyID: "test-v1", PrivateKey: privateKey}); !errors.Is(err, ErrBuild) {
		t.Fatalf("non-loopback local demo origin error = %v, want ErrBuild", err)
	}
}
