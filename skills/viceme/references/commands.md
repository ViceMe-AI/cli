# ViceMe CLI commands

## Install, diagnose, and update

Cold-start the CLI and bundled ViceMe Skill from an AI coding tool or terminal:

```bash
npx --yes --registry=https://registry.npmjs.org --@viceme-ai:registry=https://registry.npmjs.org --package=@viceme-ai/cli@latest -- viceme install
viceme install --target codex
viceme install --target codex --region global
viceme skills doctor --target codex
```

Installation defaults to `cn` and initializes the `default` profile. Pass `--region global` only for the international ViceMe service. The CLI persists that choice per profile; later commands do not take a region or API URL flag. Local/bootstrap commands return formatted bare business JSON, `skills read` returns raw file content, and only publication protocol commands under `skill` and `job` use the stable JSON envelope.

Manage profiles only when the user explicitly asks:

```bash
viceme profile list
viceme profile add --name work --region global --use
viceme profile use default
viceme --profile work auth status
viceme profile rename work company
viceme profile remove company
```

`profile use` changes the persistent active profile. Global `--profile` selects a profile for one command without changing the persistent selection. Never switch, rename, or remove a profile based only on inferred intent.

Only when the user explicitly requests controlled local/internal testing may an Agent configure an endpoint and its matching audience-bound local Profile credential:

```bash
viceme profile add --name local --region cn --api-base-url http://localhost:8090 --access-token '<vpa1.local-dev.credential>' --use
viceme profile configure local --access-token 'YOUR_ACCESS_TOKEN'
viceme profile configure local --clear-access-token
viceme profile configure local --clear-api-base-url
```

Never infer, print, or copy a token from Skill content. The `YOUR_ACCESS_TOKEN` value above must come from an explicitly authorized user-provided or staff reissue flow; `--access-token` is visible in argv and may enter shell history. Credential priority is process `VICEME_ACCESS_TOKEN` → selected local Profile → device login. Non-local audiences are pinned to their canonical API origins, including `dev-preview` at `https://viceme-envoy-dev.preview.tencent-zeabur.cn`; `local-dev` Profile credentials require an explicit loopback endpoint, while process `local-dev` additionally requires `VICEME_CLI_ALLOW_LOCAL_PROCESS_CREDENTIAL=1`. Normal login never writes a Profile token. Profile list/status expose only whether it is configured, and all API and presigned-upload redirects fail closed.

Check first when desired, then update the npm launcher, verified Go binary, and matching Skill together:

```bash
viceme update --check
viceme update --target codex
```

The update path queries the canonical registry directly, caches only a successful version result for a bounded 24-hour registry-outage fallback, and uses the `npm-cache` subdirectory of the effective ViceMe configuration directory for npm subprocesses. Normal npm-managed CLI invocations read that cache without network blocking and refresh it in the background at most once per 24 hours. When a newer release is known, structured success and error objects include `_notice.update={current,latest,message,command}`. Surface that notice to the user, but do not treat it as failure or update without approval. `VICEME_NO_UPDATE_NOTIFIER=1` suppresses the notice; standard CI environments are skipped automatically. The update uses an exact npm package version, does not execute provider installation text, and does not replace a standalone binary through an unsigned self-update path.

## Authenticate

```bash
viceme auth status
viceme auth login
viceme auth login --no-wait --json
viceme auth login --device-code <device-code> --json
viceme auth logout
```

Use plain `viceme auth login` for a person at a terminal: it prints the browser URL and waits for completion. AI Agents must use `--no-wait --json`, ask the user to open `verification_url`, and stop the current turn; when the server provides `verification_url_complete`, the CLI makes that prefilled direct browser link the canonical `verification_url`. Continue with the returned device code and `--json` in a later turn using the same profile. On macOS, device-login tokens stay in AES-256-GCM encrypted files; all platforms isolate them by profile plus normalized API origin. Normal login never backfills explicit local profile overrides.

When a publication ends in `binding_required`, read its signed browser action with:

```bash
viceme job bind <publication-id>
```

Give `binding_url` to the user and stop. After the browser flow succeeds, repeat inspect and publish with a fresh client request ID. The blocked publication is intentionally terminal and is not resumed.

An `unsupported` or `rejected` publication is also immutable: do not retry or
resume that publication ID. This does not permanently ban the same frozen
source. If the user later explicitly asks to publish again, repeat the ordinary
inspect/publish flow with the same source unchanged and a fresh client request
ID. Keep the server-resolved Target; never require an artificial source commit,
upload replacement, Target change, or version bump. The server's current full
compile identity determines whether it reuses a result or runs the Compiler
again.

Login preflights credential persistence before creating or consuming a device authorization. On macOS, an explicit login automatically creates a private local encryption key when the current sandbox cannot access Keychain. Do not tell the user to run a separate setup command before logging in.

If the user wants to reuse an existing Keychain-protected login inside Codex or Claude Code without logging in again, they can migrate that existing credential once from an interactive macOS Terminal:

```bash
viceme config keychain-downgrade
```

The command copies only encryption key material to a private `0600` local file and imports configured legacy credentials into encrypted files. It never prints a token. It is optional for explicit re-login and exists only to preserve an existing login without reauthorization.

## Authentication and server-resolved ownership

An authenticated user publishes with the standard commands. No owner, creator, or authorization selector is accepted from the command line:

```bash
viceme skill publish --resolution-id <resolution-id> --yes
```

For a staff-authorized operation, an audience-bound credential may be injected for one process (`source=process`, `persistent=false`) or explicitly configured in a controlled local Profile (`source=local_profile`, `persistent=true`). Standard inspect/publish/job commands remain unchanged and use the normal `x-api-key` transport. Login and logout fail closed while either override is active. The credential is never printed or inherited by update subprocesses.

## GitHub or trusted provider

```bash
viceme skill inspect https://github.com/acme/poster-skill --skill-root .
viceme skill publish --resolution-id <resolution-id> --yes
viceme job wait <publication-id> --timeout 60s
```

`--skill-root` is the exact repository-relative directory containing `SKILL.md`; use `.` only when `SKILL.md` is at the repository root. The calling Agent must determine it from the user-provided path or read-only repository tree before invoking ViceMe. ViceMe does not discover or rank GitHub Skill roots.

The internal Core pilot can also exercise direct admission, but this is not a substitute for the T2 exact-candidate preview/confirmation required before public rollout:

```bash
viceme skill publish https://github.com/acme/poster-skill --skill-root . --yes
```

## Xiaohongshu or RedSkill source

The Host LLM extracts the exact RedSkill identifier from the user's request or
copied command, then passes a typed SourceSpec on stdin:

```bash
viceme skill inspect --source-stdin
viceme skill publish --resolution-id <resolution-id> --yes
```

stdin:

```json
{"kind":"redskill","value":"ai-desk-card"}
```

Do not pass the original natural-language command to CLI/Core, execute
`install.md`, invoke a marketplace command, or substitute another provider.

## ZIP and folder

Create a logical Agent only when the user intends a new one:

```bash
viceme skill publish --file ./poster-skill.zip --new-target --target-alias poster --yes
viceme skill publish --dir ./poster-skill --new-target --target-alias poster --yes
```

Update the same logical Agent and URL with optimistic concurrency:

```bash
viceme skill target get target_123
viceme skill publish --file ./poster-skill-v2.zip --target-id target_123 --expected-target-version 4 --yes
```

Never omit Target selection for uploaded input and never convert a conflict into `--new-target`.

## Multiple roots

After asking the user to select a candidate, send only the structured payload through stdin:

```bash
viceme job resume pub_123 --action-id act_123 --expected-payload-digest sha256:abc --payload-stdin
```

Example stdin: `{"selector":"skills/poster"}`.

## Metadata confirmation (T2)

After a successful compile the publication parks at `meta_review` with a
`confirm_metadata` action — before any target/agent/build asset exists. Read
the parsed basic info, ask the user to confirm, supplement, or cancel. Preserve
`next_action.action_id` and `next_action.payload_digest` from the latest
`job wait` / `job get`; `job metadata` returns the fields under review but does
not replace that action receipt:

```bash
viceme job metadata pub_123
viceme job metadata pub_123 --action-id meta_1 \
  --expected-payload-digest sha256:abc --decision confirm --edits-stdin
```

User-authored fields travel as one JSON object on stdin — never interpolate
the user's text into a quoted shell command line (quotes, backticks, `$()`
and newlines escape the argument boundary). Example stdin:
`{"title":"探针海报","description":"为产品海报写一句主标题","author":"acme/ops"}`.

`missing` lists absent fields (title/description/author) — guide the user to
fill them (same JSON keys); `author` also covers source-author edits (1-100
visible characters). Cancel maps to `cancelled` with zero assets and no preview
link; report it and stop. A confirm
returns a `meta_confirmed` action receipt, not the next Candidate. Continue with
another bounded wait:

```bash
viceme job wait pub_123 --timeout 60s
```

## Interaction steps confirmation, private Candidate preview, edit, and confirmation (T2)

When `next_action.type` is `confirm_steps`, the exact release candidate is
ready but **no private Candidate preview exists yet**. Show the interaction steps from the
action `payload.steps` (title/description/author/input method/usage/output
description), then resolve inside the conversation — confirm, edit
(natural language, below), or decline:

```bash
viceme job resume pub_123 --action-id act_steps \
  --expected-payload-digest sha256:abc \
  --expected-release-candidate-digest sha256:def \
  --expected-public-summary-digest sha256:sum \
  --decision confirm
```

Read the three binding digests from these exact `job get` JSON paths:
`next_action.payload_digest`,
`next_action.payload.expected_release_candidate_digest`, and
`next_action.payload.expected_public_summary_digest`. In particular,
`payload_digest` is on the action itself, not inside its `payload`. Do **not**
call `job preview` at this stage: the preview only exists after the steps gate
passes, so the digest can never come from it here. `--decision cancel` maps to `cancelled` with zero
preview link. After a confirmed steps gate the publication issues
`confirm_publish` (with `payload.preview_share_url`); an applied edit
supersedes the steps action, pending share projection, and temporary preview
artifacts, and the fresh candidate must be confirmed again.

When `next_action.type` is `confirm_publish`, the exact release candidate is
ready. Read the owner-only private browser link from
`next_action.payload.preview_share_url` in the latest `job wait` / `job get`.
It resolves `/p/{code}` and is separate from the official `/v/{code}` link
returned after release. Show the frozen public summary from `data.preview`;
that response carries the candidate, payload, and `public_summary_digest`
receipts that confirmation binds:

```bash
viceme job preview pub_123 [--action-id act_123]
```

If the current `confirm_steps` or `confirm_publish` receipt expires, keep the
same publication and frozen Candidate. First read the durable publication and
verify that the expired action ID is still the current action, then explicitly
renew that exact receipt:

```bash
viceme job get pub_123
viceme job renew pub_123 --action-id act_expired
```

The success envelope contains `data.publication_id`, `data.status`, and the new
`data.next_action`. Continue only with the new action ID, digests, and
`preview_share_url`; never replay the expired receipt. Do not inspect/publish
again or create another publication to recover an expired confirmation action.
`job renew` is not a generic retry: it must never be used for terminal
`failed`, `unsupported`, `rejected`, `cancelled`, or `binding_required`
publications, and it does not replace `job retry` for an explicitly retryable
compiler platform failure. The server rejects any non-expired, superseded, or
non-confirmation action.

Edits happen only as natural language inside the conversation — never via a
page editor or JSON Patch. Bind the digest shown by the preview and start the
CLI with an explicit stdin input mode:

```bash
viceme job edit pub_123 --candidate-digest sha256:def \
  --request-stdin [--timeout 2m]
```

Send the user's exact request through the Host's subprocess stdin channel.
Never interpolate it into a command string, argv, an environment variable, or
a shell pipeline. The CLI preserves the complete input, including newlines and
shell metacharacters.

When a bounded edit wait times out, the command still prints the created
`edit_id` with `meta.wait_timed_out=true` — resume with that same ID instead
of starting a second logical operation:

```bash
viceme job edit-get pub_123 edit_1 [--timeout 2m]
```

The same-ID read works after a process restart; `--timeout` resumes the bounded
wait and keeps returning `wait_timed_out` honestly.

An applied edit supersedes the old preview/action receipt and temporary share
preview data — re-run `job get` / `job preview` for the fresh candidate before
continuing. Identical retries are deduplicated server-side; 409
`candidate_changed` means the digest is stale.

Give `preview_share_url` to the user and ask them to use the Agent normally on
that page. The ordinary share page has no special test, accept-result, or
publish controls. The creator must complete at least one successful ordinary
run of the exact pending Candidate, then return to the Agent Host and explicitly
confirm or cancel. The CLI intentionally has no `job run`, `job run-get`, or
`job accept` command.

Confirmation requires a successful ordinary share run bound to that same
pending Candidate; otherwise `job resume --decision confirm` is rejected with
409 `preview_share_run_required`. `--expected-public-summary-digest` is
required too — take `public_summary_digest` from the `job preview` output,
binding the decision to the exact summary receipt the user saw:

```bash
viceme job resume pub_123 --action-id act_123 \
  --expected-payload-digest sha256:abc \
  --expected-release-candidate-digest sha256:def \
  --expected-public-summary-digest sha256:sum \
  --decision confirm
viceme job wait pub_123 --timeout 60s
```

`job resume --decision confirm` returns a `release_authorized` action receipt;
it does not return the final share link. Wait until `data.status` becomes
`share_published`, then return `data.result.share_url`,
`data.result.published_noop`, and warnings. Do not require the official
`share_url` to equal the private `preview_share_url`; after publication the old
preview URL redirects to the official link. If the bounded wait reports
`meta.wait_timed_out=true`, use `job get` or another bounded wait in a later
turn rather than looping indefinitely.

Use `--decision cancel` when the user declines; it maps to `cancelled`
everywhere, so report cancellation and stop without expecting a share link.
Never infer the decision from earlier conversation, never cache it across
candidates: if the preview or candidate digest changes, ask the user again
with the fresh action. A stale or expired action fails closed — fetch `job get`
and present the new `next_action` instead of retrying the old one.

All data produced while using the pending Candidate on the private preview link is
temporary. Preview inputs, outputs, files, media, sessions, Runner events, and
workspace history are purged after confirm, cancel, expiry, supersede, or
Release commit. They do not count as public views, usage, works, or history.
Never promise that a preview result can be recovered after publication; run
again on the published Agent when the user needs a durable result.

## Bounded jobs, explicit compiler retry, and cancellation

```bash
viceme job get pub_123
viceme job wait pub_123 --timeout 60s
viceme job renew pub_123 --action-id act_expired
viceme job retry pub_123 --yes
viceme job cancel pub_123 --yes
```

`job renew` and `job retry` are separate recovery contracts. Renewal only
reissues an expired confirmation action on the same frozen Candidate. `job
retry` is valid only when the durable compiler failure is a retryable
`PLATFORM_FAILURE`; it reuses the frozen source and same publication, has a
server-enforced attempt limit, and always requires explicit user confirmation.
Cancellation also requires explicit confirmation.
