---
name: viceme-danmaku
description: 在创作者网站安装或修复 ViceMe 托管弹幕。适用于 Website Work 创建或复用、DNS 所有权验证、Work SDK access、官方 loader 接入，以及响应式、键盘和减少动画验收。
---

# 接入 ViceMe 托管弹幕

弹幕只接入一个已经发布且完成 DNS 所有权验证的 Website Work，并要求其 SDK access 已启用 `danmaku`。宿主只安装官方 loader，不复制 ViceMe 组件、iframe、API client 或持久化代码。开始前完整阅读 [cdn-sdk.md](references/cdn-sdk.md)。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部挑战值或敏感字段。

## 流程

1. 查看仓库说明、页面入口、精确线上 HTTPS Origin、部署路径、CSP 和浏览器测试。Origin 只包含 scheme 与 host，不含凭证、路径、查询、片段或尾部斜杠。
2. 先交给 `$viceme-creator-onboarding` 在当前 CLI 上下文完成登录、创作者资格检查和 Merchant 选择。只有它确认当前用户通过 OWNER 成员关系拥有有效 Merchant 后才继续；本 Skill 不自行运行登录、申请或商家账户选择命令。随后运行 `viceme profile list`，只记录并固定当前 Profile，不得从公开创作者身份、旧本地状态或其他 Profile 推断环境。
3. 运行 `viceme --profile <profile> merchant work list --merchant <merchant-id>`。只有 `website.canonicalOrigin` 与实际部署 Origin 完全一致时才复用 Website Work。

   对精确匹配的现有 Work，在任何 Work 创建、更新、Website verification、发布、SDK access 写入或页面编辑前，立即读取：

   ```bash
   viceme --profile <profile> merchant work sdk-access get <work-id> \
     --merchant <merchant-id>
   ```

   如果 feature 快照包含 `tip`，立即停止本独立流程并加载 `viceme-engagement`。组合 Skill 负责 CN 边界、完整双区域 Tip 发布预检、Website 修复和一次完整配置更新；本流程不得保留或重新启用 `tip`。

   没有精确 Work 时，用稳定 `clientRequestId` 和只包含观察事实的严格输入创建：

   ```json
   {
     "kind": "WEBSITE",
     "merchantAccountId": "<merchant-id>",
     "clientRequestId": "<stable-idempotency-key>",
     "slug": "website-slug",
     "title": "Website title",
     "canonicalOrigin": "https://creator.example",
     "content": {
       "summary": "Observed public purpose",
       "bodyMarkdown": "Observed public description",
       "templateType": "WEBSITE",
       "tags": [],
       "media": [],
       "actionConfig": {}
     }
   }
   ```

   运行 `viceme --profile <profile> merchant work create --input <json>`。响应丢失时用同一 `clientRequestId` 原样重放，不创建新身份。
4. 无论 Work 是复用还是新建，都运行 `viceme --profile <profile> merchant work get <work-id> --merchant <merchant-id>`。如果状态是 `SUSPENDED` 或 `ARCHIVED`，立即停止并报告，不创建挑战、不修改 DNS，也不静默恢复或另建 Work。

   `website.ownershipStatus` 不是 `VERIFIED` 时先运行：

   ```bash
   viceme --profile <profile> merchant work website-verification get <work-id> \
     --merchant <merchant-id>
   ```

   最新 verification 状态是 `PENDING` 时，只有当前执行仍持有 create 刚返回且未过期的明文 challenge 才继续使用；GET 不用于找回 challenge。否则读取最新 Work revision 并创建替代挑战：

   ```bash
   viceme --profile <profile> merchant work website-verification create <work-id> \
     --merchant <merchant-id> --expected-revision <work-revision>
   ```

   把返回的 `challenge` 原样发布到 `dnsRecordName` 指定的记录。公共 DNS 精确解析后运行：

   ```bash
   viceme --profile <profile> merchant work website-verification verify <work-id> \
     --merchant <merchant-id> \
     --expected-verification-version <verification-version>
   ```

   验证后重新读取 Work。不得为了恢复带 `PENDING` verification 的 `DRAFT` Work 创建第二个 Work，也不得猜测 challenge、verification version 或 revision。
5. 当前 Work 状态是 `DRAFT` 时，从最新 revision 发布：

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "expectedRevision": 2,
     "status": "PUBLISHED"
   }
   ```

   运行 `viceme --profile <profile> merchant work update <work-id> --input <json>`，然后再次运行 `viceme --profile <profile> merchant work get <work-id> --merchant <merchant-id>`。Work 已经是 `PUBLISHED` 时跳过更新；变为 `SUSPENDED` 或 `ARCHIVED` 时停止并报告。绝不复用过期 revision。
6. 新建 Work 现在读取其 SDK access；复用 Work 则刷新第 3 步的快照。快照必须包含 `keys.test`、`keys.live`、完整 hosted `features`、完整 `accessFeatures`、状态和精确 `configVersion`。如果并发出现 `tip`，在写入前停止并加载 `viceme-engagement`。

   资源不存在时创建：

   ```bash
   viceme --profile <profile> merchant work sdk-access create <work-id> \
     --merchant <merchant-id> --feature danmaku
   ```

   已存在时，从当前 `configVersion` 替换完整 hosted feature set；不要传 access flags，因此完整 `accessFeatures` 必须保持不变：

   ```bash
   viceme --profile <profile> merchant work sdk-access update <work-id> \
     --merchant <merchant-id> --expected-config-version <config-version> \
     --feature danmaku
   ```

   update 不是增量 append。状态为 `DISABLED` 时用同一完整配置重新启用。写入后重读并确认两个永久 key 未变、hosted features 精确、`accessFeatures` 未变且 `configVersion` 单调增加。
7. 使用所选 Profile 的精确 `webBaseUrl`、返回的公开 `keys.live` 和 Profile 的 `marketRegion`（`cn` 或 `global`）插入唯一 loader：

   ```html
   <script
     defer
     src="<web-base-url>/viceme-sdk/v1/viceme.min.js"
     data-viceme-work="<production-work-key>"
     data-viceme-region="<cn-or-global>"
     data-viceme-features="danmaku"
     data-viceme-target="body"
     data-viceme-theme="auto"
   ></script>
   ```

8. 保留现有 CSP nonce，只向浏览器证明确实需要的指令加入精确 Profile Web Origin；不得加入 `*`、`unsafe-eval` 或宽泛子域通配符。
9. 运行仓库检查并验证真实线上页面的桌面和移动端。确认只挂载一个 SDK root、宿主控件仍可点击、键盘操作正常、减少动画生效，并且一条弹幕刷新后仍存在。

## 边界

- `keys.live` 与 `keys.test` 是永久公开标识，不是凭据。线上弹幕使用 production key；不得换成 Work UUID、Merchant ID、Product ID、slug 或 token。
- Website Work 拥有页面身份与 verified Origin；SDK access 只启用托管能力，不是登录、权益或支付规则。Danmaku 始终要求 `PUBLISHED` Website Work 和 DNS `VERIFIED`。
- Tip 由 `viceme-tip` 或 `viceme-engagement` 负责。本独立流程绝不保留、恢复或重新启用 `tip`。
- 报告变更文件、公开 Work ID/key、检查和未验证运行时边界。不得报告凭据或 DNS challenge。

## 恢复

- Work 或 SDK access 响应丢失时，先读取同一资源再决定是否重试。
- `configVersion` 冲突时重读全部 hosted features 与 `accessFeatures`；如果发现 `tip` 就转交组合流程，不得覆盖。
- 页面接入失败且本流程已写 SDK access 时，从最新版本恢复写入前的完整 hosted 与 access 配置；原资源不存在时 disable 新资源。永久 key 不轮换。
