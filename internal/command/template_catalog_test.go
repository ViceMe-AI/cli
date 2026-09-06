package command

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/commerceartifact"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	"github.com/ViceMe-AI/cli/internal/templatecatalog"
)

func TestTemplateListReturnsVerifiedCatalogURL(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	manifest := templatecatalog.Manifest{SchemaVersion: 1, Templates: []templatecatalog.PublishedTemplate{{
		ID: "bonjour-card", Status: "production", Name: "Bonjour Card", Version: "1.0.0", Scenario: "作品", Description: "个人名片",
		PreviewURL: server.URL + "/preview", SourceURL: server.URL + "/source.zip", SourceSHA256: "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", License: "ViceMe template license",
	}}}
	canonical, err := commerceartifact.CanonicalDocument(manifest)
	if err != nil {
		t.Fatal(err)
	}
	signature := templatecatalog.Signature{
		SchemaVersion: 1, Algorithm: "Ed25519", KeyID: "test-v1",
		Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)),
	}
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	signatureBody, err := json.Marshal(signature)
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/manifest.json":
			_, _ = writer.Write(manifestBody)
		case "/manifest.sig":
			_, _ = writer.Write(signatureBody)
		default:
			http.NotFound(writer, request)
		}
	})
	previous := buildinfo.TemplateCatalogTrustKeys
	buildinfo.TemplateCatalogTrustKeys = "test-v1:" + base64.RawURLEncoding.EncodeToString(publicDER)
	t.Cleanup(func() { buildinfo.TemplateCatalogTrustKeys = previous })
	t.Setenv("VICEME_TEMPLATE_CATALOG_ORIGIN", server.URL)

	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	exit := Execute([]string{"template", "list"}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), HTTPClient: server.Client(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, StartBackgroundUpdate: func() error { return nil },
	})
	if exit != 0 {
		t.Fatalf("template list failed: exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK {
		t.Fatalf("template list returned invalid JSON: %s (%v)", stdout.String(), err)
	}
	if got := envelope.Data["catalog_url"]; got != server.URL+"/index.html" {
		t.Fatalf("catalog_url = %#v, want %q", got, server.URL+"/index.html")
	}
}
