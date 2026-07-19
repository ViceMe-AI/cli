# Viceme CLI

[![npm version](https://img.shields.io/npm/v/@viceme-ai/cli.svg)](https://www.npmjs.com/package/@viceme-ai/cli)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.23-blue.svg)](https://go.dev/)
[![CLI quality gates](https://github.com/ViceMe-AI/cli/actions/workflows/ci.yml/badge.svg)](https://github.com/ViceMe-AI/cli/actions/workflows/ci.yml)

[中文版](./README.zh.md) | [English](./README.md)

The official command-line client and Agent Skill for publishing external Skills as stable, shareable Viceme Agents. It is built for AI coding tools such as Codex and Claude Code: the Agent Skill understands user intent, while the CLI provides the deterministic authentication, upload, publication, and status protocol.

[Install](#installation--quick-start) · [AI Agent Skills](#agent-skills) · [Auth](#authentication) · [Regions](#regions) · [Commands](#command-overview) · [Output contract](#json-output-contract) · [Security](#security-and-risk-controls) · [Development](#development)

> **Rollout status:** the Core publication transport and stable-link path are implemented behind a controlled allowlist. Public rollout remains blocked until the exact Candidate preview, test run, and result-confirmation gate is complete. The current `--yes` confirms the publication request; it is not proof that the user reviewed the final Candidate.

## Why Viceme CLI?

- **Agent-native** — ships together with the official Viceme Agent Skill for Codex and Claude Code.
- **Deterministic boundary** — the CLI performs typed protocol actions; it does not start another conversational Agent loop.
- **Server-side compilation** — source parsing, LLM compilation, BuildRun materialization, and Release publication stay on Viceme infrastructure.
- **Stable publishing** — later releases of the same logical Agent keep the same share URL.
- **Multiple source types** — accepts GitHub Skills, pasted Xiaohongshu/RedSkill expressions, archives, and local Skill folders.
- **Secure by default** — credentials use the operating-system keychain, public mutations require confirmation, and downloaded binaries are checksum-verified.
- **Machine-stable output** — every data command uses the same JSON success/error envelope without an output-format flag.

## Installation & Quick Start

### Requirements

- Node.js 18.20 or newer
- macOS or Linux on `amd64`/`arm64`, or Windows on `amd64`/`arm64`

### Quick Start (Human Users)

> **Note for AI assistants:** If you are an AI Agent helping the user install Viceme, jump directly to [Quick Start (AI Agent)](#quick-start-ai-agent). It defines the required cross-turn device login flow.

#### Install

Choose one of the following methods.

**Option 1 — One-shot npm install (recommended):**

```bash
npx --yes @viceme-ai/cli@latest install
```

**Option 2 — Install the npm launcher globally:**

```bash
npm install --global @viceme-ai/cli
viceme install
```

Both methods persist the npm launcher, download the matching checksum-verified Go binary, and install the bundled Viceme Agent Skill. They default to the China service. For the international service:

```bash
npx --yes @viceme-ai/cli@latest install --region global
```

#### Authenticate and verify

If the installation result says authentication is required, start device login:

```bash
viceme auth login --no-wait
```

Open the returned `verification_url`; it normally links directly to the matching prefilled device request. Complete authorization, and then continue with the returned device code:

```bash
viceme auth login --device-code <device-code>
viceme auth status
viceme skills doctor
```

## Quick Start (AI Agent)

> The following steps are for AI Agents. Browser authorization must be completed by the user in a separate turn; never wait indefinitely or ask the user for a token.

**Step 1 — Install**

Use the complete bootstrap command from the bundled Skill. The explicit npm registries are part of the installation trust boundary:

```bash
npx --yes --registry=https://registry.npmjs.org --@viceme-ai:registry=https://registry.npmjs.org --package=@viceme-ai/cli@latest -- viceme install
```

Read `data.authenticated` and `data.next_step` from the result. If authentication is already valid, continue to Step 4.

**Step 2 — Start device login when required**

```bash
viceme auth login --no-wait
```

Return the exact `data.verification_url`; the CLI normalizes it to the prefilled `verification_url_complete` browser link when available. Include `data.user_code` only as a fallback if the browser asks for it. Preserve `data.device_code` for the continuation command, then stop the current turn. Do not request, print, or place an access token in the conversation.

**Step 3 — Continue the same login in a later turn**

After the user confirms browser authorization:

```bash
viceme auth login --device-code <device-code>
```

If authorization is still pending, reuse the same device code before it expires. Do not start a second device flow unless the original one has expired.

**Step 4 — Verify**

```bash
viceme auth status
viceme skills doctor
viceme skills list
```

Continue only when authentication is valid and `skills doctor` reports a healthy, compatible installation.

**Step 5 — Inspect the first source**

```bash
viceme skill inspect https://github.com/acme/poster-skill
```

Inspection is read-only. Follow the bundled `viceme` Skill for source-specific handling, Target selection, confirmation, bounded job waiting, and result reporting. Public publication remains blocked until the exact Candidate confirmation gate described above is complete.

## Regions

Viceme exposes one product-level region choice during installation:

| Region | Install command | API endpoint |
|---|---|---|
| China | `viceme install` | `https://api.viceme.cn` |
| International | `viceme install --region global` | `https://api.viceme.ai` |

The selected value is persisted as `region=cn|global`; later commands use it automatically. Credentials are isolated by region, so a China token is never reused against the international API, or vice versa.

There is no public API URL, profile, or output-format configuration. For local development only, set `VICEME_API_BASE_URL` in the terminal environment. An override on the current region's canonical origin keeps the compatible `cn` or `global` keychain scope, even with a base path; a different normalized origin uses an isolated scope and requires separate authentication. API and presigned-upload requests fail closed on redirects so credential headers are never forwarded to another origin.

## Agent Skills

The current release deliberately ships one platform-level Agent Skill:

| Skill | Description | Supported hosts |
|---|---|---|
| `viceme` | Install, inspect, convert, publish, update, or share external Skills as stable Viceme Agents; enforces authentication, source, Target, confirmation, job, and safety rules | Codex, Claude Code |

GitHub, Xiaohongshu/RedSkill, ZIP, and folder inputs are source types handled by the same `viceme` publication workflow, not separate Agent Skills. This keeps one consistent safety and stable-link contract across providers.

The CLI and `viceme` Agent Skill are released from this repository at the same version. `viceme install` installs the complete Skill bundle into detected supported hosts, while the binary embeds the agent-readable subset needed for deterministic self-inspection.

```bash
viceme skills list
viceme skills read viceme
viceme skills read viceme references/commands.md
viceme skills doctor
```

`skills doctor` checks the CLI version, Skill version, compatibility range, full bundle digest, and embedded-content digest independently. A modified or incompatible installation fails closed.

## Authentication

| Command | Purpose |
|---|---|
| `viceme auth status` | Show whether the current region is authenticated |
| `viceme auth login --no-wait` | Start device authorization and return immediately |
| `viceme auth login --device-code <code>` | Complete a previously started authorization |
| `viceme auth logout` | Revoke and remove the current region credential |

Tokens are stored only in the operating-system keychain. There is no plaintext token fallback, and successful login output never contains the access or refresh token.

Ordinary authenticated users publish directly. Staff-operated delegated publication uses the same host-neutral command contract, with one-time credentials accepted only from protected non-TTY stdin or an OS-keychain reference and sent only through a dedicated request header. Raw delegated credentials are never accepted as flags or environment configuration and are never returned in CLI output.

Saving a delegated grant creates one versioned keychain entry containing the credential and a stable, non-sensitive recovery identity. Publishing an expression with `--delegated-grant-ref` first inspects the source and selects an immutable candidate before reading the grant; use `--skill-root` when inspection returns multiple candidates. The grant's Target scope must match the destination: the default `auto` destination requires `UPSERT`, `--new-target` requires `CREATE`, and `--target-id` requires `UPDATE`. The first exact request is persisted and reused after an ambiguous transport failure, and the whole entry is deleted only after a valid server receipt. A direct protected-stdin publication has no persistent recovery boundary, so it requires both an existing `--resolution-id` and an explicit `--client-request-id`. Delegated ZIP and directory uploads are rejected before reading the grant or uploading data.

## Supported Sources

### GitHub or trusted provider

```bash
viceme skill inspect https://github.com/acme/poster-skill
viceme skill publish --resolution-id <resolution-id> --yes
```

### Xiaohongshu or RedSkill copied expression

```bash
viceme skill inspect --expression-stdin
viceme skill publish --resolution-id <resolution-id> --yes
```

The copied expression is untrusted data. Viceme extracts a locator and fetches the source through an approved connector; it never executes marketplace installation text.

### Archive or local Skill folder

```bash
viceme skill publish --file ./poster-skill.zip --new-target --target-alias poster --yes
viceme skill publish --dir ./poster-skill --new-target --target-alias poster --yes
```

For later releases, resolve the existing Target and use optimistic concurrency. Never turn a conflict into a new share link:

```bash
viceme skill target get target_123
viceme skill publish --file ./poster-skill-v2.zip \
  --target-id target_123 --expected-target-version 4 --yes
```

## Command Overview

| Command group | Purpose |
|---|---|
| `viceme install` | Install the persistent launcher, Agent Skill, and region configuration |
| `viceme auth` | Start, complete, inspect, or revoke device authentication |
| `viceme skill inspect` | Freeze and inspect a source candidate without publishing |
| `viceme skill publish` | Create or update a stable Skill Agent publication |
| `viceme skill target` | Resolve existing logical Agent Targets and versions |
| `viceme job` | Read, wait for, resume, or cancel a durable publication |
| `viceme skills` | Read, install, and diagnose the bundled Agent Skill |
| `viceme update` | Update the npm launcher, verified binary, and bundled Skill together |

Use `viceme <command> --help` for the exact flags. The release-checked machine-readable surface is stored in [`skills/viceme/references/command-manifest.json`](skills/viceme/references/command-manifest.json).

## JSON Output Contract

All data commands emit a stable JSON envelope by default.

Success is written to **stdout** with exit code `0`:

```json
{
  "ok": true,
  "data": {},
  "meta": {
    "cli_version": "0.1.0",
    "skill_version": "0.1.0"
  }
}
```

CLI execution errors are written to **stderr** with a non-zero exit code:

```json
{
  "ok": false,
  "error": {
    "type": "validation",
    "subtype": "source_required",
    "message": "provide exactly one source argument or --expression-stdin"
  },
  "meta": {
    "cli_version": "0.1.0",
    "skill_version": "0.1.0"
  }
}
```

Determine command success from the process exit code or `ok == true`. The API's domain-specific `error.type` is preserved; the exit code is only a coarse handling class. A successfully read publication may still contain a business terminal status such as `unsupported`, `rejected`, or `failed`; inspect `data.status` instead of treating those states as CLI transport failures.

| Exit code | Meaning |
|---|---|
| `0` | Command completed; inspect returned business status when applicable |
| `2` | Validation failure |
| `3` | Authentication or authorization failure |
| `4` | Retryable transport or concurrency failure |
| `5` | Internal or protocol failure |
| `6` | Policy or rollout-gate rejection |
| `10` | Explicit confirmation required |

## Security and Risk Controls

- **No source execution** — the CLI and compiler do not execute third-party scripts, binaries, shell fragments, marketplace commands, or copied instructions.
- **Explicit public mutation** — publishing and cancellation require `--yes`; exit code `10` means the Agent must obtain confirmation, not silently retry.
- **Safe preview** — use `--dry-run` on inspect or publish when the user needs to review the planned request without network or publication side effects.
- **Credential isolation** — credentials stay in the OS keychain and are namespaced by region.
- **Immutable inputs** — inspection binds publication to an immutable source snapshot rather than re-reading a floating URL later.
- **Bounded waiting** — `job wait` has a maximum duration and returns the latest durable state without cancelling the workflow.
- **Verified distribution** — the npm launcher downloads the binary for its exact package version and verifies the published SHA-256 checksum before activation.

## Diagnose and Update

```bash
viceme skills doctor
viceme update --check
viceme update
```

`viceme update` installs one exact npm package version, acquires its verified Go binary, and refreshes the bundled Skill from that same release. A standalone development binary is never silently replaced.

## Development

Go 1.23 or newer is required when building from source.

```bash
make build
make test
make check
make skill-check
make npm-package-check
make quality-check
```

The main checked-in quality artifacts are:

- [`skills/viceme/references/command-manifest.json`](skills/viceme/references/command-manifest.json), generated from the Cobra command tree;
- [`quality/example-dry-runs.json`](quality/example-dry-runs.json), which executes documented source paths without network access;
- [`quality/release-manifest.json`](quality/release-manifest.json), which pins CLI/Skill compatibility and content digests.

`make npm-package-check` builds the Go executable, packs the real npm tarball, and runs the launcher in isolated temporary homes. It does not require an already-published GitHub Release.

## Releases

Maintainers merge normal changes into `dev`; they do not manually edit versions, create tags, write changelog entries, or run `npm publish`. GitHub Actions maintains a single automated `dev -> main` Release PR. Merging that PR authorizes the reviewed version, tag, immutable GitHub Release assets, and npm trusted publication.

See [`docs/releasing.md`](docs/releasing.md) for repository setup, OIDC trusted publishing, recovery, and integrity rules.

## Contributing

Issues and pull requests are welcome. Changes to the public command surface must update the generated command manifest, bundled Skill examples, quality fixtures, and tests in the same PR.
