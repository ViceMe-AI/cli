---
name: viceme-danmaku
description: 把已由 $viceme-publish 发布并启用弹幕的 Website Work 接入创作者网站，复用官方浏览器 SDK、宿主页面和现有 loader；不登录创作者，也不创建或修改平台资源。
---

# 接入 ViceMe 弹幕

只修改创作者网站的宿主代码。Website Work、域名验证、`danmaku` 能力和创作者资格全部由 `$viceme-publish` 负责。开始前完整阅读 [integration.md](references/integration.md)。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部协议字段或敏感值。

## 流程

1. 查看仓库说明、页面入口、部署路径、现有 ViceMe loader、组件系统、CSP 和浏览器测试。确认弹幕不遮挡宿主主要操作。
2. 确认 `$viceme-publish` 已返回公开 `workKey`、精确 Web base URL、区域、canonical Origin 和包含 `danmaku` 的完整 hosted feature set。缺少发布结果时立即交回 publish；不得在本 Skill 中登录、申请创作者、创建 Work、验证 DNS 或配置 SDK access。
3. 一个页面只保留一个官方 loader。已有同一 `workKey` 的 loader 时更新它，不要插入第二个。弹幕单独启用时使用 `data-viceme-features="danmaku"`；同时启用打赏时规范化为 `data-viceme-features="danmaku,tip"`。
4. 使用宿主已有页面和目标元素。默认目标可为 `body`；已有明确容器时复用。不得复制 ViceMe iframe、消息持久化、API client 或限流逻辑。
5. 页面有 CSP 时，只把 publish 返回的精确 Shop Origin 加入实际需要的 `script-src`、`connect-src` 和 `frame-src`，保留 nonce 与其他规则；不得加入通配符或 `unsafe-eval`。
6. 运行仓库检查并验证真实公开页面的桌面与移动状态：只挂载一个 SDK root、宿主控件仍可点击、键盘可用、减少动画生效，并确认弹幕消息刷新后仍存在。
7. 报告修改文件、公开页面、使用的 `workKey`、hosted feature set、已执行检查和未验证运行时边界。不得报告 token、DNS 挑战、Merchant ID 或其他内部资源 ID。

## 嵌入示例

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

如果 publish 返回的 hosted set 同时包含打赏，把 `data-viceme-features` 改为 `danmaku,tip`，仍然只保留这一条 loader。

## 边界

- 本 Skill 不拥有发布配置；不得运行任何创作者登录、Merchant、Website Work、域名验证或 SDK access 写命令。
- `workKey` 公开且不透明。不得替换为 Work UUID、Merchant ID、Product ID、slug、token 或凭证。
- Website Work 和 verified Origin 拥有身份；弹幕只是一项 hosted feature，不是登录、权益或支付策略。
- 与打赏共存时共享一个 loader 和一个客户端，不得重复挂载。
- Shop 拥有弹幕渲染、消息持久化、限流、键盘和减少动画行为；宿主不得复制这些逻辑。

## 恢复

- 缺少或禁用 `danmaku`：交回 `$viceme-publish`，不要在宿主端修补。
- 组件未出现：检查 loader URL、`workKey`、feature set、目标元素、CSP 和重复脚本。
- 与打赏重复加载：合并为一个 loader，并使用规范顺序 `danmaku,tip`。
