---
name: viceme-tip
description: Enable open ViceMe tips for any public merchant Work, then integrate the official UI or a Headless host-owned UI with permanent test and live Work keys.
---

# Integrate ViceMe Tips

Build the smallest open-tip integration for one existing Work. Read
[integration-contract.md](references/integration-contract.md) before editing the
host. A Work is the only tip target; Website ownership and Commerce Application
resources are not default prerequisites.

## Workflow

1. Run `viceme profile list` and keep the active Profile fixed for the entire
   workflow. Record its exact API base URL and `marketRegion`. Never derive a
   Shop URL from DNS, locale, memory, or another Profile. The first tip release
   supports only a `cn` Profile and CNY; stop and explain the boundary for
   another market.
2. Run `viceme --profile <profile> auth status`. If the selected Profile is not
   authenticated or lacks `merchant-commerce:read` or
   `merchant-commerce:write`, run `viceme --profile <profile> auth login` and
   wait for that same Profile to become authorized.
3. Run `viceme --profile <profile> merchant accounts`; use an active Merchant
   owned by the current login, asking the user to choose when several exist.
   Then run `viceme --profile <profile> merchant work list --merchant
   <merchant-id>`. A candidate may have any Work kind. It is eligible only when
   its response shows `owner.kind: MERCHANT`, the matching Merchant account,
   and `status: PUBLISHED`. Show the eligible Works and ask the user to confirm
   the selected Work before changing its tip feature.
4. Ask the user to choose the official UI or Headless before any Work
   publication, SDK access create/update, or host edit. First prove that the
   exact `0.4.0` Tip release is complete in both publication regions. Every
   route requires all four commands to succeed:

   ```bash
   curl --fail --silent --show-error --output /dev/null https://s3.viceme.cn/viceme-sdk/0.4.0/index.js
   curl --fail --silent --show-error --output /dev/null https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js
   curl --fail --silent --show-error --output /dev/null https://s3.viceme.ai/viceme-sdk/0.4.0/index.js
   curl --fail --silent --show-error --output /dev/null https://s3.viceme.ai/viceme-sdk/0.4.0/tip.js
   ```

   Official UI and CDN Headless use the preflighted immutable CN ESM files. For
   Headless, ask whether the host will use npm or CDN ESM. The npm route also
   requires this official-registry command to return exactly `0.4.0`:

   ```bash
   npm view @viceme-ai/sdk@0.4.0 version --json \
     --registry=https://registry.npmjs.org \
     --@viceme-ai:registry=https://registry.npmjs.org
   ```

   If any required check fails, stop. Do not create, verify, or publish a Work,
   create or update SDK access, or edit the host. Never substitute `latest`,
   `/v1`, another version, a declarative loader, any browser global, or copied
   SDK source. Only after the complete dual-region preflight and any selected
   npm check succeed, continue to Work publication or any later mutation.
5. If no eligible Work exists, stop the tip setup and load `viceme-publish`.
   Follow that Skill's authoritative publication route for the user's actual
   deliverable through a public Work, then return here with the resulting Work
   ID. Do not invent a Website request or duplicate publication JSON inside
   this Skill.
6. Read the current resource with `viceme --profile <profile> merchant work
   sdk-access get <work-id> --merchant <merchant-id>`. Do not write yet. When it
   exists, snapshot its complete feature set, status, and exact `configVersion`
   for conflict handling and rollback. Record that it was absent otherwise.
7. Apply the desired server state:

   - When absent, create it with `viceme --profile <profile> merchant work
     sdk-access create <work-id> --merchant <merchant-id> --feature tip`.
   - When present without tip, update from its exact `configVersion`:

     ```bash
     viceme --profile <profile> merchant work sdk-access update <work-id> \
       --merchant <merchant-id> --expected-config-version <config-version> \
       --feature tip
     ```

     This replaces the complete feature set. Also pass `--feature danmaku` when
     it exists in the snapshot; never omit an unrelated enabled feature.
   - When status is `DISABLED`, update from the exact `configVersion` with the
     complete desired feature set to re-enable the same keys.
   - When status is `ACTIVE` and tip is already present, do not write it again.

   Create returns both permanent public identifiers at once. Record
   `keys.test` and `keys.live`; neither is a credential. Update and disable
   never rotate them. There is no rotate command.
8. Integrate the selected path below. Never create a Commerce Application as
   part of the default integration.

## Official UI

Start with `keys.test` and the exact preflighted CDN ESM files:

```html
<div id="viceme-tip"></div>
<script type="module">
  import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.4.0/index.js";
  import { mountTip } from "https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js";

  const target = document.querySelector("#viceme-tip");
  if (!target) throw new Error("ViceMe Tip target is missing");

  const client = createViceMe({ workKey: "wrk_test_...", region: "cn" });
  await client.ready();
  const mountHandle = await mountTip(client, {
    target,
    theme: "auto",
  });

  function destroyViceMeTip() {
    mountHandle.destroy();
    client.destroy();
  }
</script>
```

Verify the SANDBOX flow on desktop, mobile, and keyboard navigation. Only after
the user explicitly confirms the SANDBOX result, replace the public identifier
with `keys.live`; do not change the pinned imports or other options:

```html
<div id="viceme-tip"></div>
<script type="module">
  import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.4.0/index.js";
  import { mountTip } from "https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js";

  const target = document.querySelector("#viceme-tip");
  if (!target) throw new Error("ViceMe Tip target is missing");

  const client = createViceMe({ workKey: "wrk_live_...", region: "cn" });
  await client.ready();
  const mountHandle = await mountTip(client, {
    target,
    theme: "auto",
  });

  function destroyViceMeTip() {
    mountHandle.destroy();
    client.destroy();
  }
</script>
```

Keep the `mountHandle` with the owning component or route. On every real SPA,
component, or route unmount, call `destroyViceMeTip()` so
`mountHandle.destroy()` runs before `client.destroy()`. A plain static document
has no in-document component unmount; do not invent one with `pagehide`, which
also fires for bfcache.

Preserve any existing CSP and nonce. Add only the exact CDN and regional runtime
origins proven necessary by browser evidence. The embedding origin is recorded
by ViceMe but is not a default payment gate.

## Headless

Use the npm and exact CDN ESM examples in
[integration-contract.md](references/integration-contract.md). The host renders
amount and provider controls from `getConfig()`, then calls `open()`. It does
not create or inspect orders. The final ViceMe confirmation is read-only and
cannot be replaced, restyled, or supplemented with host-controlled payment
claims.

Start Headless with `keys.test` too. Run the Local Fake and SANDBOX checks from
the contract. Move to `keys.live` only after showing the verified
result and receiving explicit user confirmation.

## Completion

- Report the Work ID, Work kind, selected Profile, public test/live
  key identifiers, integration route, changed files, and checks.
- State whether SANDBOX simulation and any real production payment were
  exercised. A production key cannot simulate a payment.
- Never report login credentials, cookies, payment details, or internal
  capabilities. Do not label Work keys as secrets.
- To stop tips while retaining danmaku, replace the full feature set without
  `tip`. Disable all SDK access only when every feature should stop; neither
  action rotates the permanent keys.

## Recovery

- On a lost create/update response, read the same SDK access before retrying.
- On a config conflict, read the current `configVersion`, preserve unrelated
  enabled features, and ask again before changing the selected Work.
- If this workflow changed SDK access but integration cannot be completed, read
  the latest config and restore that complete feature set from the pre-change
  snapshot in one update. If access was previously `DISABLED`, disable it again
  after restoring its complete feature set; if access was absent, disable the
  newly created resource. Never "roll back" with a partial feature list.
- Do not create another Work because a tip setup response is unknown. Resume
  publication only through `viceme-publish` and its authoritative identity.
