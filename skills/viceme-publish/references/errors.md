# 发布错误处理

- `SKILL_PUBLICATION_PRICE_REQUIRED`：取得并展示当前完整上架信息，同时询问准确人民币分价以及希望修改的标题、文案或媒体。不得只问价格。继续同一私有 Publication。
- `SKILL_SECRET_DETECTED` 或 `SKILL_SENSITIVE_FILE`：停止并从包中删除凭证或敏感文件，绝不打印内容。
- `PUBLICATION_SOURCE_CHANGED`：恢复包与开始发布时不同；恢复原包或开始新的发布。
- `SKILL_PUBLICATION_REVIEW_CHANGED`：获取并展示最新预览，针对新 digest 重新取得“确认并发布”授权。
- `SKILL_LISTING_MEDIA_REQUIRED`：上传真实封面和图库图片，获取新预览，再提交新的 Agent 建议。只有已明确选择平台分析兜底时才重试该兜底。
- `SKILL_LISTING_DRAFT_CHANGED`：获取最新权威预览，根据其 `draftRevision` 重新生成 Agent 建议，不得重放旧建议。
- `SKILL_PUBLICATION_ANALYSIS_IN_PROGRESS`：已明确选择的平台兜底正在处理该 Draft。等待同一 Publication 完成后获取新预览，不得并行提交 Agent 建议。
- `AUTHORIZATION_PENDING`、`NOT_LOGGED_IN`、`token_expired`、`MERCHANT_COMMERCE_SCOPE_REQUIRED` 或 `PUBLICATION_SCOPE_REQUIRED`：停止当前发布并在同一 CLI 上下文重新调用 `$viceme-creator-onboarding`。由它完成一次等待式登录并重新确认商家；确认后才重试刚才的同一发布操作。不得在本发布流程自行启动第二套登录。
- `MERCHANT_REQUIRED`：当前用户不拥有有效商家。停止当前发布并交回 `$viceme-creator-onboarding`；由它用白话说明并处理申请、认领或人工审核。本发布流程不得自行重复登录、查询申请或提交申请。
- `MERCHANT_SELECTION_REQUIRED`：这个错误发生在创建发布记录之前。停止当前发布并交回 `$viceme-creator-onboarding`，由它展示可用名称并让用户选择。取得它返回的商家后，用相同来源和 `--merchant <merchant-account-id>` 重新发起刚才的发布命令；不得声称存在可恢复的原 Publication，本发布流程也不得自行查询或选择账户。
- `MERCHANT_SUSPENDED`：先区分是否已经创建发布记录。新发布尚未创建记录时，交回 `$viceme-creator-onboarding` 选择其他有效商家，再用相同来源和所选 `--merchant <merchant-account-id>` 重新发起。恢复已有 Publication 时，原商家不可更换；停止并用白话说明需要等待工作人员恢复该商家。用户明确希望另起一次独立发布时，才可以确认后使用 `--new-listing` 开始新的发布；不得在同一 Publication 上静默切换商家。
- `PUBLICATION_MERCHANT_CHANGED`：本地恢复状态、所选商家与服务端 Publication 不一致。不得覆盖恢复状态或创建替代项；检查同一 Publication 并解决所有权不一致。
- `PUBLICATION_RECOVERY_PERMISSION_REQUIRED`：进程不能写本地发布恢复目录。为原命令申请准确的文件权限，并用同一来源和价格重试。不得删除锁文件或创建另一发布。
- `SKILL_BINDING_PERMISSION_REQUIRED`：允许原命令写入 `.viceme/skill.json`、相邻 ZIP sidecar 或报告的端点范围索引目录，再用同一来源身份重试。
- `SKILL_BINDING_SCOPE_MISMATCH` 或 `SKILL_LISTING_BINDING_OWNER_MISMATCH`：不得查看或切换环境，也不得覆盖原作者 Listing。询问用户是否明确希望使用 `--new-listing` 在当前 CLI 上下文创建独立 Work。
- `SKILL_LISTING_SOURCE_AMBIGUOUS`：展示候选 Listings，请用户选择后使用 `skill listing bind`；不得根据标题或文件名猜测。

`retryable=true` 时，使用同一 Publication 或 client request identity 做有界退避重试；否则先改变输入或状态。
