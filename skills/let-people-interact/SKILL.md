---
name: let-people-interact
description: 为公开的创作者网站同时接入或修复 ViceMe 托管弹幕和赞赏。先复用 $become-a-creator 完成登录与创作者资格守卫，再处理 Website Work、域名验证、SDK access、Website Widget、官方加载器和真实页面验证。
---

# 接入 ViceMe 网站互动

使用一个 Website Work、一份 SDK access 和一个官方加载器同时完成弹幕与赞赏。两项能力不创建第二个 Work，也不得创建平行 Work。修改前完整阅读 [integration-contract.md](references/integration-contract.md)。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部协议字段或敏感值。

本 Skill 的目标特性集合固定为 `danmaku,tip`。底层 CLI 可以独立配置 hosted feature，但单独弹幕或单独赞赏不属于本玩法；不得在流程中要求用户二选一。

## 流程

1. 运行 `viceme profile list`，记录当前 Profile，不要切换。后续所有 CLI 命令固定使用它。完成标准：Profile、Web 地址和市场区域明确。
2. 第一项业务动作以资格守卫模式调用 `$become-a-creator`。它统一完成登录、资格检查、必要时的一次申请确认和商家选择；只有返回当前用户的有效 Merchant 后才继续。不得在本 Skill 内自行重复登录、申请或选择商家。
3. 查看目标仓库说明，找到公开页面、精确部署 HTTPS Origin、部署方式、CSP 和浏览器测试。Origin 只包含 scheme 与 host，不含路径、查询、片段、凭证或尾部斜杠。优先复用已有页面；只有用户需要新示例时才使用 [single-html.html](templates/single-html.html)。不得仅因 HTML 移动就创建重复项。此时只读检查当前 `/viceme-sdk/v1/viceme.min.js`，以及历史受管 `widget/engagement-embed.js`、`tip-embed.js` 标签；历史标签只作为迁移输入，不能继续运行。存在多个时先停止并请用户确认；存在一个时记录其 `data-viceme-work` 或 `data-creator-app-id` 及特性。缺少这两种可验证身份时停止，不得在确认归属前写入任何 Work、SDK access 或 Website Widget。
4. 复用 `$become-a-creator` 返回的 Merchant，不再运行独立商家选择逻辑，也不得从公开创作者身份或旧本地状态推断。
5. 页面已有当前 loader，或历史 loader 带 `data-viceme-work` 时，先运行 `viceme merchant work sdk-access list --merchant <merchant-id>`，按公开 `workKey` 精确定位 SDK access，再读取对应 Work。历史 `tip-embed.js` 只有 `data-creator-app-id` 时，先运行 `viceme merchant commerce-application get <application-id> --merchant <merchant-id>`，确认它是当前商家的 Website Widget 并从 `workId` 读取 Work。两条路径都只有在 `website.canonicalOrigin` 与部署 Origin 完全一致时才继续，否则在任何资源写入前停止。

   页面没有任何当前或历史 loader 时，运行 `viceme merchant work list --merchant <merchant-id>`，只保留 `kind: WEBSITE` 且 `website.canonicalOrigin` 与部署 Origin 完全一致的候选：

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

   运行 `viceme merchant work create --input <json>`。响应丢失时用同一 `clientRequestId` 原样重放；不得创建新身份。
6. 无论复用还是新建，都运行 `viceme merchant work get <work-id> --merchant <merchant-id>`。任何网站验证写入前先看状态：`SUSPENDED` 或 `ARCHIVED` 停止并报告；`DRAFT` 或 `PUBLISHED` 才继续。`website.ownershipStatus` 不是 `VERIFIED` 时，仅当当前执行仍持有 `website-verification create` 刚返回、未过期的 `PENDING` 响应才复用挑战；最新验证 GET 不含明文 `challenge`。否则读取最新 Work revision 并创建替代挑战：

   ```bash
   viceme merchant work website-verification create <work-id> \
     --merchant <merchant-id> --expected-revision <work-revision>
   ```

   把 `challenge` 原样发布到 `dnsRecordName`，公共 DNS 精确解析后运行：

   ```bash
   viceme merchant work website-verification verify <work-id> \
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

   运行 `viceme merchant work update <work-id> --input <json>`，随后再次读取 Work；已 `PUBLISHED` 时跳过更新。绝不猜测过期 revision。
8. 读取该 Work 唯一的 SDK access：

   ```bash
   viceme merchant work sdk-access get <work-id> \
     --merchant <merchant-id>
   ```

   缺失时同时创建 `danmaku` 和 `tip`；已存在时使用当前 `configVersion`，保留其他 access feature，并确保 hosted feature 同时包含 `danmaku` 和 `tip`：

   ```bash
   viceme merchant work sdk-access create <work-id> \
     --merchant <merchant-id> --feature danmaku --feature tip

   viceme merchant work sdk-access update <work-id> \
     --merchant <merchant-id> --expected-config-version <config-version> \
     --feature danmaku --feature tip
   ```

   更新命令必须一次提交完整 hosted feature 集合，不得执行两次独立更新互相覆盖。记录 access 返回的公开 `workKey`，并确认与页面已有值一致。
9. Website Widget 按 `(workId, environment, kind)` 唯一；本流程使用 `(workId, PRODUCTION, WEBSITE_WIDGET)` 这一份 Work 级共享资源。付费访问可能在其中保存 Shop 管理的 Product 绑定和 canonical Origin return URL；本 Skill 不把不同的 `displayName`、非空 `returnUrls` 或非空 `products` 视为赞赏配置不一致，不绑定或解绑 Product，也不修改现有 return URL。

   运行 `viceme merchant commerce-application list --merchant <merchant-id>`，定位作用域限定到本 Work、`kind: WEBSITE_WIDGET`、`environment: PRODUCTION` 的唯一应用。不存在时创建：

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

   运行 `viceme merchant commerce-application create --input <json>`。复用或新建后都运行 `viceme merchant commerce-application get <application-id> --merchant <merchant-id>` 复核。绝不因现有应用显示名、Origin、return URL 或 Product 不同就创建第二个。`REVOKED` 时停止并报告该终态资源。

   已有应用的 `origins` 包含 canonical Origin 时不更新配置；非空但缺少 canonical Origin 时停止并报告共享配置冲突，不得替换现有 Origin 集合。只有 `origins` 为空时才补写 canonical Origin。`ACTIVE` 且 `origins` 为空时停止并报告共享配置冲突，不得挂起正在服务付费访问的共享应用。对 `origins` 为空的 `DRAFT` 或 `SUSPENDED` 应用，按最新 revision 写更新输入：

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "expectedRevision": 2,
     "origins": ["https://creator.example"]
   }
   ```

   更新请求省略 `displayName` 与 `returnUrls`，其中省略 `returnUrls` 才会保留现有值；`"returnUrls": []` 会退役全部现有 URL，严禁在本流程中使用。`products` 不是更新字段。运行 `viceme merchant commerce-application update <application-id> --input <json>`，Origin 合同满足后，状态为 `DRAFT` 或 `SUSPENDED` 时按精确 revision 运行：

   ```bash
   viceme merchant commerce-application activate <application-id> \
     --merchant <merchant-id> --expected-revision <application-revision>
   ```

   已 `ACTIVE` 且包含 canonical Origin 时跳过激活。create 响应丢失时先 list 再决定。Engagement 不管理 Product 绑定；平台管理的绑定和 return URL 可以非空。
10. 页面只能保留一个官方加载器。页面目标特性集合非空时，当前 loader 复用并更新该标签，不得追加第二个；历史受管 loader 则替换为下面的唯一标签；新页面才插入标签。最终集合为空时删除受管 loader，不留下空标签。使用当前固定 Profile 的精确 `webBaseUrl`、`marketRegion` 和 SDK access 返回的 `workKey`，不得从页面语言推断或手工拼接其他地址：

    ```html
    <div id="viceme-interaction"></div>
    <script
      defer
      src="<web-base-url>/viceme-sdk/v1/viceme.min.js"
      data-viceme-work="<work-key>"
      data-viceme-region="<cn-or-global>"
      data-viceme-features="danmaku,tip"
      data-viceme-target="#viceme-interaction"
      data-viceme-theme="auto"
    ></script>
    ```

    `data-viceme-features` 必须严格为 `danmaku,tip`。宿主不得复制 ViceMe 组件、iframe、API client、持久化或支付代码。
11. 页面有 CSP 时，只为浏览器实际需要的指令加入精确 Shop Origin，保留 nonce 与其他规则，不使用 `*`、宽泛子域或 `unsafe-eval`。完成本地检查后部署真实页面，并在桌面和 320px 宽度验证：只挂载一个 SDK root、宿主控件可点击、键盘可达且减少动画生效；弹幕消息刷新后仍存在；赞赏支付可打开且 Escape 恢复初始金额表单；一个特性失败不会移除另一个。界面能打开不代表支付成交。
12. 报告公开 Work ID/key、应用 ID、规范 Origin、已执行检查和是否实际执行支付。不得报告登录 token、DNS 挑战、cookie、支付凭证、访问令牌或签名 URL。

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
