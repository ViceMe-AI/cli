package templatecatalog

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
