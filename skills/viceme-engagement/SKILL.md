---
name: viceme-engagement
description: 为公开的创作者网站接入或修复 ViceMe 托管互动。适用于仅弹幕、仅赞赏或同时接入两者，包括登录、Website Work、域名验证、SDK access、Website Widget、官方加载器和真实页面验证。
---

# 接入 ViceMe 网站互动

使用一个 Website Work、一份 SDK access 和一个官方加载器完成用户要求的互动能力。弹幕与赞赏不得创建平行 Work。修改前完整阅读 [integration-contract.md](references/integration-contract.md)。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部协议字段或敏感值。

## 先确定特性

全新页面选择符合用户请求的最小分支：

| 分支 | SDK access 参数 | 加载器特性 | Website Widget |
| --- | --- | --- | --- |
| 仅弹幕 | `--feature danmaku` | `data-viceme-features="danmaku"` | 不需要 |
| 仅赞赏 | `--feature tip` | `data-viceme-features="tip"` | 需要 |
| 弹幕和赞赏 | `--feature danmaku --feature tip` | `data-viceme-features="danmaku,tip"` | 需要 |

先检查目标页面是否已有官方加载器。给已有页面新增能力时，页面目标特性集合是现有加载器特性与用户新增特性的并集。用户明确要求改成更小集合时，先说明 SDK access 是 Work 级配置、确认其他页面影响，再把明确移除项从集合中减去。SDK access 更新会写入完整特性集合，并保留已经启用但用户没有要求移除的特性。

## 流程

1. 运行 `viceme profile list`，记录当前 Profile，不要切换。后续所有 CLI 命令固定使用它。完成标准：Profile、Web 地址和市场区域明确。
2. 运行 `viceme --profile <profile> auth status`，要求同时具备 `merchant-commerce:read` 与 `merchant-commerce:write`。未登录或缺 scope 时运行 `viceme --profile <profile> auth login`，只展示一次完整授权链接并等待成功。完成标准：所选 Profile 返回 `authenticated: true`。
3. 查看目标仓库说明，找到公开页面、精确部署 HTTPS Origin、部署方式、CSP 和浏览器测试。Origin 只包含 scheme 与 host，不含路径、查询、片段、凭证或尾部斜杠。优先复用已有页面；只有用户需要新示例时才使用 [single-html.html](templates/single-html.html)。不得仅因 HTML 移动就创建重复项。此时只读检查当前 `/viceme-sdk/v1/viceme.min.js`，以及历史受管 `widget/engagement-embed.js`、`tip-embed.js` 标签；历史标签只作为迁移输入，不能继续运行。存在多个时先停止并请用户确认；存在一个时记录其 `data-viceme-work` 或 `data-creator-app-id` 及特性。缺少这两种可验证身份时停止，不得在确认归属前写入任何 Work、SDK access 或 Website Widget。
4. 运行 `viceme --profile <profile> merchant accounts`。使用当前登录唯一有效且属于用户的商家；存在多个时请用户选择。不得从公开创作者身份或旧本地状态推断。
5. 页面已有当前 loader，或历史 loader 带 `data-viceme-work` 时，先运行 `viceme --profile <profile> merchant work sdk-access list --merchant <merchant-id>`，按公开 `workKey` 精确定位 SDK access，再读取对应 Work。历史 `tip-embed.js` 只有 `data-creator-app-id` 时，先运行 `viceme --profile <profile> merchant commerce-application get <application-id> --merchant <merchant-id>`，确认它是当前商家的 Website Widget 并从 `workId` 读取 Work。两条路径都只有在 `website.canonicalOrigin` 与部署 Origin 完全一致时才继续，否则在任何资源写入前停止。

   页面没有任何当前或历史 loader 时，运行 `viceme --profile <profile> merchant work list --merchant <merchant-id>`，只保留 `kind: WEBSITE` 且 `website.canonicalOrigin` 与部署 Origin 完全一致的候选：

   - 0 个候选时才创建新 Work。
   - 恰好 1 个候选时复用。
   - 多个候选时，在任何 Work、Website Verification、DNS、SDK access、Website Widget 或页面写入前停止，逐项向用户展示 `id`、`title`、`status` 和 `website.ownershipStatus`，并请用户按 Work ID 选择。不得默认选择第一项、最新项、`PUBLISHED` 项或 `VERIFIED` 项。

   用户选择后重新读取该 Work，再次确认商家、kind 和 canonical Origin，之后才继续。确实不存在时才用严格输入创建，只填实际观察到的内容：

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

   运行 `viceme --profile <profile> merchant work create --input <json>`。响应丢失时用同一 `clientRequestId` 原样重放；不得创建新身份。
6. 无论复用还是新建，都运行 `viceme --profile <profile> merchant work get <work-id> --merchant <merchant-id>`。任何网站验证写入前先看状态：`SUSPENDED` 或 `ARCHIVED` 停止并报告；`DRAFT` 或 `PUBLISHED` 才继续。`website.ownershipStatus` 不是 `VERIFIED` 时，仅当当前执行仍持有 `website-verification create` 刚返回、未过期的 `PENDING` 响应才复用挑战；最新验证 GET 不含明文 `challenge`。否则读取最新 Work revision 并创建替代挑战：

   ```bash
   viceme --profile <profile> merchant work website-verification create <work-id> \
     --merchant <merchant-id> --expected-revision <work-revision>
   ```

   把 `challenge` 原样发布到 `dnsRecordName`，公共 DNS 精确解析后运行：

   ```bash
   viceme --profile <profile> merchant work website-verification verify <work-id> \
     --merchant <merchant-id> \
     --expected-verification-version <verification-version>
   ```

   验证后再次读取 Work。绝不为了找回带 `PENDING` 验证的 `DRAFT` Work 而创建第二个 Work。
7. Work 状态为 `DRAFT` 时按最新 revision 写发布输入：

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "expectedRevision": 2,
     "status": "PUBLISHED"
   }
   ```

   运行 `viceme --profile <profile> merchant work update <work-id> --input <json>`，随后再次读取 Work；已 `PUBLISHED` 时跳过更新。绝不猜测过期 revision。
8. 读取该 Work 唯一的 SDK access：

   ```bash
   viceme --profile <profile> merchant work sdk-access get <work-id> \
     --merchant <merchant-id>
   ```

   缺失时按页面目标特性集合创建；已存在时使用当前 `configVersion`，以“现有 SDK hosted features 减去明确移除项，再并入页面目标特性集合”为完整 hosted 特性集合后更新：

   ```bash
   viceme --profile <profile> merchant work sdk-access create <work-id> \
     --merchant <merchant-id> [--feature danmaku] [--feature tip]

   viceme --profile <profile> merchant work sdk-access update <work-id> \
     --merchant <merchant-id> --expected-config-version <config-version> \
     [--feature danmaku] [--feature tip]
   ```

   非空集合用重复 `--feature` 完整列出；同时包含两者时使用 `--feature danmaku --feature tip`。最终 hosted 集合为空但仍有 follow/purchase access feature 时，使用同一更新命令的 `--clear-hosted`；所有 access feature 均为空时，运行 `viceme --profile <profile> merchant work sdk-access disable <work-id> --merchant <merchant-id>`。不得执行两次独立更新互相覆盖。记录非空 access 返回的公开 `workKey`，并确认与页面已有值一致。
9. 只有页面目标特性集合包含 `tip` 时执行本步；否则直接进入加载器。Website Widget 是 `(workId, PRODUCTION, WEBSITE_WIDGET)` 唯一的 Work 级共享资源。付费访问可能在其中保存 Shop 管理的 Product 绑定和 canonical Origin return URL；本 Skill 不把不同的 `displayName`、非空 `returnUrls` 或非空 `products` 视为赞赏配置不一致，不绑定或解绑 Product，也不修改现有 return URL。

   运行 `viceme --profile <profile> merchant commerce-application list --merchant <merchant-id>`，定位作用域限定到本 Work、`kind: WEBSITE_WIDGET`、`environment: PRODUCTION` 的唯一应用。不存在时创建：

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "workId": "<work-id>",
     "kind": "WEBSITE_WIDGET",
     "environment": "PRODUCTION",
     "displayName": "<website name>",
     "origins": ["https://creator.example"]
   }
   ```

   运行 `viceme --profile <profile> merchant commerce-application create --input <json>`。复用或新建后都运行 `viceme --profile <profile> merchant commerce-application get <application-id> --merchant <merchant-id>` 复核。绝不因现有应用显示名、Origin、return URL 或 Product 不同就创建第二个。`REVOKED` 时停止并报告该终态资源。

   已有应用的 `origins` 包含 canonical Origin 时不更新配置；非空但缺少 canonical Origin 时停止并报告共享配置冲突，不得替换现有 Origin 集合。只有 `origins` 为空时才补写 canonical Origin；若此时状态为 `ACTIVE`，先按精确 revision 挂起：

   ```bash
   viceme --profile <profile> merchant commerce-application suspend <application-id> \
     --merchant <merchant-id> --expected-revision <application-revision>
   ```

   挂起后复核。对 `origins` 为空的 `DRAFT` 或 `SUSPENDED` 应用，按最新 revision 写更新输入：

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "expectedRevision": 2,
     "origins": ["https://creator.example"]
   }
   ```

   更新请求省略 `returnUrls` 才会保留现有值；`"returnUrls": []` 会退役全部现有 URL，严禁在本流程中使用。`products` 不是更新字段。运行 `viceme --profile <profile> merchant commerce-application update <application-id> --input <json>`，Origin 合同满足后，状态为 `DRAFT` 或 `SUSPENDED` 时按精确 revision 运行：

   ```bash
   viceme --profile <profile> merchant commerce-application activate <application-id> \
     --merchant <merchant-id> --expected-revision <application-revision>
   ```

   已 `ACTIVE` 且包含 canonical Origin 时跳过激活。create 响应丢失时先 list 再决定。Engagement 不管理 Product 绑定；平台管理的绑定和 return URL 可以非空。
10. 页面只能保留一个官方加载器。页面目标特性集合非空时，当前 loader 复用并更新该标签，不得追加第二个；历史受管 loader 则替换为下面的唯一标签；新页面才插入标签。最终集合为空时删除受管 loader，不留下空标签。使用当前固定 Profile 的精确 `webBaseUrl`、`marketRegion` 和 SDK access 返回的 `workKey`，不得从页面语言推断或手工拼接其他地址：

    ```html
    <div id="viceme-engagement"></div>
    <script
      defer
      src="<web-base-url>/viceme-sdk/v1/viceme.min.js"
      data-viceme-work="<work-key>"
      data-viceme-region="<cn-or-global>"
      data-viceme-features="<requested-feature-value>"
      data-viceme-target="#viceme-engagement"
      data-viceme-theme="auto"
    ></script>
    ```

    `data-viceme-features` 必须严格取页面目标特性集合对应的 `danmaku`、`tip` 或 `danmaku,tip`。宿主不得复制 ViceMe 组件、iframe、API client、持久化或支付代码。
11. 页面有 CSP 时，只为浏览器实际需要的指令加入精确 Shop Origin，保留 nonce 与其他规则，不使用 `*`、宽泛子域或 `unsafe-eval`。完成本地检查后部署真实页面，并在桌面和 320px 宽度验证：只挂载一个 SDK root、宿主控件可点击、键盘可达且减少动画生效。弹幕分支验证消息刷新后仍存在；赞赏分支验证支付可打开且 Escape 恢复初始金额表单；组合分支还要验证一个特性失败不会移除另一个。界面能打开不代表支付已成交。
12. 报告所选分支、公开 Work ID/key、赞赏分支的应用 ID、规范 Origin、已执行检查和是否实际执行支付。不得报告登录 token、DNS 挑战、cookie、支付凭证、访问令牌或签名 URL。

## 边界

- `workKey` 是公开且不透明的加载器身份，不得替换为 Work UUID、商家 ID、Product ID、slug、token 或凭证。
- SDK access 只启用托管特性，不是登录、权益或支付策略；静态公开文件不能因此受到保护。
- ViceMe 负责弹幕持久化、限流、结账、支付渠道、订单状态和结算；宿主只负责自身内容和一个加载器标签。
- 访客打开弹幕或赞赏不要求创作者配置登录；登录只用于当前创作者配置资源。

## 恢复

- 登录超时：在同一 Profile 重跑 `auth login`。
- Work create 响应丢失：用同一 `clientRequestId` 原样重放。
- SDK access 写入冲突：重新读取当前 `configVersion`，按完整目标集合重试，不创建第二份 access。
- Website Widget create 响应丢失：先 list 并按 Work、kind、environment 定位唯一应用；不创建第二个。已有非空 Origin 集合缺少 canonical Origin 时按共享配置冲突停止。
- 任何 revision 冲突：重新读取权威资源，不猜测下一个版本。
