<h1 align="center">ViceMe CLI</h1>

<p align="center">
  <strong>在熟悉的 Agent 对话中，把本地 AI Agent Skill 变成可发布的商品。</strong>
</p>

<p align="center">
  一次安装 CLI 与官方 Agent Skills，然后直接让 Codex、Claude Code 或 WorkBuddy 完成后续流程。
</p>

<p align="center">
  <a href="./README.md">English</a> ·
  <a href="#快速开始">快速开始</a> ·
  <a href="#官方-agent-skills">Agent Skills</a> ·
  <a href="#命令参考">命令参考</a> ·
  <a href="#安全边界">安全边界</a>
</p>

## 为什么使用 ViceMe CLI？

- **Agent 原生**——用自然语言描述目标，不需要记住一整套发布命令和状态流转。
- **一次安装**——原生 CLI 与匹配版本的官方 Skills 一起安装到 Codex、Claude
  Code、WorkBuddy，并写入 `~/.agents/skills` 兼容目录。
- **由用户控制发布**——模型可以建议双语文案和素材，但价格与最终公开发布只能由
  用户决定。
- **确定且可恢复**——CLI 在本地校验和打包；中断后继续同一个发布任务，响应丢失
  也不会重复创建商品。
- **同时适合 Agent 和自动化**——稳定 JSON、错误码、Dry Run 和明确状态让每个动作
  都可检查、可复现。
- **默认安全**——设备码授权、绑定 Origin 的 Profile、本地 Secret 检查、不可变摘要
  和已验证上传共同保护发布链路。

## 能做什么

| 能力 | ViceMe 提供的内容 |
| --- | --- |
| 发布 Skill | 校验本地 Skill 目录或 ZIP、设置人民币价格、上传、审核平台建议，并发布付费商品。 |
| 构建组件 | 让 Agent 把随包提供的生产级弹幕蓝图接入 React 与 Tailwind CSS v4 项目。 |
| 配置 Agent | 把 CLI 与官方 Skills 作为同一个兼容版本安装、登录、更新、诊断和修复。 |
| 安全恢复 | 网络或进程中断后继续原发布任务，不重复上传和创建商品。 |

## 快速开始

### 使用 AI Agent（推荐）

1. 一次安装 CLI 与官方 Skills：

   ```bash
   curl -fsSL https://s3.viceme.cn/start/install.sh | sh
   ```

2. 新建一个 Codex、Claude Code 或 WorkBuddy 对话，让 Agent 重新发现刚安装的
   Skills。
3. 附上本地 Skill 目录或 ZIP，然后直接说：

   > 帮我把这个 Skill 以人民币 1 元发布到 ViceMe。

Agent 会先检查登录状态，在整个流程中固定使用同一个 Profile，校验 Skill 后立即
上传私有草稿并打开真实创作者预览，然后再询问价格。ViceMe 完成双语文案和素材建议后，
Agent 会把完整成品和图片直接展示出来，最后只询问一次是否确认并立即公开发布。

```text
本地 Skill → 登录 → 校验并私有上传 → 创作者预览 → 定价
           → 平台分析 → 最终成品 → 确认并发布 → 公开商品链接
```

最初的“帮我发布”不等于授权公开上架。只有完整审核稿已经展示、用户明确确认后，
商品才会公开。

### 直接使用终端

```bash
# 检查安装和当前账户。
viceme doctor
viceme auth status

# 仅在未登录时发起浏览器登录。
viceme auth login

# 定价前先上传真实私有草稿，并打开创作者预览。
viceme skill publish --path ./my-skill

# 在同一草稿上设置人民币 1 元，并继续媒体上传与分析。
viceme skill publish --resume <publication-id> --price-minor 100
```

## 安装

官方 Bootstrap 从同一个不可变 Release 一次安装原生 CLI 和全部官方 Agent Skills。

### macOS 或 Linux

中国区：

```bash
curl -fsSL https://s3.viceme.cn/start/install.sh | sh
```

海外：

```bash
VICEME_REGION=global sh -c "$(curl -fsSL https://s3.viceme.ai/start/install.sh)"
```

### Windows PowerShell

中国区：

```powershell
irm https://s3.viceme.cn/start/install.ps1 | iex
```

海外：

```powershell
$env:VICEME_REGION="global"; irm https://s3.viceme.ai/start/install.ps1 | iex
```

不能使用 Bootstrap 时，以 npm 作为备用入口：

```bash
npx --yes @viceme-ai/cli@latest install
```

安装器始终写入兼容目录 `~/.agents/skills`，并为检测到的 Agent 写入原生用户目录：

| Agent | 原生目录 |
| --- | --- |
| Codex | `~/.codex/skills` |
| Claude Code | `~/.claude/skills` |
| WorkBuddy | `~/.workbuddy/skills` |

可通过 `viceme install --agent codex`、`claude`、`workbuddy` 或 `agents` 指定
目标。安装或修复后运行 `viceme doctor`。

## 官方 Agent Skills

| Skill | 适用场景 |
| --- | --- |
| `viceme-shared` | 安装 ViceMe、通过浏览器登录、管理 Profile、更新、诊断或修复本地环境。 |
| `viceme-publish` | 把本地 Skill 目录或 ZIP 校验、上传、审核、恢复或发布为 ViceMe 付费商品。 |
| `viceme-danmaku` | 在已有项目中构建或适配随包提供的生产级 React 与 Tailwind CSS v4 弹幕组件。 |

Agent Skills 负责对话流程和授权规则；CLI 负责确定性本地操作与 API 调用。因此 Agent
可以解释每一步决策，而相同的命令契约仍可在终端或自动化中复现。

## Skill 发布流程

当前发布流程接受一个包含根级 `SKILL.md` 的本地目录，或一个本地 ZIP。常见的、只有
一层外包装目录的 GitHub ZIP 会被自动规范化。不接受远程 URL 和多 Skill 批量包。

| 阶段 | 责任边界 |
| --- | --- |
| 登录 | 受保护 API 操作前完成浏览器授权；所有后续命令固定使用同一个 Profile 与 API Origin。 |
| 校验 | 本地拒绝危险路径、特殊文件、超限内容、敏感文件和常见 Secret 模式。 |
| 私有上传与预览 | 用户发起发布即授权上传私有草稿；真实包验证后先打开创作者预览。 |
| 定价 | 用户随后在同一 Publication 上以“分”为单位提供人民币价格。 |
| 平台分析 | ViceMe 建议中英文短简介、中英文使用说明、封面和有序画廊；建议不能替代用户决定。 |
| 图文审核 | Agent 展示精确文案、价格、封面和所有画廊图片。短简介最大显示宽度为 30：ASCII 计 1，中文及其他非 ASCII 计 2。 |
| 公开发布 | 用户只需做一次最终明确确认；随后完成审核确认并立即、不可逆地公开上架。 |

上传或响应中断后，继续原来的 Publication：

```bash
viceme --profile <publication-profile> skill publish --resume <publication-id>
```

不能因为上一次响应未知就创建第二个 Publication，应先查询或恢复原 ID。

## 登录与 Profile

每个 Profile 绑定一个区域、一个 API Endpoint 和一个通过设备码授权的账户。没有
自定义 Endpoint 的 Profile 使用对应区域的 ViceMe 官方 API。

```bash
viceme auth login
viceme auth status
viceme auth logout

viceme profile list
viceme profile use default
```

连接测试环境或私有部署时，使用通用 HTTPS 占位地址创建独立 Profile，再登录：

```bash
viceme profile add \
  --name private-cn \
  --region cn \
  --api-base-url https://api.example.com \
  --use
viceme auth login
```

`VICEME_API_BASE_URL` 只是单进程 CI / 调试覆盖，不是 Profile 状态。远程自定义
Endpoint 必须使用 HTTPS；只有 localhost 和 loopback 本地开发可以使用 HTTP。
凭据按 Profile 和 API Origin 隔离；Agent 不能因为另一个 Profile 已登录就擅自切换。

`viceme auth login` 会一直等待，直到浏览器授权完成或达到有界超时。Agent 必须保持
命令运行，向用户展示一次性完整链接并等待最终结果。页面会在必要时先完成登录，随后
自动授权 CLI；用户不需要输入设备码。等待超时后重新运行 `viceme auth login` 发起新
流程。不要在对话中复制 Access Token。

## 命令参考

| 命令 | 用途 |
| --- | --- |
| `viceme version` | 显示 CLI 与随包 Skills 版本。 |
| `viceme doctor` | 检查 CLI、当前 Profile、凭据、API readiness 和已安装官方 Skills。 |
| `viceme auth status` | 显示当前 Profile 是否已登录。 |
| `viceme access init --name <名称> [--follow key] [--product slug --purchase key]` | 单命令创建、配置、激活并应用创作者网页接入；购买能力未指定作品时，唯一作品会自动绑定，多个作品会返回候选项。 |
| `viceme profile list` | 显示 Profile 及其实际 API Endpoint。 |
| `viceme skill inspect --path <path>` | 无副作用校验本地 Skill。 |
| `viceme skill listing prepare --path <path>` | 创建或恢复稳定的创作者私有预览，并保存本地绑定。 |
| `viceme skill listing get <listing-id>` | 读取权威的私有 Listing 状态。 |
| `viceme skill listing bind <listing-id> --path <path>` | 将来源明确绑定到用户选定且拥有的 Listing。 |
| `viceme skill publish --path <path>` | 定价前上传真实私有包并返回创作者预览。 |
| `viceme skill publish --resume <id> --price-minor <fen>` | 为同一 Draft 定价、上传媒体候选并启动分析。 |
| `viceme publication wait <id>` | 等待后台分析，不重复上传。 |
| `viceme publication review <id>` | 读取权威双语文案、价格、选定素材和审核状态。 |
| `viceme publication asset upload ...` | 确认前替换或新增封面、画廊图片。 |
| `viceme publication update ...` | 用严格 JSON 文件替换完整 Listing Draft。 |
| `viceme publication confirm ...` | 确认当前精确 Review Digest。 |
| `viceme publication publish ...` | 公开已经确认的 Listing。 |
| `viceme update` | 同时更新 CLI 与匹配版本的官方 Skills。 |

运行 `viceme <command> --help` 查看完整参数和 JSON 字段。

## 输出与更新

业务结果默认使用 JSON。成功时 stdout 只包含最终结果，进度和诊断写入 stderr。
失败使用非零退出码和稳定的 `error.code`，因此 Agent 与自动化不需要解析 message 文本。

```json
{
  "ok": true,
  "data": {},
  "meta": {
    "executingCliVersion": "<version>",
    "requestId": "optional"
  }
}
```

`meta.executingCliVersion` 表示输出本次响应的进程版本。对于 `viceme update`，
新安装版本单独由 `data.cli_version` 返回，因为最终响应仍由启动更新的旧进程输出。

正式安装最多每 24 小时读取一次权威发布渠道。发现新版本后，普通 JSON 响应通过
`_notice.update` 返回当前版本、最新版本和 `viceme update`。检查失败不会改变业务
命令的退出码。

```bash
viceme update --check
viceme update
```

更新器会校验精确 Release、刷新匹配版本的官方 Skills，并把中断的激活过程恢复成
一个完整、兼容的本地版本。

## 安全边界

- 本地打包拒绝路径穿越、绝对路径、符号链接、特殊文件、超限内容、敏感文件和常见
  Secret 模式。
- API 独立校验不可变 ZIP、Digest 与对象元数据，不信任客户端自报结果。
- 凭据保存在 CLI 的本地安全存储中，并绑定 Profile 与 API Origin，不进入 Agent
  对话状态。
- Pending operation 不保存预签名上传 URL。
- 模型分析只接收筛选后的文本、元数据和图片缩略图。
- 公开发布前必须展示并明确授权当前精确文案、价格、封面和有序画廊。

## 开发

需要 Go 1.23+；npm 包检查还需要 Node.js 22+。

```bash
make check
make npm-package-check
make release-manifest
```

CLI 与官方 Agent Skills 同版本发布。GitHub、npm、`s3.viceme.cn` 和
`s3.viceme.ai` 的产物来自同一个已评审 Commit。
