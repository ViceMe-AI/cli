package pagepackage

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
)

const (
	MaxArchiveBytes      = 5 * 1024 * 1024
	MaxUncompressedBytes = 10 * 1024 * 1024
	MaxFileBytes         = 2 * 1024 * 1024
	MaxFiles             = 100
)

var (
	allowedExtensions = map[string]struct{}{
		".avif": {}, ".css": {}, ".gif": {}, ".html": {}, ".jpeg": {}, ".jpg": {},
		".js": {}, ".json": {}, ".mjs": {}, ".png": {}, ".webp": {}, ".woff2": {},
	}
	allowedCapabilities = map[string]struct{}{
		"context.read": {}, "navigation.open": {}, "auth.request-login": {},
		"work.like": {}, "comments.read": {}, "comments.write": {},
		"checkout.open": {}, "creator.subscribe": {},
	}
	forbiddenBrowserSource = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bnavigator\s*\.\s*serviceWorker\b`),
		regexp.MustCompile(`\b(?:fetch|XMLHttpRequest|WebSocket|EventSource|importScripts)\s*\(`),
	}
	forbiddenHTMLMarkup = []*regexp.Regexp{
		regexp.MustCompile(`(?i)<\s*(?:iframe|object|embed|base)\b`),
		regexp.MustCompile(`(?i)<\s*(?:script|link)\b[^>]*(?:src|href)\s*=\s*["'](?:https?:)?//`),
	}
	credentialPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}\b`),
		regexp.MustCompile(`(?i)\bsk-(?:proj-)?[A-Za-z0-9_-]{20,}\b`),
		regexp.MustCompile(`(?i)(?:api[_-]?key|client[_-]?secret|access[_-]?token)\s*[:=]\s*["']?[A-Za-z0-9_./+=-]{16,}`),
	}
)

type Package struct {
	SourcePath string                        `json:"sourcePath"`
	Manifest   api.PageCustomizationManifest `json:"manifest"`
	Artifact   api.PageCustomizationArtifact `json:"artifact"`
	FileCount  int                           `json:"fileCount"`
	Bytes      []byte                        `json:"-"`
}

func Inspect(source string) (Package, error) {
	abs, err := filepath.Abs(strings.TrimSpace(source))
	if err != nil {
		return Package{}, output.Validation("PAGE_PATH_INVALID", "could not resolve the page package path").WithCause(err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return Package{}, output.Validation("PAGE_PATH_NOT_FOUND", "page package does not exist").WithCause(err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || filepath.Ext(abs) != ".zip" {
		return Package{}, output.Validation("PAGE_SOURCE_UNSUPPORTED", "page source must be a regular ZIP file")
	}
	if info.Size() <= 0 || info.Size() > MaxArchiveBytes {
		return Package{}, output.Validation("PAGE_PACKAGE_TOO_LARGE", "page ZIP must be between 1 byte and 5 MiB")
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return Package{}, output.Internal("PAGE_PACKAGE_READ_FAILED", "could not read the page ZIP", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Package{}, output.Validation("PAGE_PACKAGE_INVALID_ZIP", "page package is not a valid ZIP archive").WithCause(err)
	}
	manifest, fileCount, err := validateEntries(reader.File)
	if err != nil {
		return Package{}, err
	}
	return Package{
		SourcePath: abs,
		Manifest:   manifest,
		Artifact: api.PageCustomizationArtifact{
			Digest: sha256Hex(data), SizeBytes: int64(len(data)), FileName: filepath.Base(abs), ContentType: "application/zip",
		},
		FileCount: fileCount,
		Bytes:     data,
	}, nil
}

func validateEntries(entries []*zip.File) (api.PageCustomizationManifest, int, error) {
	seen := make(map[string]struct{}, len(entries))
	var manifestBytes []byte
	var entryHTML []byte
	fileCount := 0
	htmlCount := 0
	var total uint64
	for _, entry := range entries {
		name := entry.Name
		if err := validatePath(name); err != nil {
			return api.PageCustomizationManifest{}, 0, err
		}
		if entry.Flags&0x1 != 0 {
			return api.PageCustomizationManifest{}, 0, output.Validation("PAGE_PACKAGE_ENCRYPTED", "encrypted ZIP entries are not supported")
		}
		if entry.Method != zip.Store && entry.Method != zip.Deflate {
			return api.PageCustomizationManifest{}, 0, output.Validation("PAGE_PACKAGE_COMPRESSION_UNSUPPORTED", "page package uses an unsupported ZIP compression method")
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		if !entry.Mode().IsRegular() {
			return api.PageCustomizationManifest{}, 0, output.Validation("PAGE_PACKAGE_SPECIAL_FILE", "page package contains a symlink or special file")
		}
		if _, exists := seen[name]; exists {
			return api.PageCustomizationManifest{}, 0, output.Validation("PAGE_PACKAGE_DUPLICATE_PATH", "page package contains duplicate paths")
		}
		seen[name] = struct{}{}
		fileCount++
		total += entry.UncompressedSize64
		if fileCount > MaxFiles {
			return api.PageCustomizationManifest{}, 0, output.Validation("PAGE_PACKAGE_TOO_MANY_FILES", "page package contains more than 100 files")
		}
		if entry.UncompressedSize64 > MaxFileBytes || total > MaxUncompressedBytes {
			return api.PageCustomizationManifest{}, 0, output.Validation("PAGE_PACKAGE_TOO_LARGE", "page package exceeds its uncompressed size budget")
		}
		if entry.UncompressedSize64 > 1024*1024 && entry.UncompressedSize64 > max(entry.CompressedSize64, 1)*200 {
			return api.PageCustomizationManifest{}, 0, output.Validation("PAGE_PACKAGE_COMPRESSION_RATIO_EXCEEDED", "page package contains an unsafe compression ratio")
		}
		content, err := readEntry(entry)
		if err != nil {
			return api.PageCustomizationManifest{}, 0, err
		}
		if err := validateTextContent(name, content); err != nil {
			return api.PageCustomizationManifest{}, 0, err
		}
		if name == "viceme-page.json" {
			manifestBytes = content
			continue
		}
		if !strings.HasPrefix(name, "dist/") {
			return api.PageCustomizationManifest{}, 0, output.Validation("PAGE_PACKAGE_PATH_UNSUPPORTED", "page assets must be placed under dist/")
		}
		extension := strings.ToLower(path.Ext(name))
		if _, ok := allowedExtensions[extension]; !ok {
			return api.PageCustomizationManifest{}, 0, output.Validation("PAGE_PACKAGE_FILE_TYPE_UNSUPPORTED", "page package contains an unsupported file type")
		}
		if extension == ".html" {
			htmlCount++
			if name == "dist/index.html" {
				entryHTML = content
			}
		}
	}
	if len(manifestBytes) == 0 {
		return api.PageCustomizationManifest{}, 0, output.Validation("PAGE_MANIFEST_MISSING", "page package must contain viceme-page.json at its root")
	}
	manifest, err := decodeManifest(manifestBytes)
	if err != nil {
		return api.PageCustomizationManifest{}, 0, err
	}
	if htmlCount != 1 || len(entryHTML) == 0 {
		return api.PageCustomizationManifest{}, 0, output.Validation("PAGE_ENTRY_COUNT_INVALID", "page package must contain exactly one dist/index.html document")
	}
	return manifest, fileCount, nil
}

func decodeManifest(data []byte) (api.PageCustomizationManifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest api.PageCustomizationManifest
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, output.Validation("PAGE_MANIFEST_INVALID", "viceme-page.json is invalid").WithCause(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return manifest, output.Validation("PAGE_MANIFEST_INVALID", "viceme-page.json must contain exactly one JSON object")
	}
	name := strings.TrimSpace(manifest.Metadata.Name)
	if manifest.APIVersion != "page.viceme.ai/v1alpha1" || (manifest.Kind != "CreatorPage" && manifest.Kind != "WorkPage") || name == "" || len(name) > 80 || manifest.Spec.Entry != "dist/index.html" || manifest.Spec.SDKVersion != "1" || manifest.Spec.Capabilities == nil || len(manifest.Spec.Capabilities) > len(allowedCapabilities) {
		return manifest, output.Validation("PAGE_MANIFEST_INVALID", "viceme-page.json has an unsupported version, kind, name, entry, or SDK version")
	}
	unique := make(map[string]struct{}, len(manifest.Spec.Capabilities))
	for _, capability := range manifest.Spec.Capabilities {
		if _, ok := allowedCapabilities[capability]; !ok {
			return manifest, output.Validation("PAGE_MANIFEST_INVALID", fmt.Sprintf("unsupported page capability %q", capability))
		}
		unique[capability] = struct{}{}
	}
	manifest.Spec.Capabilities = manifest.Spec.Capabilities[:0]
	for capability := range unique {
		manifest.Spec.Capabilities = append(manifest.Spec.Capabilities, capability)
	}
	sort.Strings(manifest.Spec.Capabilities)
	manifest.Metadata.Name = name
	return manifest, nil
}

func validatePath(name string) error {
	if name == "" || !utf8.ValidString(name) || len([]byte(name)) > 512 || strings.Contains(name, "\\") || strings.ContainsRune(name, 0) || strings.HasPrefix(name, "/") || path.Clean(name) != strings.TrimSuffix(name, "/") {
		return output.Validation("PAGE_PACKAGE_PATH_INVALID", "page package contains an unsafe path")
	}
	for _, segment := range strings.Split(strings.ToLower(strings.TrimSuffix(name, "/")), "/") {
		if segment == ".." || segment == ".git" || segment == ".next" || segment == "node_modules" || segment == "__pycache__" || segment == ".env" || strings.HasPrefix(segment, ".env.") {
			return output.Validation("PAGE_PACKAGE_PATH_INVALID", "page package contains a forbidden path")
		}
	}
	return nil
}

func validateTextContent(name string, content []byte) error {
	extension := strings.ToLower(path.Ext(name))
	if extension != ".css" && extension != ".html" && extension != ".js" && extension != ".json" && extension != ".mjs" {
		return nil
	}
	if !utf8.Valid(content) {
		return output.Validation("PAGE_PACKAGE_TEXT_ENCODING_INVALID", "page package contains invalid UTF-8")
	}
	for _, pattern := range credentialPatterns {
		if pattern.Match(content) {
			return output.Validation("PAGE_PACKAGE_SECRET_DETECTED", "page package may contain a secret")
		}
	}
	if extension == ".html" || extension == ".js" || extension == ".mjs" {
		for _, pattern := range forbiddenBrowserSource {
			if pattern.Match(content) {
				return output.Validation("PAGE_ENTRY_UNSUPPORTED_FEATURE", "page source contains an unsupported browser feature")
			}
		}
	}
	if extension == ".html" {
		for _, pattern := range forbiddenHTMLMarkup {
			if pattern.Match(content) {
				return output.Validation("PAGE_ENTRY_UNSUPPORTED_FEATURE", "page source contains an unsupported browser feature")
			}
		}
	}
	return nil
}

func readEntry(entry *zip.File) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, output.Validation("PAGE_PACKAGE_INVALID_ZIP", "page package entry could not be opened").WithCause(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, MaxFileBytes+1))
	if err != nil || len(data) > MaxFileBytes {
		return nil, output.Validation("PAGE_PACKAGE_FILE_TOO_LARGE", "page package entry exceeds 2 MiB").WithCause(err)
	}
	return data, nil
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
