<h1 align="center">ViceMe CLI</h1>

<p align="center">
  <strong>Turn local AI Agent Skills into publishable products—from the conversation you already use.</strong>
</p>

<p align="center">
  Install the CLI and official Agent Skills together, then ask Codex, Claude Code, or WorkBuddy to do the rest.
</p>

<p align="center">
  <a href="./README.zh.md">简体中文</a> ·
  <a href="#quick-start">Quick Start</a> ·
  <a href="#official-agent-skills">Agent Skills</a> ·
  <a href="#command-reference">Commands</a> ·
  <a href="#security">Security</a>
</p>

## Why ViceMe CLI?

- **Agent-native** — describe what you want in natural language instead of
  memorizing a publication command sequence.
- **One installation** — the native CLI and matching official Skills are
  installed together for Codex, Claude Code, WorkBuddy, and the shared
  `~/.agents/skills` fallback.
- **Human-controlled publishing** — the model can suggest bilingual copy and
  media, but only the user decides the price and authorizes public publication.
- **Deterministic and resumable** — the CLI validates and packages locally,
  preserves publication identity across interruptions, and avoids duplicate
  listings when a response is lost.
- **Built for Agents and automation** — stable JSON output, error codes, dry
  runs, and explicit state transitions make every action inspectable.
- **Safe by default** — browser authorization, origin-bound profiles, local
  secret checks, immutable digests, and verified uploads protect the release
  path.

## What you can do

| Capability | What ViceMe provides |
| --- | --- |
| Publish a Skill | Validate a local Skill directory or ZIP, set a CNY price, upload it, review platform suggestions, and publish a paid listing. |
| Build a component | Ask an Agent to integrate the bundled production danmaku blueprint into a React and Tailwind CSS v4 project. |
| Set up an Agent | Install, authenticate, update, diagnose, and repair the CLI and official Skills as one compatible release. |
| Recover safely | Continue the same publication after a network or process interruption without uploading a duplicate product. |

## Quick Start

### With an AI Agent (recommended)

1. Install the CLI and official Skills:

   ```bash
   curl -fsSL https://s3.viceme.cn/start/install.sh | sh
   ```

2. Start a new Codex, Claude Code, or WorkBuddy conversation so the Agent can
   discover the installed Skills.
3. Attach a local Skill directory or ZIP and ask naturally:

   > Publish this Skill to ViceMe.

The Agent checks login, keeps the selected Profile fixed throughout the
workflow, validates the Skill, immediately uploads the private Draft, and opens
its real Owner Preview. It uploads media candidates without asking for a price,
then the user's Agent prepares the bilingual copy and media suggestions. The Agent shows
the complete listing—including the images—and asks one combined question for
the price and any desired changes. It then shows the final review and asks once
whether to confirm and publish it publicly.

```text
Local Skill → Login → Validate and private upload → Owner Preview → Agent enrichment
            → Complete listing details + price question → Final review
            → Confirm and publish → Public URL
```

The initial request to “publish” is not permission to make the listing public.
Public publication happens only after the final review is displayed and the
user explicitly confirms it.

### From the terminal

```bash
# Verify the installation and current account.
viceme doctor
viceme auth status

# Sign in only when auth status reports unauthenticated.
viceme auth login

# Upload the real private draft and open its Owner Preview before pricing.
viceme skill publish --path ./my-skill

# Continue the same unpriced draft and upload media candidates.
viceme skill publish --resume <publication-id>
viceme publication review <publication-id>

# The Agent writes a revision-protected suggestion. Platform analysis is an explicit fallback only.
viceme publication suggest <publication-id> --input <suggestion.json>
# viceme publication analyze <publication-id> && viceme publication wait <publication-id>

# After reviewing the complete listing details, set CNY 1.00 on that draft.
viceme skill publish --resume <publication-id> --price-minor 100
```

## Installation

The official bootstrap installs the native CLI and all official Agent Skills
from one immutable release.

### macOS or Linux

China:

```bash
curl -fsSL https://s3.viceme.cn/start/install.sh | sh
```

International:

```bash
VICEME_REGION=global sh -c "$(curl -fsSL https://s3.viceme.ai/start/install.sh)"
```

### Windows PowerShell

China:

```powershell
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
irm https://s3.viceme.cn/start/install.ps1 | iex
```

International:

```powershell
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
$env:VICEME_REGION="global"; irm https://s3.viceme.ai/start/install.ps1 | iex
```

If the bootstrap cannot be used, npm is the fallback:

```bash
npx --yes @viceme-ai/cli@latest install
```

The installer writes the compatibility fallback to `~/.agents/skills` and the
native user directory of every detected supported Agent:

| Agent | Native directory |
| --- | --- |
| Codex | `~/.codex/skills` |
| Claude Code | `~/.claude/skills` |
| WorkBuddy | `~/.workbuddy/skills` |

Select a target explicitly with `viceme install --agent codex`, `claude`,
`workbuddy`, or `agents`. Run `viceme doctor` after installation or repair.

## Official Agent Skills

| Skill | Use it when you want to... |
| --- | --- |
| `viceme-shared` | install ViceMe, sign in through the browser, manage Profiles, update, diagnose, or repair the local setup. |
| `viceme-publish` | validate, upload, review, resume, or publish a local Skill directory or ZIP as a paid ViceMe listing. |
| `viceme-danmaku` | build or adapt the bundled production React and Tailwind CSS v4 danmaku component in an existing project. |

The Agent Skills own the conversational workflow and approval rules. The CLI
owns deterministic local work and API calls. This separation lets an Agent
explain decisions while the same command contract remains reproducible from a
terminal or automation.

## How Skill publishing works

The current publication flow accepts one local directory containing a root
`SKILL.md`, or one local ZIP. Common GitHub ZIPs with a single wrapping
directory are normalized automatically. Remote URLs and multi-Skill bundles
are not accepted.

| Stage | Responsibility |
| --- | --- |
| Login | Browser authorization happens before protected API work. Every command remains pinned to the same Profile and API origin. |
| Inspect | Local validation rejects unsafe paths, special files, excessive content, sensitive files, and common secret patterns. |
| Private upload and preview | The initial publish request authorizes the private Draft upload. The real package is verified and its Owner Preview opens before pricing. |
| Agent enrichment | The user's Agent reads the Skill as untrusted source data, prepares bilingual copy, and selects verified cover and gallery uploads. Platform analysis is an explicit fallback only. |
| Listing details and price | After enrichment, the Agent displays the exact title, bilingual copy, cover, and gallery, then asks one combined question for the CNY price in fen and any desired changes. It never asks for price by itself. |
| Review | The Agent displays the exact copy, price, cover, and gallery images. A short summary has a maximum display width of 30; ASCII counts as 1 and Chinese/non-ASCII as 2. |
| Publish | One explicit final confirmation authorizes review confirmation followed by immediate, irreversible public publication. |

If the upload or response is interrupted, continue the existing publication:

```bash
viceme skill publish --resume <publication-id>
```

Never create a second publication merely because the previous response was
unknown. Query or resume the existing ID first.

## Authentication and Profiles

Each Profile binds one explicit API endpoint and one device-authorized account.
The separately stored distribution region selects only the CLI and bundled
Skill download source; it never selects a publication market.

```bash
viceme auth login
viceme auth status
viceme auth logout

viceme profile list
viceme profile use default
```

For a test or private deployment, persist a generic HTTPS endpoint in a
dedicated Profile before signing in:

```bash
viceme profile add \
  --name private-cn \
  --api-base-url https://api.example.com \
  --use
viceme auth login
```

`VICEME_API_BASE_URL` is a one-process CI/debug override, not Profile state.
Remote custom endpoints require HTTPS; only localhost and loopback development
may use HTTP. Credentials are isolated by Profile and API origin, and an Agent
must never switch to another Profile merely because it is already signed in.

`viceme auth login` intentionally keeps running until browser authorization
finishes or its bounded timeout expires. Keep the command active, show the
one-time complete URL to the user, and wait for the final result. The page signs
the user in when necessary and authorizes the CLI automatically; the user never
types a device code. If the wait expires, start a fresh `viceme auth login`.
Never copy an access token into the conversation.

## Command reference

| Command | Purpose |
| --- | --- |
| `viceme version` | Show the CLI and bundled Skill versions. |
| `viceme doctor` | Check the CLI, active Profile, credentials, API readiness, and installed official Skills. |
| `viceme auth status` | Show whether the active Profile is signed in. |
| `viceme profile list` | Show Profiles and their effective API endpoints. |
| `viceme skill inspect --path <path>` | Validate a local Skill without side effects. |
| `viceme skill listing prepare --path <path>` | Create or recover the stable private owner preview and persist the local binding. |
| `viceme skill listing get <listing-id>` | Read the authoritative private Listing state. |
| `viceme skill listing bind <listing-id> --path <path>` | Explicitly bind a source to a selected owned Listing. |
| `viceme skill publish --path <path>` | Upload the real private package and return its Owner Preview before pricing. |
| `viceme skill publish --resume <id>` | Continue the same unpriced Draft and upload media candidates without starting a platform model. |
| `viceme publication review <id>` | Read the authoritative bilingual copy, price, selected media, and review state. |
| `viceme publication suggest <id> --input ...` | Apply Agent-generated bilingual copy and media with Draft revision protection. |
| `viceme publication analyze <id>` | Explicitly request platform-model analysis when the current Agent cannot perform enrichment. |
| `viceme publication wait <id>` | Wait for an explicitly requested platform analysis without re-uploading. |
| `viceme skill publish --resume <id> --price-minor <fen>` | Apply the reviewed CNY price to the same Draft without creating another Listing. |
| `viceme publication asset upload ...` | Replace user-selected media, or add `--candidate-only` to stage Agent-provided media for `publication suggest`. |
| `viceme publication update ...` | Replace the complete listing draft from a strict JSON file. |
| `viceme publication confirm ...` | Confirm the exact current review digest. |
| `viceme publication publish ...` | Make a confirmed listing public. |
| `viceme update` | Update the CLI and matching official Skills together. |

Run `viceme <command> --help` for the full flags and JSON fields.

## Output and updates

Business output is JSON by default. Successful output is the only content on
stdout; progress and diagnostics use stderr. Errors use a non-zero exit code
and a stable `error.code`, so Agents and automation never need to parse message
text.

```json
{
  "ok": true,
  "data": {},
  "meta": {
    "executingCliVersion": "<version>",
    "autoUpdate": {
      "from": "<previous-version>",
      "to": "<executing-version>",
      "status": "updated"
    },
    "requestId": "optional"
  }
}
```

`meta.executingCliVersion` is the version of the process that emitted the
response. For `viceme update`, the newly installed version is reported
separately as `data.cli_version` because the response is still emitted by the
process that started the update. `meta.autoUpdate` is present only when this
command was automatically continued by a newly activated generation.

Every released installation passes through a bounded freshness gate before an
ordinary command. A validated result is reused for five minutes so a single
workflow does not repeatedly contact the release channel. When a newer stable
release exists, the CLI and all detected official Skills are activated as one
recoverable generation and the original command is automatically re-executed by
the new CLI. Release discovery is fail-open while offline; activation failure
stops the original command so an older process cannot perform a mutation after
a failed generation change.

An npm installation continues the original command automatically on every
supported platform. A standalone Windows binary may return the retryable code
`AUTO_UPDATE_RESTART_REQUIRED` once while Windows releases the old executable;
rerunning the exact command completes under the new generation.

```bash
viceme update --check
viceme update
```

`viceme update` remains an explicit repair command. The normal startup gate
already verifies the exact release, refreshes the matching official Skills, and
recovers interrupted activation as one compatible local generation.

## Security

- Local packaging rejects path traversal, absolute paths, symlinks, special
  files, oversized content, sensitive files, and common secret patterns.
- The API independently validates the immutable ZIP, digest, and object
  metadata instead of trusting a client-reported result.
- Credentials remain in the CLI's secure local store and are scoped to a
  Profile and API origin; they are never part of Agent conversation state.
- Presigned upload URLs are not written to the local pending-operation store.
- Agent enrichment treats all package content as untrusted data and cannot bypass server-side package, media, ownership, or revision validation.
- Explicit platform-model analysis receives only filtered text, metadata, and image thumbnails.
- Public publication requires the exact reviewed copy, price, cover, and
  ordered gallery to be displayed and explicitly authorized.

## Development

Requirements: Go 1.23+ and Node.js 22+ for npm packaging checks.

```bash
make check
make npm-package-check
make release-manifest
```

The CLI and official Agent Skills are versioned and released together. GitHub,
npm, `s3.viceme.cn`, and `s3.viceme.ai` artifacts come from the same reviewed
commit.
