---
name: viceme-access
description: 把 ViceMe 浏览器 SDK 接入已由 $viceme-publish 发布并配置好的创作者网站，为宿主原生入口提供登录、明确关注和付费解锁引导；不创建或修改平台发布配置。
---

# ViceMe 网站访问接入

只修改创作者网站的宿主代码。网站发布、创作者资格、Website Work、关注规则、付费价格和平台资源全部由 `$viceme-publish` 负责；本 Skill 使用其返回的 `keys.live` 作为 SDK 的 `workKey`，并使用已确认的功能键接入门控。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部协议字段。

## 流程

1. 查看足够的项目内容，定位目标动作、已有登录或支付代码、组件系统、样式变量和测试。用户已明确功能、入口和行为时直接实施；只补问无法从 publish 结果或代码确认的信息。
2. 确认已经取得 `$viceme-publish` 返回的 `keys.test`、`keys.live`、关注或付费功能键，以及服务端确认的标题和价格。生产宿主只把 `keys.live` 作为 `workKey`；`keys.test` 留给隔离测试。缺少发布结果或平台配置时立即交回 `$viceme-publish`；不得在这里登录、申请创作者、选择 Merchant、发布网站或运行平台配置命令。
3. 保留原业务动作。在已有用户触发入口外加最小门控，只有 `access.require()` 返回 `allowed: true` 后才调用原动作。若找不到安全外层接缝，说明耦合并停止，不得为了接入而重构核心行为。
4. 使用项目既有包管理器安装 `@viceme-ai/sdk`。每个生产 `keys.live` 只创建一个客户端。宿主需要显示功能名称或价格时调用 `access.getFeatures()`，不得复制 publish 阶段的标题或价格到浏览器常量。
5. 从原点击处理器调用 `access.require(featureKey)`。它会按服务端决定依次引导登录、明确关注或 Hosted Checkout；宿主不得直接写关注状态、创建结账或根据浏览器回跳自行解锁。
6. 宿主入口复用网站现有 Button、Card、Dialog、图标库、设计变量、响应式、加载状态和错误反馈。ViceMe 的 `<viceme-access-layer>` 保持 Shadow DOM 隔离，不得穿透或套用宿主样式。
7. 测试匿名、已登录但未关注、已关注、未购买、已购买、取消、弹窗阻止、键盘操作和减少动画状态，并确认拒绝路径不调用原业务动作。

完整代码和交互示例见 [references/integration.md](references/integration.md)。

## 强制边界

- 本 Skill 不拥有平台配置。不得创建或读取本地访问配置，不得登录、选择 Merchant，也不得运行任何平台访问配置命令；这些发布动作只属于 `$viceme-publish`。
- SDK access API 返回 `keys.test` 与 `keys.live`，没有顶层单一 `workKey` 字段。两个 key 都是永久公开且不透明的标识，不是凭据；生产宿主把 `keys.live` 传给 SDK 的 `workKey`，不得替换为 Work UUID、Product ID 或 token。
- 平台功能需要变更时交回 `$viceme-publish`。它必须保留完整 hosted `danmaku`/`tip` features 和完整 `accessFeatures`，从精确 `configVersion` 更新，并保证两个 key 不轮换；本 Skill 不得代做这些操作。
- 浏览器代码不得包含支付密钥、webhook、Product ID、creator ID、写死金额或权益判断。让服务端解析 `workKey → Work → WorkAccessFeature → Product → DigitalEntitlement`。
- work session 和 user token 只保存在 SDK 内存中，不得写入 cookie、localStorage、IndexedDB、URL、分析数据或日志。
- `access.check()` 只读，服务端结果是最终决定。只有用户触发 `access.require()` 才能进入交互流程。
- 登录不等于关注。匿名关注门控必须先登录，登录完成后重新检查，再由用户单独确认关注。
- 宿主不得调用内部关注写操作或用 `checkout.open()` 绕过门控；统一使用 `access.require()`。
- 支付使用平台 Hosted Checkout。SDK 可以在用户明确操作后打开受控支付窗口；宿主不得自行调用 `window.open`、拼接结账 URL 或读取支付回跳参数解锁。
- 关注界面可以展示创作者头像、名称、已发布作品数和简介，但不展示最近作品封面，也不保留封面占位。
- 不得改写、移动或重命名网站核心业务动作，也不得改变其参数、返回值、错误或副作用。
- 不得注入全局功能 CSS，也不得用宿主样式修改 ViceMe 层。
- 公开包内静态资源不能被门控追溯保护。需要强保护时，只门控行为，并从可信后端读取受保护资源。

## 完成检查

- publish 返回的 `keys.live` 被原样用作生产 `workKey`，功能键被原样使用，access 没有登录、选择 Merchant 或创建、读取、修改平台资源。
- `keys.test` 与 `keys.live` 没有被合并成旧式单 key，也没有发生轮换。
- 关注门控与付费门控相互独立；登录后不会自动关注。
- 名称和价格来自 `access.getFeatures()`，最终解锁只来自新的服务端访问决定。
- 允许路径仍以原参数调用核心动作；拒绝、取消或支付未完成路径不调用它。
- 创作者信息区域没有作品封面节点、图片请求或占位空间。
- 登录、关注、支付和取消流程支持键盘，弹窗被阻止时有明确反馈。
- 明确报告尚未验证的微信和真实支付渠道边界。
