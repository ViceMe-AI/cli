# ViceMe CLI

ViceMe CLI is the deterministic local companion for the official ViceMe Agent
Skills. Codex, Claude Code, and WorkBuddy use the Skills to guide the user; the
CLI handles installation, device authorization, validation, packaging,
uploads, review, and publication.

[中文](./README.zh.md)

## Install

The official bootstrap installs the native CLI and both official Skills from
one immutable release.

China, macOS or Linux:

```bash
curl -fsSL https://s3.viceme.cn/start/install.sh | sh
```

International, macOS or Linux:

```bash
VICEME_REGION=global sh -c "$(curl -fsSL https://s3.viceme.ai/start/install.sh)"
```

China, Windows PowerShell:

```powershell
irm https://s3.viceme.cn/start/install.ps1 | iex
```

International, Windows PowerShell:

```powershell
$env:VICEME_REGION="global"; irm https://s3.viceme.ai/start/install.ps1 | iex
```

If the bootstrap cannot be used, npm is the fallback:

```bash
npx --yes @viceme-ai/cli@latest install
```

The installer always writes the compatibility fallback to
`~/.agents/skills`. It also installs into the native user directory of every
detected supported Agent:

| Agent | Native directory |
| --- | --- |
| Codex | `~/.codex/skills` |
| Claude Code | `~/.claude/skills` |
| WorkBuddy | `~/.workbuddy/skills` |

Select a target explicitly with `viceme install --agent codex`, `claude`,
`workbuddy`, or `agents`. Run `viceme doctor` after installation or repair.

## Authentication and profiles

Each profile is bound to one region, one API endpoint, and one device-authorized
account. Profiles without a custom endpoint use the selected region's official
ViceMe API.

```bash
viceme auth login
viceme auth status
viceme auth logout

viceme profile list
viceme profile use default
```

To remove every legacy/test Profile and all of their local credentials before
creating a clean test Profile:

```bash
viceme profile remove --all --yes
```

This destructive command recreates one unauthenticated `default` Profile; it
does not leave the CLI with an invalid empty configuration.

For a test or private ViceMe deployment, persist the endpoint in a dedicated
profile before signing in:

```bash
viceme profile add \
  --name shop-dev \
  --region cn \
  --api-base-url https://viceme-shop-web.preview.tencent-zeabur.cn/api \
  --use
viceme auth login
```

`profile list` reports the effective endpoint. `VICEME_API_BASE_URL` remains a
one-process CI/debug override and is never written to profile configuration.
Remote custom endpoints require HTTPS; only localhost and loopback development
may use HTTP. Remove and recreate a profile to change its endpoint so an
existing credential cannot silently move to another origin.

For an Agent workflow that cannot wait in one turn:

```bash
viceme auth login --no-wait
viceme auth login --device-code <device-code>
```

The user completes authorization in the browser. Never copy an access token
into the conversation. Credentials are isolated by profile and API origin, so
sign in again after creating a profile for a different endpoint.

## Publish a Skill

The first release accepts a local directory containing `SKILL.md` or a local
ZIP. GitHub URLs, remote downloads, and multi-Skill bundles are not accepted.

Authenticate before inspecting the package, then keep every command pinned to
the Profile returned by `auth status`. An Agent must not switch to another
Profile merely because that Profile is already signed in.

```bash
viceme auth status
viceme --profile <publication-profile> auth login # only when unauthenticated
```

Inspect without side effects:

```bash
viceme --profile <publication-profile> skill inspect --path ./my-skill
```

Show the exact deterministic package and price plan:

```bash
viceme --profile <publication-profile> skill publish --path ./my-skill --price-minor 100 --dry-run
```

Start the resumable upload and listing analysis:

```bash
viceme --profile <publication-profile> skill publish --path ./my-skill --price-minor 100
```

Then follow the authoritative publication state:

```bash
viceme publication get <publication-id>
viceme publication review <publication-id>
viceme publication asset upload <publication-id> --role cover --path ./cover.png
viceme publication asset upload <publication-id> --role gallery --path ./demo.png
viceme publication update <publication-id> --input ./listing-draft.json
viceme publication confirm <publication-id> --review-digest <digest>
viceme publication publish <publication-id> --review-digest <digest>
```

The model proposes `summaryZhCn`, `summaryEnUs`, `usageInstructionsZhCn`,
`usageInstructionsEnUs` derived from the validated `SKILL.md`, and package
images, but it never confirms them or decides the price. Each summary has a maximum display width of
30: ASCII counts as 1 and Chinese/non-ASCII counts as 2. The Agent must show the
exact bilingual summaries, both usage instructions, price, cover, and ordered gallery
to the user and receive explicit confirmation before `confirm` and again before
public `publish`.

Resume after a connection loss with the same publication:

```bash
viceme --profile <publication-profile> skill publish --resume <publication-id>
```

Do not create a second publication when the server response is unknown. Query
or resume the existing ID first.

## Output contract

Business output is JSON by default. Successful output is the only content on
stdout; progress and diagnostics use stderr.

```json
{
  "ok": true,
  "data": {},
  "meta": {
    "cliVersion": "0.10.1",
    "requestId": "optional"
  }
}
```

Errors use a non-zero exit code and a stable `error.code`. Agent Skills branch
on the exit code, `ok`, `error.code`, and `retryable`, never on message text.

## Update

```bash
viceme update --check
viceme update
```

Bootstrap installations read the selected region's official S3 release index,
verify the exact binary checksum, refresh the matching official Skills, and
activate the binary atomically. npm installations update through the exact npm
package version. Updates never inherit `VICEME_ACCESS_TOKEN` into child
processes.

Binary or npm-launcher activation, both official Skills, and profile config are
one recoverable local generation. Standalone and npm activation share an outer
activation lock, a delegated member-commit lock, and a durable active-generation
record containing the semantic version, installation method, and immutable
identity. One startup coordinator inspects
both standalone and npm journals regardless of which launcher entered the
process. Every ordinary command reconciles an interrupted outer journal before
business logic; a recovered process whose version, method, or immutable identity
changed must be restarted. The lock-internal generation fence rejects a late
older updater. Every mutation entry repeats the same two-journal arbitration
after it owns the activation lock and before any staging or network install, so
a process paused after startup cannot introduce a second recovery protocol. The
first phase also rejects switching between standalone and npm
before any mutation; reinstall explicitly after removing the previous generation
instead of mixing recovery protocols. Every Skills/config transaction holds or
revalidates the same generation authority immediately before commit. npm child
activation is bound to the exact committing journal by a one-time nonce and
target version; its member lock prevents a parent crash from admitting a newer
generation while the child still commits. A committed target is only cleaned up
after a crash—it is never reapplied or rolled back. Private
journals can therefore only restore the complete previous generation or finish
the complete target generation. `viceme doctor`
validates Skill/version integrity and an unauthenticated API readiness probe
before installation commits.

## First-phase implementation status

The installation, device authorization, deterministic package upload, manual or
suggested listing media, review confirmation, publication, cancellation, and
terminal recovery paths are implemented. Local acceptance uses real Shop API,
PostgreSQL, Redis, and S3-compatible storage. `make check`, npm package/cold-start
tests, race tests, and Darwin/Linux/Windows amd64/arm64 builds pass. A real LLM
provider sandbox remains an environment acceptance item; without credentials,
analysis fails closed and the manual media path remains available.

## Security boundaries

- Local packaging rejects path traversal, absolute paths, symlinks, special
  files, oversized content, sensitive files, and common secret patterns.
- The API independently validates the immutable ZIP and object metadata.
- Presigned upload URLs are never written to the pending-operation store.
- LLM analysis receives only filtered text, metadata, and image thumbnails.
- A publication cannot become public until the current review digest, price,
  cover, and ordered gallery have been confirmed.

## Development

Requirements: Go 1.23+ and Node.js 22+ for npm packaging checks.

```bash
make check
make npm-package-check
make release-manifest
```

The CLI and `viceme-shared` / `viceme-publish` Skills are versioned and released
together. Release artifacts are published to GitHub, npm, `s3.viceme.cn`, and
`s3.viceme.ai` from the same reviewed commit.
