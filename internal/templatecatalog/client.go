package templatecatalog

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/commerceartifact"
	"github.com/ViceMe-AI/cli/internal/semver"
)

var (
	ErrCatalogUnavailable   = errors.New("TEMPLATE_CATALOG_UNAVAILABLE")
	ErrUntrustedManifest    = errors.New("TEMPLATE_CATALOG_SIGNATURE_INVALID")
	ErrTemplateNotFound     = errors.New("TEMPLATE_CATALOG_TEMPLATE_NOT_FOUND")
	ErrSourceDigestMismatch = errors.New("TEMPLATE_CATALOG_SOURCE_DIGEST_MISMATCH")
	ErrArchiveInvalid       = errors.New("TEMPLATE_CATALOG_ARCHIVE_INVALID")
)

const (
	maxManifestBytes   = 1 << 20
	maxArchiveBytes    = 32 << 20
	defaultHTTPTimeout = 30 * time.Second
)

type Client struct {
	Origin        string
	HTTPClient    *http.Client
	TrustedKeys   map[string]string
	AllowInsecure bool
}

func (client Client) CatalogURL() (string, error) {
	origin, err := client.parseOrigin()
	if err != nil {
		return "", err
	}
	return origin + "/index.html", nil
}

func (client Client) Load(ctx context.Context) (Manifest, error) {
	origin, err := client.parseOrigin()
	if err != nil {
		return Manifest{}, err
	}
	manifestBody, err := client.get(ctx, origin+"/manifest.json", maxManifestBytes)
	if err != nil {
		return Manifest{}, err
	}
	signatureBody, err := client.get(ctx, origin+"/manifest.sig", maxManifestBytes)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if !strictDecode(manifestBody, &manifest) || validateManifest(manifest, client.AllowInsecure) != nil {
		return Manifest{}, ErrCatalogUnavailable
	}
	var signature Signature
	if !strictDecode(signatureBody, &signature) || signature.SchemaVersion != 1 || signature.Algorithm != "Ed25519" || signature.KeyID == "" || signature.Signature == "" {
		return Manifest{}, ErrUntrustedManifest
	}
	trustedKey := client.TrustedKeys[signature.KeyID]
	if trustedKey == "" || commerceartifact.VerifyDetachedDocument(manifest, trustedKey, signature.Signature) != nil {
		return Manifest{}, ErrUntrustedManifest
	}
	for index := range manifest.Templates {
		manifest.Templates[index].PreviewURL = origin + "/" + manifest.Templates[index].PreviewURL
		manifest.Templates[index].SourceURL = origin + "/" + manifest.Templates[index].SourceURL
	}
	return manifest, nil
}

func (client Client) Fetch(ctx context.Context, templateID, version, destination string) error {
	manifest, err := client.Load(ctx)
	if err != nil {
		return err
	}
	var selected *PublishedTemplate
	for index := range manifest.Templates {
		entry := &manifest.Templates[index]
		if entry.ID == templateID && entry.Version == version {
			selected = entry
			break
		}
	}
	if selected == nil {
		return ErrTemplateNotFound
	}
	response, err := client.request(ctx, selected.SourceURL)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ErrCatalogUnavailable
	}
	temporary, err := os.CreateTemp("", "viceme-template-catalog-*.zip")
	if err != nil {
		return ErrCatalogUnavailable
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, maxArchiveBytes+1))
	closeErr := temporary.Close()
	if copyErr != nil || closeErr != nil || written > maxArchiveBytes {
		return ErrCatalogUnavailable
	}
	if "sha256:"+hex.EncodeToString(hash.Sum(nil)) != selected.SourceSHA256 {
		return ErrSourceDigestMismatch
	}
	return extractArchive(temporaryName, selected.ID, destination)
}

func (client Client) parseOrigin() (string, error) {
	origin := strings.TrimSuffix(client.Origin, "/")
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "https" && (!client.AllowInsecure || parsed.Scheme != "http" || !isLoopbackHost(parsed.Hostname()))) {
		return "", ErrCatalogUnavailable
	}
	return origin, nil
}

func (client Client) get(ctx context.Context, target string, limit int64) ([]byte, error) {
	response, err := client.request(ctx, target)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrCatalogUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, ErrCatalogUnavailable
	}
	return body, nil
}

func (client Client) request(ctx context.Context, target string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, ErrCatalogUnavailable
	}
	httpClient := client.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, ErrCatalogUnavailable
	}
	return response, nil
}

func validateManifest(manifest Manifest, _ bool) error {
	if manifest.SchemaVersion != 1 || len(manifest.Templates) == 0 {
		return ErrCatalogUnavailable
	}
	seen := make(map[string]struct{}, len(manifest.Templates))
	for _, entry := range manifest.Templates {
		if entry.Status != "production" || !idPattern.MatchString(entry.ID) || strings.TrimSpace(entry.Name) == "" ||
			strings.TrimSpace(entry.Scenario) == "" || strings.TrimSpace(entry.Description) == "" || strings.TrimSpace(entry.License) == "" {
			return ErrCatalogUnavailable
		}
		if _, err := semver.Parse(entry.Version); err != nil || !safeRelativePath(entry.PreviewURL) ||
			!safeRelativePath(entry.SourceURL) || !validDigest(entry.SourceSHA256) {
			return ErrCatalogUnavailable
		}
		key := entry.ID + "@" + entry.Version
		if _, exists := seen[key]; exists {
			return ErrCatalogUnavailable
		}
		seen[key] = struct{}{}
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func strictDecode(body []byte, value any) bool {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value) == nil && decoder.Decode(&struct{}{}) == io.EOF
}

func extractArchive(filename, templateID, destination string) error {
	file, err := os.Open(filename)
	if err != nil {
		return ErrArchiveInvalid
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() > maxArchiveBytes {
		return ErrArchiveInvalid
	}
	archive, err := zip.NewReader(file, info.Size())
	if err != nil || len(archive.File) == 0 {
		return ErrArchiveInvalid
	}
	prefix := templateID + "/"
	var total uint64
	for _, entry := range archive.File {
		if !strings.HasPrefix(entry.Name, prefix) || !safeArchivePath(strings.TrimPrefix(entry.Name, prefix)) ||
			entry.FileInfo().IsDir() || !entry.FileInfo().Mode().IsRegular() || entry.UncompressedSize64 > maxArchiveBytes {
			return ErrArchiveInvalid
		}
		total += entry.UncompressedSize64
		if total > maxArchiveBytes {
			return ErrArchiveInvalid
		}
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return ErrArchiveInvalid
	}
	temporary, err := os.MkdirTemp(destination, "."+templateID+"-")
	if err != nil {
		return ErrArchiveInvalid
	}
	defer os.RemoveAll(temporary)
	for _, entry := range archive.File {
		relative := filepath.FromSlash(strings.TrimPrefix(entry.Name, prefix))
		target := filepath.Join(temporary, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return ErrArchiveInvalid
		}
		input, err := entry.Open()
		if err != nil {
			return ErrArchiveInvalid
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			input.Close()
			return ErrArchiveInvalid
		}
		_, copyErr := io.Copy(output, input)
		closeOutputErr := output.Close()
		closeInputErr := input.Close()
		if copyErr != nil || closeOutputErr != nil || closeInputErr != nil {
			return ErrArchiveInvalid
		}
	}
	if err := os.Rename(temporary, filepath.Join(destination, templateID)); err != nil {
		return fmt.Errorf("%w: %v", ErrArchiveInvalid, err)
	}
	return nil
}

func safeArchivePath(value string) bool {
	return value != "" && !strings.Contains(value, "\\") && !strings.HasPrefix(value, "/") && path.Clean(value) == value && value != "." && value != ".." && !strings.HasPrefix(value, "../")
}
