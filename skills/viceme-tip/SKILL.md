---
name: viceme-tip
description: 为一个公开的创作者 HTML 页面接入 ViceMe 打赏。适用于添加、安装、嵌入、配置或修复打赏入口，包括登录、绑定创作者作品与域名，以及验证真实托管结账页。
---

# 接入 ViceMe 打赏

实现最小且真实的创作者接入：一个宿主拥有的 HTML 页面，加一段 ViceMe 嵌入脚本。修改前完整阅读 [integration-contract.md](references/integration-contract.md)。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部协议字段或敏感值。

## 流程

1. 运行 `viceme profile list`，记录当前 Profile，不要切换。后续所有 CLI 命令固定使用它。完成标准：Profile 和区域明确。
2. 运行 `viceme --profile <profile> auth status`。未登录时运行 `viceme --profile <profile> auth login`，只展示一次完整授权链接并等待成功。完成标准：所选 Profile 返回 `authenticated: true`。
3. 查看目标仓库说明，找到一个公开 HTML 页面、其线上 HTTPS 地址、部署方式和 CSP。优先复用已有页面；只有用户需要新示例时才使用 [single-html.html](templates/single-html.html)。完成标准：准确 HTML 文件和公开主机名已知。
4. 在 HTML 中查找已有 ViceMe 打赏脚本和 `data-creator-app-id`，并与 `viceme --profile <profile> creator-app list` 交叉核对。只有 Creator App ID 属于当前用户且应用已经验证准确的公开主机名，才算完成绑定；单有脚本标签不能证明绑定。
5. 尚未绑定时通过 CLI 创建和绑定，不得要求用户去 Creator Center 操作：
   1. 运行 `viceme --profile <profile> creator-app create --name <work-name>`，记录 `data.app.id`。首次调用会自动建立创作者身份，不得绕去处理“还不是创作者”。
   2. 运行 `viceme --profile <profile> creator-app domain add <appId> <hostname>`，记录域名项的 `verificationToken` 和 `verificationPath`。
   3. 通过 HTTPS 在 `<hostname><verificationPath>` 原样提供 token 并部署。
   4. 运行 `viceme --profile <profile> creator-app domain verify <appId> <hostname>`。失败时自行请求验证 URL，与已记录 token 比较，修复部署后重试；不要让用户点击。
   完成标准：域名项返回 `verified: true`。
   如果平台对 `creator-app` 命令返回 404，停止并说明当前所选环境尚未部署绑定能力；不得伪造 Work 或 URL，也不得退回生产地址。
6. 读取目标页面实际使用的语言（`zh-CN` 或 `en-US`），不得按 Profile 市场猜测。运行 `viceme --profile <profile> creator-app show <appId> --locale <zh-CN-or-en-US>` 生成嵌入片段，并原样使用 `data.embedSnippet`。CLI 会根据 Profile 的 `webBaseUrl` 生成它，不得自行推导脚本 URL 或 Creator App ID。只在 `</body>` 前插入一份可幂等更新的脚本：

```html
<script
  async
  src="<generated-widget-script-url>"
  data-creator-app-id="<creator-app-id>"
  data-locale="<zh-CN-or-en-US>"
></script>
```

   保留全部宿主内容；交付物必须是单 HTML 文件时，把宿主自己的 CSS/JavaScript 保持内联。完成标准：页面恰好包含一份匹配的嵌入脚本。
7. 页面有 CSP 时，只为 `script-src` 和 `frame-src` 加入浏览器实际需要的准确平台来源，保留其他指令。不得使用通配符或弱化无关规则。完成标准：浏览器能加载嵌入脚本和结账 iframe，且无 CSP 错误。
8. 先做本地语法和静态检查，再沿仓库既有路径部署。必须验证公开 HTTPS 页面而不只是 localhost：脚本返回 200、圆形打赏入口可见、键盘能打开 ViceMe 结账页、Escape 能关闭、控制台和网络没有组件错误。完成标准：真实已验证主机名能打开托管结账页。
9. 报告 HTML 文件、公开 URL、Creator App ID、验证 URL、已执行检查和尚未验证的支付边界。不得报告登录 token、验证 token、cookie、支付凭证或签名 URL。

## 边界

- 创作者登录用于配置作品；访问者仅打开打赏结账页时不需要登录。
- ViceMe 负责结账、支付渠道调用、订单状态和结算；宿主页面只负责自身内容和嵌入标签。
- `creatorAppId` 标识收款作品。可复用模板使用占位符；创作者自己的线上作品可以使用真实 ID。
- 域名验证位于单 HTML 嵌入表面之外，但属于前置条件。最终接入仍可保持单 HTML 文件，即使部署还需单独提供验证响应。
- 默认使用独立入口。只有线上嵌入脚本明确记录并证明协议时，才使用自定义组合入口。

## 恢复

- 登录超时：在同一 Profile 重跑 `auth login`。
- 已有未验证域名：复用并重新部署当前 token，不创建另一个 Work。
- 已有已验证 Work：复用 Creator App ID，不得仅因 HTML 移动就创建重复项。
- 主机名错误：绑定准确的生产主机名，不得混用 localhost、预览地址和生产地址。
- 组件未出现：依次检查脚本状态、Creator App ID、主机名绑定、CSP 和重复脚本标签。
