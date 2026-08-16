# Integration reference

## Access configuration

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

Call a gate from the user's click handler:

```ts
const decision = await viceme.access.require("dingdong");
if (decision.allowed) enableDingdong();
```

Explicit capability calls are also available:

```ts
await viceme.auth.signIn();
await viceme.follow.follow();
await viceme.follow.unfollow();
await viceme.checkout.open({ returnUrl: window.location.href });
```

After the checkout window completes, call `access.check("emperor")` again. A `PENDING` order, a success query parameter, or a locally cached value never grants access; only a server-observed `PAID` purchase does.

## Error handling

Branch only on `ViceMeError.code`. Common codes include:

- `SESSION_EXPIRED`: ask the user to retry or sign in again.
- `AUTH_POPUP_BLOCKED`: retry from a direct user gesture.
- `CAPABILITY_DISABLED`: inspect the work configuration.
- `CHECKOUT_UNAVAILABLE`: verify the bound product is active, paid, and still owned by the creator.

Always call `destroy()` during teardown.
