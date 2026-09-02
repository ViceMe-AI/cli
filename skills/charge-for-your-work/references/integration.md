# 接入参考

## 用户输入与内部边界

当前以项目方式打开时默认当前项目就是目标网站，不询问路径；没有项目上下文时才在同一轮询问网站
项目路径。首次询问使用一次交互工具调用，同时收齐尚未明确的精确 HTTPS Origin、要保护的按钮或
功能、关注或付费解锁，以及付费时的功能名称和人民币价格。价格只接受用户确认的人民币金额，内部
换算为分，不让用户输入内部字段。

Website Work、Work ID、平台发布状态、SDK access、功能键、Product、`keys.test`、`keys.live`、
`configVersion`、命令和错误码均为内部实现。不得向用户展示、解释、索取或把其中任何一项作为用户
前置任务。生产宿主在代码中仍把内部取得的 `keys.live` 作为 SDK 的 `workKey`，`keys.test` 只用于
隔离测试。创作者资格由父级流程调用 `$become-a-creator` 确认。

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
- 首次接入把项目路径（仅没有项目上下文时）、正式 Origin、保护目标、解锁方式和条件付费信息
  放在同一次交互中；后续异常处理一次只确认当前所需决定。真正开放且无法列出候选的信息才用
  普通对话直接询问。
- 当前平台没有交互式提问工具时，改为编号短选项并提示用户回复编号，不要求输入完整名称或
  长句。

用户在当前请求已经提供的名称和价格不重复索取。两个 SDK key、功能键和其他内部值在任何情况下
都不询问用户。平台资源不存在时进入下面的内部配置流程，不把缺失状态变成用户任务。

## 网站身份内部配置

1. 将用户给出的域名规范为精确小写 HTTPS `scheme + host`，不含凭据、路径、查询、片段或尾部
   斜杠。正式域名用于网站身份；本地开发地址只用于测试，不能替代正式域名。
2. 运行 `viceme merchant work list --merchant <merchant-id>`，内部筛选属于当前 Merchant、
   `kind: WEBSITE` 且 `website.canonicalOrigin` 与正式 Origin 完全一致的项：

   - 没有时，基于项目名称、页面标题、公开用途和域名生成 title、slug、summary 与稳定
     `clientRequestId`，用 `viceme merchant work create --input <json>` 幂等创建；响应丢失时使用同一
     输入重放。
   - 恰好一个时复用。
   - 多个时停止写入并用自然语言说明当前网站配置存在冲突，不能安全继续；不得向用户展示 Work、
     ID、key 或要求用户从内部候选中选择。

3. 用 `viceme merchant work get <work-id> --merchant <merchant-id>` 内部复核归属、kind 与 Origin。
   `DRAFT` 时用当前精确 revision 和完整既有内容执行一次 `merchant work update --input <json>`，将
   状态更新为 `PUBLISHED` 后重读；已经 `PUBLISHED` 时不重复写。`SUSPENDED`、`ARCHIVED` 或内容冲突
   时停止，不创建平行身份。最终只接受 `status: PUBLISHED`；`website.ownershipStatus` 不参与关注或
   付费解锁门禁，也不执行 DNS 验证。

## Access 内部配置

1. 从保护目标和用户可见名称派生稳定、可读的功能键，不让用户命名内部 key。关注使用
   `FOLLOW_OWNER`；付费使用 `WORK_ENTITLEMENT`，并把已确认人民币价格精确换算为分。
2. 先运行 `viceme merchant work sdk-access get <work-id> --merchant <merchant-id>` 读取完整快照；
   不存在时明确记录为空。资源存在时保存完整 hosted `features`、完整 `accessFeatures`、状态、
   `keys.test`、`keys.live` 和精确 `configVersion`。
3. 不存在时用 `sdk-access create` 一次创建本次关注或付费功能。存在时合并本次目标与全部既有
   hosted/access 功能，用刚读取的精确 `configVersion` 执行一次完整 `sdk-access update`；不得覆盖
   其他入口，也不得把关注策略原地改成付费策略或反向修改。付费参数内部使用 `--purchase` 与
   `--price-minor`，关注参数内部使用 `--follow`。
4. 写后重读并确认目标规则、名称和价格正确，原有 hosted/access 功能完整保留，两个 key 存在且
   未轮换，状态可用，`configVersion` 单调增加。冲突时重新读取并重新合并；响应丢失时先读同一
   资源，不创建第二份配置。
5. 只有完成上述读回后才修改宿主代码。若后续代码接入失败，从最新版本恢复写前完整 access
   快照；新创建的资源无法完成接入时将其 disable。恢复过程同样不向用户暴露内部对象。

## 浏览器 SDK

安装 `@viceme-ai/sdk`，部署页面使用内部配置读回的 `keys.live`。不要把
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

当前版本不把 DNS TXT、已登记 Origin 或 checkout iframe 宿主白名单作为关注/付费解锁的前置条件。
不要为此流程创建或验证 Website Domain。SDK 仍把短期 work/user token 绑定到实际请求 Origin，宿主
仍只能加载官方 Shop checkout URL，支付结果仍以服务端权益为准。

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
- `CAPABILITY_DISABLED`：内部重读并核对目标功能，仍不一致时用自然语言说明暂时无法启用。
- `CHECKOUT_UNAVAILABLE`：内部重读付费功能与价格，不在宿主端修补，也不展示内部错误码。

销毁时始终调用 `destroy()`。

## 完成报告

只向用户报告已经接入的关注或付费行为、付费名称与价格、修改文件、测试结果和真实支付等未验证
边界。不得报告 Website Work、平台发布动作、Work/Product ID、功能键、keys、配置版本、CLI 命令、
内部错误码或写后读回过程。
