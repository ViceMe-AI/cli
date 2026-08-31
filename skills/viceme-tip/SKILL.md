---
name: viceme-tip
description: 把已由 $viceme-publish 发布并启用打赏的 Website Work 接入创作者网站，复用官方浏览器 SDK 和宿主页面样式；不登录创作者、不验证域名，也不创建或修改平台资源。
---

# 接入 ViceMe 打赏

只修改创作者网站的宿主代码。Website Work、域名验证、`tip` 能力、Website Widget 应用和创作者资格全部由 `$viceme-publish` 负责。修改前完整阅读 [integration-contract.md](references/integration-contract.md)。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部协议字段或敏感值。

## 流程

1. 查看目标仓库说明，找到公开页面、部署路径、现有 ViceMe loader、组件系统和 CSP。优先复用已有页面；只有用户需要新示例时才使用 [single-html.html](templates/single-html.html)。
2. 确认 `$viceme-publish` 已返回公开 `workKey`、精确 Web base URL、区域、canonical Origin 和包含 `tip` 的完整 hosted feature set。缺少任何发布结果时立即交回 publish；不得在本 Skill 中登录、申请创作者、创建 Work、验证 DNS、配置 SDK access 或管理 CommerceApplication。
3. 一个页面只保留一个官方 loader。已有同一 `workKey` 的 loader 时更新它，不要插入第二个。`tip` 单独启用时使用 `data-viceme-features="tip"`；同时启用弹幕时规范化为 `data-viceme-features="danmaku,tip"`。
4. 在宿主现有位置提供打赏入口或目标容器，复用现有布局、焦点和响应式规则。不得复制 ViceMe 结账、登录、订单或支付渠道代码。
5. 页面有 CSP 时，只把 publish 返回的精确 Shop Origin 加入实际需要的 `script-src`、`connect-src` 和 `frame-src`，保留 nonce 与其他规则；不得加入通配符或 `unsafe-eval`。
6. 运行仓库检查并验证公开 HTTPS 页面：Tip 框架能自适应、键盘可达、能打开支付、Escape 恢复初始金额表单、控制台无 CSP 或 widget 错误。界面能打开不代表支付已成交。
7. 报告修改文件、公开页面、使用的 `workKey`、hosted feature set、已执行检查，以及是否实际执行真实支付。不得报告登录 token、DNS 挑战、应用 ID、cookie、支付凭证或签名 URL。

## 嵌入示例

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

如果 publish 返回的 hosted set 同时包含弹幕，把 `data-viceme-features` 改为 `danmaku,tip`，仍然只保留这一条 loader。

## 边界

- 本 Skill 不拥有发布配置；不得运行任何创作者登录、Merchant、Website Work、域名验证、SDK access 或 Website Widget 写命令。
- 访问者打开打赏结账页前不需要登录。ViceMe 负责结账、支付渠道、订单状态和结算；宿主只负责自身内容和 loader。
- `workKey` 公开且不透明。宿主不得传金额、支付渠道、应用 ID、访问令牌或支付状态。
- SDK 只接受预期 Shop Origin 和 iframe 窗口的消息。宿主不得根据浏览器消息或回跳参数声称支付成功。
- 与弹幕共存时共享一个 loader 和一个客户端，不得重复挂载。
- 静态检查不能代替真实 HTTPS、CSP、微信和支付渠道验证。

## 恢复

- 缺少或禁用 `tip`：交回 `$viceme-publish`，不要在宿主端修补。
- 主机名或 Widget 应用不匹配：交回 publish 核对 canonical Origin。
- 组件未出现：检查 loader URL、`workKey`、feature set、目标元素、CSP 和重复脚本。
- 支付未完成：保持宿主状态不变，只以 ViceMe 托管界面的权威结果为准。
