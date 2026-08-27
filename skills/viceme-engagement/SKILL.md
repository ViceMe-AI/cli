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
2. Keep one active CLI Profile. Require `merchant-commerce:read` and
   `merchant-commerce:write`, then select an active Merchant from
   `merchant accounts`.
3. Reuse or create one Website Work for the exact Origin. Complete DNS
   verification, then publish with the fresh Work revision returned by verify.
4. Read SDK access. Create or update it to the full feature set
   `danmaku,tip` using repeated flags:

   ```bash
   viceme --profile <profile> merchant work sdk-access create <work-id> \
     --merchant <merchant-id> --feature danmaku --feature tip
   ```

   For update, also pass the current `--expected-config-version`. Never run two
   independent updates that overwrite each other's feature.
5. List Commerce applications. Reuse or create one `PRODUCTION`
   `WEBSITE_WIDGET` for the same Work and exact canonical Origin, with empty
   return URLs and no Products, then activate its exact revision.
6. Insert one combined loader after a stable target element:

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

7. Preserve CSP and any nonce. Run repository checks, deploy, and verify desktop
   plus 320px width. Confirm both capabilities mount, a failure in one does not
   remove the other, host controls remain clickable, danmaku persists, Tip is
   keyboard reachable, and Escape closes the hosted payment surface.

## Constraints

- Work ID is internal resource identity; `workKey` is the only public loader
  identity.
- Profile market region controls `cn` versus `global`; page locale does not.
- Do not call Shop APIs outside the CLI, write database state, copy runtime
  source, or place credentials in page attributes.
- Report public resource IDs, checks, responsive coverage, and unverified real
  payment boundaries without exposing secrets or DNS challenge values.
