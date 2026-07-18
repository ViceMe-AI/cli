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

Use another bounded wait when `meta.wait_timed_out` is true.

## User action

- `awaiting_action`: read `next_action`, ask the user for the required selection, then resume the same publication. For `confirm_publish`, present `next_action.payload.preview_url` and the candidate digest for explicit review, then resume with `--decision` (see commands.md).
- `target_conflict`: refresh the Target. Do not use last-write-wins or create another link.
- `payment_required`: explain the requirement and stop.

## Terminal outcomes

- `share_published`: return `data.result.share_url` and `published_noop`.
- `unsupported`: a hard dependency cannot be mapped; stop without publishing a reduced Agent.
- `rejected`: source or policy validation rejected the publication.
- `cancelled`: the publication was cancelled.
- `failed`: the durable workflow failed; report `data.failure` and the publication ID.

## CLI execution errors

Nonzero exits mean the CLI invocation itself did not complete:

- `2`: validation
- `3`: authentication or authorization
- `4`: network
- `5`: internal/protocol
- `6`: policy before publication creation
- `10`: confirmation required

Read the JSON error fields `type`, `subtype`, `message`, `retryable`, and optional `hint`. Never scrape human error text.
