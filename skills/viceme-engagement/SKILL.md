---
name: viceme-engagement
description: 通过一个已验证的 Website Work、一个组合 SDK 访问资源和一个网站 Widget 应用，为公开网站接入 ViceMe 托管弹幕与打赏。
---

# 接入 ViceMe 弹幕与打赏

配置一个资源图、安装一个加载器。绝不分别为弹幕和打赏创建独立 Work，也不得复用测试数据、共享 `workKey` 或其他创作者的应用。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部协议字段。

## 流程

1. 查看准确的生产 HTTPS Origin、目标页面、部署命令、CSP、响应式布局与浏览器测试方式。
2. 运行 `viceme profile list` 并保持当前 Profile。运行 `viceme --profile <profile> auth status`，要求同时具备 `merchant-commerce:read` 与 `merchant-commerce:write`。然后从 `viceme --profile <profile> merchant accounts` 选择一个有效商家。
3. 运行 `viceme --profile <profile> merchant work list --merchant <merchant-id>`。只有当某 Website Work 的 `website.canonicalOrigin` 与部署 Origin 完全一致时才复用；没有时用以下严格输入创建（只填实际观察到的内容）：

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
4. 无论复用还是新建，都用 `viceme --profile <profile> merchant work get <work-id> --merchant <merchant-id>` 读取。任何网站验证写入前先看状态：`SUSPENDED` 或 `ARCHIVED` 停止并报告；`DRAFT` 或 `PUBLISHED` 才继续。

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
5. Work 状态为 `DRAFT` 时写入发布输入（`2` 替换为最新 revision）：

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "expectedRevision": 2,
     "status": "PUBLISHED"
   }
   ```

   运行 `viceme --profile <profile> merchant work update <work-id> --input <json>`，随后 `merchant work get` 复核。已 `PUBLISHED` 跳过更新；`SUSPENDED` 或 `ARCHIVED` 停止并报告，不得静默复活或建副本。绝不猜测 revision 或 DNS 挑战。
6. 读取 SDK 访问资源，创建或更新为完整特性集 `danmaku,tip`（特性用重复 flag）：

   ```bash
   viceme --profile <profile> merchant work sdk-access create <work-id> \
     --merchant <merchant-id> --feature danmaku --feature tip
   ```

   更新时还需传当前 `--expected-config-version`。绝不并行两个互相覆盖特性的独立更新。
7. 运行 `viceme --profile <profile> merchant commerce-application list --merchant <merchant-id>`。定位作用域限定到本 Work、`kind: WEBSITE_WIDGET`、`environment: PRODUCTION` 的唯一应用（`(workId, environment, kind)` 唯一）。不存在时创建：

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

   运行 `viceme --profile <profile> merchant commerce-application create --input <json>`。复用或新建后都用 `viceme --profile <profile> merchant commerce-application get <application-id> --merchant <merchant-id>` 复核。绝不因现有应用显示名、Origin 或 return URL 不同就创建第二个（会与唯一身份冲突）。状态 `REVOKED` 时停止并报告该终态资源。

   配置不同且状态 `ACTIVE` 时，先按其精确 revision 挂起：

   ```bash
   viceme --profile <profile> merchant commerce-application suspend <application-id> \
     --merchant <merchant-id> --expected-revision <application-revision>
   ```

   ```bash
     --merchant <merchant-id> --expected-revision <application-revision>
   ```

   挂起后复核。对配置不同的 `DRAFT` 或 `SUSPENDED` 应用，按最新 revision 运行 `viceme --profile <profile> merchant commerce-application update <application-id> --input <json>`：

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "expectedRevision": 2,
     "displayName": "<website name>",
     "origins": ["https://creator.example"],
     "returnUrls": []
   }
   ```

   运行 `update` 后复核。配置一致且状态为 `DRAFT` 或 `SUSPENDED` 时，按精确 revision 激活：

   ```bash
   viceme --profile <profile> merchant commerce-application activate <application-id> \
     --merchant <merchant-id> --expected-revision <application-revision>
   ```

   再用 `viceme --profile <profile> merchant commerce-application get <application-id> --merchant <merchant-id>` 复核。已 `ACTIVE` 且一致则跳过激活。create 响应丢失时先 list 再决定。网站 Widget 不绑定 Product。
8. 在稳定目标元素后插入一个组合加载器：

   ```html
   <div id="viceme-engagement"></div>
   <script
     defer
     src="<profile-web-base-url>/viceme-sdk/v1/viceme.min.js"
     data-viceme-work="<work-key>"
     data-viceme-region="<cn-or-global>"
     data-viceme-features="danmaku,tip"
     data-viceme-target="#viceme-engagement"
     data-viceme-theme="auto"
   ></script>
   ```

9. 保留现有 CSP 与 nonce；不得添加 `*` 或 `'unsafe-eval'`。运行仓库检查并部署，在桌面端和 320px 宽度验证：两个能力都挂载、一个失败不影响另一个、宿主控件可点击、弹幕刷新后仍在、打赏可用键盘到达、Escape 能把托管支付面板恢复到初始金额表单。商店拥有该状态迁移；SDK 只向宿主发送脱敏的关闭通知。

## 强制边界

- Work ID 是内部资源身份；`workKey` 是唯一的公开加载器身份。
- Profile 市场区域决定 `cn` 与 `global`；页面语言不参与。
- 不得绕过 CLI 直调 Shop API、写数据库、复制运行时源码，或在页面属性里放凭证。
- 汇报公开资源 ID、检查项、响应式覆盖与未验证的真实支付边界，不暴露秘密或 DNS 挑战值。
