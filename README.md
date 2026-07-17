# Viceme CLI

`viceme` is the deterministic client used by the bundled Viceme Agent Skill. Source parsing, LLM compilation, BuildRun materialization, and share publication stay server-side.

## Install

Node.js 18.20 or newer is required for the npm launcher.

```bash
npx --yes --registry=https://registry.npmjs.org --@viceme-ai:registry=https://registry.npmjs.org --package=@viceme-ai/cli@latest -- viceme install
```

The root `install` command performs the complete bootstrap: it atomically installs the matching Viceme Skill into Codex and/or Claude Code, creates a non-sensitive local config, checks authentication without printing credentials, and returns the device-login command when needed.

The npm package is intentionally a small launcher, not six copies of the Go executable. It downloads the binary for the package's exact version from the corresponding GitHub Release, verifies the published SHA-256 checksum, writes a complete pair into a unique staging directory, and atomically publishes that directory as an immutable cache generation before executing it. Concurrent cold starts may repeat a download, but never share, overwrite, or delete a mutable binary/checksum pair; an interrupted process leaves only ignored staging state. Supported release targets are macOS and Linux on `amd64`/`arm64`, plus Windows on `amd64`/`arm64`.

## Authenticate and publish

```bash
viceme auth login --no-wait --json
viceme skill inspect https://github.com/acme/poster-skill --json
viceme skill publish --resolution-id <resolution-id> --yes --json
viceme job wait <publication-id> --timeout 60s --json
```

Credentials are stored only in the operating-system keychain. There is no plaintext token fallback.

## Diagnose and update

```bash
viceme skills doctor --json
viceme update --check --json
viceme update --json
```

Every Skill installation records its CLI version, Skill version, minimum compatible CLI version, full bundle digest, and agent-readable embedded digest. `skills doctor` validates each item independently.

`viceme update` is supported for the npm launcher. It resolves the latest version, globally installs that exact package with lifecycle scripts disabled, and then invokes the same exact package to acquire its verified binary and reinstall its bundled Skill. A standalone development binary is not silently replaced; bootstrap it through npm first. Homebrew and signed standalone update manifests remain future distribution channels.

## Development and quality gates

```bash
make build
make test
make check
make npm-package-check
make quality-check
```

`make npm-package-check` does not require a published GitHub Release. It injects the locally built Go executable through `VICEME_BINARY_PATH`, packs the npm tarball, and runs the real npm launcher through `install` in an isolated temporary home. The override is for development and CI only.

The checked-in quality artifacts are:

- `skills/viceme/references/command-manifest.json`, generated from the Cobra command tree;
- `quality/example-dry-runs.json`, which executes the documented inspect/publish paths without network access;
- `quality/release-manifest.json`, which pins CLI/Skill compatibility and both Skill digests.

Set `VICEME_API_BASE_URL` for local integration. Release tags must exactly equal `v` plus the `package.json` version, and release asset names are fixed as `viceme_<version>_<goos>_<goarch>[.exe]` with a sibling `.sha256`. The workflow checks that exactly all six binary/checksum pairs exist and blocks npm publication until they have been uploaded to a completed GitHub Release.

npm publishing uses a pinned OIDC-capable npm CLI and provenance. The steady-state authentication path is GitHub Actions trusted publishing (`id-token: write`): configure `@viceme-ai/cli` to trust `ViceMe-AI/cli` and `.github/workflows/release.yml`. If npm requires a credential for the package's first-ever publication, temporarily configure a granular publish token as the `NPM_TOKEN` repository secret; the workflow exposes it only as `NODE_AUTH_TOKEN` to the final publish-or-verify step. Remove the secret after trusted publishing is configured.

Release reruns are fail-closed. If the GitHub Release already exists, the workflow byte-compares every existing asset with the binaries/checksums rebuilt from the tag, uploads only assets missing after an interrupted release, and then verifies the complete 12-asset set. It never overwrites an existing asset. npm publication accepts stable `x.y.z` versions only. If the exact version already exists, its registry integrity must equal the local `npm pack` integrity; an older `latest` tag is safely promoted to that exact version, while rerunning an older release never demotes a newer `latest`. This lets either publication stage be retried without deleting or replacing immutable content.
