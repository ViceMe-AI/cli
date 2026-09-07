package templatecatalog

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/commerceartifact"
)

func TestClientRejectsManifestSignedByUntrustedKey(t *testing.T) {
	t.Parallel()

	trustedPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, attackerPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trustedDER, err := x509.MarshalPKIXPublicKey(trustedPublic)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{SchemaVersion: 1, Templates: []PublishedTemplate{{
		ID: "bonjour-card", Status: "production", Name: "Bonjour Card", Version: "1.0.0", Scenario: "作品", Description: "个人名片",
		PreviewURL: "releases/bonjour-card/1.0.0/preview/index.html", SourceURL: "releases/bonjour-card/1.0.0/source.zip", SourceSHA256: "sha256:" + strings.Repeat("a", 64), License: "ViceMe template license",
	}}}
	signature := signedManifest(t, manifest, "release-v1", attackerPrivate)
	server := catalogServer(t, manifest, signature, nil)

	_, err = Client{
		Origin: server.URL, AllowInsecure: true,
		TrustedKeys: map[string]string{"release-v1": base64.RawURLEncoding.EncodeToString(trustedDER)},
	}.Load(context.Background())
	if !errors.Is(err, ErrUntrustedManifest) {
		t.Fatalf("Load() error = %v, want ErrUntrustedManifest", err)
	}
}

func TestFetchDoesNotExtractWhenZIPDigestDiffers(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	archive := testZIP(t, "bonjour-card/index.html", "<h1>Bonjour</h1>")
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	manifest := Manifest{SchemaVersion: 1, Templates: []PublishedTemplate{{
		ID: "bonjour-card", Status: "production", Name: "Bonjour Card", Version: "1.0.0", Scenario: "作品", Description: "个人名片",
		PreviewURL: "releases/bonjour-card/1.0.0/preview/index.html", SourceURL: "releases/bonjour-card/1.0.0/source.zip", SourceSHA256: "sha256:" + strings.Repeat("0", 64), License: "ViceMe template license",
	}}}
	signature := signedManifest(t, manifest, "release-v1", privateKey)
	server.Config.Handler = catalogHandler(t, manifest, signature, archive)
	destination := t.TempDir()
	client := Client{
		Origin: server.URL, AllowInsecure: true,
		TrustedKeys: map[string]string{"release-v1": base64.RawURLEncoding.EncodeToString(publicDER)},
	}
	err = client.Fetch(context.Background(), "bonjour-card", "1.0.0", destination)
	if !errors.Is(err, ErrSourceDigestMismatch) {
		t.Fatalf("Fetch() error = %v, want ErrSourceDigestMismatch", err)
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("digest mismatch wrote destination entries: %#v", entries)
	}
}

func TestFetchExtractsOnlyTheVerifiedTemplate(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	archive := testZIP(t, "bonjour-card/index.html", "<h1>Bonjour</h1>")
	digest := sha256.Sum256(archive)
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	manifest := Manifest{SchemaVersion: 1, Templates: []PublishedTemplate{{
		ID: "bonjour-card", Status: "production", Name: "Bonjour Card", Version: "1.0.0", Scenario: "作品", Description: "个人名片",
		PreviewURL: "releases/bonjour-card/1.0.0/preview/index.html", SourceURL: "releases/bonjour-card/1.0.0/source.zip", SourceSHA256: "sha256:" + hex.EncodeToString(digest[:]), License: "ViceMe template license",
	}}}
	signature := signedManifest(t, manifest, "release-v1", privateKey)
	server.Config.Handler = catalogHandler(t, manifest, signature, archive)
	destination := t.TempDir()
	client := Client{
		Origin: server.URL, AllowInsecure: true,
		TrustedKeys: map[string]string{"release-v1": base64.RawURLEncoding.EncodeToString(publicDER)},
	}
	if err := client.Fetch(context.Background(), "bonjour-card", "1.0.0", destination); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "bonjour-card", "index.html"))
	if err != nil || string(content) != "<h1>Bonjour</h1>" {
		t.Fatalf("verified template was not extracted: content=%q err=%v", content, err)
	}
}

func catalogServer(t *testing.T, manifest Manifest, signature Signature, archive []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(catalogHandler(t, manifest, signature, archive))
}

func catalogHandler(t *testing.T, manifest Manifest, signature Signature, archive []byte) http.Handler {
	t.Helper()
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	signatureBody, err := json.Marshal(signature)
	if err != nil {
		t.Fatal(err)
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/manifest.json":
			_, _ = writer.Write(manifestBody)
		case "/manifest.sig":
			_, _ = writer.Write(signatureBody)
		case "/releases/bonjour-card/1.0.0/source.zip":
			_, _ = writer.Write(archive)
		default:
			http.NotFound(writer, request)
		}
	})
}

func signedManifest(t *testing.T, manifest Manifest, keyID string, privateKey ed25519.PrivateKey) Signature {
	t.Helper()
	canonical, err := commerceartifact.CanonicalDocument(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return Signature{
		SchemaVersion: 1, Algorithm: "Ed25519", KeyID: keyID,
		Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)),
	}
}

func testZIP(t *testing.T, filename, contents string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
