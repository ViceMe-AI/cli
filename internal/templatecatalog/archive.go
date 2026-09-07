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
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/commerceartifact"
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
	return build(sourceRoot, catalog, nil, outputRoot, origin, signer, false)
}

// BuildForLocalDemo permits a loopback HTTP origin for an isolated local
// catalog demonstration. It must never be used for a public catalog.
func BuildForLocalDemo(sourceRoot string, catalog SourceCatalog, outputRoot, origin string, signer Signer) (BuildResult, error) {
	return build(sourceRoot, catalog, nil, outputRoot, origin, signer, true)
}

// BuildForLocalDemoWithTemplates adds preview-only, coming-soon cards to a
// loopback catalog. They are deliberately excluded from the signed manifest.
func BuildForLocalDemoWithTemplates(sourceRoot string, catalog SourceCatalog, demos DemoCatalog, outputRoot, origin string, signer Signer) (BuildResult, error) {
	return build(sourceRoot, catalog, demos.Templates, outputRoot, origin, signer, true)
}

type demoCard struct {
	DemoTemplate
	PreviewURL string
}

func build(sourceRoot string, catalog SourceCatalog, demos []DemoTemplate, outputRoot, origin string, signer Signer, allowLoopbackHTTP bool) (BuildResult, error) {
	if err := validateSourceCatalog(catalog); err != nil || sourceRoot == "" || outputRoot == "" ||
		!validBuildOrigin(origin, allowLoopbackHTTP) || signer.KeyID == "" || len(signer.PrivateKey) != ed25519.PrivateKeySize {
		return BuildResult{}, ErrBuild
	}
	if len(demos) > 0 && !allowLoopbackHTTP {
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
		basePath := fmt.Sprintf("releases/%s/%s", source.ID, source.Version)
		manifest.Templates = append(manifest.Templates, PublishedTemplate{
			ID: source.ID, Status: source.Status, Name: source.Name, Version: source.Version,
			Scenario: source.Scenario, Description: source.Description,
			PreviewURL: basePath + "/preview/index.html", SourceURL: basePath + "/source.zip",
			SourceSHA256: "sha256:" + hex.EncodeToString(digest[:]), License: source.License,
		})
		zipByTemplate[source.ID+"@"+source.Version] = archive
		previewByTemplate[source.ID+"@"+source.Version] = preview
	}
	demoCards := make([]demoCard, 0, len(demos))
	demoPreviews := make(map[string][]byte, len(demos))
	for _, demo := range demos {
		preview, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(demo.PreviewFile)))
		if err != nil || len(preview) == 0 {
			return BuildResult{}, ErrBuild
		}
		demoCards = append(demoCards, demoCard{
			DemoTemplate: demo,
			PreviewURL:   fmt.Sprintf("demo/%s/preview/index.html", demo.ID),
		})
		demoPreviews[demo.ID] = preview
	}
	if err := writeCatalog(outputRoot, manifest, demoCards, signer, zipByTemplate, previewByTemplate, demoPreviews); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Manifest: manifest, SourceZIP: zipByTemplate[catalog.Templates[0].ID+"@"+catalog.Templates[0].Version]}, nil
}

func validBuildOrigin(origin string, allowLoopbackHTTP bool) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	if !allowLoopbackHTTP || parsed.Scheme != "http" {
		return false
	}
	if parsed.Hostname() == "localhost" {
		return true
	}
	address := net.ParseIP(parsed.Hostname())
	return address != nil && address.IsLoopback()
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

func writeCatalog(outputRoot string, manifest Manifest, demos []demoCard, signer Signer, zips, previews, demoPreviews map[string][]byte) error {
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		return ErrBuild
	}
	canonicalManifest, err := commerceartifact.CanonicalDocument(manifest)
	if err != nil {
		return ErrBuild
	}
	signature := Signature{SchemaVersion: 1, Algorithm: "Ed25519", KeyID: signer.KeyID, Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(signer.PrivateKey, canonicalManifest))}
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
	if err := writeFile(filepath.Join(outputRoot, "index.html"), renderIndex(manifest, demos)); err != nil {
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
	for _, demo := range demos {
		if err := writeFile(filepath.Join(outputRoot, "demo", demo.ID, "preview", "index.html"), demoPreviews[demo.ID]); err != nil {
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

func renderIndex(manifest Manifest, demos []demoCard) []byte {
	var builder strings.Builder
	builder.WriteString(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ViceMe 模板中心</title><style>
:root{color:#1d1b20;background:#f7f6f3;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"PingFang SC","Microsoft YaHei",sans-serif}*{box-sizing:border-box}body{margin:0}.shell{max-width:1200px;margin:0 auto;padding:32px 28px 64px}.eyebrow{display:inline-flex;gap:8px;align-items:center;color:#6c665e;font-size:12px;font-weight:700;letter-spacing:.12em;text-transform:uppercase}.eyebrow::before{width:8px;height:8px;border-radius:50%;background:#ff6d3a;content:""}.hero{display:grid;grid-template-columns:minmax(0,1fr) 270px;gap:24px;align-items:end;padding:44px 0 42px;border-bottom:1px solid #dfddd7}.hero h1{max-width:680px;margin:12px 0 14px;font-size:clamp(38px,6vw,70px);letter-spacing:-.07em;line-height:.98}.hero p{max-width:580px;margin:0;color:#625f59;font-size:17px;line-height:1.65}.hero-note{padding:20px;border-radius:20px;background:#262321;color:#f4f0e8;font-size:14px;line-height:1.55}.hero-note b{display:block;margin-bottom:6px;color:#ffcfb7}.section-heading{display:flex;justify-content:space-between;gap:20px;align-items:baseline;margin:36px 0 18px}.section-heading h2{margin:0;font-size:20px;letter-spacing:-.03em}.section-heading span{color:#7b766f;font-size:14px}.grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:18px}.template-card{overflow:hidden;border:1px solid #dfddd7;border-radius:24px;background:#fff;box-shadow:0 12px 30px rgba(38,35,33,.05)}.template-card.coming{background:#fbfaf7}.preview{position:relative;height:245px;overflow:hidden;background:#e8e5df;border-bottom:1px solid #dfddd7}.preview iframe{width:143%;height:143%;border:0;transform:scale(.7);transform-origin:0 0;pointer-events:none}.preview::after{position:absolute;inset:0;box-shadow:inset 0 0 0 1px rgba(0,0,0,.03);content:""}.details{padding:22px}.meta{display:flex;justify-content:space-between;gap:12px;align-items:center;color:#746f68;font-size:13px}.badge{display:inline-flex;padding:5px 9px;border-radius:999px;background:#f1eee8;color:#5f5a53;font-size:12px;font-weight:700}.badge.live{background:#ffe3d8;color:#9b3d1a}.details h3{margin:13px 0 8px;font-size:26px;letter-spacing:-.045em}.details p{min-height:50px;margin:0;color:#625f59;line-height:1.55}.actions{display:flex;gap:10px;flex-wrap:wrap;margin-top:20px}.button{display:inline-flex;align-items:center;justify-content:center;min-height:42px;padding:0 15px;border-radius:12px;font-size:14px;font-weight:700;text-decoration:none}.button.primary{background:#24211f;color:#fff}.button.secondary{border:1px solid #d7d3cc;color:#332f2c}.button.disabled{background:#ece9e3;color:#948f87;cursor:not-allowed}.coming-copy{margin-top:28px;padding:20px 22px;border-radius:18px;background:#ebe8e2;color:#625f59;line-height:1.6}@media(max-width:760px){.shell{padding:22px 18px 48px}.hero{grid-template-columns:1fr;padding:28px 0}.hero-note{max-width:none}.grid{grid-template-columns:1fr}.preview{height:220px}}
</style><main class="shell"><header class="hero"><div><span class="eyebrow">ViceMe templates</span><h1>从一张好模板开始，做出自己的名片。</h1><p>选择一个适合你的信息结构，再用作品、经历与联系方式把它变成真正属于你的个人主页。</p></div><aside class="hero-note"><b>模板能帮你快速完成什么？</b>布局、信息层级和视觉起点已经准备好；你只需要带来自己的内容。</aside></header><section aria-labelledby="available"><div class="section-heading"><h2 id="available">现在可以使用</h2><span>已验证 · 可继续制作</span></div><div class="grid">`)
	for _, entry := range manifest.Templates {
		writeTemplateCard(&builder, entry.ID, entry.Name, entry.Version, entry.Scenario, entry.Description, entry.PreviewURL, entry.SourceURL, true)
	}
	builder.WriteString(`</div></section>`)
	if len(demos) > 0 {
		builder.WriteString(`<section aria-labelledby="coming-soon"><div class="section-heading"><h2 id="coming-soon">更多风格，正在准备</h2><span>概念预览 · 暂不可创建</span></div><div class="grid">`)
		for _, demo := range demos {
			writeTemplateCard(&builder, demo.ID, demo.Name, "即将上线", demo.Scenario, demo.Description, demo.PreviewURL, "", false)
		}
		builder.WriteString(`</div><p class="coming-copy">这些方向目前仅用于浏览与收集反馈。正式上线后，它们会和 Bonjour 一样提供可验证的源码与创建入口。</p></section>`)
	}
	builder.WriteString(`</main></html>`)
	return []byte(builder.String())
}

func writeTemplateCard(builder *strings.Builder, id, name, version, scenario, description, previewURL, sourceURL string, available bool) {
	builder.WriteString(`<article class="template-card`)
	if !available {
		builder.WriteString(` coming`)
	}
	builder.WriteString(`"><div class="preview"><iframe title="`)
	builder.WriteString(template.HTMLEscapeString(name))
	builder.WriteString(` 预览" src="`)
	builder.WriteString(template.HTMLEscapeString(previewURL))
	builder.WriteString(`" loading="lazy" tabindex="-1"></iframe></div><div class="details"><div class="meta"><span>`)
	builder.WriteString(template.HTMLEscapeString(scenario))
	builder.WriteString(`</span><span class="badge`)
	if available {
		builder.WriteString(` live`)
	}
	builder.WriteString(`">`)
	builder.WriteString(template.HTMLEscapeString(version))
	builder.WriteString(`</span></div><h3>`)
	builder.WriteString(template.HTMLEscapeString(name))
	builder.WriteString(`</h3><p>`)
	builder.WriteString(template.HTMLEscapeString(description))
	builder.WriteString(`</p><div class="actions"><a class="button secondary" href="`)
	builder.WriteString(template.HTMLEscapeString(previewURL))
	builder.WriteString(`">查看效果</a>`)
	if available {
		builder.WriteString(`<a class="button secondary" download href="`)
		builder.WriteString(template.HTMLEscapeString(sourceURL))
		builder.WriteString(`">下载模板源码</a><a class="button primary" href="#" onclick="alert('已选择 `)
		builder.WriteString(template.JSEscapeString(id))
		builder.WriteString(`@`)
		builder.WriteString(template.JSEscapeString(version))
		builder.WriteString(`，请返回 Agent 继续制作。');return false">使用此模板</a>`)
	} else {
		builder.WriteString(`<span class="button disabled" aria-disabled="true">即将上线</span>`)
	}
	builder.WriteString(`</div></div></article>`)
}
