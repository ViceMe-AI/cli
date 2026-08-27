---
name: viceme-access
description: 把 ViceMe 浏览器 SDK 接入创作者网站，通过 ViceMe 权限检查和结账实现宿主原生的登录、关注和付费功能入口。适用于 .viceme/access.yaml、功能门控界面或 viceme access 流程。用户已有明确方案时直接实施；方案缺失或不完整时才分析并提出安全入口。网站尚未发布时，先使用 $viceme-publish。
---

# ViceMe 网站访问接入

在已有且属于创作者的 `workKey` 上实现纯浏览器接入。身份、关注、购买和权益判断始终以服务端为准。保留网站核心行为和视觉语言：ViceMe 负责授权和结账层，宿主负责自身功能入口。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部协议字段。

## 流程

1. 查看足够的项目内容，定位目标动作、已有登录/支付代码、组件系统、样式变量和测试。选择最短适用路径：
   - 用户已明确付费或关注功能、入口、行为和价格时，直接实施，不要求额外分析或确认，也不重新解释已确认选择。
   - 只缺少部分信息时，只分析并补齐缺少部分。
   - 没有具体方案时，查看核心用户路径和真实界面，再提出安全的功能键、入口、规则、可复用宿主组件、受保护动作以及仍需用户决定的价格。用户选择前不得写入访问配置或宿主代码。
2. 保留原业务动作。在已有用户触发入口外加最小门控，只有 `access.require()` 返回 `allowed: true` 后才调用原动作。若找不到安全的外层接缝，说明耦合并停止，不得为了接入而重构核心行为。
3. 查看 `<website-dir>/.viceme/website.json`。文件缺失或没有 `workKey` 时，停止访问设置并先调用 `$viceme-publish`，完整完成网站资料预览和确认。只有网站明确发布成功后才回到此流程；访问命令不得隐式发布网站。
4. 运行 `viceme auth status`。若缺少 `sdk-work:read` 或 `sdk-work:write`，请用户重新运行 `viceme auth login`。
5. `.viceme/access.yaml` 不存在时，用一条命令创建并应用完整配置。需要多个功能时重复参数；只有显示标题不同于 key 时才使用 `key=title`：

   ```bash
   viceme access init --website <website-dir> --name "<website name>" \
     [--follow "<feature-key>[=<title>]"] \
     [--purchase "<feature-key>[=<title>]" --price-minor <fen>]...
   ```

   多个购买功能可以共用一个价格；不同价格时，按 `--purchase` 顺序重复 `--price-minor`。每个购买功能创建或更新自己的单次销售报价。本版本不支持创作者订阅。
6. 后续修改才编辑 `.viceme/access.yaml` 并运行 `viceme access apply`。诊断时使用 `viceme access inspect`，成功 init 后不必强制再查询一次。
7. 使用项目既有包管理器安装 `@viceme-ai/sdk`。每个 `workKey` 只创建一个客户端，等待 `ready()`，再从既有用户动作调用 `access.require()`。宿主原生入口需要展示当前名称或价格时使用 `access.getFeatures()`，不得在浏览器代码中复制这些值。除非框架确实需要，不要新增接入抽象层。
8. 宿主自己的锁定状态和访问入口复用网站现有 Button、Card、Dialog、图标库、设计变量、响应式、加载状态和错误反馈。只有没有合适原语时才添加少量局部样式，不得引入全局访问 CSS 或另一套 ViceMe 视觉语言。
9. 使用 SDK 的 `<viceme-access-layer>`。ViceMe 组件在移动端底部面板或桌面端页面内层完成登录和结账。不得改样式、穿透 Shadow DOM 或复制宿主样式。
10. 测试匿名、已登录但未关注、已关注、未购买、已购买、取消、内嵌完成、键盘操作和减少动画状态。

完整配置和代码示例见 [references/integration.md](references/integration.md)。

## 强制边界

- `workKey` 是公开且不透明的标识，不得替换为 offer ID，也不得当作秘密。
- `viceme access init` 要求网站已明确发布，只负责接入，不得创建网站发布版本，也不得代替 `$viceme-publish` 的预览确认流程。
- 浏览器代码中不得加入 Payment API key、webhook、product ID、creator ID，或写死金额、币种和价格。宿主显示的价格必须来自 `access.getFeatures()`。
- 让服务端解析 `workKey → CreatorWork → SaleOffer → Entitlement`。
- 不得把 work session token 或访问判断写入 cookie、localStorage、IndexedDB、URL、分析数据或日志。
- 不得根据结账返回参数或浏览器状态解锁；只有 `access.check()` 能授予权限。
- 权限检查不得改变身份、关注或支付状态。只有用户明确操作时才调用 `access.require()`。关注门控的登录授权层必须先展示创作者；用户同意授权即登录并自动关注，不再出现第二层关注确认。结账仍需用户选择并确认支付。
- 不得为接入改写、移动、重命名网站核心业务动作，也不得改变其参数、返回值、错误或副作用。门控是外层适配器，允许路径保持原样。
- 保持 ViceMe 授权界面完整：创作者头像位于名称上方；名称和已发布作品数同一行，用 `·` 分隔。作品数包含已发布的 Skills 和网站；两类都存在时，最近封面最多两个且各取一个。描述最多显示 50 个字符，悬停展示全文。唯一主操作为“授权”，直接进入微信二维码，不增加中间微信授权页。
- 宿主门控处理器不得调用 `follow.follow()`；关注属于 `access.require()` 打开的创作者关注界面。
- SDK 登录或结账不得使用 `window.open`、`window.location`、`confirm` 或 `alert`。完整流程留在底部面板或页面内层，并通过验证过的 SDK 消息通道返回。
- 不得注入全局功能 CSS，也不得用宿主样式修改 ViceMe 层。Web Component 保持隔离且归 ViceMe 所有；只有宿主入口使用网站设计系统。
- 关注使用 `FOLLOW_OWNER`，不得把关注建模成订阅。
- 不得配置 `ACTIVE_CREATOR_SUBSCRIPTION`；本版本保留但不支持。
- 不得声称公开包内静态资源受到保护。需要强保护时，只门控行为，并从可信后端读取受保护资源。
- 此接入不得使用更重的 ViceMe Hosted Checkout/Application 流程。

## 完成检查

- 确认 `viceme access inspect` 展示预期 Work、各功能单次报价、功能和能力。
- 确认关注门控与购买门控状态相互独立。
- 确认允许路径仍以原参数调用核心动作，结果、错误和副作用不变；拒绝或取消路径不调用它。
- 确认宿主入口匹配现有组件变体、焦点行为、断点、主题、加载状态和反馈方式。
- 确认所有公开 SDK 请求不携带浏览器凭证，只使用内存 work session。
- 确认用户同意登录授权或主动执行独立关注前，关注状态不会变化。
- 确认没有浏览器弹窗或页面跳转，登录和结账始终在底部面板或页面内层。
- 确认微信二维码和一键授权完整可见；授权 frame 保持授权层高度，不出现宿主页面遮罩或跳变；结账内容加载期间层高稳定。
- 明确报告尚未验证的微信或支付渠道边界。
