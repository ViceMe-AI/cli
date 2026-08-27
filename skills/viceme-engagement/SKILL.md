---
name: viceme-engagement
description: 通过完整的 CLI 流程，为一个公开网站同时接入 ViceMe 托管弹幕和打赏。适用于一起集成、测试或修复弹幕与打赏，并只使用一个互动嵌入脚本的请求。
---

# 同时接入 ViceMe 弹幕和打赏

通过 CLI 创建创作者自己的配置，并且只安装一段 CLI 生成的组合脚本。不得复用测试数据、共享 `workKey` 或其他创作者的应用。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部协议字段。

## 流程

1. 查看目标页面、准确的 HTTPS 主机名、部署命令、CSP 和浏览器测试方式，保留宿主现有布局与部署路径。
2. 运行 `viceme profile list`。后续所有命令固定使用一个 Profile，并确认其 `apiBaseUrl`、`webBaseUrl` 和 `marketRegion`。
3. 运行 `viceme --profile <profile> auth status`。未登录或缺少 `sdk-work:read`、`sdk-work:write`、`creator-app:read`、`creator-app:write` 时，在同一 Profile 运行 `auth login`。
4. `.viceme/access.yaml` 不存在时运行：

   ```bash
   viceme --profile <profile> access init --name "<website name>" --danmaku
   ```

   已存在时运行 `access inspect`。只有本地配置确实需要对齐时才运行 `access apply`。记录 `data.workKey`；不得因为 apply 失败就创建第二个 Work。
5. 在同一 Profile 运行 `creator-app list`。只有应用属于当前创作者并包含准确主机名时才能复用，否则运行 `creator-app create --name "<website name>"`。
6. 主机名尚未验证时，运行 `creator-app domain add`，把返回的 token 原样部署到指定验证路径，部署后请求该 HTTPS URL，再运行 `creator-app domain verify`。不得打印或提交 token。
7. 读取目标页面实际使用的语言（`zh-CN` 或 `en-US`），不得按 Profile 市场猜测。然后生成权威组合片段：

   ```bash
   viceme --profile <profile> creator-app show <app-id> --work-key <work-key> --locale <zh-CN-or-en-US>
   ```

   只在 `</body>` 前或框架等效脚本入口插入一次 `data.engagementEmbedSnippet`。不得手工拼接标签、推导来源、替换区域，也不得在旁边保留独立 `tip-embed.js` 或 `viceme.min.js`。
8. 保留现有 CSP，仅添加生成片段准确需要的脚本和 frame 来源。不得加入通配符或 `unsafe-eval`。
9. 运行仓库检查并部署，在桌面端和 320px 宽度验证真实主机名：只加载一个脚本、弹幕刷新后仍存在、打赏弹窗能打开、Escape 能关闭、焦点能返回。除非实际执行了支付，否则明确说明真实支付尚未验证。

## 强制边界

- 不得直接调用 Shop API、写数据库、伪造 ID 或从浏览器存储读取凭证。
- 不得复制 React 蓝图、为组件添加宿主样式或安装第二套 Runtime。
- 所有命令始终固定使用开始时选择的 Profile。
- 遇到结构化 CLI 错误立即停止，不得切换环境作为兜底。
