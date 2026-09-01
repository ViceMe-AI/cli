# 接入参考

## 发布流程交接

本 Skill 只接受拥有平台资源的发布流程已确认的结果：公开 `keys.test`、`keys.live`、关注或付费功能键，以及对应标题和服务端价格。SDK access API 不返回顶层单一 `workKey`；生产宿主把 `keys.live` 作为 SDK 的 `workKey`，`keys.test` 只用于隔离测试。创作者资格由父级流程调用 `$become-a-creator` 确认。

该发布流程必须已经在写后读取中确认两个 key 均存在且未轮换、hosted `danmaku`/`tip` features 完整保留、`accessFeatures` 符合用户确认值，并记录最新 `configVersion`。缺少任一平台配置时停止并说明需要先完成网站发布配置；不要创建本地访问配置，也不要运行登录、Merchant、创作者入驻或平台配置命令。

关注和付费规则由 Shop 服务端持有：

- `FOLLOW_OWNER`：访客登录后单独确认关注创作者。
- `WORK_ENTITLEMENT`：访客拥有对应 Product 的有效数字权益。

## 用户选择

用户已经明确接入目标时直接实施，不重复确认。只有存在多个合理候选且无法从上下文确定时，
才发起选择：例如多个已发布功能或多个可门控的宿主入口。先确认当前 Agent 平台及它实际暴露
的工具，不根据本 Skill 或参考资料假定运行在 WorkBuddy。当前平台明确为 WorkBuddy 且
`AskUserQuestion` 可用时，使用它展示可点击的单选卡片；其他平台使用其原生的等效交互工具。
不把选项埋在说明段落里，也不要求用户手打内部键值。

- 在 WorkBuddy 使用 `AskUserQuestion` 时，`question` 使用简短自然语言并以问号结尾，
  `header` 不超过 12 个字符。
- 每张卡片使用用户看得懂的名称，并用 `description` 说明入口位置或行为差异；内部功能键只在
  选择结果与代码之间映射。
- 一次只确认当前实施所需的决定。真正开放且无法列出候选的信息才用普通对话直接询问。
- 当前平台没有交互式提问工具时，改为编号短选项并提示用户回复编号，不要求输入完整名称或
  长句。

服务端已有的标题、价格、两个 SDK key 和功能键不向用户重复索取。发布配置缺失时按边界停止，
不得用交互问题引导用户在本 Skill 内创建或修改平台资源。

## 浏览器 SDK

安装 `@viceme-ai/sdk`，部署页面使用发布流程返回的 `keys.live`。不要把
`keys.test`、Work UUID 或 Product ID 放进生产客户端：

```ts
import { createViceMe } from "@viceme-ai/sdk";

const viceme = createViceMe({
  workKey: "wrk_live_...",
  region: "cn",
});

await viceme.ready();

const features = await viceme.access.getFeatures();
const memberFeature = features.find(
  (feature) => feature.featureKey === "member-content",
);

if (!memberFeature) {
  throw new Error("ViceMe access feature is unavailable");
}

renderTitle(memberFeature.title);
renderPrice(memberFeature.price);
```

入口需要显示标题或价格时，使用宿主已有组件和价格格式器渲染服务端返回值。不要把发布阶段看到的金额复制到代码、HTML 属性或本地配置。

从原用户动作调用统一门控：

```ts
async function handleMemberContentClick() {
  const decision = await viceme.access.require("member-content");
  if (!decision.allowed) return;
  await openMemberContent();
}
```

允许路径中的 `openMemberContent()` 保持原参数、结果、错误和副作用。组件卸载时调用 `viceme.destroy()`。

## 关注解锁

真实状态顺序是：

```text
匿名访问 → 登录 → 重新检查 → 明确关注 → 重新检查 → 解锁
```

登录不会自动关注。宿主只调用 `access.require()`，不调用关注写接口。关注层可以展示头像、名称、已发布作品数和截断简介；不展示最近作品封面，也不保留封面容器、图片请求或占位空间。

## 付费解锁

真实状态顺序是：

```text
匿名访问 → 登录 → 重新检查 → Hosted Checkout → 支付与履约 → 重新检查 → 解锁
```

SDK 在用户明确点击后把 Hosted Checkout 打开在 Access Layer 内，并持续读取新的服务端访问决定。桌面二维码支付与微信 JSAPI 保留在该层；移动 H5/WAP 可以由结账页打开支付渠道新页面或 App。支付完成后关闭或返回该页面，原宿主页继续轮询。宿主不得预开支付页面、拼接结账 URL、监听支付提供商消息，或根据 return URL、订单状态文案和本地状态授予权限。只有 `access.require()` 最终返回 `allowed: true` 才执行受保护动作。

支付新页面的创建、拦截检测和失败反馈属于 Shop 平台结账层。宿主测试可以观察平台反馈并断言
原业务动作未执行，但不得读取跨域 iframe 内部状态，也不得在宿主中预开窗口或复制降级逻辑。

## 宿主界面

宿主自己的入口复用既有 Button、Card、Dialog、字体、颜色、圆角、间距、焦点、响应式、加载和错误反馈。ViceMe 使用 `<viceme-access-layer>` 与 Shadow DOM；不得穿透样式或复制一套宿主主题到平台层。

至少覆盖：

- 匿名、已登录未关注、已关注；
- 未购买、支付取消、支付完成、已拥有；
- 桌面结账在 SDK Access Layer 内打开且关闭后恢复焦点；
- 移动支付新页面被阻止或提前关闭时不解锁，且 Shop 平台结账层显示可重试反馈；支付完成后返回时只等待新的服务端访问决定；
- 键盘关闭、焦点恢复和减少动画；
- 拒绝路径不执行原业务动作。

## 错误处理

只根据 `ViceMeError.code` 分支：

- `SESSION_EXPIRED`：提示重试或重新登录。
- `AUTH_CANCELLED`：保持原动作未执行。
- `CAPABILITY_DISABLED`：交回拥有平台资源的发布流程核对功能是否仍启用。
- `CHECKOUT_UNAVAILABLE`：交回拥有平台资源的发布流程核对付费功能与价格，不在宿主端修补。

销毁时始终调用 `destroy()`。
