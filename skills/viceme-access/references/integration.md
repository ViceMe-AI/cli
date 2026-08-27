# 接入参考

## 选择接入路径

不要强迫每个网站先做独立设计阶段。用户已经明确功能、入口、预期行为和价格时，核对相关代码后直接实施，只分析缺少部分。完全没有方案时，先查看网站主要用户路径、已有登录或支付代码、真实界面、组件库、设计变量、响应式状态和准确业务动作，再提出候选方案。

方案应说明每个功能的 key 和标题、规则、现有界面入口、保持不变的受保护动作、要复用的宿主组件与变体、影响文件、保护强度，以及仍需用户决定的价格。用户选择前不得应用配置或编辑宿主代码。核心动作没有安全外层调用点时，说明边界，不得重构核心动作来制造接缝。

## 访问配置

访问配置要求网站已有明确发布绑定。`.viceme/website.json` 缺失或没有 `workKey` 时，停止并先使用 `$viceme-publish`。完成网站资料预览、取得发布确认并发布网站，成功后再返回。 `viceme access init` 本身绝不发布网站。

随后用一条命令创建并应用常见访问配置：

```bash
viceme access init --website . --name "Dagou Tap" \
  --follow "dingdong=叮咚鸡" \
  --purchase "emperor=帝皇" --price-minor 1000 \
  --purchase "emperor-pro=帝皇 Pro" --price-minor 2000
```

命令写入以下 YAML、启用配置并返回权威 Work 响应。只有后续修改才需要手工编辑 YAML 并运行 `viceme access apply`。

```yaml
schemaVersion: 2
apiBaseUrl: https://api.viceme.cn
webBaseUrl: https://viceme.cn
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

每个 `WORK_ENTITLEMENT` 功能都有独立且大于零的 `priceCents`，一个网站可以有多个独立定价功能。多个 `--purchase` 共用一个价格时传一次 `--price-minor`；价格不同时按购买功能顺序重复该参数。

支持的规则：

- `FOLLOW_OWNER`：当前用户关注作品作者。
- `WORK_ENTITLEMENT`：当前用户拥有该定价功能的有效权益。

## 浏览器 SDK

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

入口需要显示这些值时，使用宿主已有 Button/Card 和价格格式器渲染 `emperor.title`、`emperor.price`，不得从 `.viceme/access.yaml` 写死。保留现有字体、颜色、圆角、间距、焦点、加载、响应式和错误反馈。宿主展示不用于定制 ViceMe 访问层。

从用户点击处理器调用门控，并保持原动作不变：

```ts
async function handleEmperorClick() {
  const decision = await viceme.access.require("emperor");
  if (!decision.allowed) return;
  await runEmperor();
}
```

已有权限时门控静默返回，否则打开所需的页面内登录、关注或结账界面。

权限检查本身不执行业务动作，宿主也不得直接调用关注写操作。关注门控中，ViceMe 登录授权层显示创作者头像，名称和已发布作品数同一行并用 `·` 分隔；作品数包含 Skills 和网站，两类都有时最近封面最多两个且各取一个。描述显示前 50 个字符，悬停显示全文。唯一“授权”操作直接打开微信二维码。用户同意后即登录并自动关注，不再显示登录后关注层。

SDK 使用 `<viceme-access-layer>`：移动端为底部面板，桌面端为页面内层，使用 Shadow DOM 和 ViceMe 自有样式。交互合同稳定前，不提供自定义 presenter、宿主页面样式检测或自动样式适配。

登录和结账都留在同一层。用户从原门控点击后直接加载结账 frame，不增加“去购买”中间确认；选择支付方式并支付仍需明确确认。层后方宿主页面保持可见且不加遮罩。微信 frame 继承授权层高度，确保二维码和一键授权没有跳变；结账层加载期间保持稳定高度。SDK 校验 API 来源和每次动作的消息通道，然后重新检查权限。结账 frame 用短期、可重试的启动码换取内存 bearer session，不依赖第三方 cookie。`PENDING` 订单或消息都不能授予权限，只有新的服务端 `access.check()` 结果可以。

## 错误处理

只根据 `ViceMeError.code` 分支。常见错误：

- `SESSION_EXPIRED`：请用户重试或重新登录。
- `AUTH_CANCELLED`：用户取消，或同标签页继续流程已过期。
- `CAPABILITY_DISABLED`：检查 Work 配置。
- `CHECKOUT_UNAVAILABLE`：确认单次报价已启用且当前价格大于零。

销毁时始终调用 `destroy()`。
