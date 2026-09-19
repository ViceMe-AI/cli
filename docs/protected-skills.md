# 核心保护 Skill

作者的 inspect、publish、resume、上传原包和发布确认流程保持原样。不新增 --protect、--public-purpose、--public-file 或发布文件分类配置。服务端决定新付费版本的内部 PROTECTED 交付类型，既有 SOURCE 和免费源码发布保持原有处理。

安装时只将根目录 SKILL.md 替换为通用调用入口，其余原始附件原样保留，包含原有脚本、模板、规则附件和 WORKFLOW.md。不会生成额外 WORKFLOW.md 或 viceme-cloud.json。此边界只保护 SKILL.md。

既有 .viceme 元数据绑定产品、API、市场和发布版本。凭据保存在本机私有目录；安装包不含购买秘密。购买不会解锁原始 SKILL.md。

使用 viceme skill guidance --input task.json（独立 runtime 为 python3 .viceme/scripts/trial.py guidance --input task.json）。任务包含 requestKey、prompt、可选 facts 和 sessionId；原始业务文件保留本地。ready 后本地 Agent 完整读取 executionPath，按指导调用原始脚本并校验产物。

同一任务重试必须保留原文件和 requestKey；付款完成后继续原任务。修改任务使用新键；语言调整和补充事实使用原 sessionId。服务端拒绝、补充信息和失败不会扣试用，重复恢复不重复扣。失败按 retryable 提示恢复，耗尽预算后停止。

导出 PROTECTED 渠道包使用服务端已验证的消费者包摘要，附加官方 runtime 和既有 .viceme 身份，不依赖根目录配置文件，不追加源码门禁或私有正文。Go CLI 和独立 Python runtime 必须遵守同一授权、版本和幂等协议。

本轮通过接口、隔离数据库/S3 及实际 CLI 测试，不新增或操作发布表单。完成自测后进行子代理整体评审，PR 目标 dev；不发布正式版本。
