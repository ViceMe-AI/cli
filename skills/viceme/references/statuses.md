# Publication statuses and errors

`job get` and `job wait` return exit code 0 whenever the publication was read successfully. Inspect `data.status`; do not treat a business outcome as a CLI transport failure.

## Continue polling

- `received`
- `resolving`
- `compiling`
- `materializing`
- `release_authorized`
- `release_committed`
- `waiting_projection`

Use another bounded wait when `meta.wait_timed_out` is true — resume by ID with `job edit-get <pub> <edit-id> --timeout <d>` or `job run-get <pub> <run-id> --timeout <d>`, never by re-submitting the payload.

## User action

- `meta_review`: the parsed basic info is ready for review — show `job metadata` output (title/description/author/missing), resolve with `job metadata --decision confirm|cancel`; user-authored fields go through `--edits-stdin` as one JSON object, never a quoted shell argument (see commands.md). Cancel here leaves zero assets.
- `awaiting_action`: read `next_action`, ask the user for the required selection, then resume the same publication. For `confirm_publish`, show the frozen summary via `job preview`, complete a succeeded `job run` and `job accept` on the exact candidate first (accept requires `--inputs-digest` from the run receipt), then resume with `--decision` plus `--expected-public-summary-digest` taken from the preview output's `public_summary_digest` (see commands.md).
- `binding_required`: terminal for this publication. Run `job bind`, give the user the signed browser URL, and wait for channel verification. Once bound, start a fresh normal inspect/publish request; never resume the old publication. Downloading the Skill or forking its GitHub repository are suggestions, not CLI actions.
- `target_conflict`: refresh the Target. Do not use last-write-wins or create another link.
- `selection_required`: ask the user to choose one returned selector, then resume the same publication with the exact action ID and payload digest.
- `process_credential_active`: login/logout is unavailable in a trusted-launcher process; keep using standard commands or start a normal process for persistent login management.
- `process_credential_invalid`: stop without retrying or printing the injected value; the trusted launcher must replace it.
- `payment_required`: explain the requirement and stop.

## Terminal outcomes

- `share_published`: return `data.result.share_url` and `published_noop`.
- `unsupported`: a hard dependency cannot be mapped; stop without publishing a reduced Agent.
- `rejected`: source or policy validation rejected the publication.
- `cancelled`: the publication was cancelled.
- `failed`: report `data.failure` and the publication ID. Only when `data.failure.details.type` is `PLATFORM_FAILURE` and `data.failure.details.retryable` is `true`, the user may explicitly request `viceme job retry <publication-id> --yes`. Retry the same publication at most through the server-controlled limit; never alter or re-upload the source as a workaround.

## CLI execution errors

Nonzero exits mean the CLI invocation itself did not complete. Preserve and branch on the server's domain-specific `error.type`; the exit code is only a coarse handling class:

- `2`: validation
- `3`: authentication or authorization
- `4`: retryable transport or concurrency
- `5`: internal/protocol
- `6`: policy or rollout gate
- `10`: confirmation required

Read the JSON error fields `type`, `subtype`, `message`, `retryable`, and optional `hint`. Never scrape human error text.

Update failures use stable safe subtypes instead of returning raw npm output:

- `update_registry_unavailable`: registry transport, timeout, rate-limit, or server failure; retryable when no fresh cache is available.
- `update_registry_response`: the registry returned an invalid or unsupported release response.
- `update_npm_missing`: npm is not available for an npm-managed installation.
- `update_npm_permission`: the isolated ViceMe cache or npm global prefix is not writable; follow the returned hint and do not use `sudo viceme`.
- `update_npm_failed`: npm failed for another local registry, proxy, or installation reason; follow the returned diagnostic hint.
