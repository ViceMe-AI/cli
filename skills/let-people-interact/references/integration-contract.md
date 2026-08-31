# ViceMe 互动接入契约

## 三种资源边界

仅弹幕：

```text
PUBLISHED + VERIFIED Website Work（canonical Origin 精确匹配部署 Origin）
└── ACTIVE SDK access
    ├── keys.test / keys.live
    └── hosted features 包含 danmaku
```

仅赞赏：

```text
公开 PUBLISHED Merchant Work（任意 kind）
└── ACTIVE SDK access
    ├── keys.test / keys.live
    └── hosted features 包含 tip
```

弹幕加赞赏：

```text
PUBLISHED + VERIFIED Website Work（Website 门禁只由 Danmaku 要求）
└── ACTIVE SDK access
    ├── keys.test / keys.live
    └── hosted features 包含 danmaku,tip
```

宿主页与被赞赏 Work 是独立资源。Standalone Tip 不声明或验证宿主页所有权，不要求 Work 是 Website，不要求同域，也不要求 Commerce Application。可选 Commerce Application 只提供来源归因；没有匹配归因时，来源是 unverified Origin，而不是被拒绝的 Origin。组合中的 Tip 同样不创建、更新、暂停或激活 `WEBSITE_WIDGET`；Website follow/paid access 的应用与 `HOSTED_CHECKOUT` 由拥有平台资源的发布流程和 Shop 管理。

两个 Work key 是永久公开标识，不是凭据。create 一次签发 `keys.test` 与 `keys.live`；update、disable、重新启用和恢复不得轮换。更新 hosted features 时必须使用精确 `configVersion`，发送完整 hosted feature set，并原样保留完整 `accessFeatures`。

## 发布物预检

`@viceme-ai/sdk@0.5.0` 是本合同的精确 SDK 目标。任意 Tip 路线先选官方 Mounted 或 Headless；Headless 再选 npm 或 CDN ESM。选择必须早于 Work、Website verification、SDK access 或宿主页写入。

每条 Tip 路线都先检查两个发布区域的四个不可变入口：

```bash
for asset_url in \
  https://s3.viceme.cn/viceme-sdk/0.5.0/index.js \
  https://s3.viceme.cn/viceme-sdk/0.5.0/tip.js \
  https://s3.viceme.ai/viceme-sdk/0.5.0/index.js \
  https://s3.viceme.ai/viceme-sdk/0.5.0/tip.js
do
  http_code="$(curl --silent --show-error --connect-timeout 5 --max-time 15 --output /dev/null --write-out '%{http_code}' "$asset_url")" || exit 1
  test "$http_code" = "200" || exit 1
done
```

组合路线还检查精确 CN Danmaku 入口：

```bash
asset_url="https://s3.viceme.cn/viceme-sdk/0.5.0/danmaku.js"
http_code="$(curl --silent --show-error --connect-timeout 5 --max-time 15 --output /dev/null --write-out '%{http_code}' "$asset_url")" || exit 1
test "$http_code" = "200" || exit 1
```

每个 CDN 请求必须直接返回精确 `200`；命令不使用 `--location`，不得跟随或接受重定向。Headless npm 路线还要求官方 registry 精确返回 `0.5.0`：

```bash
npm view @viceme-ai/sdk@0.5.0 version --json \
  --fetch-timeout=15000 --fetch-retries=0 \
  --registry=https://registry.npmjs.org \
  --@viceme-ai:registry=https://registry.npmjs.org
```

任一必需对象不可用时立即停止。不得改用 `latest`、alias、浏览器全局对象、Git 依赖、私有镜像同名包或复制源码。

仅弹幕不受 CN/CNY 限制。根据当前 Profile 选择 `https://s3.viceme.cn` 或 `https://s3.viceme.ai`，并在 SDK access 或页面写入前对所选区域的精确 `0.5.0/index.js` 与 `0.5.0/danmaku.js` 执行相同的 5 秒连接、15 秒总时限、无 redirect、精确 `200` 检查。

## Tip 公开配置

本版 Tip 只支持所选 Profile 的 `marketRegion: cn` 与 CNY。页面 locale 不选择市场。访客不需要登录 ViceMe 才能赞赏；对创作者可以匿名，但在支付处理、风控或法律要求下不对 ViceMe 或支付渠道匿名。

SDK 以无 Cookie、无 Authorization 的 credentialless 请求读取公开配置：

```text
GET /v1/work-sdk/:workKey/tip-config
```

请求必须使用 `credentials: "omit"`、`redirect: "error"`、`Accept: application/json` 和可取消的 `AbortSignal`。SDK 只接受 8 秒内由配置的精确官方 Origin 直接返回的精确 `200` 与 `application/json`，响应体最多 16 KiB；重定向、跨 Origin 最终 URL、超时、销毁、超限、非 JSON、未知字段和不严格 schema 都必须取消读取并拒绝。controller 销毁时必须中止未完成请求。API 收到 Cookie 或 Authorization 时返回 `TIP_CONFIG_CREDENTIALS_NOT_ALLOWED`，不得忽略凭据后继续返回配置。

`getConfig()` 是币种、可用 provider 和金额边界的权威来源。首版金额为 100..20000 fen，步长 1 分；宿主不得写死更宽范围。`open()` 的 provider 可省略，scene 由平台选择，宿主不得传 scene 或任意上下文字段。

匿名生产赞赏不使用 WeChat JSAPI。普通移动浏览器可由平台选择 H5，桌面可选择 NATIVE；微信 WebView 必须回到宿主控制并提示使用外部浏览器或其他可用渠道，不得退回旧注册用户支付链路。

宿主可以绘制金额与 provider 控件，但 `open()` 最终进入 ViceMe 的只读确认层。宿主不能替换、重绘或补充支付结论，也不得调用订单 REST API。

`open()` 只解析为 `PAID`、`CANCELLED` 或 `UNKNOWN`：

- `PAID` 是唯一携带业务数据的结果：可信 `work.id`、`work.title`、`amountCents` 与 `currency: "CNY"`。
- 只有用户在官方确认阶段且尚未尝试建单前明确取消，才返回只含 status 的 `CANCELLED`。
- 已建单、可能已建单、订单关闭、窗口被拦截、异常关闭或协议无法确认时返回只含 status 的 `UNKNOWN`。`UNKNOWN` 不代表失败，不得声称扣款失败、自动重试收费或实现跨刷新订单恢复。

公开结果不包含 provider receipt、`orderNo`、capability、handoff token、结果 token 或 `PaymentAction`。

## `sourceOrigin` 与 Headless handoff

`sourceOrigin` 只能由官方 Web 服务端从浏览器 Referer 解析并规范化，不能来自宿主、SDK、URL query、init 消息或其他 fallback。规范值是小写 HTTP/HTTPS Origin，不含凭据、路径、查询、片段、尾部斜杠或显式默认端口；它用于审计与可选归因，不要求与 Work 同域。

宿主页使用 `Referrer-Policy: no-referrer`，或浏览器没有提供有效 Referer 时，官方页面必须 fail closed 并显示 handoff 不可用。不得通过 query、locale、Commerce Application 或调用方消息伪造来源。

Headless SDK 每次 `open()` 都生成高熵随机 channel，并直接打开官方 `/widget/tip/:workKey?mode=headless`。事件名固定为 `viceme:tip-headless-ready`、`viceme:tip-headless-init`、`viceme:tip-headless-result`。SDK 只接受所选 Profile 的精确官方 Origin、自己直接打开的 `window`、匹配 channel/workKey 的严格 schema；官方页面只接受 Referer 对应的 `sourceOrigin`、直接 `opener`/`parent`、匹配 channel/workKey 的严格 init schema。

双方必须同时校验 `event.origin`、`event.source`、channel、workKey 与消息形状；`postMessage` 使用精确目标 Origin，不能使用 `*`。外部宿主不接收 handoff token、订单 capability、`orderNo` 或 `PaymentAction`，也不调用建单、状态、取消或支付渠道 API。

## 仅弹幕：精确 ESM

下面以 CN 为例；GLOBAL Profile 必须把两个 import host 一起替换为 `https://s3.viceme.ai`，并把 `region` 改为 `global`。不能混用两个区域或不同版本。

```html
<div id="viceme-danmaku"></div>
<script type="module">
  import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.5.0/index.js";
  import { mountDanmaku } from "https://s3.viceme.cn/viceme-sdk/0.5.0/danmaku.js";

  const target = document.querySelector("#viceme-danmaku");
  if (!target) throw new Error("ViceMe Danmaku target is missing");

  const client = createViceMe({ workKey: "wrk_test_...", region: "cn" });
  await client.ready();
  const danmakuHandle = await mountDanmaku(client, {
    target,
    theme: "auto",
  });

  function destroyViceMeDanmaku() {
    danmakuHandle.destroy();
    client.destroy();
  }
</script>
```

只在 SPA、组件或路由真实卸载时调用销毁函数，不要绑定 `pagehide`。Danmaku 的 Work 必须持续满足 `PUBLISHED + VERIFIED Website Work` 和精确 canonical Origin。

## 仅赞赏：官方 Mounted UI

官方 Mounted UI 使用 `createViceMe` 与 `mountTip`。先使用公开 SANDBOX key：

```html
<div id="viceme-tip"></div>
<script type="module">
  import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.5.0/index.js";
  import { mountTip } from "https://s3.viceme.cn/viceme-sdk/0.5.0/tip.js";

  const target = document.querySelector("#viceme-tip");
  if (!target) throw new Error("ViceMe Tip target is missing");

  const client = createViceMe({ workKey: "wrk_test_...", region: "cn" });
  await client.ready();
  const tipHandle = await mountTip(client, {
    target,
    theme: "auto",
  });

  function destroyViceMeTip() {
    tipHandle.destroy();
    client.destroy();
  }
</script>
```

在所属实例整个生命周期保留 handle。真实卸载时先运行 `tipHandle.destroy()`，再运行 `client.destroy()`。静态文档没有页面内卸载，不需要虚构生命周期。

## 组合：官方 Mounted UI

组合只创建一个 client。两个 mount 独立结算，一个失败不能删除另一个已成功能力：

```html
<div id="viceme-danmaku"></div>
<div id="viceme-tip"></div>
<script type="module">
  import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.5.0/index.js";
  import { mountDanmaku } from "https://s3.viceme.cn/viceme-sdk/0.5.0/danmaku.js";
  import { mountTip } from "https://s3.viceme.cn/viceme-sdk/0.5.0/tip.js";

  const danmakuTarget = document.querySelector("#viceme-danmaku");
  const tipTarget = document.querySelector("#viceme-tip");
  if (!danmakuTarget || !tipTarget) {
    throw new Error("ViceMe engagement target is missing");
  }

  const client = createViceMe({ workKey: "wrk_test_...", region: "cn" });
  await client.ready();
  const mountResults = await Promise.allSettled([
    mountDanmaku(client, { target: danmakuTarget, theme: "auto" }),
    mountTip(client, { target: tipTarget, theme: "auto" }),
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

生产实现应分别呈现 mount 失败状态，同时保留成功能力。不要因为 Tip 不可用而销毁 Danmaku，也不要因为 Danmaku 不可用而销毁 Tip。

## 仅赞赏：Headless npm

用项目既有包管理器精确安装 `@viceme-ai/sdk@0.5.0`，固定 `@viceme-ai` scope 到官方 registry，并核对 lockfile 的来源与 integrity。`renderTipControls` 代表宿主 UI，必须完全使用服务端配置；点击或键盘激活的调用栈必须直接调用 `tip.open()`，在创建窗口前不能 await、排队或调度其他工作。

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
      locale: "zh-CN",
      appearance: "light",
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

## 仅赞赏：Headless CDN ESM

同一审查版本的 `index.js` 与 `tip.js` 必须配对使用：

```js
import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.5.0/index.js";
import { createTip } from "https://s3.viceme.cn/viceme-sdk/0.5.0/tip.js";

const client = createViceMe({ workKey: "wrk_test_...", region: "cn" });
await client.ready();

const tip = createTip(client);
const config = await tip.getConfig();

renderTipControls(config, async ({ amountCents, provider }) => {
  const resultPromise = tip.open({
    amountCents,
    ...(provider ? { provider } : {}),
  });
  const result = await resultPromise;

  if (result.status === "PAID") {
    showPaid(result.work, result.amountCents, result.currency);
  } else if (result.status === "CANCELLED") {
    showCancelled();
  } else if (result.status === "UNKNOWN") {
    showUnknownWithoutClaimingFailure();
  }
});

function destroyViceMeTip() {
  tip.destroy();
  client.destroy();
}
```

## 组合：Danmaku Mounted + Tip Headless

组合 Headless 路线仍只创建一个 client，只挂载 Danmaku，不再调用 `mountTip`：

```html
<div id="viceme-danmaku"></div>
<div id="host-tip-controls"></div>
<script type="module">
  import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.5.0/index.js";
  import { mountDanmaku } from "https://s3.viceme.cn/viceme-sdk/0.5.0/danmaku.js";
  import { createTip } from "https://s3.viceme.cn/viceme-sdk/0.5.0/tip.js";

  const danmakuTarget = document.querySelector("#viceme-danmaku");
  if (!danmakuTarget) throw new Error("ViceMe Danmaku target is missing");

  const client = createViceMe({ workKey: "wrk_test_...", region: "cn" });
  await client.ready();
  const danmakuHandle = await mountDanmaku(client, {
    target: danmakuTarget,
    theme: "auto",
  });

  const tip = createTip(client);
  const config = await tip.getConfig();
  renderTipControls(config, async ({ amountCents, provider }) => {
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
  });

  function destroyViceMeEngagement() {
    tip.destroy();
    danmakuHandle.destroy();
    client.destroy();
  }
</script>
```

如果 `getConfig()` 或 Tip handoff 失败，Danmaku handle 仍由自身生命周期管理；只有所属组件真实卸载时才一起销毁。

## Local Fake、SANDBOX 与生产切换

1. Local Fake：组件测试和 Storybook 使用官方无副作用适配器覆盖全部公开结果，不伪造内部消息协议，也不增加生产测试开关。

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

2. SANDBOX：使用所选 CN Profile 的真实 `keys.test`，验证配置驱动的金额/provider、官方确认层、Referer 来源、响应式、键盘、弹窗阻止和平台模拟结果；不移动真实资金。
3. Production promotion：向用户展示 SANDBOX 证据并取得明确确认后，才把唯一 `createViceMe` 的 `wrk_test_...` 替换为同一 SDK access 返回的 `wrk_live_...`。imports、版本、UI 路线与 Work 不变。
4. 生产边界：`keys.live` 不能模拟支付。真实支付是独立且明确的用户决定；未执行时必须报告。

## CSP、生命周期与迁移

保留现有 CSP 与 nonce，只加入浏览器证明确实需要的精确 SDK CDN 和官方 Web Origin。不得加入 `*`、宽泛 ViceMe 子域、`unsafe-eval` 或宿主支付脚本。referrer policy 必须允许官方页面收到来源 Origin；`no-referrer` 会按安全合同关闭 handoff，不能通过放宽消息校验补救。

所有 mount/controller 都属于创建它们的 client。SPA、组件或路由真实卸载时先销毁 mount/controller，再销毁 client。不要绑定 `pagehide`，因为进入 bfcache 也会触发。静态文档没有页面内卸载，不需要虚构卸载事件。

旧声明式 loader、旧 embed 标签与 `data-viceme-*` 只能用于确认待迁移 Work 身份。新接入使用上面的精确 ESM。迁移必须先确认 Work 属于当前 OWNER Merchant；替换完成后删除旧标签，不能让两套运行时并行。

## 线上验收

- 仅弹幕：精确 Website Origin、DNS verified 状态、消息持久化、桌面/320px、键盘与减少动画。
- 仅赞赏：不执行 Website verification 或 Commerce Application mutation；验证目标 Work、Local Fake、SANDBOX、来源缺失 fail closed、三种公开结果和销毁顺序。
- 组合：同一 verified Website Work、同一 client、完整 `danmaku,tip` hosted set、完整 `accessFeatures` 未变，以及一个能力失败不移除另一个。
- 所有路线：SDK 文件直接 200、没有新旧 loader 并存、无 CSP/frame/script/handoff 错误。打开 UI 不代表消息持久化或支付结算成功。
