---
name: viceme-engagement
description: Add ViceMe hosted danmaku and tips to one public website through one verified Website Work, one combined SDK access resource, and one Website Widget application.
---

# Integrate ViceMe Danmaku And Tips

Configure one resource graph and install one loader. Never create separate Works
for danmaku and Tip.

## Workflow

1. Inspect the exact production HTTPS Origin, target page, deployment command,
   CSP, responsive layout, and browser tests.
2. Run `viceme profile list` and keep the active Profile. Run `viceme --profile
   <profile> auth status`; require `merchant-commerce:read` and
   `merchant-commerce:write`. Then select an active Merchant from `viceme
   --profile <profile> merchant accounts`.
3. Run `viceme --profile <profile> merchant work list --merchant <merchant-id>`.
   Reuse a Website Work only when its `website.canonicalOrigin` exactly equals
   the deployed Origin. If none exists, use this strict request, filling only
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
4. Whether the Work was reused or created, read it with `viceme --profile
   <profile> merchant work get <work-id> --merchant <merchant-id>`. If
   `website.ownershipStatus` is not `VERIFIED`, reuse a challenge only when the
   current execution still holds the immediate, unexpired `PENDING` response
   from `website-verification create`. The latest verification GET omits the
   plaintext `challenge` and cannot recover its TXT value. Otherwise, including
   after a lost create response or when ownership is `REVOKED`, read the latest
   Work revision and create a replacement challenge:

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

   Read the Work again after verify. Never create a second Work to recover a
   `DRAFT` Work with a `PENDING` verification.
5. If the current Work status is `DRAFT`, write this publish input, replacing
   `2` with the fresh Work revision:

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "expectedRevision": 2,
     "status": "PUBLISHED"
   }
   ```

   Run `viceme --profile <profile> merchant work update <work-id> --input
   <json>`, then `viceme --profile <profile> merchant work get <work-id>
   --merchant <merchant-id>`. If the Work status is already `PUBLISHED`, skip
   update. If it is `SUSPENDED` or `ARCHIVED`, stop and report it instead of
   silently reviving it or creating a duplicate. Never guess a revision or DNS
   challenge.
6. Read SDK access. Create or update it to the full feature set
   `danmaku,tip` using repeated flags:

   ```bash
   viceme --profile <profile> merchant work sdk-access create <work-id> \
     --merchant <merchant-id> --feature danmaku --feature tip
   ```

   For update, also pass the current `--expected-config-version`. Never run two
   independent updates that overwrite each other's feature.
7. Run `viceme --profile <profile> merchant commerce-application list --merchant
   <merchant-id>`. Locate the single application scoped to this Work, `kind:
   WEBSITE_WIDGET`, and `environment: PRODUCTION`; `(workId, environment, kind)`
   is unique. Only when no scoped application exists, create it with:

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "workId": "<work-id>",
     "kind": "WEBSITE_WIDGET",
     "environment": "PRODUCTION",
     "displayName": "<website name>",
     "origins": ["https://creator.example"],
     "returnUrls": []
   }
   ```

   Run `viceme --profile <profile> merchant commerce-application create --input
   <json>`. Whether reused or created, read it with `viceme --profile <profile>
   merchant commerce-application get <application-id> --merchant <merchant-id>`.
   Never create a second application when the scoped application has different
   display name, Origin, or return URLs; that would conflict with its unique
   identity. If its status is `REVOKED`, stop and report the terminal resource.

   If the existing configuration differs and its status is `ACTIVE`, suspend
   its exact revision before editing:

   ```bash
   viceme --profile <profile> merchant commerce-application suspend <application-id> \
     --merchant <merchant-id> --expected-revision <application-revision>
   ```

   Read the application again after suspend. For a differing `DRAFT` or
   `SUSPENDED` application, write this update input with its fresh revision:

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "expectedRevision": 2,
     "displayName": "<website name>",
     "origins": ["https://creator.example"],
     "returnUrls": []
   }
   ```

   Run `viceme --profile <profile> merchant commerce-application update
   <application-id> --input <json>`, then read it again. When the configuration
   matches and status is `DRAFT` or `SUSPENDED`, activate its exact revision:

   ```bash
   viceme --profile <profile> merchant commerce-application activate <application-id> \
     --merchant <merchant-id> --expected-revision <application-revision>
   ```

   Read it again with the same `get` command. If it is already `ACTIVE` and the
   configuration matches, skip activation. If a create response is lost, list
   again before another create. A Website Widget has no Product binding.
8. Insert one combined loader after a stable target element:

   ```html
   <div id="viceme-engagement"></div>
   <script
     defer
     src="<profile-web-base-url>/viceme-sdk/v1/viceme.min.js"
     data-viceme-work="<work-key>"
     data-viceme-region="<cn-or-global>"
     data-viceme-features="danmaku,tip"
     data-viceme-target="#viceme-engagement"
     data-viceme-theme="auto"
   ></script>
   ```

9. Preserve CSP and any nonce. Run repository checks, deploy, and verify desktop
   plus 320px width. Confirm both capabilities mount, a failure in one does not
   remove the other, host controls remain clickable, danmaku persists, Tip is
   keyboard reachable, and Escape returns the hosted payment surface to its
   initial amount form. Shop owns that state transition; the SDK only sends the
   sanitized close notification to the host.

## Constraints

- Work ID is internal resource identity; `workKey` is the only public loader
  identity.
- Profile market region controls `cn` versus `global`; page locale does not.
- Do not call Shop APIs outside the CLI, write database state, copy runtime
  source, or place credentials in page attributes.
- Report public resource IDs, checks, responsive coverage, and unverified real
  payment boundaries without exposing secrets or DNS challenge values.
