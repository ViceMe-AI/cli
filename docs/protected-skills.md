# 核心保护 Skill

作者的 inspect、publish、resume、上传原包和发布确认流程保持原样。不新增 --protect、--public-purpose、--public-file 或发布文件分类配置。服务端决定新付费版本的内部 PROTECTED 交付类型，既有 SOURCE 和免费源码发布保持原有处理。

安装时只将根目录 SKILL.md 替换为通用调用入口，其余原始附件原样保留，包含原有脚本、模板、规则附件和 WORKFLOW.md。不会生成额外 WORKFLOW.md 或 viceme-cloud.json。此边界只保护 SKILL.md。

既有 .viceme 元数据绑定产品、API、市场和发布版本。凭据保存在本机私有目录；安装包不含购买秘密。购买不会解锁原始 SKILL.md。

使用 viceme skill guidance --input task.json（独立 runtime 为 python3 .viceme/scripts/trial.py guidance --input task.json）。任务包含 requestKey、完整 prompt 和可选 facts，不接受或返回 sessionId；原始业务文件保留本地。每次调用只使用本次输入和私有 Skill，不保存对话历史。ready 后本地 Agent 完整读取 executionPath，按指导调用原始脚本并校验产物。

同一任务重试必须保留原文件、身份和 requestKey；付款完成后继续原任务。Go 与 Python 共用本地请求恢复记录。不同账号购买不会迁移已有请求；原身份权益失效时，明确提示使用当前账号和新 requestKey 提交独立任务。新独立任务的普通 CLI 登录被服务端明确判定失效时，可沿用已经存在的匿名购买或试用凭证；显式 token、已绑定账号的任务以及网络故障不触发回退。修改任务、语言调整、补充事实或 needs_input 时，由用户 Agent 整合全部有效要求和事实，用新键提交完整任务；服务器不替用户管理对话。超长输入由 Agent 整理精简后新键重提，不自动压缩，不无限重试原输入。服务端拒绝、补充信息和失败不会扣试用，重复恢复不重复扣。失败按 retryable 提示恢复，耗尽预算后停止。

本地 SKILL.md 是能力简介、公共门禁指引和指导调用说明。平台的 viceme-runtime.md、purchase.md 与支付展示指引随官方 .viceme runtime 提供，避免覆盖原作者附件；平台包元数据同样使用 .viceme/skill-package.json，保留作者根目录同名附件。服务端代码先判权，再决定是否调用模型。购买不解锁私有正文。安装后先处理 runtime 返回的购买校验和恢复动作，服务端确认待付款才展示支付；已有任务直接继续。没有待处理购买和任务时，Agent 根据公开能力简介在本地生成 3 个示例任务，不调用指导接口、不扣试用，不编造执行结果。只有用户选定并实际开始任务时才申请指导。

无试用商品可以在尚未提交指导任务时直接购买；运行 `viceme skill trial-purchase <productId> --skill-dir <目录> --wait 0` 或该目录的 `python3 .viceme/scripts/trial.py purchase --wait 0`。Python 可委托已安装 CLI 使用账号权益；账号已购时不另建匿名订单。普通存储登录明确失效可继续既有匿名购买恢复，显式凭据和网络故障仍停止。付款恢复遇到其他身份已购时，旧请求保留并返回完整新任务、新 requestKey 指引。

这是首次上线前的无会话协议切换，API、CLI 与独立 runtime 需配套更新；旧 sessionId 输入明确拒绝，不静默忽略或续接历史会话。

导出 PROTECTED 渠道包使用服务端已验证的消费者包摘要，附加官方 runtime 和既有 .viceme 身份，不依赖根目录配置文件，不追加源码门禁或私有正文。Go CLI 和独立 Python runtime 必须遵守同一授权、版本和幂等协议。

本轮通过接口、隔离数据库/S3 及实际 CLI 测试，不新增或操作发布表单。完成自测后进行子代理整体评审，PR 目标 dev；不发布正式版本。

2026-09-20 本地无会话验收：实现提交 `3d069e720e8a075a6212d394a7f1c4bb798cda9d`；`make release-manifest check test-race npm-package-check` 通过，包含 112 项 Python runtime 与 6 项 bootstrap 回归。配套 Shop 的实际 Go/Python 包联测 33 项通过，真实 Gemini 原文/指导对照 2 个计划和 12 个案例通过。独立复审后未发现剩余明显问题。详细证据在[配套 Shop PR](https://github.com/Leizhenpeng/ViceMe-Shop/pull/834) 的 `docs/verification/stateless-skill-20260920/README.md`；仅本地隔离环境，测试购买权益未调用真实支付提供方，尚未部署或发布。
