---
name: viceme-tip
description: 为创作者公开网站发布或复用 Website Work 并接入 ViceMe 托管赞赏。先复用 $viceme-creator-onboarding 检查资格，再完成域名验证、平台配置和宿主嵌入。
---

# 接入 ViceMe 赞赏

实现一个真实的创作者网站和一段 ViceMe 嵌入脚本。创作者登录、申请和商家选择全部交给
`$viceme-creator-onboarding`。修改前完整阅读
[integration-contract.md](references/integration-contract.md)。

面向用户的文字跟随当前语言；中文使用自然白话，不展示内部协议字段或敏感值。

## 流程

1. 第一项业务动作以资格守卫模式调用 `$viceme-creator-onboarding`。只有它返回当前用户的
   有效商家后才继续，并复用该 Merchant；不得自行运行登录、申请或商家选择命令。
2. 查看目标仓库说明，找到公开 HTML 页面、线上 HTTPS Origin、部署方式和 CSP。优先复用
   已有页面；只有用户需要新示例时才使用 [single-html.html](templates/single-html.html)。
3. 运行 `viceme merchant work list --merchant <merchant-id>`。只有
   `website.canonicalOrigin` 与部署 Origin 完全一致时才复用 Website Work；否则用稳定的
   `clientRequestId` 创建 `kind: WEBSITE` 的 Work。只填从真实页面观察到的 slug、标题、
   canonical Origin、简介和内容；响应丢失时原样重放，不创建第二个 Work。
4. 使用 `viceme merchant work get <work-id> --merchant <merchant-id>` 读取权威状态。
   `SUSPENDED` 或 `ARCHIVED` 时停止；只有 `DRAFT` 或 `PUBLISHED` 才继续。
5. 网站所有权未验证时，运行 `viceme merchant work website-verification create` 并按当前
   Work revision 创建 Website Verification，把返回的 DNS TXT challenge 原样部署到准确
   记录名。公共 DNS 精确解析后运行 `viceme merchant work website-verification verify`，
   按返回的 verification version 验证，
   再重新读取 Work。不得为了找回 challenge 创建第二个 Work。
6. Work 为 `DRAFT` 时，以最新 revision 更新为 `PUBLISHED` 并读回；已发布时跳过。任何
   publication 写入前都展示准确 Website 候选并取得用户明确确认。
7. 读取该 Work 的 `sdk-access`。缺失时创建 `tip` feature；已存在时按当前
   `configVersion` 更新并保留用户未要求删除的关注和付费访问功能。记录公开 `workKey`。
8. 列出当前 Merchant 的 Commerce Applications，定位本 Work、`WEBSITE_WIDGET`、
   `PRODUCTION` 的唯一应用（`(workId, environment, kind)` 唯一）：
   - 不存在时，用 Work、准确 Origin、空 return URLs 创建；
   - 已存在且配置相同时复用；
   - `ACTIVE` 但配置不同时，先按精确 revision 挂起，再更新并激活；
   - `DRAFT` 或 `SUSPENDED` 时按精确 revision 更新并激活；
   - `REVOKED` 是终态，立即停止；
   - create 响应丢失时先 list，再决定，不创建平行应用。
   激活时使用 `viceme merchant commerce-application activate` 并携带准确 revision。
9. 在 `</body>` 前插入一份官方加载器，使用当前 CLI 上下文的精确 Web 地址、市场和上一步
   返回的 `workKey`；不得按页面语言推断市场，也不得自行拼接另一个环境：

   ```html
   <div id="viceme-tip"></div>
   <script
     defer
     src="<web-base-url>/viceme-sdk/v1/viceme.min.js"
     data-viceme-work="<work-key>"
     data-viceme-region="<cn-or-global>"
     data-viceme-features="tip"
     data-viceme-target="#viceme-tip"
     data-viceme-theme="auto"
   ></script>
   ```

10. 页面有 CSP 时只增加浏览器实际需要的准确 Shop Origin，保留 nonce 和其他规则。沿仓库
    既有路径部署并验证公开 HTTPS 页面：入口可见、键盘可达、能打开托管结账、Escape 能
    关闭、控制台无 CSP 或 widget 错误。界面能打开不代表支付成交。
11. 报告公开 Work ID/key、应用 ID、规范 Origin、已执行检查和是否真实支付；不得报告登录
    token、DNS challenge、cookie、支付凭证或签名 URL。

## 边界与恢复

- 创作者登录只用于配置；访客打开赞赏结账时不需要创作者登录。
- ViceMe 负责结账、支付、订单和结算；宿主只负责内容与嵌入标签。
- `workKey` 公开且不透明，标识收款作品；网站 Widget 不绑定普通商品发布流程。
- 已有未验证域名时复用并重新部署当前 challenge；已有已验证 Work 时复用其 Work、
  `workKey` 与 Widget 应用。
- 登录、资格或申请问题交回 `$viceme-creator-onboarding`，不得在本 Skill 内重跑登录。
- 主机名错误时绑定准确生产 Origin，不混用 localhost、预览地址和生产地址。
- 组件未出现时依次检查脚本、workKey、Origin 绑定、CSP 和重复加载器。
