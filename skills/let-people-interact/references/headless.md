# Headless Tip 接入

本参考只在用户明确选择自定义 Tip UI 后读取。Danmaku 没有 Headless 路线；组合仍挂载 Danmaku，并在同一个 client 上创建 Headless Tip controller。

宿主只使用 `createViceMe` 与 `createTip(client).getConfig()/.open()/.destroy()` 公共 facade。不得下载或反向分析 SDK，不得自己调用订单、支付或内部 handoff API，也不得复刻官方确认层。

## 公开安全合同

SDK 以无 Cookie、无 Authorization 的 credentialless 请求读取：

```text
GET /v1/work-sdk/:workKey/tip-config
```

请求使用 `credentials: "omit"`、`redirect: "error"`、`Accept: application/json` 和可取消的 `AbortSignal`。SDK 只接受 8 秒内由所选精确官方 Origin 直接返回的精确 `200` 与 `application/json`，响应体最多 16 KiB；重定向、跨 Origin 最终 URL、超时、销毁、超限、非 JSON、未知字段和不严格 schema 都必须 fail closed。API 收到 Cookie 或 Authorization 时返回 `TIP_CONFIG_CREDENTIALS_NOT_ALLOWED`。

`getConfig()` 是 Work、CNY、可用 provider 和金额边界的权威来源；宿主不写死更宽范围。`open()` 的 provider 可省略，scene 由平台选择，宿主不传 scene 或任意上下文字段。匿名生产赞赏不使用 WeChat JSAPI；微信 WebView 无可用渠道时应回到宿主并提示使用外部浏览器。

宿主负责金额/provider 控件及其键盘、焦点、错误和响应式状态；`open()` 最终进入 ViceMe 只读确认层。只有点击或键盘激活的直接调用栈可以调用 `tip.open()`；打开窗口前不得 await、排队或调度其他工作，以免被浏览器拦截。

公开结果只有：

- `PAID`：唯一携带可信 `work.id`、`work.title`、`amountCents` 与 `currency: "CNY"` 的结果；
- `CANCELLED`：用户在官方确认阶段且尚未尝试建单前明确取消，只含 status；
- `UNKNOWN`：已建单、可能已建单、订单关闭、窗口被拦截、异常关闭或协议无法确认，只含 status。它不代表失败，宿主不得声称扣款失败、自动重试收费或实现跨刷新订单恢复。

公开结果不包含 provider receipt、`orderNo`、capability、handoff token、结果 token 或 `PaymentAction`。

## `sourceOrigin` 与 handoff

`sourceOrigin` 只能由官方 Web 服务端从浏览器 Referer 解析并规范化，不能来自宿主、SDK、URL query、init 消息或 fallback。规范值是小写 HTTP/HTTPS Origin，不含凭据、路径、查询、片段、尾部斜杠或显式默认端口；它只用于审计与可选归因，不要求与 Work 同域。

宿主页使用 `Referrer-Policy: no-referrer` 或浏览器没有提供有效 Referer 时，官方页面必须 fail closed 并显示 handoff 不可用。不得通过 query、locale、Commerce Application 或调用方消息伪造来源。

每次 `open()` 都使用高熵随机 channel，并直接打开官方 `/widget/tip/:workKey?mode=headless`。事件名固定为 `viceme:tip-headless-ready`、`viceme:tip-headless-init`、`viceme:tip-headless-result`。双方同时校验 `event.origin`、`event.source`、channel、workKey 与严格消息形状；`postMessage` 使用精确目标 Origin，不能使用 `*`。宿主不接收 token、订单 capability、`orderNo` 或 `PaymentAction`。

这些是 SDK 与官方 Web 的发布合同，不是每个宿主需要重新实现或探测的逻辑。宿主只通过下面的公开 facade 验收成功与失败结果。

## 精确安装或 import

### npm

使用项目既有包管理器 exact-save 安装预检已经固定的版本：

```bash
npm install --save-exact "@viceme-ai/sdk@<sdk_version>"
pnpm add --save-exact "@viceme-ai/sdk@<sdk_version>"
yarn add --exact "@viceme-ai/sdk@<sdk_version>"
```

只运行其中与项目一致的一条。核对依赖清单与 lockfile 都保存精确版本，来源与 integrity 正确；不得留下 `^`、`~`、tag、alias、Git 依赖或字面量 `<sdk_version>`。npm imports：

```js
import { createViceMe } from "@viceme-ai/sdk";
import { createTip } from "@viceme-ai/sdk/tip";
```

### CDN ESM

同一审查版本的 `index.js` 与 `tip.js` 必须配对，且只使用预检返回的 CN `sdk_origin`：

```js
import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/REPLACE_WITH_RESOLVED_SDK_VERSION/index.js";
import { createTip } from "https://s3.viceme.cn/viceme-sdk/REPLACE_WITH_RESOLVED_SDK_VERSION/tip.js";
```

写入前替换所有版本占位符。不得混用 npm/CDN、两个版本或两个区域。

## 仅赞赏实现

下面的 `renderTipControls` 与状态函数代表宿主现有 UI seam；只实现当前页面需要的最小控件，不另建 UI 框架：

```js
const client = createViceMe({
  workKey: "wrk_test_REPLACE_WITH_PUBLIC_TEST_KEY",
  region: "cn",
});
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

## 组合实现

组合只创建一个 client。Danmaku Mounted 与 Tip 初始化独立结算；一方失败时显示对应状态，但保留另一方的成功 handle/controller。只挂载 Danmaku，不再调用 `mountTip`：

- npm 路线另加 `import { mountDanmaku } from "@viceme-ai/sdk/danmaku";`；
- CDN ESM 路线另加与前两个 import 同源同版本的 `import { mountDanmaku } from "https://s3.viceme.cn/viceme-sdk/REPLACE_WITH_RESOLVED_SDK_VERSION/danmaku.js";`。

```js
const client = createViceMe({
  workKey: "wrk_test_REPLACE_WITH_PUBLIC_TEST_KEY",
  region: "cn",
});
await client.ready();

const danmakuTarget = document.querySelector("#viceme-danmaku");
const tip = createTip(client);
const danmakuMount = Promise.resolve().then(() => {
  if (!danmakuTarget) throw new Error("ViceMe Danmaku target is missing");
  return mountDanmaku(client, { target: danmakuTarget, theme: "auto" });
});
const [danmakuResult, tipConfigResult] = await Promise.allSettled([
  danmakuMount,
  tip.getConfig(),
]);

const danmakuHandle =
  danmakuResult.status === "fulfilled" ? danmakuResult.value : undefined;
if (tipConfigResult.status === "fulfilled") {
  renderTipControls(tipConfigResult.value, async ({ amountCents, provider }) => {
    const result = await tip.open({
      amountCents,
      ...(provider ? { provider } : {}),
    });
    if (result.status === "PAID") {
      showPaid(result.work, result.amountCents, result.currency);
    } else if (result.status === "CANCELLED") {
      showCancelled();
    } else if (result.status === "UNKNOWN") {
      showUnknownWithoutClaimingFailure();
    }
  });
} else {
  showTipUnavailable();
}
if (!danmakuHandle) showDanmakuUnavailable();

function destroyViceMeEngagement() {
  tip.destroy();
  danmakuHandle?.destroy();
  client.destroy();
}
```

## Local Fake 与 SANDBOX

组件测试和 Storybook 使用官方无副作用适配器，不伪造内部消息协议，也不增加生产测试开关：

```js
import { createTestTip } from "@viceme-ai/sdk/tip/testing";

const fakeTip = createTestTip({
  config: {
    work: { id: "00000000-0000-4000-8000-000000000001", title: "Demo" },
    workKey: "wrk_test_REPLACE_WITH_PUBLIC_TEST_KEY",
    environment: "SANDBOX",
    currency: "CNY",
    amount: { minCents: 100, maxCents: 20000, stepCents: 1 },
    providers: ["WECHAT_PAY", "ALIPAY"],
  },
  outcome: "UNKNOWN",
});

renderTipControls(await fakeTip.getConfig(), (selection) => fakeTip.open(selection));
```

Local Fake 覆盖配置失败以及 `PAID`、`CANCELLED`、`UNKNOWN`，但不当作线上协议证据。随后用真实 `keys.test` 在一个浏览器会话中批量验证：配置驱动的金额/provider、官方确认层、桌面、320px、键盘与焦点、弹窗阻止、Referer 缺失 fail closed、三种公开结果和销毁顺序。组合同时验证一个能力失败不移除另一个。

SPA、组件或路由真实卸载时先销毁 Tip controller 和 Danmaku handle，再调用 `client.destroy()`；静态页面不虚构卸载，也不绑定会在 bfcache 时触发的 `pagehide`。
