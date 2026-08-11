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
curl -fsSL https://s3.viceme.cn/install.sh | sh
```

International, macOS or Linux:

```bash
VICEME_REGION=global sh -c "$(curl -fsSL https://s3.viceme.ai/install.sh)"
```

China, Windows PowerShell:

```powershell
irm https://s3.viceme.cn/install.ps1 | iex
```

International, Windows PowerShell:

```powershell
$env:VICEME_REGION="global"; irm https://s3.viceme.ai/install.ps1 | iex
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

Each profile is bound to one official region and one device-authorized account.

```bash
viceme auth login
viceme auth status
viceme auth logout

viceme profile list
viceme profile use default
```

For an Agent workflow that cannot wait in one turn:

```bash
viceme auth login --no-wait
viceme auth login --device-code <device-code>
```

The user completes authorization in the browser. Never copy an access token
into the conversation. Credentials are isolated by profile and API origin.

## Publish a Skill

The first release accepts a local directory containing `SKILL.md` or a local
ZIP. GitHub URLs, remote downloads, and multi-Skill bundles are not accepted.

Inspect without side effects:

```bash
viceme skill inspect --path ./my-skill
```

Show the exact deterministic package and price plan:

```bash
viceme skill publish --path ./my-skill --price-minor 100 --dry-run
```

Start the resumable upload and listing analysis:

```bash
viceme skill publish --path ./my-skill --price-minor 100
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

The model may suggest package images, but it never decides the price, cover, or
gallery. The Agent must show the exact draft to the user and receive explicit
confirmation before `confirm` and again before the public `publish` action.

Resume after a connection loss with the same publication:

```bash
viceme skill publish --resume <publication-id>
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
