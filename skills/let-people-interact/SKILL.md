---
name: let-people-interact
description: 为创作者作品接入或修复 ViceMe 托管互动。先复用 $become-a-creator 完成资格守卫，再区分仅弹幕、仅赞赏或弹幕加赞赏，并按对应的 Work、市场、Origin 与官方 Mounted/Headless SDK 边界完成接入。
---

# 接入 ViceMe 互动

首次调用 CLI 或判断 CLI 不存在前，必须先按 [creator-tools 的 CLI 定位流程](../creator-tools/SKILL.md#cli-定位) 查找并验证现有 CLI；后续使用返回的完整路径。不得只凭 `command not found` 判定未安装或改走免 CLI 流程。

这是一次有界的宿主接入，不是 SDK 研究或全盘项目调查。先确定分支和源码，再按阶段读取对应参考；没有进入某条路线时，不加载它的参考文件。

面向用户的文字跟随用户当前语言；中文使用自然白话，不展示登录凭据、支付详情或内部 handoff 字段。

## 执行纪律

- 不创建任务清单，也不为每条命令单独发送进度；只在需要用户选择、展示 SANDBOX 证据和最终完成时沟通。
- 用户应提供目标 URL 与源码目录、仓库或 ZIP；当前项目明确就是目标源码时可直接使用。URL 与当前项目不匹配时，最多读取一次平台提供的精确部署记录；仍不能定位就询问用户。不得递归扫描工作区之外的目录，也不得翻查历史 trace 猜源码。
- 同一阶段互不依赖的只读检查合并为一次并行工具调用。业务 CLI 读取完整结构化响应，不把输出管道到 `head`、`grep` 或其他会掩盖退出码的命令。
- Skill 的脚本、参考和模板是公共 SDK 行为的权威输入。不得下载、读取或反向分析 SDK 构建产物；公共契约不够时停止并报告 Skill 缺口。
- 使用当前 Agent 已有的浏览器能力。缺少浏览器时先取得用户同意或明确报告未验证边界，不得自动安装浏览器或其他全局工具。
- 常规 Mounted 新接入以不超过 25 次工具调用、15 次模型回复且不触发上下文压缩为目标；Headless 上限目标为 30 次工具调用、25 次模型回复。预算不允许跳过门禁；接近上限时应合并同阶段检查，遇到真实阻塞就停止并说明。

## 1. 固定分支与输入

| 分支 | 合格 Work | 市场 | 宿主实现 |
| --- | --- | --- | --- |
| 仅弹幕 | canonical Origin 与部署 Origin 精确匹配的 `PUBLISHED Website Work` | 当前 Profile 的 `cn` 或 `global` | `createViceMe` + `mountDanmaku` |
| 仅赞赏 | 当前 OWNER Merchant 拥有的公开 `PUBLISHED` 任意 kind Work | 仅 `marketRegion: cn` / CNY | `createViceMe` + `mountTip`，或 `createTip` Headless |
| 弹幕加赞赏 | canonical Origin 与部署 Origin 精确匹配的同一个 `PUBLISHED Website Work` | 仅 `marketRegion: cn` / CNY | 同一 target 挂载 Danmaku 与 integrated Tip，只显示一个底部互动栏 |

1. 用户已经明确分支时直接固定；确实含糊时才提问。把分支、源码和部署 Origin 等尚未确定且确实需要用户回答的输入合并为一次提问。
2. 仅赞赏默认使用官方 Mounted UI；只有用户明确要求自定义 Tip UI 时才进入 Headless，并在需要时一并确认 npm 或 CDN ESM。组合固定使用官方 integrated Mounted UI，不提供 Headless。不得为默认选择增加一轮提问，也不得在后续自行换路线。
3. 固定目标源码和部署方式。包含 Danmaku 时再固定精确小写 HTTPS 部署 Origin；仅赞赏只记录浏览器验收实际使用的宿主 URL，不把它变成 Work 或同域门禁。源码未定位前不运行平台写入。

三个分支均不创建、读取、验证或撤销 Website ownership verification，也不要求 DNS 或域名所有权验证。仅弹幕和组合要求 Website Work；仅赞赏不要求 Website kind、仓库、HTTPS Origin 或 Commerce Application，Tip 本身不增加 Origin 或 Commerce Application 门禁。

## 2. 资格、Profile 与发布物预检

1. 先运行一次无需业务写入的 `viceme profile list`，固定当前 Profile、API/Web base URL 与精确 `marketRegion`。页面 locale 不选择市场，也不切换 Profile。任意 Tip 路线遇到 GLOBAL 必须立即停止，且不得调用资格申请或留下其他业务写入；仅弹幕不受 CN/CNY 限制，保留当前区域。
2. 通过市场门禁后，第一项业务动作以玩法守卫模式调用 `$become-a-creator`；只复用它返回的有效 `MerchantAccountMember(role=OWNER)` Merchant。本 Skill 不重复登录、申请或商家选择。
3. 只运行一次 Skill 自带的发布物预检，将结果中的纯 semver `sdk_version` 和 `sdk_origin` 固定到本次接入：

   ```bash
   node "<skill-dir>/scripts/preflight-sdk-release.mjs" \
     --route <danmaku|tip|combined> \
     --region <cn|global>
   ```

   该命令失败就停止。不得再次解析版本、改用 alias、复制 SDK 源码或另行探测构建产物。详细检查边界见 [发布物预检](references/release-preflight.md)。

## 3. Work 与 SDK access

只在本阶段读取 [公共资源边界](references/integration-contract.md) 和 [Work 与 SDK access](references/work-and-access.md)。按照已固定分支选择或创建 Work；需要 Danmaku 时复核 Website kind、`status: PUBLISHED` 与 canonical Origin，只有 Tip 时复核用户明确选择的任意 kind Merchant Work。

对 SDK access 先读取完整快照，再一次性合并本次 hosted features：仅弹幕加入 `--feature danmaku`，仅赞赏加入 `--feature tip`，组合同时加入两者。写后重读并确认完整 `features`、完整 `accessFeatures`、状态、`keys.test`、`keys.live` 与精确 `configVersion`；永久公开 key 不得轮换。

完成标准：Work 归属、kind、状态与 Origin 满足当前分支，SDK access 为预期完整集合，并已保存写前快照用于失败恢复。

## 4. 修改宿主页

先一次性检查目标文件中现有 ViceMe runtime、CSP、Referrer Policy 与真实卸载边界，然后只读取所选 UI 路线：

- Mounted：读取 [Mounted 接入](references/mounted.md)。仅赞赏单 HTML 可从 [single-html.html](templates/single-html.html) 取片段；仅弹幕使用 [mounted-danmaku.html](templates/mounted-danmaku.html)；组合使用 [mounted-combination.html](templates/mounted-combination.html)。
- Headless：只适用于仅赞赏，并读取 [Headless Tip 接入](references/headless.md)；组合不得生成 Headless Tip 控件。

所有路线先写入 `keys.test`，只使用本次固定的 `sdk_version` 和 `sdk_origin`。修改尽量一次完成；随后一次静态检查确认无占位符、无第二套 runtime、无不兼容 CSP/Referrer Policy，且一个能力失败不会移除另一个成功能力。组合还必须确认只有一个 target、没有正文 Tip 卡片或宿主 Headless 控件。

## 5. SANDBOX 与 LIVE

1. 部署 test key 版本后，在一个浏览器会话中完成当前路线的 SANDBOX 验收；按所选参考的矩阵批量检查挂载、官方请求、控制台、桌面、320px 与键盘。Mounted 不重复测试 SDK 内部 Headless 协议；Headless 才运行 Local Fake、弹窗阻止与 Referer fail-closed。
2. 展示 SANDBOX 证据和未验证边界，取得用户明确确认后，才把唯一 `createViceMe` 中的 `keys.test` 替换为同一 access 的 `keys.live`。不改变 SDK 版本、imports、Work 或 UI 路线。
3. 重新部署同一源码并用一个浏览器会话复核 LIVE key、挂载和官方请求。production key 不能模拟支付；真实支付必须另行取得明确同意，未执行时如实报告。

## 6. 完成与恢复

报告分支、Work ID/kind、Profile market、UI 路线、变更文件、部署 URL、SANDBOX/LIVE 检查及未执行的真实支付；Work key 是公开标识，但无需重复完整展示。不报告 token、cookie、订单 capability、handoff token 或支付详情。

SDK access 写入后若宿主接入失败，按 [Work 与 SDK access](references/work-and-access.md) 用最新 `configVersion` 恢复写前完整快照。响应丢失先读取同一资源；revision/configVersion 冲突先重读，不猜版本，也不创建平行资源。
