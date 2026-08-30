# Integration reference

## Choose the integration path

Do not force every website through a separate design phase. When the user has
already identified the feature, entry point, expected behavior, and price,
verify the referenced code and implement that plan directly. Analyze only
missing details. When no plan exists, inspect the site's main user journey,
existing auth or payment code, rendered UI, component library, design tokens,
responsive states, and the exact business actions before proposing candidates.

A proposal should identify each feature key and title, policy, current UI entry,
unchanged protected action, host component and variant to reuse, affected files,
protection strength, and price still requiring the user's decision. Do not apply
configuration or edit host code until the user chooses a proposal. If the core
action has no safe outer call site, report that boundary instead of refactoring
the action to manufacture one.

## Access configuration

Access configuration requires an explicitly published Website Work. Use
`$viceme-publish` first, complete its metadata review and confirmation, then
take the authoritative Work ID from the publication result or
`viceme merchant work list`. Retired local website bindings are not a source of
truth. `viceme access init` never publishes a website itself.

Then create and apply the common access configuration in one command:

```bash
viceme access init --work 22222222-2222-4222-8222-222222222222 \
  --follow "dingdong=叮咚鸡" \
  --purchase "emperor=帝皇" --price-minor 1000 \
  --purchase "emperor-pro=帝皇 Pro" --price-minor 2000
```

The command writes the YAML below, activates it, and returns the authoritative
work response. Manual YAML editing plus `viceme access apply` is only needed
for later changes.

```yaml
schemaVersion: 3
apiBaseUrl: https://api.viceme.cn
webBaseUrl: https://viceme.cn
region: cn
merchantAccountId: 11111111-1111-4111-8111-111111111111
workId: 22222222-2222-4222-8222-222222222222
workKey: wrk_example
hostedFeatures: []
features:
  dingdong:
    title: 叮咚鸡
    policy:
      type: FOLLOW_OWNER
  emperor:
    title: 帝皇
    priceCents: 1000
    policy:
      type: WORK_ENTITLEMENT
  emperor-pro:
    title: 帝皇 Pro
    priceCents: 2000
    policy:
      type: WORK_ENTITLEMENT
configVersion: 1
```

Each `WORK_ENTITLEMENT` feature requires its own positive `priceCents`. One
website may therefore expose multiple independently priced features. A single
`--price-minor` value is shared when several `--purchase` flags use one price;
otherwise repeat the flag once per purchase feature.

Supported policies:

- `FOLLOW_OWNER`: current user follows the work owner.
- `WORK_ENTITLEMENT`: current user has an active entitlement for this priced feature.

## Browser SDK

```ts
import { createViceMe } from "@viceme-ai/sdk";

const viceme = createViceMe({
  workKey: "wrk_example",
  region: "cn",
});

await viceme.ready();

const features = await viceme.access.getFeatures();
const emperor = features.find((feature) => feature.featureKey === "emperor");

const decisions = await viceme.access.checkMany(["dingdong", "emperor"]);
setDingdongUnlocked(decisions.dingdong.allowed);
setEmperorUnlocked(decisions.emperor.allowed);
```

Render `emperor.title` and `emperor.price` with the host site's existing
Button/Card components and price formatter when the entry needs those values.
Do not hard-code them from `.viceme/access.yaml`. Preserve existing typography,
color, radius, spacing, focus, loading, responsive, and error-feedback patterns.
This host-owned presentation does not customize the ViceMe access layer.

Call a gate from the user's click handler and keep the original action unchanged:

```ts
async function handleEmperorClick() {
  const decision = await viceme.access.require("emperor");
  if (!decision.allowed) return;
  await runEmperor();
}
```

The gate silently returns when access is already granted; otherwise it opens
the required in-page sign-in, owner-follow, or checkout interface.

The access check never performs the action by itself, and host sites must not
call a follow mutation directly. Login authenticates only. If `FOLLOW_OWNER`
still requires attention after login, the ViceMe layer shows the creator and a
separate `关注` action so consent remains explicit.

The SDK uses `<viceme-access-layer>`: a mobile bottom sheet and desktop in-page
layer with Shadow DOM and ViceMe-owned styles. Custom presenters, host-page
style detection, and generated style adaptations are intentionally unavailable
until the interaction contract is stable.

Sign-in starts from that layer and may open the ViceMe-hosted login popup;
checkout stays inside the bottom sheet or in-page layer. From the original
gated user click, the SDK loads the checkout frame directly without
an intermediate “去购买” confirmation; selecting a payment method and paying still
requires explicit confirmation. The host page remains visually unobscured behind
the layer. The WeChat frame inherits the consent layer's height so the QR code and
one-tap authorization remain visible without a transition jump, while checkout
keeps a stable layer height throughout loading. The SDK validates the API origin plus a
per-action message channel, then checks access again. The
checkout frame uses a server-created hosted checkout session backed by the
registered CommercePrincipal. A `PENDING` order or a message never grants
access; only a new server `access.check()` decision does.

## Error handling

Branch only on `ViceMeError.code`. Common codes include:

- `SESSION_EXPIRED`: ask the user to retry or sign in again.
- `AUTH_CANCELLED`: the user cancelled or the same-tab continuation expired.
- `CAPABILITY_DISABLED`: inspect the work configuration.
- `CHECKOUT_UNAVAILABLE`: verify the feature's unlisted Product is active and has a positive current price.

Always call `destroy()` during teardown.
