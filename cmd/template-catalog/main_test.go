package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestRunAllowsLoopbackHTTPOnlyWithExplicitDemoFlag(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "template"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "template", "index.html"), []byte("<h1>Bonjour</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "preview.html"), []byte("<h1>Preview</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo-preview.html"), []byte("<h1>Demo preview</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(root, "production.json")
	if err := os.WriteFile(sourcePath, []byte(`{"schema_version":1,"templates":[{"id":"bonjour-card","status":"production","version":"1.0.0","name":"Bonjour Card","scenario":"作品","description":"个人名片","source_dir":"template","preview_file":"preview.html","license":"ViceMe template license"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	demoPath := filepath.Join(root, "demo.json")
	if err := os.WriteFile(demoPath, []byte(`{"schema_version":1,"templates":[{"id":"demo","name":"Demo","scenario":"演示","description":"演示卡片","preview_file":"demo-preview.html"}]}`), 0o644); err != nil {
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
	if err := run([]string{
		"--source", sourcePath, "--demo-source", demoPath, "--root", root, "--output", filepath.Join(root, "output"),
		"--origin", "http://127.0.0.1:8765/templates", "--signing-key-file", keyPath, "--allow-insecure-origin",
	}); err != nil {
		t.Fatalf("loopback local demo build failed: %v", err)
	}
}

func TestRunBuildsComingSoonDemoCardsForLoopbackCatalog(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "template"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "template", "index.html"), []byte("<h1>Bonjour</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "preview.html"), []byte("<h1>Bonjour preview</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "maker-preview.html"), []byte("<h1>Maker preview</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	productionPath := filepath.Join(root, "production.json")
	if err := os.WriteFile(productionPath, []byte(`{"schema_version":1,"templates":[{"id":"bonjour-card","status":"production","version":"1.0.0","name":"Bonjour Card","scenario":"作品","description":"个人名片","source_dir":"template","preview_file":"preview.html","license":"ViceMe template license"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	demoPath := filepath.Join(root, "demo.json")
	if err := os.WriteFile(demoPath, []byte(`{"schema_version":1,"templates":[{"id":"maker-portfolio","name":"Maker Portfolio","scenario":"产品与独立开发","description":"以项目成果为主线的作品集布局。","preview_file":"maker-preview.html"}]}`), 0o644); err != nil {
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
		"--source", productionPath, "--demo-source", demoPath, "--root", root, "--output", output,
		"--origin", "http://127.0.0.1:8765/templates", "--signing-key-file", keyPath, "--allow-insecure-origin",
	}); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(output, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Maker Portfolio", "即将上线", "查看效果", "下载模板源码", "使用此模板", "请返回 Agent 继续制作", "aria-disabled=\"true\""} {
		if !strings.Contains(string(index), expected) {
			t.Fatalf("catalog page omitted %q: %s", expected, index)
		}
	}
	if _, err := os.Stat(filepath.Join(output, "demo", "maker-portfolio", "preview", "index.html")); err != nil {
		t.Fatalf("demo preview missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "releases", "maker-portfolio", "source.zip")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("demo template must not publish source ZIP: %v", err)
	}
}

func TestWorkflowPublishesSignedCatalogToBothRegions(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "template-catalog.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"CATALOG_BUCKET='templates'",
		"KEY_PREFIX='dev/'",
		"s3api put-bucket-policy",
		"s3:GetObject",
		"s3api list-objects-v2",
		"list-type=2",
		"bonjour-card/public/viceme-page.json",
		"VICEME_TEMPLATE_CATALOG_SIGNING_KEY",
		"VICEME_RELEASE_S3_ENDPOINT_CN",
		"VICEME_RELEASE_S3_ENDPOINT_GLOBAL",
		"manifest.sig",
		"sha256sum",
	} {
		if !strings.Contains(string(body), required) {
			t.Fatalf("catalog workflow omitted %q", required)
		}
	}
	for _, forbidden := range []string{"CN_BUCKET:", "GLOBAL_BUCKET:"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("catalog workflow must not reuse release bucket setting %q", forbidden)
		}
	}
}
