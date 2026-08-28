---
name: viceme-engagement
description: Add ViceMe hosted danmaku and open tips to one verified Website Work through one SDK access resource and either the official Tip UI or Headless Tip.
---

# Integrate ViceMe Danmaku And Tips

Configure one resource graph and one selected Tip UI path. Never create separate
Works for danmaku and Tip. The Website and DNS requirements below belong to
danmaku's independent security contract; open tips do not add an application or
Origin gate.

## Workflow

1. Inspect the exact production HTTPS Origin, target page, deployment command,
   CSP, responsive layout, and browser tests.
2. Run `viceme profile list` and keep the active Profile. Record its exact
   `marketRegion`; never infer the market from locale or hostname. Stop and
   explain the boundary unless `marketRegion` is exactly `cn`, because the
   first open-tip release is CN/CNY only. Run `viceme --profile <profile> auth
   status`; require `merchant-commerce:read` and
   `merchant-commerce:write`. Then select an active Merchant from `viceme
   --profile <profile> merchant accounts`.
3. Ask the user to choose the official Tip UI or Headless Tip before any Work,
   Website verification, SDK access, or page write. First prove that the exact
   `0.4.0` Tip release is complete in both publication regions. Every combined
   route requires these four objects:

   ```bash
   curl --fail --silent --show-error --output /dev/null https://s3.viceme.cn/viceme-sdk/0.4.0/index.js
   curl --fail --silent --show-error --output /dev/null https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js
   curl --fail --silent --show-error --output /dev/null https://s3.viceme.ai/viceme-sdk/0.4.0/index.js
   curl --fail --silent --show-error --output /dev/null https://s3.viceme.ai/viceme-sdk/0.4.0/tip.js
   ```

   Official UI and CDN Headless also require the immutable CN Danmaku entry:

   ```bash
   curl --fail --silent --show-error --output /dev/null https://s3.viceme.cn/viceme-sdk/0.4.0/danmaku.js
   ```

   For Headless Tip, ask whether the host uses npm or CDN ESM. The npm route
   instead also requires this command to return exactly `0.4.0`:

   ```bash
   npm view @viceme-ai/sdk@0.4.0 version --json \
     --registry=https://registry.npmjs.org \
     --@viceme-ai:registry=https://registry.npmjs.org
   ```

   If any required check fails, stop. Do not create, verify, or publish a Work,
   create or update SDK access, or edit the page. Never substitute `latest`,
   `/v1`, another version, a declarative loader, any browser global, or copied
   SDK source. Only after the complete dual-region preflight and any selected
   npm or Danmaku check succeed, continue to Work selection and later mutation.
4. Run `viceme --profile <profile> merchant work list --merchant <merchant-id>`.
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
5. Whether the Work was reused or created, read it with `viceme --profile
   <profile> merchant work get <work-id> --merchant <merchant-id>`. If
   `website.ownershipStatus` is not `VERIFIED`, first run:

   ```bash
   viceme --profile <profile> merchant work website-verification get <work-id> \
     --merchant <merchant-id>
   ```

   If the latest verification status is `PENDING` and it is unexpired, reuse
   its existing `challenge` and version. If no verification exists or its status
   is `FAILED` or `EXPIRED`, create one from the current Work revision:

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
6. If the current Work status is `DRAFT`, write this publish input, replacing
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
7. After the user confirms this Work, read SDK access but do not write it yet.
   Snapshot the previous complete feature set, status, and exact
   `configVersion`, or record that the resource was absent. This snapshot is the
   rollback source.
8. Apply the complete desired feature set `danmaku,tip` with one create or
   update. For an absent resource:

   ```bash
   viceme --profile <profile> merchant work sdk-access create <work-id> \
     --merchant <merchant-id> --feature danmaku --feature tip
   ```

   For an existing resource, make one replacement update from the current
   version:

   ```bash
   viceme --profile <profile> merchant work sdk-access update <work-id> \
     --merchant <merchant-id> --expected-config-version <config-version> \
     --feature danmaku --feature tip
   ```

   Never run independent feature updates that overwrite each other. Creation
   returns permanent public `keys.test` and `keys.live`; updates and disable do
   not rotate either identifier.
9. Start either route with `keys.test` so Tip can be simulated without real
   funds. For the official UI, mount both capabilities from the exact CDN ESM
   release after a stable target:

   ```html
   <div id="viceme-engagement"></div>
   <script type="module">
     import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.4.0/index.js";
     import { mountDanmaku } from "https://s3.viceme.cn/viceme-sdk/0.4.0/danmaku.js";
     import { mountTip } from "https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js";

     const target = document.querySelector("#viceme-engagement");
     if (!target) throw new Error("ViceMe engagement target is missing");

     const client = createViceMe({ workKey: "wrk_test_...", region: "cn" });
     await client.ready();
     const mountResults = await Promise.allSettled([
       mountDanmaku(client, { target, theme: "auto" }),
       mountTip(client, { target, theme: "auto" }),
     ]);
     const mountHandles = mountResults.flatMap((result) =>
       result.status === "fulfilled" ? [result.value] : [],
     );

     function destroyViceMeEngagement() {
       for (const handle of mountHandles) handle.destroy();
       client.destroy();
     }
   </script>
   ```

   For Headless Tip, load `viceme-tip` and follow its integration contract for
   config-driven controls, result handling, and SANDBOX validation. Mount only
   danmaku and create the host-owned Tip controller from the same exact release.
   The example shows CDN ESM; use the equivalent exact npm subpaths only when
   the npm preflight was selected:

   ```html
   <div id="viceme-engagement"></div>
   <script type="module">
     import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.4.0/index.js";
     import { mountDanmaku } from "https://s3.viceme.cn/viceme-sdk/0.4.0/danmaku.js";
     import { createTip } from "https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js";

     const target = document.querySelector("#viceme-engagement");
     if (!target) throw new Error("ViceMe engagement target is missing");

     const client = createViceMe({ workKey: "wrk_test_...", region: "cn" });
     await client.ready();
     const danmakuHandle = await mountDanmaku(client, {
       target,
       theme: "auto",
     });
     const tip = createTip(client);
     const config = await tip.getConfig();
     renderTipControls(config, (selection) => tip.open(selection));

     function destroyViceMeEngagement() {
       tip.destroy();
       danmakuHandle.destroy();
       client.destroy();
     }
   </script>
   ```

   Keep every returned handle/controller with the owning instance. In a real
   SPA, component, or route unmount, run `destroyViceMeEngagement()` so mounts
   or Headless Tip are destroyed before `client.destroy()`. Do not use
   `pagehide`, which also fires for bfcache.
10. Preserve CSP and any nonce. Verify desktop plus 320px width. Confirm both
   capabilities mount, a failure in one does not remove the other, host controls
   remain clickable, danmaku persists, and Tip is keyboard reachable. Tip
   visitors do not sign in to ViceMe. The browser source is recorded as an
   unverified Origin unless optional trusted attribution exists; that does not
   gate a tip.
11. Show the SANDBOX evidence and obtain explicit user confirmation before
   replacing every selected route's test key with `keys.live`
   (`wrk_live_...`) in the single `createViceMe` call. Do not change imports or
   mount the official Tip UI on the Headless route. A production key cannot
   simulate payment.

## Constraints

- Work ID is internal resource identity. Both Work keys are permanent public
  identifiers, not credentials.
- The verified Website Origin remains mandatory for danmaku in this combined
  flow. It is not a Tip payment gate.
- Profile market region controls runtime selection; the first open-tip release
  supports CN/CNY. Page locale does not select a market.
- Do not call Shop payment APIs, inspect raw payment state, or put credentials
  in page code.
- Report public resource IDs and keys, checks, responsive coverage, SANDBOX
  evidence, and unverified real-payment boundaries without exposing secrets or
  DNS challenge values.

## Recovery

- On a lost create/update response, read the same SDK access before retrying.
- On conflict, read the latest exact `configVersion`; never guess or split the
  combined feature update.
- If integration cannot be completed after a server write, read the latest
  version and restore the complete pre-change feature set in one update. If the
  resource was previously disabled, disable it again after restoring its
  features; if it was absent, disable the newly created resource. Permanent
  test/live keys do not rotate during rollback.
