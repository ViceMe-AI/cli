---
name: viceme-danmaku
description: Install or repair the ViceMe-hosted danmaku SDK in a website. Use for Work setup, DNS ownership verification, workKey activation, script-tag integration, responsive behavior, keyboard access, and reduced-motion verification.
---

# Install ViceMe Danmaku

Use one published, verified Website Work and its active `danmaku` SDK access.
The host installs the official loader; it must not copy ViceMe component, iframe,
API-client, or persistence code. Read [cdn-sdk.md](references/cdn-sdk.md) first.

## Workflow

1. Inspect the repository instructions, page entry point, exact deployed HTTPS
   Origin, deployment path, Content Security Policy, and browser tests. An
   Origin has scheme and host only, with no path, query, fragment, credentials,
   or trailing slash.
2. Run `viceme profile list`, keep the active Profile, then run
   `viceme --profile <profile> auth status`. Re-authenticate on that Profile if
   `merchant-commerce:read` or `merchant-commerce:write` is missing.
3. Run `viceme --profile <profile> merchant accounts`. Use the sole active
   Merchant owned by the current login, or ask the user to choose when several
   are available. Never infer it from a public creator identity or old local
   state.
4. Run `viceme --profile <profile> merchant work list --merchant <merchant-id>`.
   Reuse a Website Work only when its `website.canonicalOrigin` exactly equals
   the deployed Origin. Otherwise use this strict request, filling only
   observed content:

   ```json
   {
     "kind": "WEBSITE",
     "merchantAccountId": "<merchant-id>",
     "clientRequestId": "<stable-idempotency-key>",
     "slug": "website-slug",
     "title": "Website title",
     "canonicalOrigin": "https://creator.example",
     "content": {
       "summary": "Observed public purpose",
       "bodyMarkdown": "Observed public description",
       "templateType": "WEBSITE",
       "tags": [],
       "media": [],
       "actionConfig": {}
     }
   }
   ```

   Run `viceme --profile <profile> merchant work create --input <json>`. If the
   response is lost, replay the identical request with the same
   `clientRequestId`; do not create a new identity.
5. If `website.ownershipStatus` is not `VERIFIED`, run:

   ```bash
   viceme --profile <profile> merchant work website-verification create <work-id> \
     --merchant <merchant-id> --expected-revision <work-revision>
   ```

   Publish the returned `challenge` verbatim at `dnsRecordName`. After public
   DNS resolves exactly, run:

   ```bash
   viceme --profile <profile> merchant work website-verification verify <work-id> \
     --merchant <merchant-id> \
     --expected-verification-version <verification-version>
   ```

   Then write this publish input, replacing `2` with the Work revision returned
   by verify:

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "expectedRevision": 2,
     "status": "PUBLISHED"
   }
   ```

   Run `viceme --profile <profile> merchant work update <work-id> --input
   <json>`, then `viceme --profile <profile> merchant work get <work-id>
   --merchant <merchant-id>`. Never reuse or guess a stale revision.
6. Read `merchant work sdk-access get <work-id> --merchant <merchant-id>`.
   Create it with `--feature danmaku` when absent. When it exists, update with
   its current `configVersion` and the full desired feature set, preserving
   `tip` if already enabled. Do not interpret update as an incremental append.
7. Insert one loader tag using the selected Profile's exact `webBaseUrl`, the
   returned public `workKey`, and the Profile's `marketRegion` (`cn` or
   `global`):

   ```html
   <script
     defer
     src="<web-base-url>/viceme-sdk/v1/viceme.min.js"
     data-viceme-work="<work-key>"
     data-viceme-region="<cn-or-global>"
     data-viceme-features="danmaku"
     data-viceme-target="body"
     data-viceme-theme="auto"
   ></script>
   ```

8. Preserve an existing CSP nonce. Add only the exact Profile Web Origin to the
   directives proven necessary by the page; never add `*`, `unsafe-eval`, or a
   broad subdomain wildcard.
9. Run repository checks and verify the real deployed page on desktop and
   mobile. Confirm one SDK root mounts, host controls remain clickable,
   keyboard controls work, reduced motion is honored, and a message survives
   refresh.

## Boundaries

- `workKey` is public and opaque. Never replace it with a Work UUID, Merchant
  ID, Product ID, slug, token, or credential.
- The Website Work owns identity and verified Origin. SDK access only enables
  hosted features; it is not a login, entitlement, or payment policy.
- Tipping is handled by `viceme-tip`. Preserve it when updating shared access.
- Report changed files, public Work ID/key, checks, and unverified runtime
  boundaries. Never report credentials or DNS challenge values.
