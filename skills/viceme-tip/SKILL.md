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
2. Keep the active result from `viceme profile list`. Run `auth status` and
   re-authenticate on that Profile if `merchant-commerce:read` or
   `merchant-commerce:write` is missing.
3. Run `merchant accounts`; select an active Merchant owned by the current
   login. Run `merchant work list --merchant <merchant-id>` and reuse only a
   Website Work whose canonical Origin is exact.
4. If needed, create a Website Work with `merchant work create --input <json>`.
   Complete DNS ownership with `merchant work website-verification create` and
   `verify`, then publish using the fresh returned Work revision. The TXT value
   is the returned `challenge` verbatim.
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
   opens payment, closes with Escape, and emits no CSP or widget errors.
9. Report the public Work ID/key, application ID, canonical Origin, checks, and
   whether a real payment was exercised. Never report login tokens, DNS
   challenges, cookies, payment credentials, access tokens, or signed URLs.

## Recovery

- Recover Work creation by replaying the same stable `clientRequestId` and
  identical request.
- Recover SDK access by reading it before attempting another create.
- Recover application creation by listing and matching Work, kind, environment,
  and Origin before another create.
- Never guess a new config/revision after a conflict; read the resource again.
