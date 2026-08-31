# 接入参考

## 选择接入路径

不要强迫每个网站先做独立设计阶段。用户已明确功能、入口、预期行为和价格时，核对相关代码后直接实施，只分析缺少部分。完全没有方案时，查看网站主要路径、已有登录或支付代码、真实界面、组件库、设计变量、响应式状态和准确业务动作，再提出候选方案。

方案应说明每个 feature key、标题、规则、现有界面入口、保持不变的受保护动作、复用的宿主组件与变体、影响文件、保护强度和仍需用户决定的价格。用户选择前不得写服务端配置或宿主代码；核心动作没有安全外层调用点时停止，不重构核心动作制造接缝。

面向用户的文字跟随用户当前语言；机器字段与 CLI 参数保持原样。

## Work SDK access 配置

先从权威服务端选择当前用户以 OWNER 身份拥有的 `PUBLISHED` Merchant Work，再读取其统一 SDK access。不要从本地文件推断 Work 或配置：

```bash
viceme --profile <profile> merchant work sdk-access get <work-id> \
  --merchant <merchant-id>
```

资源不存在时，可以一次创建多个访问功能：

```bash
viceme --profile <profile> merchant work sdk-access create <work-id> \
  --merchant <merchant-id> \
  --follow "dingdong=叮咚鸡" \
  --purchase "emperor=帝皇" --price-minor 1000 \
  --purchase "emperor-pro=帝皇 Pro" --price-minor 2000
```

资源已存在时，先保存返回的两个 key、hosted `features`、完整 `accessFeatures` 和 `configVersion`，再替换全部访问功能：

```bash
viceme --profile <profile> merchant work sdk-access update <work-id> \
  --merchant <merchant-id> \
  --expected-config-version <config-version> \
  --follow "dingdong=叮咚鸡" \
  --purchase "emperor=帝皇" --price-minor 1000 \
  --purchase "emperor-pro=帝皇 Pro" --price-minor 2000
```

update 没有 `--feature` 或 `--clear-hosted`，因此必须保留原有 `danmaku`/`tip` hosted features。它会完整替换 `accessFeatures`，不是增量追加。写后重新 get；正确响应形状至少包含：

```json
{
  "workId": "00000000-0000-4000-8000-000000000001",
  "keys": {
    "test": "wrk_test_...",
    "live": "wrk_live_..."
  },
  "status": "ACTIVE",
  "configVersion": 2,
  "features": ["danmaku", "tip"],
  "accessFeatures": [
    {
      "featureKey": "dingdong",
      "title": "叮咚鸡",
      "policyType": "FOLLOW_OWNER",
      "price": null,
      "status": "ACTIVE"
    },
    {
      "featureKey": "emperor",
      "title": "帝皇",
      "policyType": "WORK_ENTITLEMENT",
      "price": {
        "currency": "CNY",
        "amountCents": 1000
      },
      "status": "ACTIVE"
    }
  ]
}
```

`features` 示例只表示预先存在的 hosted 能力，不要求访问接入必须启用它们。写后必须与写前逐项相同；`keys.test`/`keys.live` 也必须保持不变，只有 `configVersion` 增加。

每个 `WORK_ENTITLEMENT` feature 都有独立且大于零的 CNY 分价。多个 purchase 共用一个价格时传一次 `--price-minor`；价格不同时按 purchase 顺序重复。

支持的规则：

- `FOLLOW_OWNER`：当前用户关注作品作者。
- `WORK_ENTITLEMENT`：当前用户拥有该定价 feature 的有效权益。

## 浏览器 SDK

部署页面使用响应中的 `keys.live`；`keys.test` 留给隔离测试。不要把 Work UUID 或 Product ID 放入客户端：

```ts
import { createViceMe } from "@viceme-ai/sdk";

const viceme = createViceMe({
  workKey: "wrk_live_...",
  region: "cn",
});

await viceme.ready();

const features = await viceme.access.getFeatures();
const emperor = features.find((feature) => feature.featureKey === "emperor");

const decisions = await viceme.access.checkMany(["dingdong", "emperor"]);
setDingdongUnlocked(decisions.dingdong.allowed);
setEmperorUnlocked(decisions.emperor.allowed);
```

入口需要显示名称或价格时，用宿主已有 Button/Card 与价格格式器渲染 `emperor.title`、`emperor.price`，不得从旧本地配置写死。保留现有字体、颜色、圆角、间距、焦点、加载、响应式和错误反馈；宿主展示不用于定制 ViceMe 访问层。

从用户点击处理器调用门控，原动作保持不变：

```ts
async function handleEmperorClick() {
  const decision = await viceme.access.require("emperor");
  if (!decision.allowed) return;
  await runEmperor();
}
```

已有权限时门控静默返回；否则打开所需的页面内登录、关注或结账界面。只有后续新的 `access.check()` 服务端结果能授予权限。

SDK 使用 `<viceme-access-layer>`：移动端为底部面板，桌面端为页面内层，使用 Shadow DOM 和 ViceMe 自有样式。宿主不提供自定义 presenter，也不检测或改写内部样式。

登录和结账留在同一层。用户从原门控点击后直接加载必要流程，不增加宿主自制支付确认。结账 frame 使用短期启动码换取内存 session，不依赖第三方 cookie；任何 `PENDING` 状态或消息都不能解锁。

## 错误处理

只根据 `ViceMeError.code` 分支：

- `SESSION_EXPIRED`：请用户重试或重新登录。
- `AUTH_CANCELLED`：用户取消或当前动作已过期。
- `CAPABILITY_DISABLED`：重新读取 Work SDK access，检查状态与完整 feature 配置。
- `CHECKOUT_UNAVAILABLE`：确认一次性 purchase feature 已启用且价格大于零。

销毁页面实例时始终调用 `destroy()`。
