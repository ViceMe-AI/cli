# Cloud Creator Template Catalog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish a signed, independently released cloud catalog of production creator-page templates and let the CLI/Agent list and fetch a user-selected, checksum-verified template.

**Architecture:** The repository owns template source metadata and builds static catalog artifacts. A dedicated GitHub Actions workflow publishes immutable template versions and a short-lived stable manifest to the existing CN and Global static origins without a CLI release. The CLI verifies the manifest against an embedded Ed25519 trust ring and verifies the selected ZIP before the Skill creates a local project.

**Tech Stack:** Go 1.24, Cobra, `internal/commerceartifact` Ed25519 verification, Go `httptest`, deterministic ZIP archives, GitHub Actions, AWS CLI-compatible RustFS/S3, static HTML/JSON.

**Spec:** `docs/superpowers/specs/2026-09-07-creator-template-catalog-design.md`

## Global Constraints

- Public production catalog roots are `https://s3.viceme.cn/templates/` and `https://s3.viceme.ai/templates/`.
- A production manifest exposes only `status: "production"`; `dev_mock` entries never publish or download.
- Every immutable template source ZIP has a SHA-256 in the signed manifest and is verified before extraction.
- `manifest.sig` uses Ed25519 and an official CLI-embedded trust ring; a manifest cannot choose its own trusted key.
- The creator sees cloud links, never local template paths or dev fixtures.
- The Agent owns template choice, data gathering, preview, and publication. The right-side browser only previews in this phase.
- The “使用此模板” webpage button cannot alter Agent state until a separate origin-validated host handoff is implemented.

---

## File structure

| Path | Responsibility |
|---|---|
| `templates/creator-pages/production.json` | Versioned, production-only source metadata for the catalog builder. |
| `cmd/template-catalog/main.go` | Builds `index.html`, `manifest.json`, `manifest.sig`, immutable previews and deterministic source ZIPs. |
| `cmd/template-catalog/main_test.go` | End-to-end builder tests using a temporary output directory and generated Ed25519 key. |
| `internal/templatecatalog/catalog.go` | Strict source/public manifest types, schema validation, canonical URL derivation and deterministic output paths. |
| `internal/templatecatalog/archive.go` | Deterministic template ZIP creation and SHA-256 calculation. |
| `internal/templatecatalog/catalog_test.go` | Schema, production-only filtering, duplicate-version, path and ZIP tests. |
| `internal/templatecatalog/client.go` | Region-aware manifest retrieval, signature verification, template resolution, ZIP checksum verification and safe extraction. |
| `internal/templatecatalog/client_test.go` | `httptest` coverage for valid, untrusted, malformed, tampered and mismatched responses. |
| `internal/buildinfo/buildinfo.go` | Embedded `TemplateCatalogTrustKeys` public-key ring. |
| `internal/commerceartifact/artifact.go` | Reusable detached-document signature verifier for the template manifest. |
| `internal/command/template_catalog.go` | `viceme template list` and `viceme template fetch` commands. |
| `internal/command/template_catalog_test.go` | Command-level JSON contract and no-write-on-validation-failure tests. |
| `internal/command/root.go` | Registers the new `template` command. |
| `cmd/validate-template-catalog-trust-ring/main.go` | Fails CI when the release trust-ring variable is malformed. |
| `Makefile` | Adds local catalog build and catalog test targets; injects the template trust ring into development builds. |
| `.github/workflows/release.yml` | Injects and validates the public template-catalog trust ring in official release binaries. |
| `.github/workflows/template-catalog.yml` | Independently validates and publishes dev/prod cloud catalogs to both regions. |
| `skills/customize-your-page/SKILL.md` | Replaces local catalog wording with verified cloud catalog commands and creator-facing cloud links. |
| `internal/skillcontent/official_content_test.go` | Locks the cloud catalog contract and rejects local-path/dev-mock user exposure. |
| `docs/creator-template-catalog.md` | Documents catalog source format, template contribution, release channels and rollback. |

## Task 1: Define production template source and deterministic catalog builder

**Files:**
- Create: `templates/creator-pages/production.json`
- Create: `internal/templatecatalog/catalog.go`
- Create: `internal/templatecatalog/archive.go`
- Create: `internal/templatecatalog/catalog_test.go`
- Create: `cmd/template-catalog/main.go`
- Create: `cmd/template-catalog/main_test.go`

**Interfaces:**
- Consumes: the existing source directory `skills/customize-your-page/templates/bonjour-card` and its existing static preview `skills/customize-your-page/templates/catalog-previews/bonjour-card.html`.
- Produces: `templatecatalog.Build(sourceRoot, outputRoot, origin, signer)` and static artifacts under `releases/<id>/<version>/`.

- [ ] **Step 1: Write failing catalog-schema tests**

```go
func TestLoadSourceCatalogRejectsDevMockAndDuplicateVersion(t *testing.T) {
    _, err := templatecatalog.LoadSourceCatalog(strings.NewReader(`{
      "schema_version":1,
      "templates":[
        {"id":"bonjour-card","status":"production","version":"1.0.0","source_dir":"a","preview_file":"b","license":"ViceMe template license"},
        {"id":"bonjour-card","status":"dev_mock","version":"1.0.0","source_dir":"a","preview_file":"b","license":"ViceMe template license"}
      ]
    }`))
    if !errors.Is(err, templatecatalog.ErrInvalidSourceCatalog) { t.Fatalf("err = %v", err) }
}
```

- [ ] **Step 2: Run the schema test and verify it fails because `templatecatalog` does not exist**

Run: `go test ./internal/templatecatalog -run TestLoadSourceCatalogRejectsDevMockAndDuplicateVersion -count=1`

Expected: FAIL with an unresolved package, type, or function error.

- [ ] **Step 3: Add the source catalog and strict types**

Create `templates/creator-pages/production.json` with exactly one Bonjour entry:

```json
{
  "schema_version": 1,
  "templates": [{
    "id": "bonjour-card",
    "status": "production",
    "version": "1.0.0",
    "name": "Bonjour Card",
    "scenario": "作品、资料与公开联系方式",
    "description": "带 Block 编辑能力的个人名片，适合持续补充作品与社交链接。",
    "source_dir": "skills/customize-your-page/templates/bonjour-card",
    "preview_file": "skills/customize-your-page/templates/catalog-previews/bonjour-card.html",
    "license": "ViceMe template license"
  }]
}
```

Implement `LoadSourceCatalog(io.Reader) (SourceCatalog, error)`. Reject unknown JSON fields, any status other than `production`, duplicate `id + version`, IDs outside `[a-z0-9-]`, non-semver versions, absolute or escaping source paths, and missing name/scenario/description/license.

- [ ] **Step 4: Add deterministic ZIP generation tests**

```go
func TestBuildWritesDeterministicZipAndManifestDigest(t *testing.T) {
    first := buildCatalog(t)
    second := buildCatalog(t)
    if !bytes.Equal(first.SourceZIP, second.SourceZIP) { t.Fatal("ZIP bytes changed") }
    if got := sha256.Sum256(first.SourceZIP); first.Manifest.Templates[0].SourceSHA256 != "sha256:"+hex.EncodeToString(got[:]) {
        t.Fatalf("source digest = %q", first.Manifest.Templates[0].SourceSHA256)
    }
}
```

- [ ] **Step 5: Implement the builder and command**

Implement `Build` with sorted file traversal, epoch timestamps, mode `0644`, no directory ZIP entries, and no ZIP extra fields. It must write:

```text
<output>/index.html
<output>/manifest.json
<output>/manifest.sig
<output>/releases/bonjour-card/1.0.0/preview/index.html
<output>/releases/bonjour-card/1.0.0/source.zip
```

Generate `preview_url` and `source_url` from the supplied origin; never accept public URLs from `production.json`. `index.html` must list each production template with “查看效果”, “使用此模板”, and “下载模板源码” links. The first two links point to the preview; “使用此模板” only displays the selected `id + version` and tells the user to return to the Agent.

- [ ] **Step 6: Run builder tests and inspect artifacts**

Run: `go test ./internal/templatecatalog ./cmd/template-catalog -count=1`

Run: `go run ./cmd/template-catalog --source templates/creator-pages/production.json --output /tmp/viceme-template-catalog --origin https://s3.viceme.cn/templates --signing-key-file /tmp/template-catalog-test-key`

Expected: all tests pass; output contains only Bonjour’s immutable preview and ZIP paths.

- [ ] **Step 7: Commit the builder**

```bash
git add templates/creator-pages/production.json internal/templatecatalog cmd/template-catalog
git commit -m "feat: build creator template catalog artifacts"
```

## Task 2: Add signed remote retrieval and verified template fetch commands

**Files:**
- Modify: `internal/buildinfo/buildinfo.go`
- Modify: `internal/commerceartifact/artifact.go`
- Modify: `internal/commerceartifact/artifact_test.go`
- Create: `internal/templatecatalog/client.go`
- Create: `internal/templatecatalog/client_test.go`
- Create: `internal/command/template_catalog.go`
- Create: `internal/command/template_catalog_test.go`
- Create: `cmd/validate-template-catalog-trust-ring/main.go`
- Modify: `internal/command/root.go`
- Modify: `Makefile`
- Modify: `.github/workflows/release.yml`

**Interfaces:**
- Consumes: `manifest.json`, `manifest.sig`, the `TemplateCatalogTrustKeys` build-time ring and a selected `id + version`.
- Produces: `viceme template list` JSON and `viceme template fetch <id> --version <version> --destination <directory>` JSON.

- [ ] **Step 1: Write failing signature and checksum tests**

```go
func TestClientRejectsManifestSignedByUntrustedKey(t *testing.T) {
    server := signedCatalogServer(t, signer("attacker"))
    _, err := templatecatalog.Client{Origin: server.URL, TrustedKeys: map[string]string{"release-v1": trustedSPKI}}.Load(context.Background())
    if !errors.Is(err, templatecatalog.ErrUntrustedManifest) { t.Fatalf("err = %v", err) }
}

func TestFetchDoesNotExtractWhenZIPDigestDiffers(t *testing.T) {
    destination := t.TempDir()
    err := client.Fetch(context.Background(), "bonjour-card", "1.0.0", destination)
    if !errors.Is(err, templatecatalog.ErrSourceDigestMismatch) { t.Fatalf("err = %v", err) }
    entries, _ := os.ReadDir(destination)
    if len(entries) != 0 { t.Fatal("digest mismatch wrote files") }
}
```

- [ ] **Step 2: Run the new client tests and verify they fail**

Run: `go test ./internal/templatecatalog -run 'Test(ClientRejectsManifestSignedByUntrustedKey|FetchDoesNotExtractWhenZIPDigestDiffers)' -count=1`

Expected: FAIL because `Client`, signature handling and fetch logic are absent.

- [ ] **Step 3: Reuse the official trust-ring parser for detached manifest signatures**

Add `VerifyDetachedDocument(value any, trustedPublicKey, encodedSignature string) error` to `internal/commerceartifact/artifact.go`; it delegates to the existing canonical-JSON and Ed25519 verification path used by `VerifyDocument`. Add tests for valid, malformed and wrong-key signatures.

Add this build-time value to `internal/buildinfo/buildinfo.go`:

```go
// TemplateCatalogTrustKeys is keyId:base64url-spki[,keyId:base64url-spki].
TemplateCatalogTrustKeys = ""
```

Add `TEMPLATE_CATALOG_TRUST_KEYS` and the matching `-X github.com/ViceMe-AI/cli/internal/buildinfo.TemplateCatalogTrustKeys=$(TEMPLATE_CATALOG_TRUST_KEYS)` injection to `Makefile`. Create `cmd/validate-template-catalog-trust-ring/main.go`; it reads `TEMPLATE_CATALOG_TRUST_KEYS`, calls `commerceartifact.ParseTrustRing`, requires at least one key, and exits non-zero on malformed input.

Update `.github/workflows/release.yml` so its quality job passes `vars.TEMPLATE_CATALOG_TRUST_KEYS` into this validator and every release build through `TEMPLATE_CATALOG_TRUST_KEYS`. Add an explicit `go run ./cmd/validate-template-catalog-trust-ring` step immediately beside the existing Commerce Skill trust-ring validation. This ensures a published CLI always contains the public key that verifies the independently published catalog.

- [ ] **Step 4: Implement remote client validation and atomic extraction**

`Client.Load` must fetch only `<origin>/manifest.json` and `<origin>/manifest.sig`, require HTTPS in production, strictly decode both files, select the key from embedded `TemplateCatalogTrustKeys`, and verify the canonical manifest before returning it. `Client.Fetch` must resolve the requested `id + version` only from that verified manifest, download only its `source_url`, stream SHA-256 validation into a temporary file, validate ZIP paths, then atomically extract beneath `--destination/<id>`.

Return these stable errors: `TEMPLATE_CATALOG_UNAVAILABLE`, `TEMPLATE_CATALOG_SIGNATURE_INVALID`, `TEMPLATE_CATALOG_TEMPLATE_NOT_FOUND`, `TEMPLATE_CATALOG_SOURCE_DIGEST_MISMATCH`, and `TEMPLATE_CATALOG_ARCHIVE_INVALID`.

- [ ] **Step 5: Add command-level failing tests and implement the commands**

```go
exit, envelope, _ := executeCommand(t, server, "template", "list")
if exit != 0 || envelope.Data["catalog_url"] != server.URL+"/index.html" { t.Fatalf("%d %#v", exit, envelope) }

exit, envelope, _ = executeCommand(t, server, "template", "fetch", "bonjour-card", "--version", "1.0.0", "--destination", t.TempDir())
if exit != 0 || envelope.Data["template_id"] != "bonjour-card" { t.Fatalf("%d %#v", exit, envelope) }
```

Implement `newTemplateCatalogCommand(runtime)` in `internal/command/template_catalog.go`, register it in `NewRoot`, and use `runtime.deps.HTTPClient` plus `runtime.region` to choose the CN or Global origin. Permit `VICEME_TEMPLATE_CATALOG_ORIGIN` only when `buildinfo.Version == "dev"`; it is the dev integration override and is ignored by release binaries.

- [ ] **Step 6: Run the remote-client and command tests**

Run: `go test ./internal/commerceartifact ./internal/templatecatalog ./internal/command -run 'Template|DetachedDocument' -count=1`

Expected: valid catalog lists and fetches; invalid signature, untrusted key, bad SHA, invalid ZIP and release-binary override all fail without destination writes.

- [ ] **Step 7: Commit verified retrieval**

```bash
git add internal/buildinfo/buildinfo.go internal/commerceartifact internal/templatecatalog internal/command/template_catalog.go internal/command/template_catalog_test.go internal/command/root.go cmd/validate-template-catalog-trust-ring Makefile .github/workflows/release.yml
git commit -m "feat: fetch verified cloud creator templates"
```

## Task 3: Publish catalogs independently to CN and Global

**Files:**
- Create: `.github/workflows/template-catalog.yml`
- Create: `docs/creator-template-catalog.md`
- Modify: `Makefile`

**Interfaces:**
- Consumes: `make template-catalog`, `VICEME_TEMPLATE_CATALOG_SIGNING_KEY`, existing release S3 credentials, and the source catalog.
- Produces: identical immutable catalog artifacts in both regional template prefixes and a stable signed manifest per channel.

- [ ] **Step 1: Write a failing workflow-content test**

Add a Go test in `cmd/template-catalog/main_test.go` that reads `.github/workflows/template-catalog.yml` and requires `templates/dev/`, `templates/`, `VICEME_TEMPLATE_CATALOG_SIGNING_KEY`, both regional release credential sets, `manifest.sig`, and a post-upload `sha256sum` comparison. Run it before the workflow exists.

Run: `go test ./cmd/template-catalog -run TestWorkflowPublishesSignedCatalogToBothRegions -count=1`

Expected: FAIL because the workflow file is absent.

- [ ] **Step 2: Add local build targets**

Add these Make targets:

```make
template-catalog:
	$(GO) run ./cmd/template-catalog --source templates/creator-pages/production.json --output dist/template-catalog --origin $(TEMPLATE_CATALOG_ORIGIN) --signing-key-file $(TEMPLATE_CATALOG_SIGNING_KEY_FILE)

template-catalog-check:
	$(GO) test ./internal/templatecatalog ./cmd/template-catalog ./internal/command -run 'Template|DetachedDocument' -count=1
```

The target must fail when either `TEMPLATE_CATALOG_ORIGIN` or `TEMPLATE_CATALOG_SIGNING_KEY_FILE` is empty; it must never generate an unsigned manifest.

- [ ] **Step 3: Implement independent publication workflow**

Create `.github/workflows/template-catalog.yml` with:

- `pull_request` validation for changes under `templates/creator-pages/**`, `skills/customize-your-page/templates/**`, `internal/templatecatalog/**`, `cmd/template-catalog/**`, and the workflow itself;
- `push` to `dev` publishing `templates/dev/` in both regions with `public,max-age=60` for its stable manifest;
- `push` to `main` publishing `templates/` in both regions with `public,max-age=300` for stable `index.html`, `manifest.json`, and `manifest.sig`;
- immutable version paths using `public,max-age=31536000,immutable` and the same existing-object byte comparison used by `.github/workflows/release.yml`;
- `VICEME_RELEASE_S3_ENDPOINT_CN`, `VICEME_RELEASE_S3_BUCKET_CN`, `VICEME_RELEASE_S3_ACCESS_KEY_ID_CN`, `VICEME_RELEASE_S3_SECRET_ACCESS_KEY_CN`, `CN_S3_HTTPS_PROXY`, and corresponding `GLOBAL` secrets;
- `VICEME_TEMPLATE_CATALOG_SIGNING_KEY` for the Ed25519 private key;
- post-upload GET, SHA-256, signature and `index.html` link checks in both regions.

- [ ] **Step 4: Document contributor and rollback paths**

In `docs/creator-template-catalog.md`, document the source JSON fields, source/preview ownership and license requirement, local `make template-catalog` invocation, dev versus production prefixes, how to remove a template from the stable manifest, and the rule that immutable artifacts are never overwritten.

- [ ] **Step 5: Run workflow and documentation checks**

Run: `go test ./cmd/template-catalog -run TestWorkflowPublishesSignedCatalogToBothRegions -count=1`

Run: `make template-catalog-check GO=$(command -v go)`

Expected: workflow contract test and local catalog checks pass.

- [ ] **Step 6: Commit cloud publication**

```bash
git add .github/workflows/template-catalog.yml docs/creator-template-catalog.md Makefile cmd/template-catalog
git commit -m "ci: publish creator template catalog"
```

## Task 4: Switch the creator-page Skill to cloud catalog behavior

**Files:**
- Modify: `skills/customize-your-page/SKILL.md`
- Modify: `internal/skillcontent/official_content_test.go`
- Modify: `quality/release-manifest.json`

**Interfaces:**
- Consumes: `viceme template list` output with `catalog_url` and verified production templates; `viceme template fetch` output with the extracted source path.
- Produces: creator-facing “查看所有模板” and preview links, plus a verified source path after a user selects a template in the conversation.

- [ ] **Step 1: Write failing user-contract tests**

Replace the current local registry assertions with:

```go
for _, required := range []string{
    "viceme template list",
    "查看所有模板",
    "viceme template fetch <模板 ID>",
    "不得向正式用户称为模板来源",
    "不得展示本地绝对路径",
    "不得自动选择 Bonjour",
} {
    if !strings.Contains(pageText, required) { t.Fatalf("cloud catalog contract omitted %q", required) }
}
for _, forbidden := range []string{"读取本机模板册", "catalog-previews/", "dev_mock"} {
    if strings.Contains(creatorFacingSection, forbidden) { t.Fatalf("creator-facing catalog leaked %q", forbidden) }
}
```

- [ ] **Step 2: Run the contract test and verify it fails**

Run: `go test ./internal/skillcontent -run TestCreatorPersonalCardUsesVerifiedCloudCatalog -count=1`

Expected: FAIL because the Skill still reads `templates/registry.json`.

- [ ] **Step 3: Update the Skill’s template path**

Replace the local registry instruction with this execution sequence:

```text
用户选择查看模板后，运行 viceme template list。先给 list 返回的 catalog_url 作为“查看所有模板”链接，
再只列出响应中 production 模板的名称、场景、简介和 preview_url。用户在对话中明确选择模板后，
运行 viceme template fetch <模板 ID> --version <版本> --destination <当前工作区>；仅使用命令返回的
已校验 source_path，继续资料收集与本机预览。
```

Keep the current natural final question: “你想选择哪一款，还是想导入一个已有主页？” Keep the existing import behavior and four-category data collection unchanged.

- [ ] **Step 4: Regenerate the release manifest and run Skill checks**

Run: `make release-manifest GO=$(command -v go)`

Run: `go test ./internal/skillcontent -count=1`

Expected: the Skill contains no user-visible local catalog wording and all official-content tests pass.

- [ ] **Step 5: Commit the Agent integration**

```bash
git add skills/customize-your-page/SKILL.md internal/skillcontent/official_content_test.go quality/release-manifest.json
git commit -m "feat: use cloud creator template catalog"
```

## Task 5: Verify the dev channel end to end and prepare production promotion

**Files:**
- Modify: `docs/creator-template-catalog.md`
- Test: `internal/command/template_catalog_test.go`
- Test: `internal/skillcontent/official_content_test.go`

**Interfaces:**
- Consumes: deployed `templates/dev/` artifacts, a development CLI built with `TemplateCatalogTrustKeys`, and the explicit dev origin override.
- Produces: evidence that the Agent can show cloud links and fetch only the chosen Bonjour version without publishing a page.

- [ ] **Step 1: Add a dev-origin command integration test**

```go
func TestDevelopmentBuildAcceptsDevCatalogOverride(t *testing.T) {
    t.Setenv("VICEME_TEMPLATE_CATALOG_ORIGIN", server.URL+"/templates/dev")
    exit, envelope, _ := executeTemplateCommand(t, "template", "list")
    if exit != 0 || envelope.Data["catalog_url"] != server.URL+"/templates/dev/index.html" { t.Fatalf("%d %#v", exit, envelope) }
}
```

Also add a release-build test that sets the same environment variable and expects the normal regional origin, not the override.

- [ ] **Step 2: Run the test and verify it fails before the dev override is wired**

Run: `go test ./internal/command -run TestDevelopmentBuildAcceptsDevCatalogOverride -count=1`

Expected: FAIL because the command does not yet resolve the dev catalog origin.

- [ ] **Step 3: Run the deployed dev acceptance sequence**

After the `dev` workflow finishes, use a CLI built from this branch with `CI=1` and `VICEME_TEMPLATE_CATALOG_ORIGIN=https://s3.viceme.cn/templates/dev`:

```bash
CI=1 viceme template list
CI=1 viceme template fetch bonjour-card --version 1.0.0 --destination /tmp/viceme-template-acceptance
```

Verify the returned catalog URL opens, the preview opens in the right browser, the ZIP digest matches, and the extracted root is `/tmp/viceme-template-acceptance/bonjour-card`. Do not upload, preview, publish or alter a real creator page in this test.

- [ ] **Step 4: Run the full repository verification**

Run: `make check GO=$(command -v go)`

Run: `make npm-package-check GO=$(command -v go)`

Expected: both commands exit `0`; the release smoke output includes non-empty digests for `customize-your-page`.

- [ ] **Step 5: Commit verification documentation**

```bash
git add docs/creator-template-catalog.md internal/command/template_catalog_test.go internal/skillcontent/official_content_test.go
git commit -m "test: verify cloud creator template catalog"
```

## Deferred follow-up: browser-to-Agent template handoff

Do not include this in the cloud catalog pull request. It requires a confirmed right-side-browser host capability and belongs in a separate design and implementation plan. Its acceptance contract is limited to an allowlisted page origin and the exact message:

```json
{ "type": "viceme.template.use", "id": "bonjour-card", "version": "1.0.0" }
```

The host must render this as a visible user message; it must reject arbitrary URLs, source paths, Merchant IDs, creator data and publish arguments.
