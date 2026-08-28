---
name: viceme-tip
description: Integrate ViceMe tips into one public website through a verified Website Work, active tip SDK access, and a production Website Widget application.
---

# Integrate ViceMe Tips

Build the smallest real integration: one host-owned page plus the official
ViceMe loader. Read [integration-contract.md](references/integration-contract.md)
before editing.

## Workflow

1. Inspect the target instructions and find the exact deployed HTTPS Origin,
   page entry point, deployment path, CSP, and browser tests. Do not treat
   localhost or a preview hostname as production.
2. Run `viceme profile list` and keep the active Profile. Run `viceme --profile
   <profile> auth status` and re-authenticate on that Profile if
   `merchant-commerce:read` or `merchant-commerce:write` is missing.
3. Run `viceme --profile <profile> merchant accounts`; select an active Merchant
   owned by the current login. Run `viceme --profile <profile> merchant work
   list --merchant <merchant-id>` and reuse a Website Work only when its
   `website.canonicalOrigin` exactly equals the deployed Origin.
4. If no exact Work exists, create one using this strict request. Fill its
   observed content; do not add or guess fields:

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
   `clientRequestId`; do not create a new identity. Create DNS ownership with:

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
   --merchant <merchant-id>`. Never guess a revision or DNS challenge.
5. Get SDK access for that Work. Create it with `--feature tip` when absent, or
   update from its current `configVersion` with the full feature set while
   preserving `danmaku` when present. Record the returned public `workKey`.
6. Run `merchant commerce-application list --merchant <merchant-id>`. Reuse only
   the application matching this Work, `kind: WEBSITE_WIDGET`, `environment:
   PRODUCTION`, and the exact canonical Origin. Otherwise create it with:

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

   Use `merchant commerce-application create --input <json>`, then activate it
   with its exact revision. A Website Widget has no Product binding.
7. Insert exactly one official loader tag. Use the selected Profile's exact Web
   base URL and market region; do not infer either from page language:

   ```html
   <div id="viceme-engagement"></div>
   <script
     defer
     src="<web-base-url>/viceme-sdk/v1/viceme.min.js"
     data-viceme-work="<work-key>"
     data-viceme-region="<cn-or-global>"
     data-viceme-features="tip"
     data-viceme-target="#viceme-engagement"
     data-viceme-theme="auto"
   ></script>
   ```

8. Preserve CSP and any script nonce. Add only the exact Shop Origin to the
   directives browser evidence requires. Run repository checks, deploy, and
   verify the real Origin: the Tip frame resizes, remains keyboard reachable,
   opens payment, returns to its initial amount form on Escape, and emits no CSP
   or widget errors. Shop performs this close before the SDK sends the sanitized
   `viceme:widget-close` notification; the host page needs no close listener.
9. Report the public Work ID/key, application ID, canonical Origin, checks, and
   whether a real payment was exercised. Never report login tokens, DNS
   challenges, cookies, payment credentials, access tokens, or signed URLs.

## Recovery

- Recover Work creation only by replaying the identical request with the same
  stable `clientRequestId`.
- Recover SDK access by reading it before attempting another create.
- Recover application creation by listing and matching Work, kind, environment,
  and Origin before another create.
- Never guess a new config/revision after a conflict; read the resource again.
