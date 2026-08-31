# ViceMe 开放赞赏接入契约

## 资源边界

一次开放赞赏接入只面向一个由 Merchant 拥有、符合公开可见性且已经发布的 Work，Work kind 不受宿主页类型限制：

```text
PUBLISHED Merchant Work
└── ACTIVE SDK access（hosted features 包含 tip）
    ├── keys.test = wrk_test_...
    └── keys.live = wrk_live_...
```

宿主页与被赞赏 Work 是独立资源。宿主展示 Tip UI 不会把页面变成 Website Work，Standalone Tip 不验证或声明宿主页所有权。只有用户独立选择“网站本身”作为 Work 时，Website 发布才要求 DNS 所有权验证。

两个 Work key 是永久公开标识，不是凭据。create 一次签发 `keys.test` 与 `keys.live`；update、disable、重新启用和回滚都不得轮换。更新时必须从精确 `configVersion` 出发，完整保留 hosted features 与 `accessFeatures`。只停 Tip 时从完整 hosted feature set 移除 `tip`；只有所有 hosted 和 access 能力都应停止时才 disable。

可选 Commerce Application 只提供可信来源归因，不是 Tip 门禁，默认流程不创建它。ViceMe 只记录浏览器观察到的规范 Origin，不记录完整 URL、路径、查询、片段或 campaign 数据。没有匹配归因时只是未验证 Origin，不会拒绝公开赞赏。

## 写入前选择发布物

首个公开 Tip SDK 版本精确为 `@viceme-ai/sdk@0.4.0`。先请用户选择官方 UI 或 Headless，再验证所选发布物；这些检查必须早于 Work 创建/验证/发布、SDK access 变更和宿主页编辑。发布物不可用时，流程不应留下新的 Work 或 feature 状态。

每条路线都先检查两个发布区域的四个不可变 Tip 入口，避免接受只发布一部分的区域产物：

```bash
for asset_url in \
  https://s3.viceme.cn/viceme-sdk/0.4.0/index.js \
  https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js \
  https://s3.viceme.ai/viceme-sdk/0.4.0/index.js \
  https://s3.viceme.ai/viceme-sdk/0.4.0/tip.js
do
  http_code="$(curl --silent --show-error --connect-timeout 5 --max-time 15 --output /dev/null --write-out '%{http_code}' "$asset_url")" || exit 1
  test "$http_code" = "200" || exit 1
done
```

每个 CDN 请求都必须在 15 秒内直接返回精确 `200`，不得跟随或接受重定向。官方 UI 与 CDN Headless 使用已检查的 CN ESM。Headless 也可以使用精确 npm 包；npm 路线还要求以下官方注册表命令精确返回 `0.4.0`：

```bash
npm view @viceme-ai/sdk@0.4.0 version --json \
  --fetch-timeout=15000 --fetch-retries=0 \
  --registry=https://registry.npmjs.org \
  --@viceme-ai:registry=https://registry.npmjs.org
```

版本不可用时立即停止。任一对象不可用时同样停止，不得改用 `latest`、可变版本别名、声明式 loader、浏览器全局对象、Git 依赖、项目私有镜像中的同名包或复制的 SDK 源码。服务端变更前保存完整旧配置；回滚必须恢复完整 hosted features 和 `accessFeatures`，不能只发送要移除的 feature。

## 公开配置与运行时

本版支持 CN 与 CNY。访客不需要登录 ViceMe 才能赞赏；对创作者保持匿名，但在支付处理、风控或法律要求下，不对 ViceMe 或支付渠道匿名。

SDK 以无 Cookie、无 Authorization 的 credentialless 请求读取公开配置：

```text
GET /v1/work-sdk/:workKey/tip-config
```

该请求必须使用 `credentials: "omit"`、`redirect: "error"`、`Accept: application/json` 和可取消的 `AbortSignal`。SDK 只接受 8 秒内由配置的官方 Origin 直接返回的精确 `200` 与 `application/json`，响应体最多 16 KiB；重定向、跨 Origin 最终 URL、超时、销毁、超限、非 JSON、未知字段和不严格 schema 都必须取消读取并拒绝。宿主销毁 controller 时必须中止未完成请求。API 收到 Cookie 或 Authorization 时返回 `TIP_CONFIG_CREDENTIALS_NOT_ALLOWED`，不得忽略凭据后继续返回配置。

`getConfig()` 是当前币种、可用渠道和金额边界的权威来源。首版金额为 100..20000 fen，步长 1 分；宿主不得写死更宽范围。`open()` 中 provider 可省略，scene 由平台选择，宿主不得传 scene 或任意上下文字段。

匿名生产赞赏不使用 WeChat JSAPI，因为旧场景要求绑定 ViceMe User 的 OpenID。普通移动浏览器可由平台选择 H5，桌面可选择 NATIVE；微信 WebView 必须回到宿主控制并提示使用外部浏览器或其他可用渠道，不得退回旧注册用户支付链路。

宿主可以绘制金额与 provider 控件，但 `open()` 最终总会进入 ViceMe 的 read-only confirmation layer，宿主不能替换、重绘或补充支付结论。宿主不得调用订单 REST API，也不得接收订单号、支付动作或 capability。

`open()` 只解析为 `PAID`、`CANCELLED` 或 `UNKNOWN`：

- `PAID` 是唯一携带业务数据的结果：可信 `work.id`、`work.title`、`amountCents` 与 `currency: "CNY"`。
- 只有用户在官方确认阶段、尚未尝试建单前明确取消，才返回只含 status 的 `CANCELLED`。
- 已建单、可能已建单、订单关闭、窗口被拦截、异常关闭或协议无法确认时返回只含 status 的 `UNKNOWN`。`UNKNOWN` 不代表失败；不得声称扣款失败、自动重试收费或实现跨刷新订单恢复。同一 channel 后续确认权威支付成功时仍可收到 `PAID`。

公开结果不包含 provider receipt、`orderNo`、capability、handoff token、结果 token 或 `PaymentAction`。

## `sourceOrigin` 与安全 handoff

`sourceOrigin` 只能由官方 Web 服务端从浏览器 Referer 解析并规范化，不能来自宿主、SDK 或 URL query。规范值是小写 HTTP/HTTPS Origin，不含凭证、路径、查询、片段、尾部斜杠或显式默认端口。ViceMe 记录来源用于审计与可选归因，不用它要求宿主页必须与 Work 同域。

宿主若发送 `Referrer-Policy: no-referrer`，或浏览器没有提供有效 Referer，官方页面必须 fail closed 并显示 handoff 不可用；绝不读取 query、locale、Commerce Application 或调用方消息作为来源 fallback。宿主应保留浏览器能够向官方 Origin 发送来源 Origin 的 referrer policy，不得为了“修复”接入伪造 `sourceOrigin`。

Headless SDK 在每次用户手势触发的 `open()` 中创建高熵随机 channel，并直接挂载官方 `/widget/tip/:workKey?mode=headless` 窗口。`mode=headless` 与该次随机 channel 必须由 SDK 写入官方 URL，宿主不得自建或改写。协议事件名固定为 `viceme:tip-headless-ready`、`viceme:tip-headless-init` 和 `viceme:tip-headless-result`，不得改名或增加宿主私有结果事件。SDK 只接受：

- 所选 Profile 的精确官方 Web Origin；
- 该次调用自己打开的 `window`；
- 匹配本次随机 channel 与 workKey 的严格消息 schema。

官方页面只接受服务端 Referer 得到的 `sourceOrigin`、直接 `opener`/`parent`、匹配 channel/workKey 的严格 init schema。双方收发消息都必须同时验证 `event.origin`、`event.source`、channel、workKey 与消息形状，`postMessage` 必须使用精确目标 Origin，不能用 `*`。

外部宿主不直接接收签名 handoff 或订单 capability；创建、状态和取消都只由官方页面发起。生产建单的短期签名上下文绑定 Work key 环境与 `sourceOrigin`，过期时只能由官方同源页面在固定续期窗口内更新；API 始终拒绝过期或环境不匹配的上下文。

## 官方 UI：精确 CDN ESM

官方 UI 使用 `createViceMe` 与 `mountTip`，不使用全局对象或声明式 loader。先使用公开 SANDBOX key：

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

在所属实例的整个生命周期保留 `mountHandle`。每次 SPA、组件或路由真实卸载都调用 `destroyViceMeTip()`，确保 mount 先于 client 销毁。不要把清理绑定到 `pagehide`，因为文档进入 bfcache 时也会触发；静态文档没有页面内卸载，不需要虚构生命周期。

## Headless：npm

用宿主项目既有包管理器精确安装 `@viceme-ai/sdk@0.4.0`。先要求官方注册表检查返回精确版本；不可用时停止。为 `@viceme-ai` scope 固定 `https://registry.npmjs.org`，并核对 lockfile 的来源与 integrity。

`renderTipControls` 代表宿主自己的 UI，必须完全使用返回配置，不能维护独立金额或 provider 常量。控件必须直接在按钮点击或键盘激活的调用栈中调用回调；在 `tip.open()` 创建安全 handoff 前不得 await、排队或调度其他工作。

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

从组件或路由的真实卸载生命周期调用 `destroyViceMeTip()`。不要绑定 `pagehide`。

## Headless：精确 CDN ESM

同一审查版本的两个不可变文件必须配对使用。编辑宿主前完成上面的四对象 CN/GLOBAL 预检；任一对象不可用就停止。宿主控制也必须从点击或键盘激活调用栈直接调用 `tip.open()`。

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

只在所属组件或路由真实卸载时调用 `destroyViceMeTip()`，不要使用 `pagehide`。

## 验证阶梯

1. Local Fake：组件测试与 Storybook 使用官方无副作用测试适配器，覆盖所有公开结果；不得伪造 ViceMe 内部协议或增加生产专用测试开关。

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

2. SANDBOX key：使用所选 CN Profile 的真实 SDK，验证配置驱动的金额/provider、ViceMe-controlled handoff、Referer 来源、响应式、键盘与平台模拟结果；不移动真实资金。
3. Production promotion：展示 SANDBOX 证据并取得用户明确确认后，才把 `wrk_test_...` 换为 `wrk_live_...`。
4. 生产边界：PRODUCTION key 不能模拟支付。真实支付是独立且明确的用户决定；未执行时必须报告。

## CSP 与线上验收

保留现有 CSP 和 nonce，只加入浏览器证明确实需要的精确 SDK CDN 与官方 Web Origin。不得加入 `*`、宽泛 ViceMe 子域、`unsafe-eval` 或宿主自提供的支付脚本。referrer policy 必须允许官方页面收到来源 Origin；`no-referrer` 会按安全契约关闭 handoff，不能通过放宽消息校验补救。

最终在公开 HTTPS 页面验证：SDK 文件返回 200，控件可见且键盘可达，来源缺失时正确 fail closed，正常来源能打开预期 Work 的官方确认，Escape/关闭路径符合公开结果语义，无 CSP、frame、script 或 handoff 错误。打开 UI 不代表支付已结算，必须分别报告 SANDBOX、生产建单和真实到账的验证边界。
