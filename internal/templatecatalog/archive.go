package templatecatalog

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrBuild = errors.New("TEMPLATE_CATALOG_BUILD_INVALID")

type Signer struct {
	KeyID      string
	PrivateKey ed25519.PrivateKey
}

type Manifest struct {
	SchemaVersion int                 `json:"schema_version"`
	Templates     []PublishedTemplate `json:"templates"`
}

type PublishedTemplate struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	Scenario     string `json:"scenario"`
	Description  string `json:"description"`
	PreviewURL   string `json:"preview_url"`
	SourceURL    string `json:"source_url"`
	SourceSHA256 string `json:"source_sha256"`
	License      string `json:"license"`
}

type Signature struct {
	SchemaVersion int    `json:"schema_version"`
	Algorithm     string `json:"algorithm"`
	KeyID         string `json:"key_id"`
	Signature     string `json:"signature"`
}

type BuildResult struct {
	Manifest  Manifest
	SourceZIP []byte
}

func Build(sourceRoot string, catalog SourceCatalog, outputRoot, origin string, signer Signer) (BuildResult, error) {
	if err := validateSourceCatalog(catalog); err != nil || sourceRoot == "" || outputRoot == "" ||
		!strings.HasPrefix(origin, "https://") || signer.KeyID == "" || len(signer.PrivateKey) != ed25519.PrivateKeySize {
		return BuildResult{}, ErrBuild
	}
	origin = strings.TrimSuffix(origin, "/")
	manifest := Manifest{SchemaVersion: 1, Templates: make([]PublishedTemplate, 0, len(catalog.Templates))}
	zipByTemplate := make(map[string][]byte, len(catalog.Templates))
	previewByTemplate := make(map[string][]byte, len(catalog.Templates))
	for _, source := range catalog.Templates {
		archive, err := buildSourceZIP(filepath.Join(sourceRoot, filepath.FromSlash(source.SourceDir)), source.ID)
		if err != nil {
			return BuildResult{}, err
		}
		preview, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(source.PreviewFile)))
		if err != nil || len(preview) == 0 {
			return BuildResult{}, ErrBuild
		}
		digest := sha256.Sum256(archive)
		baseURL := fmt.Sprintf("%s/releases/%s/%s", origin, source.ID, source.Version)
		manifest.Templates = append(manifest.Templates, PublishedTemplate{
			ID: source.ID, Status: source.Status, Name: source.Name, Version: source.Version,
			Scenario: source.Scenario, Description: source.Description,
			PreviewURL: baseURL + "/preview/index.html", SourceURL: baseURL + "/source.zip",
			SourceSHA256: "sha256:" + hex.EncodeToString(digest[:]), License: source.License,
		})
		zipByTemplate[source.ID+"@"+source.Version] = archive
		previewByTemplate[source.ID+"@"+source.Version] = preview
	}
	if err := writeCatalog(outputRoot, manifest, signer, zipByTemplate, previewByTemplate); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Manifest: manifest, SourceZIP: zipByTemplate[catalog.Templates[0].ID+"@"+catalog.Templates[0].Version]}, nil
}

func buildSourceZIP(sourceDirectory, root string) ([]byte, error) {
	entries := make([]string, 0)
	err := filepath.WalkDir(sourceDirectory, func(filename string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return ErrBuild
		}
		relative, err := filepath.Rel(sourceDirectory, filename)
		if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return ErrBuild
		}
		entries = append(entries, relative)
		return nil
	})
	if err != nil || len(entries) == 0 {
		return nil, ErrBuild
	}
	sort.Strings(entries)
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, relative := range entries {
		body, err := os.ReadFile(filepath.Join(sourceDirectory, relative))
		if err != nil {
			return nil, ErrBuild
		}
		header := &zip.FileHeader{Name: filepath.ToSlash(filepath.Join(root, relative)), Method: zip.Store}
		header.SetModTime(time.Unix(0, 0).UTC())
		header.SetMode(0o644)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return nil, ErrBuild
		}
		if _, err := entry.Write(body); err != nil {
			return nil, ErrBuild
		}
	}
	if err := writer.Close(); err != nil {
		return nil, ErrBuild
	}
	return output.Bytes(), nil
}

func writeCatalog(outputRoot string, manifest Manifest, signer Signer, zips, previews map[string][]byte) error {
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		return ErrBuild
	}
	signature := Signature{SchemaVersion: 1, Algorithm: "Ed25519", KeyID: signer.KeyID, Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(signer.PrivateKey, manifestBody))}
	signatureBody, err := json.Marshal(signature)
	if err != nil {
		return ErrBuild
	}
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		return ErrBuild
	}
	if err := writeFile(filepath.Join(outputRoot, "manifest.json"), manifestBody); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(outputRoot, "manifest.sig"), signatureBody); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(outputRoot, "index.html"), renderIndex(manifest)); err != nil {
		return err
	}
	for _, entry := range manifest.Templates {
		key := entry.ID + "@" + entry.Version
		base := filepath.Join(outputRoot, "releases", entry.ID, entry.Version)
		if err := writeFile(filepath.Join(base, "source.zip"), zips[key]); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(base, "preview", "index.html"), previews[key]); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(filename string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return ErrBuild
	}
	if err := os.WriteFile(filename, body, 0o644); err != nil {
		return ErrBuild
	}
	return nil
}

func renderIndex(manifest Manifest) []byte {
	var builder strings.Builder
	builder.WriteString("<!doctype html><html lang=\"zh-CN\"><meta charset=\"utf-8\"><title>ViceMe 模板中心</title><main><h1>个人名片模板</h1>")
	for _, entry := range manifest.Templates {
		builder.WriteString("<article><h2>")
		builder.WriteString(template.HTMLEscapeString(entry.Name))
		builder.WriteString("</h2><p>")
		builder.WriteString(template.HTMLEscapeString(entry.Description))
		builder.WriteString("</p><a href=\"")
		builder.WriteString(template.HTMLEscapeString(entry.PreviewURL))
		builder.WriteString("\">查看效果</a> <a href=\"")
		builder.WriteString(template.HTMLEscapeString(entry.PreviewURL))
		builder.WriteString("\">使用此模板</a> <a href=\"")
		builder.WriteString(template.HTMLEscapeString(entry.SourceURL))
		builder.WriteString("\">下载模板源码</a></article>")
	}
	builder.WriteString("</main></html>")
	return []byte(builder.String())
}
