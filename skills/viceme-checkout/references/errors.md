# Payment error handling

- `PAYMENT_PROJECT_NOT_INITIALIZED`: run `payment init` only after confirming the application slug/name and remote write.
- `PAYMENT_CONTEXT_MISMATCH`: stop. The local file and authenticated remote application differ; do not overwrite or switch profiles automatically.
- `NOT_LOGGED_IN`, `token_expired`, or CLI scope errors: log in again with the same pinned profile. A token created before Payment scopes were added must be reissued.
- `PAYMENT_CREATOR_ELIGIBILITY_REQUIRED`: the user must claim at least one Creator before API Key create/rotate. Product configuration may continue.
- `PAYMENT_API_KEY_NOT_STORED`: issue a key for this environment or repair the secure store. Never ask the user to paste a raw key into chat.
- `PAYMENT_SECRET_STORE_UNAVAILABLE`: stop before retrying issuance. Fix OS keychain/private-store access; do not downgrade to a plaintext project file.
- `PAYMENT_ENV_FILE_TRACKED`: remove the target dotenv file from Git tracking before delivery; adding an ignore rule does not untrack an existing file.
- `PAYMENT_ENV_FILE_INVALID` or `PAYMENT_ENV_FILE_UNSAFE`: select a regular project-relative `.env*` file whose parent exists. Never bypass path, symlink, binary, example/template, or size checks.
- `PAYMENT_ENV_VARIABLE_DUPLICATE`: consolidate the server-only variable to one dotenv assignment before retrying; do not guess which duplicate the application loads.
- `PAYMENT_ENV_VARIABLE_PUBLIC`: choose a server-only variable name; never place a Payment API Key behind a browser-exposed framework prefix.
- `PAYMENT_GITIGNORE_*`: repair the project `.gitignore` as a regular writable file. Do not deliver the key until the exact target can be ignored.
- `CHECKOUT_TEMPLATE_NOT_FOUND`: an explicit `templateCode` is missing, inactive, or not bound to the Product. Remove `templateCode` to use the Product/ViceMe default, or create and bind the intended custom template.
- `CAPABILITY_INSTALLATION_NOT_FOUND` after `payment environment use live`: the Application predates default LIVE installation or is inconsistent. Stop and escalate to an operator; do not edit `.viceme/payment.yaml` or the database manually.
- `PAYMENT_LIVE_API_KEY_ISSUANCE_NOT_ENABLED`: the default LIVE environment exists, but an authorized Admin has not enabled key issuance for this Application. Continue in SANDBOX or request Admin authorization.
- `CHANNEL_ROUTE_UNAVAILABLE` in LIVE: the selected CN WeChat Pay or Alipay Merchant Route, its active version, or its production provider configuration is unavailable. Refresh `payment checkout products`; if the channel is still absent or a previously listed channel now fails, stop and escalate to an operator. Do not substitute SANDBOX or an unlisted channel.
- `CREDENTIAL_ROTATION_NOT_ABORTABLE`: do not restore an old key or invent a recovery path. Inspect remote status and issue a new key through an authorized flow.
- `IDEMPOTENCY_*` conflict: reuse the original body for the original key, or create a genuinely new external order number and idempotency key.
- `PAYMENT_SCOPE_REQUIRED`: issue a least-privilege key containing the required runtime scope; do not reuse the user CLI token.
- Webhook verification/challenge failure: inspect endpoint reachability, HTTPS/DNS, and raw-body signature handling. Never disable signature validation to make the test pass.

Retry only errors marked `retryable: true`, preserving the same idempotency identity for the same checkout operation.
