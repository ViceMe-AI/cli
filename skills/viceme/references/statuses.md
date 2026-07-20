# Publication statuses and errors

`job get` and `job wait` return exit code 0 whenever the publication was read successfully. Inspect `data.status`; do not treat a business outcome as a CLI transport failure.

## Continue polling

- `received`
- `resolving`
- `compiling`
- `materializing`
- `release_committed`
- `waiting_projection`

Use another bounded wait when `meta.wait_timed_out` is true.

## User action

- `awaiting_action`: read `next_action`, ask the user for the required selection, then resume the same publication.
- `target_conflict`: refresh the Target. Do not use last-write-wins or create another link.
- `delegated_grant_not_found`: save the credential through protected stdin or choose an existing keychain reference.
- `delegated_grant_invalid`: discard the local value and obtain a new one-time grant through the staff control plane; never print the rejected value.
- `delegated_grant_ref_exists`: do not overwrite an entry that may contain recovery state; delete it explicitly only after deciding the old grant is no longer recoverable.
- `delegated_grant_request_mismatch`: the ref is already bound to another exact request. Retry the original command or explicitly abandon/delete the ref; never replay it against the new request.
- `delegated_grant_cleanup_conflict`: the server receipt succeeded but local recovery state changed. Keep the reported ref and inspect/delete it explicitly; do not resend the grant.
- `selection_required`: no publication or grant reservation exists yet. Ask the user for one returned selector and rerun the same delegated-ref command with `--skill-root`.
- `client_request_id_required`, `resolution_id_required`: protected-stdin delegated publication must provide both stable values so an ambiguous retry has the same request digest.
- `delegated_grant_tty_unsupported`: use a protected pipe or keychain ref; never paste the grant into an echoing terminal.
- `delegated_upload_unsupported`: delegated publication accepts immutable provider resolutions, not local file or directory uploads.
- `delegated_grant_expired`, `delegated_grant_replayed`: stop and request a new grant. Do not retry the credential against another source or creator.
- `delegated_grant_scope_mismatch`: stop and request a grant scoped to both the immutable source and the Target operation: `UPSERT` for default `auto`, `CREATE` for `--new-target`, or `UPDATE` for `--target-id`. Never reuse the rejected credential against another destination.
- `payment_required`: explain the requirement and stop.

## Terminal outcomes

- `share_published`: return `data.result.share_url` and `published_noop`.
- `unsupported`: a hard dependency cannot be mapped; stop without publishing a reduced Agent.
- `rejected`: source or policy validation rejected the publication.
- `cancelled`: the publication was cancelled.
- `failed`: the durable workflow failed; report `data.failure` and the publication ID.

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
