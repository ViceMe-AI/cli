# ViceMe Open Tip Integration Contract

## Resource Boundary

One open tip integration targets one public Merchant-owned Work of any kind:

```text
PUBLISHED Merchant Work
└── ACTIVE SDK access whose features contain tip
    ├── keys.test = wrk_test_...
    └── keys.live = wrk_live_...
```

The two Work keys are permanent public identifiers, not credentials. Creation
issues both at once. Configuration updates and disable operations do not rotate
them. Remove the `tip` feature from the Work's complete feature set to stop tips
while retaining danmaku; disable SDK access only to stop every feature.

An optional Commerce Application supplies trusted-source attribution. It is not
a tip gate and the default flow does not create one. ViceMe records only the
browser-observed normalized Origin as source attribution, never the full URL,
path, query, fragment, or campaign metadata. Without matching optional
attribution it is an unverified Origin rather than a rejected Origin.

## Release Selection Before Mutation

The first public Tip SDK release is exactly `@viceme-ai/sdk@0.4.0`. Ask the user
to choose the official UI or Headless, then verify the exact distribution for
that path before creating, verifying, or publishing a Work, changing SDK
access, or editing the host. This ordering is a server-state invariant: if the
selected release is unavailable, the workflow leaves no new Work or feature
state behind.

Every route first requires the immutable Tip entries in both publication
regions. This proves the exact release is complete rather than accepting a
partially published regional artifact set:

```bash
curl --fail --silent --show-error --output /dev/null https://s3.viceme.cn/viceme-sdk/0.4.0/index.js
curl --fail --silent --show-error --output /dev/null https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js
curl --fail --silent --show-error --output /dev/null https://s3.viceme.ai/viceme-sdk/0.4.0/index.js
curl --fail --silent --show-error --output /dev/null https://s3.viceme.ai/viceme-sdk/0.4.0/tip.js
```

Official UI and CDN Headless use the preflighted CN files. Headless may instead
use the exact npm package. For npm, additionally require the following
official-registry command to return exactly `0.4.0`:

```bash
npm view @viceme-ai/sdk@0.4.0 version --json \
  --registry=https://registry.npmjs.org \
  --@viceme-ai:registry=https://registry.npmjs.org
```

Do not fall back to `latest`, `/v1`, another mutable alias, a declarative
loader, any browser global, a Git dependency, or copied SDK source. Snapshot the
complete pre-change feature set before the one create/update. A rollback
restores that complete feature set; it never sends only the feature being
removed.

## Public Runtime

The first release supports CN and CNY. A visitor does not sign in to ViceMe to
tip. The visitor is anonymous to the creator, but not anonymous to ViceMe or the
payment provider where payment processing, risk controls, or law require data.

`getConfig()` is authoritative for the current currency, available providers,
and amount bounds. The first release permits 100..20000 fen. The host must not
hard-code a broader range. A provider is optional in `open()`; scene is
platform-selected and is never supplied by the host.

Anonymous production tips do not use WeChat JSAPI because that legacy scene
requires a ViceMe User-linked OpenID. On ordinary mobile browsers the platform
may select H5, and on desktop it may select NATIVE. In the WeChat webview the
platform must return to creator controls with guidance to use an external
browser or another available provider; it must not create an order through the
old registered-user path.

The host may draw amount and provider controls. `open()` always ends at the
ViceMe read-only confirmation layer, which the host cannot customize. The host
must not call order REST endpoints or receive an order number, payment action,
or capability. It must not add arbitrary context fields to the request.

`open()` resolves only `PAID`, `CANCELLED`, or `UNKNOWN`. UNKNOWN is not a
failure: it means the SDK could not conclusively confirm the final state. The
host must not claim failure, retry a charge automatically, or implement
cross-refresh order recovery. Refresh starts a new host interaction.

Only `PAID` carries data: trusted `work.id` and `work.title`, `amountCents`, and
`currency: "CNY"`. `CANCELLED` and `UNKNOWN` carry only their status. No result
contains a provider receipt, order number, capability, or payment action.

## Official UI With Exact CDN ESM

The official UI uses `createViceMe` and `mountTip`; it does not use a global or
declarative loader. Start with the public SANDBOX Work key:

```js
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
```

Retain `mountHandle` for the lifetime of the owning instance. Invoke
`destroyViceMeTip()` from every real SPA, component, or route unmount so the
mount is destroyed before the client. Do not bind cleanup to `pagehide`, because
that event also fires when a document enters bfcache. A static document has no
in-document route unmount and needs no synthetic lifecycle hook.

## Headless With npm

Use the host project's existing package manager to install exact
`@viceme-ai/sdk@0.4.0`, then keep rendering in host code. First require this
official-registry preflight to return exactly `0.4.0`:

```bash
npm view @viceme-ai/sdk@0.4.0 version --json \
  --registry=https://registry.npmjs.org \
  --@viceme-ai:registry=https://registry.npmjs.org
```

If the version is unavailable, stop. Do not fall back to `latest`, a Git
dependency, copied source, or a package from a project-specific registry. Bind
the selected package manager's install to `https://registry.npmjs.org` for the
`@viceme-ai` scope and verify the lockfile's resolved package integrity against
the official registry response. The `renderTipControls` function below
represents the host's own UI and must use the returned config rather than
independent amount or provider constants. It must invoke the callback directly
from the button click or keyboard activation stack; do not await or schedule
other work before `tip.open()` creates the secure handoff.

```js
import { createViceMe } from "@viceme-ai/sdk";
import { createTip } from "@viceme-ai/sdk/tip";

const client = createViceMe({ workKey: "wrk_test_...", region: "cn" });
await client.ready();

const tip = createTip(client);
const config = await tip.getConfig();

renderTipControls(
  {
    currency: config.currency,
    minAmountCents: config.amount.minCents,
    maxAmountCents: config.amount.maxCents,
    stepAmountCents: config.amount.stepCents,
    providers: config.providers,
  },
  async ({ amountCents, provider }) => {
    const resultPromise = tip.open({
      amountCents,
      ...(provider ? { provider } : {}),
    });
    const result = await resultPromise;

    switch (result.status) {
      case "PAID":
        showPaid(result.work, result.amountCents, result.currency);
        break;
      case "CANCELLED":
        showCancelled();
        break;
      case "UNKNOWN":
        showUnknownWithoutClaimingFailure();
        break;
    }
  },
);

function destroyViceMeTip() {
  tip.destroy();
  client.destroy();
}
```

Call `destroyViceMeTip()` from the owning component or route's real unmount
lifecycle. Do not bind cleanup to `pagehide`; that event also fires when a page
enters the back/forward cache.

## Headless With Exact CDN ESM

Use both immutable files from the same reviewed SDK release. Do not replace the
version with `/v1`, `latest`, or another mutable alias. The first Headless
release is `0.4.0` and the first open-tip market is CN.

Before editing the host, complete the four-object CN/GLOBAL preflight in
"Release Selection Before Mutation" above. The host then imports the verified
CN pair below. If any object is unavailable, stop. Do not substitute the mutable
`/v1` alias or copy SDK source into the host. As with npm, the host control must
call `tip.open()` directly from its click or keyboard activation stack.

```js
import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.4.0/index.js";
import { createTip } from "https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js";

const client = createViceMe({ workKey: "wrk_test_...", region: "cn" });
await client.ready();

const tip = createTip(client);
const config = await tip.getConfig();

renderTipControls(
  {
    currency: config.currency,
    minAmountCents: config.amount.minCents,
    maxAmountCents: config.amount.maxCents,
    stepAmountCents: config.amount.stepCents,
    providers: config.providers,
  },
  async ({ amountCents, provider }) => {
    const resultPromise = tip.open({
      amountCents,
      ...(provider ? { provider } : {}),
    });
    const result = await resultPromise;

    switch (result.status) {
      case "PAID":
        showPaid(result.work, result.amountCents, result.currency);
        break;
      case "CANCELLED":
        showCancelled();
        break;
      case "UNKNOWN":
        showUnknownWithoutClaimingFailure();
        break;
    }
  },
);

function destroyViceMeTip() {
  tip.destroy();
  client.destroy();
}
```

Call `destroyViceMeTip()` only from the owning component or route's real
unmount lifecycle, not `pagehide`.

## Validation Ladder

1. Local Fake: use the official side-effect-free test adapter in component tests
   and Storybook. Exercise each public outcome; do not fake ViceMe internals or
   add a production-only testing switch.

   ```js
   import { createTestTip } from "@viceme-ai/sdk/tip/testing";

   const fakeTip = createTestTip({
     config: {
       work: { id: "00000000-0000-4000-8000-000000000001", title: "Demo" },
       workKey: "wrk_test_...",
       environment: "SANDBOX",
       currency: "CNY",
       amount: { minCents: 100, maxCents: 20000, stepCents: 1 },
       providers: ["WECHAT_PAY", "ALIPAY"],
     },
     outcome: "UNKNOWN",
   });

   renderTipControls(await fakeTip.getConfig(), (selection) =>
     fakeTip.open(selection),
   );
   ```

2. SANDBOX key: run the real SDK for the selected CN Profile, then verify
   config-driven amounts/providers, the ViceMe-controlled handoff, responsive
   layout, keyboard access, and platform sandbox payment outcomes. No real
   funds move.
3. Production promotion: show the SANDBOX evidence and obtain explicit user
   confirmation before replacing `wrk_test_...` with `wrk_live_...`.
4. Production boundary: a PRODUCTION key cannot simulate payment. Treat an
   exercised real payment as a separate, explicit user decision and report when
   it was not tested.
