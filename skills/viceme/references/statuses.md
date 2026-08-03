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

For publication `job wait`, `meta.wait_timed_out=true` means only that the bounded observation window ended. It is not a failure and must not be described as a stuck Compiler. While the receipt remains nonterminal, use up to five consecutive 60-second waits (five minutes total), then report that processing continues in the background and preserve the publication ID for a later bounded wait. If the receipt includes `progress.kind=in_progress`, its `phase` is the authoritative server phase and `poll_after_seconds` is the minimum suggested polling interval.

For edit sub-jobs, resume by ID with `job edit-get <pub> <edit-id> --timeout <d>`, never by re-submitting the payload. Ordinary preview usage happens on the private Candidate preview link and has no CLI PreviewRun sub-job.

If `skill inspect` returns
`data.destination.recovery.mode=resume_existing_publication`, do not publish
the new inspection resolution. Read
`data.destination.recovery.publication_id` with `job get` and resume that
durable receipt. The recovery pointer intentionally carries no action digest;
only `job get` and `job renew` are authoritative for the current action
generation.

## User action

- `meta_review`: the parsed basic info is ready for review — show `job metadata` output (title/description/author/missing), resolve with `job metadata --decision confirm|cancel`; user-authored fields go through `--edits-stdin` as one JSON object, never a quoted shell argument (see commands.md). Cancel here leaves zero assets.
- `awaiting_action`: read `next_action`, ask the user for the required selection, then resume the same publication. For `confirm_steps`, show `payload.steps` and resolve first (confirm/cancel) — no private Candidate preview exists before this gate passes. For `confirm_publish`, read the frozen summary and classification schema with `job preview`. For each requested pre-run classification, run `job preview --classification key=value` and give the returned receipt-bearing private URL to the user; a changed selection creates a new preview-session generation. Only successful runs with a real result enter the sparse mounted-result manifest, and the publisher need not exhaust every combination. If the confirmation action expires, renew the same Publication/action generation; never create another Publication. If the user edits the Candidate, pass the exact natural-language request through `job edit --candidate-digest <digest> --request-stdin`, then fetch and confirm the fresh Candidate again. Before final confirm, re-run `job preview` and pass both `--expected-public-summary-digest` and the latest `--expected-result-manifest-digest`; 409 `preview_share_run_required` means no eligible result exists, while 409 `result_manifest_stale` means the result set changed and must be reviewed again. Preview sessions/history and unreferenced files are temporary; accepted result bundles and referenced internal artifacts are frozen under the released main Skill. After publication, return the distinct official `data.result.share_url`; the old preview URL redirects to it.
- `binding_required`: terminal for this publication. Run `job bind`, give the user the signed browser URL, and wait for channel verification. Once bound, start a fresh normal inspect/publish request; never resume the old publication. Downloading the Skill or forking its GitHub repository are suggestions, not CLI actions.
- `target_conflict`: refresh the source with `skill inspect`. If the returned destination contains `recovery.mode=resume_existing_publication`, use its `publication_id` with `job get` and continue that Publication; otherwise report the conflict and stop. Do not use last-write-wins, create another Publication, or create another link.
- `selection_required`: ask the user to choose one returned selector, then resume the same publication with the exact action ID and payload digest.
- `process_credential_active`: login/logout is unavailable while a process credential is active; keep using standard commands or start a process without `VICEME_ACCESS_TOKEN` for persistent login management.
- `process_credential_invalid`: stop without retrying or printing the injected value; replace the process credential.
- `local_profile_credential_active`: login/logout is unavailable while the selected Profile has an explicit publication credential; keep using standard commands or explicitly clear the Profile override.
- `profile_credential_invalid` / `profile_credential_origin_mismatch`: stop without printing the stored value; an operator must replace or clear the Profile credential and its bound endpoint.
- `payment_required`: explain the requirement and stop.

## Terminal outcomes

- `share_published`: return `data.result.share_url` and `published_noop`.
- `unsupported`: a hard dependency cannot be mapped under the current compilation contract; stop this publication without publishing a reduced Agent.
- `rejected`: source or policy validation rejected this publication under the current compilation contract.
- `cancelled`: the publication was cancelled.
- `failed`: report `data.failure` and the publication ID. Only when `data.failure.details.type` is `PLATFORM_FAILURE` and `data.failure.details.retryable` is `true`, the user may explicitly request `viceme job retry <publication-id> --yes`. Retry the same publication at most through the server-controlled limit; never alter or re-upload the source as a workaround.

`unsupported` and `rejected` are terminal only for their publication ID, not a
permanent ban on the frozen source. Never retry or resume the terminal
publication. When the user later makes a new explicit publish request, use a
fresh client request ID and the ordinary inspect/publish flow with the same
source unchanged. Do not manufacture a source, Target, or version change. The
server's current full compile identity decides whether to reuse a prior result
or compile again.

## CLI execution errors

Nonzero exits mean the CLI invocation itself did not complete. Preserve and branch on the server's domain-specific `error.type`; the exit code is only a coarse handling class:

- `2`: validation
- `3`: authentication or authorization
- `4`: retryable transport or concurrency
- `5`: internal/protocol
- `6`: policy or rollout gate
- `10`: confirmation required

Read the JSON error fields `type`, `subtype`, `message`, `retryable`, and optional `hint`. Never scrape human error text.

Structured success and error objects may also contain `_notice.update`. This is a non-blocking machine-readable advisory, not an execution error. Tell the user its `current`, `latest`, and exact `command`; never silently discard it or auto-update without approval.

Update failures use stable safe subtypes instead of returning raw npm output:

- `update_registry_unavailable`: registry transport, timeout, rate-limit, or server failure; retryable when no fresh cache is available.
- `update_registry_response`: the registry returned an invalid or unsupported release response.
- `update_npm_missing`: npm is not available for an npm-managed installation.
- `update_npm_permission`: the isolated ViceMe cache or npm global prefix is not writable; follow the returned hint and do not use `sudo viceme`.
- `update_npm_failed`: npm failed for another local registry, proxy, or installation reason; follow the returned diagnostic hint.
