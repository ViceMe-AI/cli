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
- **Safe by default** — device authorization, origin-bound profiles, local
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

   > Publish this Skill to ViceMe for CNY 1.00.

The Agent checks login before reading the package, keeps the selected Profile
fixed throughout the workflow, validates the Skill, and asks before uploading.
After ViceMe prepares the bilingual copy and media suggestions, the Agent shows
the complete review—including the images—and asks once whether to confirm and
publish it publicly.

```text
Local Skill → Login → Validate → Confirm price → Upload → Platform analysis
            → Visual review → Confirm and publish → Public product URL
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

# Inspect without uploading anything.
viceme skill inspect --path ./my-skill

# Create or recover the stable private owner preview before pricing
viceme skill listing prepare --path ./my-skill

# Preview the exact package and CNY 1.00 price plan.
viceme skill publish --path ./my-skill --price-minor 100 --dry-run
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
irm https://s3.viceme.cn/start/install.ps1 | iex
```

International:

```powershell
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
| Login | Device authorization happens before package inspection. Every command remains pinned to the same Profile and API origin. |
| Inspect | Local validation rejects unsafe paths, special files, excessive content, sensitive files, and common secret patterns. |
| Price and upload | The user supplies an exact CNY price in fen and explicitly allows the deterministic package to be uploaded. |
| Analysis | ViceMe proposes short Chinese and English summaries, bilingual usage instructions, a cover, and an ordered gallery. Suggestions are never treated as user decisions. |
| Review | The Agent displays the exact copy, price, cover, and gallery images. A short summary has a maximum display width of 30; ASCII counts as 1 and Chinese/non-ASCII as 2. |
| Publish | One explicit final confirmation authorizes review confirmation followed by immediate, irreversible public publication. |

If the upload or response is interrupted, continue the existing publication:

```bash
viceme --profile <publication-profile> skill publish --resume <publication-id>
```

Never create a second publication merely because the previous response was
unknown. Query or resume the existing ID first.

## Authentication and Profiles

Each Profile binds one region, one API endpoint, and one device-authorized
account. Profiles without a custom endpoint use the selected region's official
ViceMe API.

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
  --region cn \
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
| `viceme access init --name <name> [--follow key] [--product slug --purchase key]` | Create, configure, activate, and apply a creator website access integration in one command. |
| `viceme profile list` | Show Profiles and their effective API endpoints. |
| `viceme skill inspect --path <path>` | Validate a local Skill without side effects. |
| `viceme skill listing prepare --path <path>` | Create or recover the stable private owner preview and persist the local binding. |
| `viceme skill listing get <listing-id>` | Read the authoritative private Listing state. |
| `viceme skill listing bind <listing-id> --path <path>` | Explicitly bind a source to a selected owned Listing. |
| `viceme skill publish --path <path> --price-minor <fen> --dry-run` | Preview the exact deterministic package and price. |
| `viceme skill publish --path <path> --price-minor <fen>` | Upload the package and start listing analysis. |
| `viceme publication wait <id>` | Wait for background analysis without re-uploading. |
| `viceme publication review <id>` | Read the authoritative bilingual copy, price, selected media, and review state. |
| `viceme publication asset upload ...` | Replace or add a cover or gallery image before confirmation. |
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
    "cliVersion": "<version>",
    "requestId": "optional"
  }
}
```

Released installations check their authoritative release channel at most once
every 24 hours. When a newer version is available, ordinary JSON responses
include `_notice.update` with the current version, latest version, and
`viceme update`. Discovery is fail-open and never changes the business
command's exit code.

```bash
viceme update --check
viceme update
```

The updater verifies the exact release, refreshes the matching official Skills,
and recovers interrupted activation as one compatible local generation.

## Security

- Local packaging rejects path traversal, absolute paths, symlinks, special
  files, oversized content, sensitive files, and common secret patterns.
- The API independently validates the immutable ZIP, digest, and object
  metadata instead of trusting a client-reported result.
- Credentials remain in the CLI's secure local store and are scoped to a
  Profile and API origin; they are never part of Agent conversation state.
- Presigned upload URLs are not written to the local pending-operation store.
- Model analysis receives only filtered text, metadata, and image thumbnails.
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
