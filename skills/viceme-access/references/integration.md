# Integration reference

## Configure the existing Work resource

Read the published Website Work's SDK access before changing it:

```bash
viceme merchant work sdk-access get 22222222-2222-4222-8222-222222222222 \
  --merchant 11111111-1111-4111-8111-111111111111
```

Create the resource only when the API reports that it does not exist:

```bash
viceme merchant work sdk-access create 22222222-2222-4222-8222-222222222222 \
  --merchant 11111111-1111-4111-8111-111111111111 \
  --follow "dingdong=叮咚鸡" \
  --purchase "emperor=帝皇" --price-minor 1000
```

For an existing resource, use the returned `configVersion`. Omitted hosted feature
flags preserve the current danmaku and tip values:

```bash
viceme merchant work sdk-access update 22222222-2222-4222-8222-222222222222 \
  --merchant 11111111-1111-4111-8111-111111111111 \
  --expected-config-version 3 \
  --follow "dingdong=叮咚鸡" \
  --purchase "emperor=帝皇" --price-minor 1000
```

`FOLLOW_OWNER` checks the current creator subscription. `WORK_ENTITLEMENT`
provisions one server-managed, unlisted Product per feature key and checks its
active DigitalEntitlement.

## Browser SDK

```ts
import { createViceMe } from "@viceme-ai/sdk";

const viceme = createViceMe({ workKey: "wrk_example", region: "cn" });
await viceme.ready();

const features = await viceme.access.getFeatures();
const decision = await viceme.access.require("emperor");
if (decision.allowed) await runEmperor();
```

Use `access.getFeatures()` for current titles and prices. Keep the host action and
its UI intact; only place the gate at its existing user-triggered boundary.

Checkout opens a ViceMe-hosted window and the SDK polls `access.check()` until the
server observes an active entitlement. Closing or cancelling the window never
grants access.
