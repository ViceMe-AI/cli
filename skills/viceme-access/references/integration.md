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

Access configuration requires an explicitly published website binding. When
`.viceme/website.json` is missing or has no `workKey`, stop and use
`$viceme-publish` first. Complete its website metadata review, obtain the
user's publication confirmation, and publish the website. Return here only
after that succeeds. `viceme access init` never publishes a website itself.

Then create and apply the common access configuration in one command:

```bash
viceme access init --website . --name "Dagou Tap" \
  --follow "dingdong=叮咚鸡" \
  --purchase "emperor=帝皇" --price-minor 1000 \
  --purchase "emperor-pro=帝皇 Pro" --price-minor 2000
```

The command writes the YAML below, activates it, and returns the authoritative
work response. Manual YAML editing plus `viceme access apply` is only needed
for later changes.

```yaml
schemaVersion: 1
workKey: wrk_example
region: cn
displayName: Dagou Tap
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
status: ACTIVE
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
call a follow mutation directly. For a follow gate, the ViceMe login consent
layer shows the creator avatar above the display name and published-work count on
one line, separated by `·`. The count includes both published Skills and websites;
up to two recent covers include one of each when both kinds are available. The
description shows its first 50 characters and exposes the full text on hover. Its
single `授权` action opens the WeChat QR flow
directly, without a separate WeChat authorization transition page. Accepting
authorization signs the user in and automatically follows the creator; no
post-login follow layer is shown.

The SDK uses `<viceme-access-layer>`: a mobile bottom sheet and desktop in-page
layer with Shadow DOM and ViceMe-owned styles. Custom presenters, host-page
style detection, and generated style adaptations are intentionally unavailable
until the interaction contract is stable.

Sign-in and checkout stay inside that same bottom sheet or in-page layer. From
the original gated user click, the SDK loads the checkout frame directly without
an intermediate “去购买” confirmation; selecting a payment method and paying still
requires explicit confirmation. The host page remains visually unobscured behind
the layer. The WeChat frame inherits the consent layer's height so the QR code and
one-tap authorization remain visible without a transition jump, while checkout
keeps a stable layer height throughout loading. The SDK validates the API origin plus a
per-action message channel, then checks access again. The
checkout frame exchanges its short-lived, retryable bootstrap code for an
in-memory bearer session and does not depend on third-party cookies. A `PENDING` order or a
message never grants access; only a new server `access.check()` decision does.

## Error handling

Branch only on `ViceMeError.code`. Common codes include:

- `SESSION_EXPIRED`: ask the user to retry or sign in again.
- `AUTH_CANCELLED`: the user cancelled or the same-tab continuation expired.
- `CAPABILITY_DISABLED`: inspect the work configuration.
- `CHECKOUT_UNAVAILABLE`: verify the work's one-time offer is active and has a positive current price.

Always call `destroy()` during teardown.
