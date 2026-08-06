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
- `cli_refresh_token_invalid`: the refresh credential or Session is unavailable, expired, revoked, or for another Audience; sign in again.
- `cli_refresh_token_reused`: an old refresh token was presented with another request identity or outside its bounded recovery window, so the server revoked the token family; sign in again. Never construct or change a refresh request identity manually.
- `cli_scope_required`: the current session lacks the command's scope. Sign in again only if the server policy changed; do not bypass the guard.

## Creator App

- `creator_app_not_found`: the App does not exist or is not owned by the authenticated user. Do not distinguish those cases or try another owner's ID.
- `idempotency_key_reused`: branch on the command being retried. For `app link`, preserve the original pending App intent and inspect the remote result before removing local pending metadata. For `commerce offer create`, reuse the key only with the exact original name, amount, currency, and purpose; if the user intentionally wants a different Offer, generate a new UUID. Never delete or rewrite App pending metadata to recover an Offer conflict.
- `app_binding_conflict`: local `--app` and `.viceme/app.json` disagree. Ask the user which project/App is intended; do not overwrite silently.
- `app_origin_not_allowed`: register the exact canonical Origin with `app link --origin ...`; never relax CORS or place a token in the browser.
- `app_context_not_found`: the publishable key is unknown or its App/environment is unavailable. Run `app link` and `app doctor`.
- `app_doctor_unhealthy`: inspect `error.details.checks` and repair the authoritative source of each failed check.

## Capability

- `capability_not_available`: the capability is planned but not implemented in this CLI/API Slice. Stop; do not emulate it client-side.
- `commerce_live_funds_model_disabled`: LIVE Commerce has not been approved for the current deployment. Continue in TEST or wait for the Merchant of Record decision; do not insert Provider credentials into the project.
- `commerce_live_provider_not_configured`: staff activation cannot proceed until the deployment has a configured WeChat Pay or Alipay adapter. This is an operator boundary, not a creator-side configuration task.
- `commerce_capability_not_enabled`: the App, environment, or Commerce capability is not ACTIVE for the requested public flow. Run `app doctor` and wait for operator activation; do not edit the local manifest to hide it.
- `app_capability_not_found`: add the requested available capability first.
- `capability_doctor_unhealthy`: compare local contract/SDK versions with the server result and rerun `capability add` only when the operation is idempotent.

## Listing and Ledger

- `listing_not_publishable`: repair the server-reported prerequisite. EXTERNAL Listings need a deployed URL; an attached Offer must be an ACTIVE LIVE Offer from the same App.
- `listing_slug_taken`: choose a different stable public slug. Do not overwrite another App's Listing.
- `listing_offer_not_owned`: remove the Offer or select the same App's LIVE Offer; never copy another App's ID.
- `listing_not_found`: the App has no Listing, or the public slug is not currently PUBLIC.
- `listing_input_file` / `listing_input_json`: create one complete Listing JSON document with the file-editing tool, including explicit nullable and array fields; never splice user content into a shell command.
- `creator_ledger_cursor_invalid`: discard only the malformed pagination cursor and restart the read from the first page. Ledger entries themselves are immutable.

## Exit codes

- `2`: validation or missing resource
- `3`: authentication or authorization
- `4`: retryable network, rate limit, or upstream availability
- `5`: internal or protocol failure
- `6`: policy/doctor failure
- `10`: explicit confirmation required

Successful install, auth, profile, App, and Capability commands return their business object directly on stdout. `skills read` returns raw bytes. `_notice.update`, when present, is advisory and never authorizes an automatic update.
