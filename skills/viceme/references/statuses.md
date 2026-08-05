# CLI statuses and errors

CLI errors use a stable JSON envelope on stderr:

```json
{
  "ok": false,
  "error": {
    "type": "validation",
    "subtype": "app_origin_not_allowed",
    "message": "Request origin is not registered for this App environment",
    "retryable": false,
    "details": { "request_id": "..." }
  }
}
```

Branch on `error.subtype`; never scrape `message`.

## Authentication

- `authorization_pending`: browser authorization is not complete. Wait at least the returned `interval_seconds` before polling again.
- `device_poll_too_fast`: polling exceeded the server interval. Increase the next interval; do not create another device request.
- `authorization_denied`: the user rejected the request. Stop.
- `device_code_expired`: start a new `auth login` flow.
- `device_authorization_context_missing`: the secure pending context is unavailable; start a new split login flow.
- `device_authorization_context_mismatch`: use the exact `continue_args` returned by the first turn. Never retry against another Profile or endpoint.
- `cli_access_token_invalid`: the session is expired, revoked, or invalid. The CLI refreshes locally expired access tokens automatically; if this remains, sign in again.
- `cli_refresh_token_invalid` / `cli_refresh_token_reused`: the bounded recovery window is unavailable or expired; sign in again. Never intentionally replay an old refresh token.
- `cli_scope_required`: the current session lacks the command's scope. Sign in again only if the server policy changed; do not bypass the guard.

## Creator App

- `creator_app_not_found`: the App does not exist or is not owned by the authenticated user. Do not distinguish those cases or try another owner's ID.
- `idempotency_key_reused`: a pending App creation request ID was reused with different input. Preserve the original intent and inspect the remote result before removing local pending metadata.
- `app_binding_conflict`: local `--app` and `.viceme/app.json` disagree. Ask the user which project/App is intended; do not overwrite silently.
- `app_origin_not_allowed`: register the exact canonical Origin with `app link --origin ...`; never relax CORS or place a token in the browser.
- `app_context_not_found`: the publishable key is unknown or its App/environment is unavailable. Run `app link` and `app doctor`.
- `app_doctor_unhealthy`: inspect `error.details.checks` and repair the authoritative source of each failed check.

## Capability

- `capability_not_available`: the capability is planned but not implemented in this CLI/API Slice. Stop; do not emulate it client-side.
- `app_capability_not_found`: add the requested available capability first.
- `capability_doctor_unhealthy`: compare local contract/SDK versions with the server result and rerun `capability add` only when the operation is idempotent.

## Exit codes

- `2`: validation or missing resource
- `3`: authentication or authorization
- `4`: retryable network, rate limit, or upstream availability
- `5`: internal or protocol failure
- `6`: policy/doctor failure
- `10`: explicit confirmation required

Successful install, auth, profile, App, and Capability commands return their business object directly on stdout. `skills read` returns raw bytes. `_notice.update`, when present, is advisory and never authorizes an automatic update.
