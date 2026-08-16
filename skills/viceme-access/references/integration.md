# Integration reference

## Access configuration

Create and apply the common configuration in one command:

```bash
viceme access init --name "Dagou Tap" \
  --follow "dingdong=叮咚鸡" \
  --product "dagou-tap" \
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
productSlug: dagou-tap
features:
  dingdong:
    title: 叮咚鸡
    policy:
      type: FOLLOW_OWNER
  emperor:
    title: 帝皇
    policy:
      type: PURCHASE_BOUND_PRODUCT
status: ACTIVE
configVersion: 1
```

`productSlug` is only control-plane input. The CLI/API validates that the globally unique slug belongs to the current creator and stores the product ID. Multiple workKeys may bind the same product; one paid purchase then satisfies `PURCHASE_BOUND_PRODUCT` on each of them.

Omit `productSlug` when the website only needs login or following. Purchase policies require a bound product.

Supported policies:

- `FOLLOW_OWNER`: current user follows the work owner.
- `PURCHASE_BOUND_PRODUCT`: current user has a paid purchase for the bound product.
- `PURCHASE_ANY_OWNER_PRODUCT`: current user has a paid purchase for any product of the work owner; the bound product remains the checkout target.

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
already granted; otherwise it opens the required in-page sign-in, creator-follow,
or checkout interface:

```ts
const decision = await viceme.access.require("dingdong");
if (decision.allowed) enableDingdong();
```

The access check never performs the action by itself. In particular, creator
sites must not call a follow mutation directly. The user follows from the
creator-follow interface shown by `require()`.

When the site already has a React/HTML design system, pass a site-owned
presenter so the SDK can use its Sheet/Drawer and Button components:

```ts
const viceme = createViceMe({
  workKey: "wrk_example",
  region: "cn",
  presenter: siteAccessPresenter,
});
```

The presenter receives a `perform()` callback and may call it only from the
corresponding user-activated button. When no presenter is supplied, the SDK
uses `<viceme-access-layer>`: a mobile bottom sheet and desktop in-page layer
with Shadow DOM, inherited typography/colors, CSS variables, and `::part()`.

Sign-in and checkout stay inside that same bottom sheet or in-page layer. The
SDK embeds the provider/checkout frame only after the user confirms, validates
the API origin plus a per-action message channel, then checks access again. The
checkout frame exchanges its one-time bootstrap code for an in-memory bearer
session and does not depend on third-party cookies. A `PENDING` order or a
message never grants access; only a new server `access.check()` decision does.

## Error handling

Branch only on `ViceMeError.code`. Common codes include:

- `SESSION_EXPIRED`: ask the user to retry or sign in again.
- `AUTH_CANCELLED`: the user cancelled or the same-tab continuation expired.
- `CAPABILITY_DISABLED`: inspect the work configuration.
- `CHECKOUT_UNAVAILABLE`: verify the bound product is active, paid, and still owned by the creator.

Always call `destroy()` during teardown.
