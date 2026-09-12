---
name: charge-for-your-work
description: 为现有网站接入关注或匿名付费解锁，支持 CN/GLOBAL 配置、原项目绑定和接入后作品资料完善；先尝试高层命令，按返回动作补登录或创作者资格。
---

# ViceMe 网站访问接入

默认当前项目就是目标网站，没有项目上下文时才询问路径。先读取足够的入口、README、组件、依赖与已有接入，识别要保护的按钮或功能，列出少量有依据的接入建议。原动作能独立调用时，在其外层加门控；私有资源需要可信后端验证；用户要交付源码时转到做同款流程。

需要选择时先确认当前 Agent 平台：当前平台明确为 WorkBuddy 且有 `AskUserQuestion` 时使用它；其他平台使用原生的等效交互工具。没有交互式提问工具时才退回编号短选项并提示回复编号。真正开放且无法列出候选的信息才直接询问。用户已明确目标时直接采用符合目标的接缝，不强制再选一次。

## 接入步骤

1. 整理 WorkInput：真实标题、简介、正文事实、可公开素材、可选正式部署域名，以及每项保护入口。Origin 可不填，不能因缺域名阻塞接入。一次交互工具调用收齐真正缺少的业务信息：关注解锁还是付费解锁、付费功能名称和价格／币种。CN 使用人民币价格，GLOBAL 依据市场能力；不猜价格，不改用户玩法。两个 SDK key、功能键和其他内部值都不询问用户，不要求用户手打内部键值。
2. 首次调用 CLI 前按 [creator-tools 的 CLI 定位流程](../creator-tools/SKILL.md#cli-定位) 找到现有完整路径。直接执行 `website access configure --project <项目> --input <JSON>`；不先跑 auth status、merchant qualification 或全套诊断。输入和回执见 [接入参考](references/integration.md)。平台资源由命令内部处理，不把缺失状态变成用户任务。
3. 只依据稳定 error.code、结构化 nextAction 和 resumeArgs 继续。`AUTHENTICATE_CREATOR` 时用 creator-tools 的统一登录编排；资格动作出现后才调用 `$become-a-creator`，复用已授权申请、已选商家和用户名。审核待处理时保留请求。登录／资格恢复后用原 resumeArgs，不重新建 Work 或发布源码。PROVIDE_INPUT／COLLECT_INPUT 需补业务输入时重新 configure 同一项目；不要改写受管状态文件。
4. `PLATFORM_CONFIGURED` 只表示平台配置成功。CLI 已完成写后重读并提供永久公开 `keys.test`、`keys.live`，没有顶层单一 `workKey` 字段。生产宿主把 `keys.live` 传给 SDK。完整 hosted `danmaku`/`tip` features、完整 `accessFeatures`、精确 `configVersion` 及两个 key 应保留且没有发生轮换。部署市场依据同一 profile，不能由界面语言猜测。
5. 检查项目现有 SDK／锁文件及对应正式发布的能力说明，安装 `@viceme-ai/sdk` 中已支持 Access v3 的正式版本并记录实际版本；不能把仓库未发布版本或旧版兼容模式当成匿名付费／绑定已完成。真实验证首个 Access 请求协商 `accessProtocolVersion: 3` 和 `marketCapabilities`，不记录 token。新 SDK 或服务端尚未发布时保留 PLATFORM_CONFIGURED，明确等待依赖，不提交 verified=true。复用原项目的包管理器和组件。在原用户点击处理器调用 `access.require(featureKey)`，只有 `allowed: true` 后执行原动作，保持其参数、返回值、错误和副作用。名称和价格使用 `access.getFeatures()`。每个 live key 复用一个客户端，卸载时 destroy。
6. 测试实际允许、拒绝、取消、匿名购买恢复、登录绑定、已购以及关注路径。支付窗口失败反馈由 Shop 平台层负责；未完成时宿主不解锁。提交真实宿主回执给 `website access resume --receipt <JSON>`，主 Agent 核对 diff、配置及验证证据后才报告完成。没有实际验证不能写 verified=true。

## 异步接入与完成后资料

本次必需信息和授权齐备后，可把 SDK 修改、平台配置与最小验证异步交给执行 sub-agent；传明确方案、目标、价格、文件责任、恢复 ID 和验收标准，不传原始凭据。主 Agent 继续整理可选介绍，等待回执并核验；已派发不等于完成。新歧义回报主 Agent，不由子 Agent 猜商家或改价。宿主没有后台能力时顺序执行。

接入完成后引导补充当前作品实际缺少的文字、封面、截图、说明等，准备真实草稿，复用已提供的公开授权。用户可以跳过，不阻塞已完成接入。使用 `website work enrich --project <项目> --input <内容差量>` 更新同一作品；命令会保留正文、媒体与 actionConfig 等完整快照，不重新接入、改价或发布源码。上传仅使用平台已支持的媒体流程，不把本地文件路径当媒体引用。失败后可不带 input 恢复本次 enrich，资料失败不表示访问接入失败。

## 访客行为与边界

- 付费不要求登录 ViceMe。匿名用户可付款并由平台保存、恢复权益；支付成功后可选“登录并保存购买权益”，登录后绑定已授权范围，无重复确认。普通登录不自动吞并全部匿名历史订单。
- 当前作品场景登录成功、已认证付款成功后默认关注创作者，无二次确认；匿名付款跳过。已登录用户点击“关注并解锁”直接提交；宿主仍只调用 require，不复制关注写接口。取消关注由平台处理，旧事件不能把已取消关系加回。
- `access.check()` 只读；只有用户触发 `access.require()` 才进入交互。桌面结账保留在 SDK Access Layer，移动 H5/WAP 由平台结账层打开渠道页；支付返回后仅根据新的服务端访问决定解锁。宿主不拼 URL、不监听渠道消息授予权益。
- GLOBAL 无渠道时可完成配置与代码准备，付费状态为 PENDING_CHANNEL；隐藏不可用公开付费 CTA，明确“接入准备完成，收款待启用”。不显示微信／支付宝，不跨市场付款，不把待启用功能自动改免费。已有合法购买恢复不依赖新购渠道开启。
- Origin 不作为本期网站访问门禁，也不要求 DNS 验证。仍校验平台身份、Work、市场、环境、期限和服务端权益。宿主不能把本地 paid=true 或订单号当授权，不保存长期凭据。
- `<viceme-access-layer>` 使用 Shadow DOM，不穿透样式或重写核心业务动作。公开静态内容不能靠前端门控追溯保护。
- 平台对象、配置文件、命令、Work/Product ID、keys、功能键不得向用户展示、解释、索取；必要故障用自然语言说明实际未完成阶段。最终不得报告 Website Work 内部状态为用户待办。

最终报告实际接入行为、修改和测试证据、尚未验证的真实支付，以及当前作品资料补充建议。区分代码接入、外站部署、真实收款；已有授权不包含部署时不宣称已上线。
