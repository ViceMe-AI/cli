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
2. 运行 `viceme --profile <profile> auth status`。未登录或缺少 `sdk-work:read`、`sdk-work:write` 时，运行 `viceme --profile <profile> auth login` 并等待完成。
3. `.viceme/access.yaml` 不存在时运行：

   ```bash
   viceme --profile <profile> access init --name "<website name>" --danmaku
   ```

   文件已存在时运行 `viceme --profile <profile> access inspect`。若本地配置已启用，但权威响应没有 `data.embedSnippet`，只运行一次 `viceme --profile <profile> access apply` 进行对齐。不得手改配置，也不得创建第二个 Work。
4. 从成功响应读取 `data.embedSnippet`，只在 `</body>` 前或框架等效脚本入口插入一次。CLI 会根据所选 Profile 的 `webBaseUrl` 生成 URL；不得猜测、拼接、替换或退回生产地址。
5. 保留插入脚本现有的逐响应 CSP nonce。只添加 `data.embedSnippet` 实际需要的脚本和 frame 来源；不得添加 `*`、`'unsafe-eval'` 或宽泛的 ViceMe 子域通配符。
6. 运行目标项目的格式化、lint、类型检查、测试和生产构建。
7. 在桌面端和移动端检查真实页面：只挂载一个 SDK 根节点、宿主控件仍可点击、键盘操作正常、减少动画设置生效、发送的消息刷新后仍存在，而且没有重复加载脚本。

完成标准是目标项目只有一份 CLI 生成的脚本接入，并且浏览器检查使用同一个固定 Profile 通过。

## 边界

- `workKey` 是公开且不透明的标识，不得替换为 creator ID、product ID、slug、token 或凭证。
- 托管 SDK 负责渲染、iframe 位置、公开 API 请求、持久化和清理。
- 宿主只负责脚本位置和自身页面内容。
- 打赏由 `viceme-tip` 处理，不属于本 Skill。
- 自托管或复制组件源码不属于本 Skill。

## 交付说明

报告修改文件、所选 Profile、公开 `workKey`、执行过的检查、响应式和键盘覆盖，以及尚未验证的 CSP 或托管 Runtime 边界。不得报告任何凭证。
