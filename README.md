# ViceMe CLI

[![npm version](https://img.shields.io/npm/v/@viceme-ai/cli.svg)](https://www.npmjs.com/package/@viceme-ai/cli)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.23-blue.svg)](https://go.dev/)
[![CLI PR checks](https://github.com/ViceMe-AI/cli/actions/workflows/ci.yml/badge.svg)](https://github.com/ViceMe-AI/cli/actions/workflows/ci.yml)

[中文版](./README.zh.md) | [English](./README.md)

The official command-line client and Agent Skill for publishing external Skills as stable, shareable ViceMe Agents. It is built for AI coding tools such as Codex and Claude Code: the Agent Skill understands user intent, while the CLI provides the deterministic authentication, upload, publication, and status protocol.

[Install](#installation--quick-start) · [AI Agent Skills](#agent-skills) · [Auth](#authentication) · [Regions & profiles](#regions--profiles) · [Commands](#command-overview) · [Output contract](#json-output-contract) · [Security](#security-and-risk-controls) · [Development](#development)

> **Rollout status:** the Core publication transport and stable-link path are implemented, and the metadata review plus exact Candidate preview → test run → result-confirmation gates are enforced. After `--yes`, the publication first parks at `meta_review` for `job metadata`, then at `awaiting_action` for `job preview`, optional `job edit`, `job run`, `job accept`, and `job resume`. Confirm is accepted only after the exact Candidate has a succeeded, owner-accepted preview run (otherwise 409 `preview_run_required`). A confirm receipt authorizes release but does not contain the final share link; run another bounded `job wait` until `share_published`. `--yes` confirms only the publication request, not the metadata or final Candidate.

## Why ViceMe CLI?

- **Agent-native** — ships together with the official ViceMe Agent Skill for Codex and Claude Code.
- **Deterministic boundary** — the CLI performs typed protocol actions; it does not start another conversational Agent loop.
- **Server-side compilation** — source parsing, LLM compilation, BuildRun materialization, and Release publication stay on ViceMe infrastructure.
- **Stable publishing** — later releases of the same logical Agent keep the same share URL.
- **Multiple source types** — accepts GitHub Skills, pasted Xiaohongshu/RedSkill expressions, archives, and local Skill folders.
- **Secure by default** — on macOS, device-login credentials use AES-256-GCM encrypted files with Keychain-backed key material; other platforms retain their native credential manager. Explicit local overrides require a private profile file, public mutations require confirmation, and downloaded binaries are checksum-verified.
- **Human and Agent login modes** — `viceme auth login` guides a person in the terminal, while Agent split-flows use explicit JSON.

## Installation & Quick Start

### Requirements

- Node.js 18.20 or newer
- macOS or Linux on `amd64`/`arm64`, or Windows on `amd64`/`arm64`

### Quick Start (Human Users)

> **Note for AI assistants:** If you are an AI Agent helping the user install ViceMe, jump directly to [Quick Start (AI Agent)](#quick-start-ai-agent). It defines the required cross-turn device login flow.

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

Both methods persist the npm launcher, download the matching checksum-verified Go binary, and install the bundled ViceMe Agent Skill. Binary downloads try GitHub Release first, then a configured non-default npm registry's `/-/binary/viceme-cli/` mirror, and finally the public npmmirror binary mirror. The launcher uses system `curl`, so standard proxy environment variables are honored. They default to the China service. For the international service:

```bash
npx --yes @viceme-ai/cli@latest install --region global
```

#### Authenticate and verify

If the installation result says authentication is required, start the guided device login:

```bash
viceme auth login
```

The CLI prints the browser URL, waits for authorization, and reports success in the same terminal. Then verify the installation:

```bash
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

Read `data.authenticated` and `data.next_step` from the result. If authentication is already valid, continue to Step 4. If login is required, do not execute the human-oriented `data.next_step` inside the Agent; use the JSON split-flow in Step 2.

**Step 2 — Start device login when required**

```bash
viceme auth login --no-wait --json
```

Return the exact `data.verification_url`; the CLI normalizes it to the prefilled `verification_url_complete` browser link when available. Include `data.user_code` only as a fallback if the browser asks for it. Preserve `data.device_code` for the continuation command, then stop the current turn. Do not request, print, or place an access token in the conversation.

**Step 3 — Continue the same login in a later turn**

After the user confirms browser authorization:

```bash
viceme auth login --device-code <device-code> --json
```

If authorization is still pending, reuse the same device code before it expires. Do not start a second device flow unless the original one has expired.
For a non-default profile, pass the same global `--profile <name>` when starting and continuing device login; the start result reports the effective `profile` and `region`.

**Step 4 — Verify**

```bash
viceme auth status
viceme skills doctor
viceme skills list
```

Continue only when authentication is valid and `skills doctor` reports a healthy, compatible installation.

**Step 5 — Inspect the first source**

```bash
viceme skill inspect https://github.com/acme/poster-skill --skill-root .
```

Inspection is read-only. Follow the bundled `viceme` Skill for source-specific handling, Target selection, confirmation, bounded job waiting, and result reporting. If a publication ends at `binding_required`, run `viceme job bind <publication-id>`, give the signed ViceMe URL to the user, and stop. Downloading or forking is only an informational alternative; the CLI never performs it automatically. After the user binds the exact GitHub/Xiaohongshu channel account, inspect again and create a fresh ordinary publication rather than resuming the terminal one. At `meta_review`, show and resolve the metadata using the exact action ID and payload digest, then wait again. At `awaiting_action`, show the frozen Candidate summary, run and accept one exact-Candidate test result, and obtain the user's decision before `job resume`. After confirm, wait again until `share_published` before returning the share link.

## Regions & Profiles

Each profile selects one ViceMe region:

| Region | Install command | API endpoint |
|---|---|---|
| China | `viceme install` | `https://api.viceme.cn` |
| International | `viceme install --region global` | `https://api.viceme.ai` |

The first install creates the `default` profile. Device-login credentials are isolated by profile plus normalized API origin. On macOS they are stored as private AES-256-GCM encrypted files, while the encryption master key normally remains in the operating-system Keychain; other platforms retain their native credential manager. The profile file is normally non-sensitive; explicitly configuring a local access-token override turns it into a secret-bearing `0600` file.

```bash
viceme profile list
viceme profile add --name work --region global --use
viceme profile use default
viceme --profile work auth status
viceme profile rename work company
viceme profile remove company
```

`profile use` changes the persistent active profile; the global `--profile` flag overrides only one command. AI Agents must not switch or remove profiles unless the user explicitly requests it.

For an explicitly authorized local/internal test, create a dedicated profile with explicit endpoint and token overrides:

```bash
viceme profile add --name local --region cn \
  --api-base-url http://localhost:8090 --access-token 'YOUR_ACCESS_TOKEN' --use

viceme profile configure local --access-token 'YOUR_ACCESS_TOKEN'
viceme profile configure local --clear-access-token
viceme profile configure local --clear-api-base-url
```

Normal `viceme auth login` never writes `apiBaseUrl` or `accessToken` into a profile. An explicit local token is bound to that profile's normalized API origin and takes precedence over the profile's secure-store login only on that origin; changing origins requires replacing or clearing the token in the same command. While it is active, `auth login` and `auth logout` fail closed. `profile list` and `auth status` report only `source=local_profile`, never the token. Remove the override as soon as the internal test is complete.

`VICEME_CLI_CONFIG_DIR` can override the config root. `VICEME_API_BASE_URL` and `VICEME_ACCESS_TOKEN` remain available as one-process overrides and take precedence over the selected profile. Otherwise the profile's explicit `apiBaseUrl`/`accessToken` is used before the region endpoint and secure-store login. A different normalized origin uses an isolated scope. API and presigned-upload requests fail closed on redirects so credential headers are never forwarded to another origin.

Update checks query the npm registry directly and store only the last successful version result in `~/.viceme-cli/update-state.json`. A result is used as a fallback for at most 24 hours when the registry is temporarily unavailable. npm operations launched by `viceme install` or `viceme update` use the isolated `~/.viceme-cli/npm-cache`, so a broken user-level `~/.npm` cache does not block the CLI. Both files are non-secret and can be deleted safely; credentials never enter either cache.

## Agent Skills

The current release deliberately ships one platform-level Agent Skill:

| Skill | Description | Supported hosts |
|---|---|---|
| `viceme` | Install, inspect, convert, publish, update, or share external Skills as stable ViceMe Agents; enforces authentication, source, Target, confirmation, job, and safety rules | Codex, Claude Code |

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
| `viceme auth status` | Show whether the current profile is authenticated |
| `viceme auth login` | Guide a human through browser authorization and wait for completion |
| `viceme auth login --no-wait --json` | Start an Agent split-flow and return structured device authorization |
| `viceme auth login --device-code <code> --json` | Complete an Agent split-flow in a later turn |
| `viceme auth logout` | Revoke and remove the current profile credential |

On macOS, tokens created by device login are stored only in private encrypted credential files and their master key normally remains in the operating-system Keychain; other platforms retain their native credential manager. Normal login never backfills explicit local profile fields, and successful login output never contains the access or refresh token.

Before starting or exchanging a device authorization, the CLI verifies the full local persistence path. If that preflight fails, no one-time authorization is consumed. If storage fails after a successful exchange despite the preflight, the CLI attempts to revoke the issued credential and returns `credential_persistence_failed` with an explicit instruction to start a new device flow; it never reports login success or outputs a token.

### macOS sandboxes (Codex and Claude Code)

A fresh sandboxed installation can create a private `0600` file master key when the macOS Keychain is blocked. If credentials were previously created from Terminal with a Keychain-backed master key, run this once from that same interactive macOS user session:

```bash
viceme config keychain-downgrade
```

The command copies the existing master key into `~/.viceme-cli/credentials/master.key.file` and imports configured legacy Keychain credentials into encrypted files. Existing Keychain entries are preserved as a cold backup. The command is idempotent and never prints or stores a plaintext token. Afterward, Codex and Claude Code sandboxes for the same macOS user can read the encrypted credential files without Keychain access. The trade-off is explicit: security is then enforced by the user's filesystem permissions (`0700` directory and `0600` files) instead of the Keychain per-process access boundary.

The public CLI exposes one standard authentication and publication surface. A short-lived generic credential may be supplied by `VICEME_ACCESS_TOKEN` (`source=process`) or deliberately persisted in a dedicated internal-test profile with `--access-token` (`source=local_profile`). Both use the normal inspect/publish/job commands and the standard `x-api-key` header. Neither path adds identity-selection, delegated-publication, or authorization-issuance commands. Tokens are never printed; login/logout fail closed while either override is active, and update subprocesses do not inherit process credentials. Because the explicit flag may be visible in shell history and process arguments, use it only in the trusted internal test environment described here.

## Supported Sources

### GitHub or trusted provider

```bash
viceme skill inspect https://github.com/acme/poster-skill --skill-root .
viceme skill publish --resolution-id <resolution-id> --yes
```

For GitHub, `--skill-root` is required and names the exact repository-relative directory containing `SKILL.md`; use `.` only for a root-level Skill. The calling Agent determines this path from the user input or read-only repository tree. ViceMe does not scan the repository to guess a Skill.

### Xiaohongshu or RedSkill copied expression

```bash
viceme skill inspect --expression-stdin
viceme skill publish --resolution-id <resolution-id> --yes
```

The copied expression is untrusted data. ViceMe extracts a locator and fetches the source through an approved connector; it never executes marketplace installation text.

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
| `viceme install` | Install the persistent launcher, Agent Skill, and default profile |
| `viceme auth` | Start, complete, inspect, or revoke device authentication |
| `viceme config` | Manage the controlled macOS Keychain-to-file sandbox fallback |
| `viceme profile` | Add, list, switch, rename, or remove local profiles |
| `viceme skill inspect` | Freeze and inspect a source candidate without publishing |
| `viceme skill publish` | Create or update a stable Skill Agent publication |
| `viceme skill target` | Resolve existing logical Agent Targets and versions |
| `viceme job` | Read or wait for a publication, review metadata, preview/edit/test/accept its Candidate, show a signed channel-binding URL, resume an action, explicitly retry, or cancel |
| `viceme skills` | Read, install, and diagnose the bundled Agent Skill |
| `viceme update` | Update the npm launcher, verified binary, and bundled Skill together |

Use `viceme <command> --help` for the exact flags. The release-checked machine-readable surface is stored in [`skills/viceme/references/command-manifest.json`](skills/viceme/references/command-manifest.json).

## JSON Output Contract

Automation-oriented data commands emit a stable JSON envelope by default. Interactive `viceme auth login` is the deliberate human-facing exception; AI Agents and scripts must use `--no-wait --json`, then continue with `--device-code <code> --json` in a later turn.

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
- **Untrusted text stays off argv** — AI Hosts must pass copied provider expressions and natural-language Candidate edits through the explicit `--expression-stdin` and `--request-stdin` modes. Never interpolate that text into command strings, argv, environment variables, or shell pipelines.
- **Explicit public mutation** — publishing, compiler retry, and cancellation require `--yes`; exit code `10` means the Agent must obtain confirmation, not silently retry.
- **Safe preview** — use `--dry-run` on inspect or publish when the user needs to review the planned request without network or publication side effects.
- **Credential isolation** — on macOS, device-login credentials stay in AES-256-GCM encrypted files, with Keychain-backed or explicitly downgraded private key material; filenames do not expose profile/origin names. Other platforms retain their native credential manager. Explicit internal-test overrides are namespaced by profile, stored only in a private `0600` config, and never emitted by CLI output.
- **Immutable inputs** — inspection binds publication to an immutable source snapshot rather than re-reading a floating URL later.
- **Bounded waiting** — `job wait` has a maximum duration and returns the latest durable state without cancelling the workflow.
- **Bounded compiler recovery** — `job retry` keeps the frozen source and publication, accepts only an explicitly retryable platform failure, and is capped by the server.
- **Verified distribution** — the npm launcher downloads the binary for its exact package version from GitHub or a binary mirror and verifies it against the checksum manifest bundled in the npm package before activation.

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
