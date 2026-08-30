---
name: viceme-danmaku
description: 在网站中安装或修复 ViceMe 托管的弹幕 SDK。适用于脚本标签接入、workKey 配置、滚动消息、互动、评论入口、响应式行为、键盘操作和减少动画验证。
---

# 安装 ViceMe 弹幕

托管 SDK 是默认且权威的接入方式。把 CLI 生成的脚本标签加入网站；宿主项目不得复制 ViceMe 的 React、Tailwind、iframe、API 客户端或持久化代码。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，技术值只在确有必要时展示。

## 开始前检查

修改前完整查看：

1. 目标仓库说明、页面入口、部署方式、内容安全策略和浏览器测试方式。
2. `viceme profile list`，包括所选 Profile 的 `apiBaseUrl`、`webBaseUrl` 和 `marketRegion`。
3. 完整阅读 [cdn-sdk.md](references/cdn-sdk.md)。

## 流程

1. 运行 `viceme profile list`，后续所有命令固定使用所选 Profile。不得只为复用另一份登录而切换 Profile。
2. 运行 `viceme --profile <profile> auth status`，要求同时具备 `merchant-commerce:read` 与 `merchant-commerce:write`；缺少时在同一 Profile 重新登录并等待完成。
3. 运行 `viceme --profile <profile> merchant accounts`。使用当前登录拥有的唯一有效商家；多个时请用户选择。不得从公开创作者身份或旧本地状态推断。
4. 运行 `viceme --profile <profile> merchant work list --merchant <merchant-id>`。只有 `website.canonicalOrigin` 与部署 Origin 完全一致时才复用 Website Work；否则用严格输入创建（只填实际观察到的内容）：

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

   运行 `viceme --profile <profile> merchant work create --input <json>`。响应丢失时用同一 `clientRequestId` 原样重放；不得创建新身份，也不得创建第二个 Work。
5. 无论复用还是新建，都用 `viceme --profile <profile> merchant work get <work-id> --merchant <merchant-id>` 读取。任何网站验证写入前先看状态：`SUSPENDED` 或 `ARCHIVED` 停止并报告；`DRAFT` 或 `PUBLISHED` 才继续。

   `website.ownershipStatus` 不是 `VERIFIED` 时：仅当当前执行仍持有 `website-verification create` 刚返回、未过期的 `PENDING` 响应才复用挑战（最新验证 GET 不含明文 `challenge`）。否则读取最新 Work revision 创建替代挑战：

   ```bash
   viceme --profile <profile> merchant work website-verification create <work-id> \
     --merchant <merchant-id> --expected-revision <work-revision>
   ```

   把 `challenge` 原样发布到 `dnsRecordName`。公共 DNS 精确解析后运行：

   ```bash
   viceme --profile <profile> merchant work website-verification verify <work-id> \
     --merchant <merchant-id> \
     --expected-verification-version <verification-version>
   ```

   验证后再次读取 Work。绝不为了找回带 `PENDING` 验证的 `DRAFT` Work 而创建第二个 Work。
6. Work 状态为 `DRAFT` 时按最新 revision 写发布输入：

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "expectedRevision": 2,
     "status": "PUBLISHED"
   }
   ```

   运行 `viceme --profile <profile> merchant work update <work-id> --input <json>`，随后 `merchant work get` 复核；已 `PUBLISHED` 跳过更新；`SUSPENDED`/`ARCHIVED` 停止并报告。绝不复用或猜测过期 revision。
7. 读取 `merchant work sdk-access get <work-id> --merchant <merchant-id>`。缺失时用 `--feature danmaku` 创建；已存在时按当前 `configVersion` 与完整目标特性集更新——已启用 `tip` 要保留。更新不是增量追加。
8. 用所选 Profile 的精确 `webBaseUrl`、返回的公开 `workKey` 和 `marketRegion`（`cn` 或 `global`）插入一个加载器标签：

   ```html
   <script
     defer
     src="<web-base-url>/viceme-sdk/v1/viceme.min.js"
     data-viceme-work="<work-key>"
     data-viceme-region="<cn-or-global>"
     data-viceme-features="danmaku"
     data-viceme-target="body"
     data-viceme-theme="auto"
   ></script>
   ```

9. 保留现有 CSP nonce。只把页面确实需要的精确 Profile Web Origin 加进对应指令；不得添加 `*`、`'unsafe-eval'` 或宽泛子域通配符。
10. 运行目标项目的格式化、lint、类型检查、测试和生产构建。在桌面端和移动端检查真实页面：只挂载一个 SDK 根节点、宿主控件仍可点击、键盘操作正常、减少动画设置生效、发送的消息刷新后仍存在，而且没有重复加载脚本。

完成标准是目标项目只有一份基于已验证 Website Work 的加载器接入，并且浏览器检查使用同一个固定 Profile 通过。

## 边界

- `workKey` 是公开且不透明的标识，不得替换为 creator ID、product ID、slug、token 或凭证。
- 托管 SDK 负责渲染、iframe 位置、公开 API 请求、持久化和清理。
- 宿主只负责脚本位置和自身页面内容。
- Website Work 拥有身份与已验证 Origin；SDK 访问只启用托管特性，不是登录、权益或支付策略。
- 打赏由 `viceme-tip` 处理，不属于本 Skill；更新共享访问时要保留已启用的 `tip`。
- 自托管或复制组件源码不属于本 Skill。

## 交付说明

报告修改文件、所选 Profile、公开 `workKey`、执行过的检查、响应式和键盘覆盖，以及尚未验证的 CSP 或托管 Runtime 边界。不得报告任何凭证。
