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
- `awaiting_action`: read `next_action`, ask the user for the required selection, then resume the same publication. For `confirm_steps`, show `payload.steps` and resolve first (confirm/cancel) — no preview link exists before this gate passes. For `confirm_publish`, show the frozen summary via `job preview`. If the user requests a change, pass their exact request only through subprocess stdin to `job edit --candidate-digest <digest> --request-stdin`; an applied edit invalidates the old steps/action, preview, and run receipts, so fetch the fresh Candidate and confirm its steps again. Complete a succeeded `job run` and `job accept` on that exact candidate first (accept requires `--inputs-digest` from the run receipt), then resume with `--decision` plus `--expected-public-summary-digest` taken from the preview output's `public_summary_digest` (see commands.md).
- `binding_required`: terminal for this publication. Run `job bind`, give the user the signed browser URL, and wait for channel verification. Once bound, start a fresh normal inspect/publish request; never resume the old publication. Downloading the Skill or forking its GitHub repository are suggestions, not CLI actions.
- `target_conflict`: refresh the Target. Do not use last-write-wins or create another link.
- `selection_required`: ask the user to choose one returned selector, then resume the same publication with the exact action ID and payload digest.
- `process_credential_active`: login/logout is unavailable while a process credential is active; keep using standard commands or start a process without `VICEME_ACCESS_TOKEN` for persistent login management.
- `process_credential_invalid`: stop without retrying or printing the injected value; replace the process credential.
- `local_profile_credential_active`: login/logout is unavailable while the selected Profile has an explicit publication credential; keep using standard commands or explicitly clear the Profile override.
- `profile_credential_invalid` / `profile_credential_origin_mismatch`: stop without printing the stored value; an operator must replace or clear the Profile credential and its bound endpoint.
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
