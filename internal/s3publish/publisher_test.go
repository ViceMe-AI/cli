package s3publish

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHostedContentType(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"use-a-skill.zip":          "application/zip",
		"manifest.json":            "application/json; charset=utf-8",
		"SKILL.md":                 markdownType,
		"notes.markdown":           markdownType,
		"readme.txt":               "text/plain; charset=utf-8",
		"scripts/make_copy.py":     "text/x-python; charset=utf-8",
		"scripts/resolve-cli.sh":   "text/x-shellscript; charset=utf-8",
		"agents/openai.yaml":       "text/yaml; charset=utf-8",
		"config.yml":               "text/yaml; charset=utf-8",
		"pyproject.toml":           "application/toml; charset=utf-8",
		"page.html":                "text/html; charset=utf-8",
		"index.htm":                "text/html; charset=utf-8",
		"scripts/preflight.mjs":    "text/javascript; charset=utf-8",
		"app.js":                   "text/javascript; charset=utf-8",
		"src/styles.css":           "text/css; charset=utf-8",
		"icon.svg":                 "image/svg+xml",
		"trial-runtime.bin":        "",
		"viceme_0.1.0_linux_amd64": "",
	}
	for name, want := range cases {
		if got := HostedContentType(name); got != want {
			t.Errorf("%s: got %q want %q", name, got, want)
		}
	}
}

func TestHTTPClientProxyIsolation(t *testing.T) {
	t.Parallel()
	cn, err := httpClientFor("http://127.0.0.1:8888", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	global, err := httpClientFor("", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	cnReq, _ := http.NewRequest(http.MethodGet, "https://s3.viceme.cn/start", nil)
	proxy, err := cn.Transport.(*http.Transport).Proxy(cnReq)
	if err != nil || proxy == nil || proxy.Host != "127.0.0.1:8888" {
		t.Fatalf("CN client must use the explicit proxy, got %#v err=%v", proxy, err)
	}
	globalReq, _ := http.NewRequest(http.MethodGet, "https://s3.viceme.ai/start", nil)
	proxy, err = global.Transport.(*http.Transport).Proxy(globalReq)
	if err != nil || proxy != nil {
		t.Fatalf("Global client must not proxy, got %#v err=%v", proxy, err)
	}
}

func TestInvalidProxyURL(t *testing.T) {
	t.Parallel()
	if _, err := httpClientFor("://bad", time.Second); err == nil {
		t.Fatal("expected invalid proxy URL to fail")
	}
	if _, err := httpClientFor("not-a-url", time.Second); err == nil {
		t.Fatal("expected invalid proxy URL to fail")
	}
}

func TestPublishFirstRelease(t *testing.T) {
	dist := writeDist(t)
	cn, global := newFakeS3(), newFakeS3()
	defer cn.close()
	defer global.close()
	var logs bytes.Buffer
	cfg := testConfig(dist, cn, global, &logs)
	if err := Publish(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	body, ok := cn.get("start", "cli/releases/v0.42.0/agent-install.md")
	if !ok || string(body.Body) != "install-doc\n" {
		t.Fatalf("missing versioned agent-install.md: %#v", body)
	}
	if body.CacheControl != cacheImmutable {
		t.Fatalf("versioned cache-control=%q", body.CacheControl)
	}
	if body.ContentType != markdownType {
		t.Fatalf("versioned markdown type=%q", body.ContentType)
	}
	latest, ok := cn.get("start", "cli/releases/latest")
	if !ok || string(latest.Body) != "0.42.0\n" {
		t.Fatalf("latest pointer=%q present=%t", latest.Body, ok)
	}
	root, ok := cn.get("start", "agent-install.md")
	if !ok || string(root.Body) != "install-doc\n" {
		t.Fatal("root agent-install.md missing")
	}
	if root.CacheControl != cacheStable || root.ContentType != markdownType {
		t.Fatalf("root headers cache=%q type=%q", root.CacheControl, root.ContentType)
	}
	manifest, ok := cn.get("skills", "manifest.json")
	if !ok || string(manifest.Body) != `{"schema_version":1}`+"\n" {
		t.Fatal("stable skills manifest missing")
	}
	zip, ok := global.get("skills", "demo-skill.zip")
	if !ok || zip.ContentType != "application/zip" {
		t.Fatalf("hosted zip content-type=%q present=%t", zip.ContentType, ok)
	}
	unknown, ok := cn.get("start", "cli/releases/v0.42.0/skills/demo.bin")
	if !ok {
		t.Fatal("unknown extension object missing")
	}
	if unknown.ContentType != "" && unknown.ContentType != "application/octet-stream" && unknown.ContentType != "binary/octet-stream" {
		t.Fatalf("unknown extension must not invent a text content-type: %q", unknown.ContentType)
	}
	if _, ok := cn.get("start", "policy-probe/run-1"); ok {
		t.Fatal("policy probe object must be deleted")
	}
	if !strings.Contains(logs.String(), "Published CN release v0.42.0") || !strings.Contains(logs.String(), "Published GLOBAL release v0.42.0") {
		t.Fatalf("missing published lines: %s", logs.String())
	}
	if !strings.Contains(logs.String(), " put ") {
		t.Fatal("expected put progress logs")
	}
}

func TestPublishRecoveryIdenticalSkipsVersionedPuts(t *testing.T) {
	dist := writeDist(t)
	cn, global := newFakeS3(), newFakeS3()
	defer cn.close()
	defer global.close()
	seedVersionTree(cn, dist)
	seedVersionTree(global, dist)
	if err := Publish(context.Background(), testConfig(dist, cn, global, ioDiscard())); err != nil {
		t.Fatal(err)
	}
	if cn.putCount("start", "cli/releases/v0.42.0/agent-install.md") != 0 {
		t.Fatalf("identical recovery put versioned object %d times", cn.putCount("start", "cli/releases/v0.42.0/agent-install.md"))
	}
	if global.putCount("start", "cli/releases/v0.42.0/agent-install.md") != 0 {
		t.Fatal("global identical recovery rewrote a versioned object")
	}
}

func TestPublishRecoveryConflict(t *testing.T) {
	dist := writeDist(t)
	cn, global := newFakeS3(), newFakeS3()
	defer cn.close()
	defer global.close()
	cn.seed("start", "cli/releases/v0.42.0/agent-install.md", []byte("tampered"), cacheImmutable, markdownType)
	err := Publish(context.Background(), testConfig(dist, cn, global, ioDiscard()))
	if err == nil {
		t.Fatal("expected immutable conflict")
	}
	message := err.Error()
	if !strings.Contains(message, "cli/releases/v0.42.0/agent-install.md") {
		t.Fatalf("error must name the object key: %s", message)
	}
	if strings.Contains(message, "cn-secret") || strings.Contains(message, "cn-key") || strings.Contains(message, "http://") {
		t.Fatalf("error leaked a credential or endpoint: %s", message)
	}
}

func TestPublishDoesNotPromoteOlderVersion(t *testing.T) {
	dist := writeDist(t)
	cn, global := newFakeS3(), newFakeS3()
	defer cn.close()
	defer global.close()
	cn.seed("start", "cli/releases/latest", []byte("9.9.9\n"), cacheLatest, plainType)
	global.seed("start", "cli/releases/latest", []byte("9.9.9\n"), cacheLatest, plainType)
	cn.seed("start", "agent-install.md", []byte("previous-root\n"), cacheStable, markdownType)
	cn.seed("start", "commerce-skill-install.md", []byte("previous-commerce\n"), cacheStable, markdownType)
	global.seed("start", "agent-install.md", []byte("previous-root\n"), cacheStable, markdownType)
	global.seed("start", "commerce-skill-install.md", []byte("previous-commerce\n"), cacheStable, markdownType)
	if err := Publish(context.Background(), testConfig(dist, cn, global, ioDiscard())); err != nil {
		t.Fatal(err)
	}
	latest, _ := cn.get("start", "cli/releases/latest")
	if string(latest.Body) != "9.9.9\n" {
		t.Fatalf("older release moved latest: %q", latest.Body)
	}
	root, _ := cn.get("start", "agent-install.md")
	if string(root.Body) != "previous-root\n" {
		t.Fatalf("older release rewrote root pointer: %q", root.Body)
	}
	if _, ok := cn.get("skills", "manifest.json"); ok {
		t.Fatal("older release must not overwrite the stable skills bucket")
	}
	versioned, ok := cn.get("start", "cli/releases/v0.42.0/agent-install.md")
	if !ok || string(versioned.Body) != "install-doc\n" {
		t.Fatal("older release still publishes its immutable version tree")
	}
}

func TestPublishListDeniedFallsBackToHead(t *testing.T) {
	dist := writeDist(t)
	cn, global := newFakeS3(), newFakeS3()
	defer cn.close()
	defer global.close()
	cn.denyList = true
	global.denyList = true
	if err := Publish(context.Background(), testConfig(dist, cn, global, ioDiscard())); err != nil {
		t.Fatal(err)
	}
	if cn.heads.Load() == 0 {
		t.Fatal("list denial must fall back to HeadObject")
	}
	if _, ok := cn.get("start", "cli/releases/v0.42.0/agent-install.md"); !ok {
		t.Fatal("list denial still has to publish the version tree")
	}
}

func TestPublishConcurrentImmutableUsesPerObjectBuffers(t *testing.T) {
	dist := writeDist(t)
	files := []string{"one.md", "two.md", "three.md", "four.md", "five.md", "six.md", "seven.md", "eight.md"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dist, name), []byte(name+"-body\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cn, global := newFakeS3(), newFakeS3()
	defer cn.close()
	defer global.close()
	seedVersionTree(cn, dist)
	seedVersionTree(global, dist)
	cfg := testConfig(dist, cn, global, ioDiscard())
	cfg.Concurrency = 8
	if err := Publish(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		obj, ok := cn.get("start", "cli/releases/v0.42.0/"+name)
		if !ok || string(obj.Body) != name+"-body\n" {
			t.Fatalf("concurrent immutable compare mixed buffers for %s: %#v", name, obj)
		}
	}
}

func testConfig(dist string, cn, global *fakeS3, log *bytes.Buffer) Config {
	return Config{
		DistDir:     dist,
		Version:     "0.42.0",
		RunID:       "run-1",
		Concurrency: 8,
		Log:         log,
		Regions: []Region{
			{
				Label:        "CN",
				Endpoint:     cn.endpoint(),
				Bucket:       "start",
				AccessKey:    "cn-key",
				SecretKey:    "cn-secret",
				PublicOrigin: cn.publicOrigin(),
			},
			{
				Label:        "GLOBAL",
				Endpoint:     global.endpoint(),
				Bucket:       "start",
				AccessKey:    "global-key",
				SecretKey:    "global-secret",
				PublicOrigin: global.publicOrigin(),
			},
		},
	}
}

func writeDist(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"agent-install.md":                     "install-doc\n",
		"commerce-skill-install.md":            "commerce-doc\n",
		"agent-release-manifest.json":          `{"version":"0.42.0"}` + "\n",
		"agent-release-manifest.sigstore.json": `{"bundle":"sig"}` + "\n",
		"install.sh":                           "#!/bin/sh\n",
		"install.ps1":                          "Write-Output 'ok'\n",
		"bootstrap-contract.json":              `{"ok":true}` + "\n",
		"release-manifest.json":                `{"cli":"0.42.0"}` + "\n",
		"skills/manifest.json":                 `{"schema_version":1}` + "\n",
		"skills/demo-skill.zip":                "zip-bytes",
		"skills/demo-skill/SKILL.md":           "# demo\n",
		"skills/demo.bin":                      "binary",
	}
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func seedVersionTree(fake *fakeS3, dist string) {
	prefix := "cli/releases/v0.42.0/"
	_ = filepath.Walk(dist, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dist, path)
		rel = filepath.ToSlash(rel)
		body, _ := os.ReadFile(path)
		key := prefix + rel
		contentType := HostedContentType(rel)
		if !strings.Contains(rel, "/") {
			contentType = flatDistContentType(rel)
		}
		fake.seed("start", key, body, cacheImmutable, contentType)
		return nil
	})
}

func ioDiscard() *bytes.Buffer {
	return &bytes.Buffer{}
}
