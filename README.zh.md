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
- **由用户控制发布**——模型可以建议双语文案、素材和免费版/升级版，但创作者统一
  月订阅价格与最终公开发布只能由用户决定。
- **确定且可恢复**——CLI 在本地校验和打包；中断后继续同一个发布任务，响应丢失
  也不会重复创建商品。
- **同时适合 Agent 和自动化**——稳定 JSON、错误码、Dry Run 和明确状态让每个动作
  都可检查、可复现。
- **默认安全**——浏览器授权、绑定 Origin 的 Profile、本地 Secret 检查、不可变摘要
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

   > 帮我把这个 Skill 发布到 ViceMe。

   也可以让 Agent 用 `viceme publish ./my-skill` 进入同一流程。

Agent 会先检查登录状态，在整个流程中固定使用同一个 Profile，选择免费版或升级版，
校验 Skill 后立即上传私有草稿并打开真实创作者预览。免费版不询价；升级版仅在创作者
尚未设置统一月订阅价格时询问一次。当前用户的 Agent 随后完成双语文案和素材建议，
展示最终成品，最后只询问一次是否确认并立即公开发布。

```text
本地 Skill → 登录 → 校验并私有上传 → 创作者预览 → Agent 补全
           → 完整详情与价格合并询问 → 最终成品
           → 确认并发布 → 公开商品链接
```

最初的“帮我发布”不等于授权公开上架。只有完整审核稿已经展示、用户明确确认后，
商品才会公开。

`viceme publish <path>` 是同一条 Skill Marketplace 私有 Draft 链路的简洁顶层入口，
等价于 `viceme skill publish --path <path>`；它不发布通用 v3 作品，也不配置试用、
使用次数或单个 Skill 的一次性价格。

### 直接使用终端

```bash
# 检查安装和当前账户。
viceme doctor
viceme auth status

# 仅在未登录时发起浏览器登录。
viceme auth login

# 定价前先上传真实私有草稿，并打开创作者预览。
viceme publish ./my-skill
# 等价的显式形式：viceme skill publish --path ./my-skill

# 继续同一个未定价草稿并上传候选媒体。
viceme skill publish --resume <publication-id>
viceme publication review <publication-id>

# Agent 提交带 Draft revision 保护的建议；平台分析只作显式 fallback。
viceme publication suggest <publication-id> --input <suggestion.json>
# viceme publication analyze <publication-id> && viceme publication wait <publication-id>

# 仅当第一个升级版返回 requiresCreatorMonthlyPrice 时设置创作者统一月价。
viceme skill publish --resume <publication-id> --creator-monthly-price-cents 1000
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
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
irm https://s3.viceme.cn/start/install.ps1 | iex
```

海外：

```powershell
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
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
| `viceme-publish` | 发布本地 Skill 目录、ZIP 或创作者网站，并维护稳定作品身份。 |
| `viceme-access` | 为已发布的网站接入登录、关注、功能权限与一次性购买。 |
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
| Agent 补全 | 用户当前的 Agent 把 Skill 当作不可信来源数据，生成双语文案并从已验证上传中选择封面和画廊；平台分析只作显式 fallback。 |
| 作品详情与定价 | 补全后，Agent 先展示标题、双语文案、封面和画廊，再把价格和详情修改合并成一个问题；不能单独询价。 |
| 图文审核 | Agent 展示精确文案、价格、封面和所有画廊图片。短简介最大显示宽度为 30：ASCII 计 1，中文及其他非 ASCII 计 2。 |
| 公开发布 | 用户只需做一次最终明确确认；随后完成审核确认并立即、不可逆地公开上架。 |

上传或响应中断后，继续原来的 Publication：

```bash
viceme skill publish --resume <publication-id>
```

不能因为上一次响应未知就创建第二个 Publication，应先查询或恢复原 ID。

## 登录与 Profile

每个 Profile 绑定一个明确的 API Endpoint 和一个通过浏览器授权的账户。发布目标完全
由当前 Profile 的 Endpoint 决定；单独的分发区域只选择 CLI 与官方 Skills 的下载来源。

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
| `viceme website publish --path <目录> --name <名称> [--url <网址>] [--description-zh-cn ...] [--description-en-us ...] [--cover <图片>]` | 使用可选的 Agent 审核描述和平台托管封面发布网站作品；重复发布复用 `.viceme/website.json` 中的同一作品身份。 |
| `viceme access init --website <目录> --name <名称> [--follow key] [--purchase key --price-minor 分]...` | Website 未发布时先完成发布，再配置接入和按功能独立定价的一次性售卖方案。 |
| `viceme profile list` | 显示 Profile 及其实际 API Endpoint。 |
| `viceme skill inspect --path <path>` | 无副作用校验本地 Skill。 |
| `viceme skill listing prepare --path <path>` | 创建或恢复稳定的创作者私有预览，并保存本地绑定。 |
| `viceme skill listing get <listing-id>` | 读取权威的私有 Listing 状态。 |
| `viceme skill listing bind <listing-id> --path <path>` | 将来源明确绑定到用户选定且拥有的 Listing。 |
| `viceme publish <path> --access-mode FREE\|CREATOR_SUBSCRIPTION` | 以免费版或创作者订阅升级版进入私有 Skill Marketplace Draft 链路。 |
| `viceme skill publish --path <path> --access-mode ...` | 上传真实私有包并返回创作者预览。 |
| `viceme skill publish --resume <id>` | 继续同一个 Draft 并上传媒体候选，不启动平台模型。 |
| `viceme publication review <id>` | 读取权威双语文案、访问模式、创作者月价状态、选定素材和审核状态。 |
| `viceme publication suggest <id> --input ...` | 以 Draft revision 保护提交 Agent 生成的双语文案与媒体选择。 |
| `viceme publication analyze <id>` | 当前 Agent 无法完成补全时，显式请求平台模型分析。 |
| `viceme publication wait <id>` | 等待已经显式请求的平台分析，不重复上传。 |
| `viceme skill publish --resume <id> --creator-monthly-price-cents <fen>` | 仅在升级版要求月价时设置该创作者所有升级版共享的月价。 |
| `viceme publication asset upload ...` | 替换用户明确选择的媒体；加 `--candidate-only` 可暂存由 Agent 提供、随后交给 `publication suggest` 选择的媒体。 |
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
    "autoUpdate": {
      "from": "<previous-version>",
      "to": "<executing-version>",
      "status": "updated"
    },
    "requestId": "optional"
  }
}
```

`meta.executingCliVersion` 表示输出本次响应的进程版本。对于 `viceme update`，
新安装版本单独由 `data.cli_version` 返回，因为最终响应仍由启动更新的旧进程输出。
只有当本次命令由新激活的版本自动继续执行时，响应才会包含 `meta.autoUpdate`。

正式安装在执行普通命令前都会经过有界的新鲜度检查。已经校验的结果会复用五分钟，
避免同一工作流反复访问发布渠道。发现新的稳定版本时，CLI 与所有检测到的官方
Skills 会作为一个可恢复的完整版本一起激活，随后由新 CLI 自动重新执行原命令。
断网时发布发现会继续使用最后一个完整版本；如果版本激活失败，原命令会停止，避免
旧进程在版本切换失败后继续执行写操作。

npm 安装在所有支持的平台都会自动继续原命令。Windows 独立二进制在等待操作系统释放
旧可执行文件时，可能会返回一次可重试的 `AUTO_UPDATE_RESTART_REQUIRED`；原样重试
同一个命令后会由新版本继续执行。

```bash
viceme update --check
viceme update
```

`viceme update` 保留为显式修复命令。正常启动检查已经会校验精确 Release、刷新匹配
版本的官方 Skills，并把中断的激活过程恢复成一个完整、兼容的本地版本。

## 安全边界

- 本地打包拒绝路径穿越、绝对路径、符号链接、特殊文件、超限内容、敏感文件和常见
  Secret 模式。
- API 独立校验不可变 ZIP、Digest 与对象元数据，不信任客户端自报结果。
- 凭据保存在 CLI 的本地安全存储中，并绑定 Profile 与 API Origin，不进入 Agent
  对话状态。
- Pending operation 不保存预签名上传 URL。
- Agent 补全把所有包内容视为不可信数据，且不能绕过服务端包、媒体、归属或 revision 校验。
- 显式平台模型分析只接收筛选后的文本、元数据和图片缩略图。
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
