# Integration reference

## Access configuration

This workflow starts from a website binding already created by `$viceme-publish`. Create and apply the common access configuration in one command:

```bash
viceme access init --website . --name "Dagou Tap" \
  --follow "dingdong=叮咚鸡" \
  --price-minor 1000 \
  --purchase "emperor=帝皇"
```

The command writes the YAML below, activates it, and returns the authoritative
work response. Manual YAML editing plus `viceme access apply` is only needed
for later changes.

```yaml
schemaVersion: 1
workKey: wrk_example
region: cn
displayName: Dagou Tap
priceCents: 1000
features:
  dingdong:
    title: 叮咚鸡
    policy:
      type: FOLLOW_OWNER
  emperor:
    title: 帝皇
    policy:
      type: WORK_ENTITLEMENT
status: ACTIVE
configVersion: 1
```

Omit `priceCents` when the website only needs login or following. `WORK_ENTITLEMENT` requires a positive one-time price.

Supported policies:

- `FOLLOW_OWNER`: current user follows the work owner.
- `WORK_ENTITLEMENT`: current user has an active entitlement for this work.

## Browser SDK

```ts
import { createViceMe } from "@viceme-ai/sdk";

const viceme = createViceMe({
  workKey: "wrk_example",
  region: "cn",
});

await viceme.ready();

const decisions = await viceme.access.checkMany(["dingdong", "emperor"]);
setDingdongUnlocked(decisions.dingdong.allowed);
setEmperorUnlocked(decisions.emperor.allowed);
```

Call a gate from the user's click handler. It silently returns when access is
already granted; otherwise it opens the required in-page sign-in, owner-follow,
or checkout interface:

```ts
const decision = await viceme.access.require("dingdong");
if (decision.allowed) enableDingdong();
```

The access check never performs the action by itself. In particular, host sites
must not call a follow mutation directly. The user follows from the owner-follow
interface shown by `require()`.

The SDK uses `<viceme-access-layer>`: a mobile bottom sheet and desktop in-page
layer with Shadow DOM and ViceMe-owned styles. Custom presenters, host-page
style detection, and generated style adaptations are intentionally unavailable
until the interaction contract is stable.

Sign-in and checkout stay inside that same bottom sheet or in-page layer. From
the original gated user click, the SDK loads the checkout frame directly without
an intermediate “去购买” confirmation; selecting a payment method and paying still
requires explicit confirmation. The SDK validates the API origin plus a
per-action message channel, then checks access again. The
checkout frame exchanges its one-time bootstrap code for an in-memory bearer
session and does not depend on third-party cookies. A `PENDING` order or a
message never grants access; only a new server `access.check()` decision does.

## Error handling

Branch only on `ViceMeError.code`. Common codes include:

- `SESSION_EXPIRED`: ask the user to retry or sign in again.
- `AUTH_CANCELLED`: the user cancelled or the same-tab continuation expired.
- `CAPABILITY_DISABLED`: inspect the work configuration.
- `CHECKOUT_UNAVAILABLE`: verify the work's one-time offer is active and has a positive current price.

Always call `destroy()` during teardown.
