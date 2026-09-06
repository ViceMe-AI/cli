package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestRunBuildsCatalogArtifacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "template")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "index.html"), []byte("<h1>Bonjour</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "preview.html"), []byte("<h1>Preview</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := `{"schema_version":1,"templates":[{"id":"bonjour-card","status":"production","version":"1.0.0","name":"Bonjour Card","scenario":"作品","description":"个人名片","source_dir":"template","preview_file":"preview.html","license":"ViceMe template license"}]}`
	sourcePath := filepath.Join(root, "production.json")
	if err := os.WriteFile(sourcePath, []byte(catalog), 0o644); err != nil {
		t.Fatal(err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(root, "signing-key")
	if err := os.WriteFile(keyPath, []byte(base64.RawURLEncoding.EncodeToString(der)), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "output")

	if err := run([]string{
		"--source", sourcePath,
		"--root", root,
		"--output", output,
		"--origin", "https://s3.viceme.cn/templates/dev",
		"--signing-key-file", keyPath,
		"--key-id", "test-v1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(output, "releases", "bonjour-card", "1.0.0", "source.zip")); err != nil {
		t.Fatalf("source ZIP missing: %v", err)
	}
}
