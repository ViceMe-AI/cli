---
name: viceme-access
description: 把 ViceMe 浏览器 SDK 接入已有稳定 Merchant Work 关联的创作者网站，通过 Work SDK accessFeatures 实现宿主原生的登录、关注和一次性付费功能入口。用户已有方案时直接实施；缺少细节时才分析安全入口。本 Skill 不创建或发布 Work。
---

# ViceMe 网站访问接入

在一个现有且由创作者拥有的 `PUBLISHED` Work 上配置访问能力。身份、关注、购买和权益判断始终以服务端为准。ViceMe 负责授权与结账层，宿主只保留自己的功能入口和原业务动作。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部协议字段或敏感值。

## 流程

1. 查看足够的项目内容，定位目标动作、已有登录/支付代码、组件系统、样式变量与测试，并选择最短路径：
   - 用户已经明确功能、入口、行为和价格时，核对相关代码后直接实施，不重复解释或要求重新确认。
   - 只缺部分信息时，只分析并补齐缺少部分。
   - 没有具体方案时，查看核心用户路径和真实界面，再提出安全 feature key、标题、规则、现有入口、受保护动作、复用组件和仍需用户决定的价格。用户选择前不得写 SDK access 或宿主代码。
2. 保留原业务动作，只在已有用户触发入口外增加最小门控。只有 `access.require()` 返回 `allowed: true` 后才原样调用原动作。找不到安全外层接缝时说明耦合并停止，不得为接入重构核心行为。
3. 先交给 `$viceme-creator-onboarding` 在当前 CLI 上下文完成登录、创作者资格检查和 Merchant 选择。只有它确认当前用户通过 OWNER 关系拥有有效 Merchant 后才继续；本 Skill 不自行运行登录、申请或商家账户选择命令。随后运行 `viceme profile list`，只记录并固定当前 Profile，不得为了复用其他登录切换环境。
4. 运行 `viceme --profile <profile> merchant work list --merchant <merchant-id>`。只选择权威响应证明由该 Merchant 拥有且状态为 `PUBLISHED` 的稳定 Work；本地 Website binding 只能作为查找提示，不能替代服务端读取。没有合格 Work 时停止，并说明应先用 `$viceme-publish` 发布用户真正要经营的作品；本 Skill 不隐式创建或发布 Work。
5. 先读取当前统一资源：

   ```bash
   viceme --profile <profile> merchant work sdk-access get <work-id> \
     --merchant <merchant-id>
   ```

   资源存在时记录 `keys.test`、`keys.live`、完整 hosted `features`、完整 `accessFeatures`、状态和精确 `configVersion`；不存在时明确记录。不得把旧本地配置当作权威状态。
6. 把用户确认的全部访问功能一次写入。feature key 必须稳定；只有显示标题不同于 key 时才使用 `key=title`。关注规则使用 `--follow`，一次性购买规则使用 `--purchase` 与按分计价的 `--price-minor`：

   - 资源不存在时创建：

     ```bash
     viceme --profile <profile> merchant work sdk-access create <work-id> \
       --merchant <merchant-id> \
       [--follow "<feature-key>[=<title>]"] \
       [--purchase "<feature-key>[=<title>]" --price-minor <fen>]...
     ```

   - 资源存在时，从刚读取的 `configVersion` 完整替换 `accessFeatures`。重复传入全部目标 follow/purchase 功能；不要传 `--feature` 或 `--clear-hosted`，从而原样保留 Danmaku/Tip hosted features：

     ```bash
     viceme --profile <profile> merchant work sdk-access update <work-id> \
       --merchant <merchant-id> \
       --expected-config-version <config-version> \
       [--follow "<feature-key>[=<title>]"] \
       [--purchase "<feature-key>[=<title>]" --price-minor <fen>]...
     ```

   多个购买功能共用一个价格时只传一次 `--price-minor`；价格不同时按 `--purchase` 顺序重复。每个购买功能对应独立单次权益，本版不支持创作者订阅。状态为 `DISABLED` 时，同样从精确版本更新完整 access feature set 以重新启用。
7. 写入后再次 get，确认 `keys.test` 与 `keys.live` 都存在且未轮换、完整 hosted features 与写入前一致、`accessFeatures` 精确等于用户确认值、状态为 `ACTIVE` 且 `configVersion` 单调增加。冲突时必须重读并重新确认，绝不猜版本。
8. 用项目既有包管理器安装兼容的 `@viceme-ai/sdk`。一个页面中每个 `keys.live` 只创建一个客户端，等待 `ready()` 后再从既有用户动作调用 `access.require()`。宿主入口需要显示名称或价格时使用 `access.getFeatures()`，不得在浏览器代码复制这些值。完整示例见 [references/integration.md](references/integration.md)。
9. 复用宿主已有 Button、Card、Dialog、图标、设计变量、响应式、加载和错误反馈。只有缺少合适原语时才添加少量局部样式，不引入另一套 ViceMe 视觉语言。
10. 登录和结账使用 SDK 的 `<viceme-access-layer>`。移动端底部面板或桌面端页面内层由 ViceMe 拥有；宿主不得穿透 Shadow DOM、改样式或复制支付界面。
11. 测试匿名、已登录但未关注、已关注、未购买、已购买、取消、内嵌完成、键盘和减少动画状态，并确认关注与购买 feature 相互独立。

## 强制边界

- `keys.test` 与 `keys.live` 是永久公开且不透明的 Work key，不是凭据。不得用 offer ID、Work UUID、Product ID 或 token 替代，也不得只保存一个旧式 key 字段。
- 更新访问能力时必须保护同一资源上的完整 hosted features；更新 hosted 能力时也必须保护完整 `accessFeatures`。所有更新都使用精确 `configVersion`，且不能轮换 key。
- 浏览器代码不得包含 Payment API key、webhook、product ID、creator ID，也不得写死金额、币种或价格。宿主显示价格必须来自 `access.getFeatures()`。
- 服务端负责解析 Work key 到 Work、销售报价和权益；浏览器不读取或猜测这些内部资源。
- 不得把 work session token 或访问判断写入 cookie、localStorage、IndexedDB、URL、分析数据或日志。
- 不得根据结账返回参数或浏览器状态解锁；只有 `access.check()` 能授予权限。`PENDING` 或消息通知都不能授权。
- `access.check()` 不改变身份、关注或支付状态；只有用户明确操作时才调用 `access.require()`。关注门控的授权层展示创作者，用户同意授权后登录并自动关注，不增加第二层关注确认。宿主处理器不得直接调用关注写操作。
- 不得改变其参数、返回值、错误或副作用。这里的“其”指原有核心业务动作；门控只是外层适配器，拒绝或取消路径不调用原动作。
- SDK 登录或结账不得使用 `window.open`、`window.location`、`confirm` 或 `alert`。流程留在底部面板或页面内层，并通过 SDK 验证过的消息通道返回。
- 关注使用 `FOLLOW_OWNER`，一次性购买使用 `WORK_ENTITLEMENT`。不得配置 `ACTIVE_CREATOR_SUBSCRIPTION`。
- 不得声称公开包中的静态资源受到强保护。需要强保护时只门控行为，并从可信后端获取资源。

## 完成检查

- get 响应展示预期 Work、两个永久 key、原有 hosted features、全部访问功能、独立报价与最新 `configVersion`。
- 关注门控和购买门控状态相互独立。
- 允许路径仍以原参数调用核心动作，结果、错误与副作用不变；拒绝或取消路径不调用。
- 宿主入口匹配现有组件变体、焦点、断点、主题、加载和反馈方式。
- 所有公开 SDK 请求不携带浏览器凭据，只使用内存 work session。
- 用户同意登录授权或主动执行独立关注前，关注状态不改变。
- 没有浏览器弹窗或页面跳转；登录与结账始终留在 ViceMe layer。
- 明确报告尚未验证的微信或支付渠道边界，不报告凭据或支付详情。

## 恢复

- create/update 响应丢失时先 get 同一 Work，不能创建第二个 SDK access。
- 配置冲突时读取最新 `configVersion`、完整 hosted features 和 `accessFeatures`，让用户确认合并后的访问规则再做一次更新。
- 宿主接入失败且本流程已经写入时，从最新版本恢复写入前完整配置；原资源不存在时 disable 新资源。两个永久 key 都不轮换。
