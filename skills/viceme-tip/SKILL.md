---
name: viceme-tip
description: 为一个公开的创作者 HTML 页面接入 ViceMe 打赏。适用于添加、安装、嵌入、配置或修复打赏入口，包括登录、绑定创作者作品与域名，以及验证真实托管结账页。
---

# 接入 ViceMe 打赏

实现最小且真实的创作者接入：一个宿主拥有的 HTML 页面，加一段 ViceMe 嵌入脚本。修改前完整阅读 [integration-contract.md](references/integration-contract.md)。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部协议字段或敏感值。

## 流程

1. 运行 `viceme profile list`，记录当前 Profile，不要切换。后续所有 CLI 命令固定使用它。完成标准：Profile 和区域明确。
2. 运行 `viceme --profile <profile> auth status`，要求同时具备 `merchant-commerce:read` 与 `merchant-commerce:write`。未登录或缺 scope 时运行 `viceme --profile <profile> auth login`，只展示一次完整授权链接并等待成功。完成标准：所选 Profile 返回 `authenticated: true`。
3. 查看目标仓库说明，找到一个公开 HTML 页面、其线上 HTTPS 地址、部署方式和 CSP。优先复用已有页面；只有用户需要新示例时才使用 [single-html.html](templates/single-html.html)。完成标准：准确 HTML 文件和公开主机名已知。
4. 运行 `viceme --profile <profile> merchant accounts`，选择一个有效商家；多个时请用户选择。不得从公开创作者身份或旧本地状态推断。
5. 运行 `viceme --profile <profile> merchant work list --merchant <merchant-id>`。只有 `website.canonicalOrigin` 与部署 Origin 完全一致时才复用 Website Work；否则用严格输入创建（只填实际观察到的内容）：

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
6. 无论复用还是新建，都用 `viceme --profile <profile> merchant work get <work-id> --merchant <merchant-id>` 读取。任何网站验证写入前先看状态：`SUSPENDED` 或 `ARCHIVED` 停止并报告；`DRAFT` 或 `PUBLISHED` 才继续。`website.ownershipStatus` 不是 `VERIFIED` 时：仅当当前执行仍持有 `website-verification create` 刚返回、未过期的 `PENDING` 响应才复用挑战（最新验证 GET 不含明文 `challenge`），否则读取最新 Work revision 创建替代挑战：

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

   运行 `viceme --profile <profile> merchant work update <work-id> --input <json>`，随后 `merchant work get` 复核；已 `PUBLISHED` 跳过更新。绝不猜测过期 revision。
8. 读取该 Work 的 SDK 访问（`sdk-access`）。缺失时用 `--feature tip` 创建；已存在时按当前 `configVersion` 与完整特性集更新——已启用 `danmaku` 要保留。记录返回的公开 `workKey`。
9. 运行 `viceme --profile <profile> merchant commerce-application list --merchant <merchant-id>`。定位作用域限定到本 Work、`kind: WEBSITE_WIDGET`、`environment: PRODUCTION` 的唯一应用（`(workId, environment, kind)` 唯一）。不存在时创建：

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "workId": "<work-id>",
     "kind": "WEBSITE_WIDGET",
     "environment": "PRODUCTION",
     "displayName": "<website name>",
     "origins": ["https://creator.example"],
     "returnUrls": []
   }
   ```

   运行 `viceme --profile <profile> merchant commerce-application create --input <json>`。复用或新建后都运行 `viceme --profile <profile> merchant commerce-application get <application-id> --merchant <merchant-id>` 复核。绝不因现有应用显示名、Origin 或 return URL 不同就创建第二个（会与唯一身份冲突）。配置不同且状态 `ACTIVE` 时，先按其精确 revision 挂起：

   ```bash
   viceme --profile <profile> merchant commerce-application suspend <application-id> \
     --merchant <merchant-id> --expected-revision <application-revision>
   ```

   挂起后复核。对配置不同的 `DRAFT` 或 `SUSPENDED` 应用，按最新 revision 写更新输入：

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "expectedRevision": 2,
     "displayName": "<website name>",
     "origins": ["https://creator.example"],
     "returnUrls": []
   }
   ```

   运行 `viceme --profile <profile> merchant commerce-application update <application-id> --input <json>`，配置一致后按精确 revision 运行：

   ```bash
   viceme --profile <profile> merchant commerce-application activate <application-id> \
     --merchant <merchant-id> --expected-revision <application-revision>
   ```

   已 `ACTIVE` 且一致则跳过激活。create 响应丢失时先 list 再决定。`REVOKED` 时停止并报告该终态资源。网站 Widget 不绑定 Product。
10. 只插入一个官方加载器标签，使用所选 Profile 的精确 Web 地址和市场区域，不从页面语言推断：

   ```html
   <div id="viceme-engagement"></div>
   <script
     defer
     src="<web-base-url>/viceme-sdk/v1/viceme.min.js"
     data-viceme-work="<work-key>"
     data-viceme-region="<cn-or-global>"
     data-viceme-features="tip"
     data-viceme-target="#viceme-engagement"
     data-viceme-theme="auto"
   ></script>
   ```

   保留全部宿主内容；交付物必须是单 HTML 文件时，把宿主自己的 CSS/JavaScript 保持内联。
11. 页面有 CSP 时只为浏览器确实需要的指令加入精确 Shop Origin，保留 nonce 与其他规则。先做本地语法和静态检查，再沿仓库既有路径部署。必须验证公开 HTTPS 页面：Tip 框架能自适应、键盘可达、能打开支付、Escape 恢复初始金额表单、控制台无 CSP 或 widget 错误。商店先完成关闭，SDK 只发送脱敏的 `viceme:widget-close` 通知；宿主页面无需关闭监听器。界面能打开不代表支付已成交。
12. 报告公开 Work ID/key、应用 ID、规范 Origin、已执行检查和是否实际执行了支付。不得报告登录 token、DNS 挑战、cookie、支付凭证、访问令牌或签名 URL。

## 边界

- 创作者登录用于配置作品；访问者仅打开打赏结账页时不需要登录。
- ViceMe 负责结账、支付渠道调用、订单状态和结算；宿主页面只负责自身内容和嵌入标签。
- `workKey` 标识收款作品，公开且不透明。可复用模板使用占位符；创作者自己的线上作品可以使用真实值。
- 域名验证位于单 HTML 嵌入表面之外，但属于前置条件。最终接入仍可保持单 HTML 文件，即使部署还需单独提供验证响应。
- 默认使用独立加载器。需要弹幕+打赏组合时使用 `viceme-engagement`。

## 恢复

- 登录超时：在同一 Profile 重跑 `auth login`。
- 已有未验证域名：复用并重新部署当前 token，不创建另一个 Work。
- 已有已验证 Work：复用其 `workKey` 与 Widget 应用，不得仅因 HTML 移动就创建重复项。
- 主机名错误：绑定准确的生产主机名，不得混用 localhost、预览地址和生产地址。
- 组件未出现：依次检查脚本状态、Creator App ID、主机名绑定、CSP 和重复脚本标签。
