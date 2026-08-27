# 发布错误处理

- `CREATOR_DISPLAY_NAME_REQUIRED`：使用 `--creator-display-name "<creator name>"` 重跑同一次网站发布；现有本地绑定保留原 `clientWorkId`。
- `WEBSITE_PROFILE_MISMATCH`、`WEBSITE_BINDING_SCOPE_MISMATCH`、`WEBSITE_REGION_MISMATCH`：恢复预期的当前 CLI 上下文。不得覆盖绑定或仅为绕过不匹配创建另一个 Work。
- `WEBSITE_IDENTITY_CONFLICT`：停止并检查现有绑定和权威 Work 响应。不得用新编造的身份替换本地 `clientWorkId`、`workId` 或 `workKey`。
- `SKILL_PUBLICATION_PRICE_REQUIRED`：取得并展示当前完整上架信息，同时询问准确人民币分价以及希望修改的标题、文案或媒体。不得只问价格。继续同一私有 Publication。
- `SKILL_SECRET_DETECTED` 或 `SKILL_SENSITIVE_FILE`：停止并从包中删除凭证或敏感文件，绝不打印内容。
- `PUBLICATION_SOURCE_CHANGED`：恢复包与开始发布时不同；恢复原包或开始新的发布。
- `SKILL_PUBLICATION_REVIEW_CHANGED`：获取并展示最新预览，针对新 digest 重新取得“确认并发布”授权。
- `SKILL_LISTING_MEDIA_REQUIRED`：上传真实封面和图库图片，获取新预览，再提交新的 Agent 建议。只有已明确选择平台分析兜底时才重试该兜底。
- `SKILL_LISTING_DRAFT_CHANGED`：获取最新权威预览，根据其 `draftRevision` 重新生成 Agent 建议，不得重放旧建议。
- `SKILL_PUBLICATION_ANALYSIS_IN_PROGRESS`：已明确选择的平台兜底正在处理该 Draft。等待同一 Publication 完成后获取新预览，不得并行提交 Agent 建议。
- `AUTHORIZATION_PENDING`：在当前 CLI 上下文重新运行 `viceme auth login`，保持命令运行直到浏览器授权完成。
- `NOT_LOGGED_IN` 或 `token_expired`：在当前 CLI 上下文使用 `viceme-shared` 登录流程。发布期间不得查看或切换其他已保存环境。
- `PUBLICATION_SCOPE_REQUIRED`：在当前 CLI 上下文重新登录，授予发布所需范围。
- `MERCHANT_REQUIRED`：当前用户不拥有有效商家。停止并用白话说明需要先完成商家申请或认领。
- `MERCHANT_SELECTION_REQUIRED`：展示返回的有效商家，请用户选择，再用 `--merchant <merchant-account-id>` 对同一来源重试。
- `MERCHANT_SUSPENDED`：所选商家不能发布。不得静默切换；请用户选择另一个自己拥有的有效商家，或请工作人员恢复当前商家。
- `PUBLICATION_MERCHANT_CHANGED`：本地恢复状态、所选商家与服务端 Publication 不一致。不得覆盖恢复状态或创建替代项；检查同一 Publication 并解决所有权不一致。
- `PUBLICATION_RECOVERY_PERMISSION_REQUIRED`：进程不能写本地发布恢复目录。为原命令申请准确的文件权限，并用同一来源和价格重试。不得删除锁文件或创建另一发布。
- `SKILL_BINDING_PERMISSION_REQUIRED`：允许原命令写入 `.viceme/skill.json`、相邻 ZIP sidecar 或报告的端点范围索引目录，再用同一来源身份重试。
- `SKILL_BINDING_SCOPE_MISMATCH` 或 `SKILL_LISTING_BINDING_OWNER_MISMATCH`：不得查看或切换环境，也不得覆盖原作者 Listing。询问用户是否明确希望使用 `--new-listing` 在当前 CLI 上下文创建独立 Work。
- `SKILL_LISTING_SOURCE_AMBIGUOUS`：展示候选 Listings，请用户选择后使用 `skill listing bind`；不得根据标题或文件名猜测。

`retryable=true` 时，使用同一 Publication 或 client request identity 做有界退避重试；否则先改变输入或状态。
