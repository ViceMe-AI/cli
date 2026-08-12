# Payment error handling

- `PAYMENT_PROJECT_NOT_INITIALIZED`: run `payment init` only after confirming the application slug/name and remote write.
- `PAYMENT_CONTEXT_MISMATCH`: stop. The local file and authenticated remote application differ; do not overwrite or switch profiles automatically.
- `NOT_LOGGED_IN`, `token_expired`, or CLI scope errors: log in again with the same pinned profile. A token created before Payment scopes were added must be reissued.
- `PAYMENT_CREATOR_ELIGIBILITY_REQUIRED`: the user must claim at least one Creator before API Key create/rotate. Product configuration may continue.
- `PAYMENT_API_KEY_NOT_STORED`: issue a key for this environment or repair the secure store. Never ask the user to paste a raw key into chat.
- `PAYMENT_SECRET_STORE_UNAVAILABLE`: stop before retrying issuance. Fix OS keychain/private-store access; do not downgrade to a plaintext project file.
- `PAYMENT_LIVE_ACCESS_REQUIRED`: LIVE is unsupported in this Skill. Continue in SANDBOX only.
- `CREDENTIAL_ROTATION_NOT_ABORTABLE`: do not restore an old key or invent a recovery path. Inspect remote status and issue a new key through an authorized flow.
- `IDEMPOTENCY_*` conflict: reuse the original body for the original key, or create a genuinely new external order number and idempotency key.
- `PAYMENT_SCOPE_REQUIRED`: issue a least-privilege key containing the required runtime scope; do not reuse the user CLI token.
- Webhook verification/challenge failure: inspect endpoint reachability, HTTPS/DNS, and raw-body signature handling. Never disable signature validation to make the test pass.

Retry only errors marked `retryable: true`, preserving the same idempotency identity for the same checkout operation.
