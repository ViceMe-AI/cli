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
- **默认安全**——浏览器授权、绑定 Origin 的 Profile、本地 Secret 检查、不可变摘要
  和已验证上传共同保护发布链路。

## 能做什么

| 能力 | ViceMe 提供的内容 |
| --- | --- |
| 发布 Skill | 校验本地 Skill 目录或 ZIP、设置人民币价格、上传、审核平台建议，并发布付费商品。 |
| 发布网站 | 使用稳定 Website Work 身份登记并发布创作者网站；只有商务、关注或付费访问能力需要时才另行验证所有权。 |
| 接入创作者访问 | 在原站接入登录、关注门槛和按功能独立定价的一次性访问权限。 |
| 接入托管互动 | 先分支：弹幕使用已发布且 canonical Origin 精确匹配的 Website Work；开放赞赏可使用任意合格且已发布的 Merchant Work；组合共用同一个 Website Work；三个分支均不要求 DNS 所有权验证。 |
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

Agent 会先检查登录状态，在整个流程中固定使用同一个 Profile，校验 Skill 后立即
上传私有草稿并打开真实创作者预览，不询价就继续上传候选媒体。当前用户的 Agent 随后
完成双语文案和素材建议，先把完整作品详情和图片直接展示出来，再把价格和详情修改合并成一个
问题；应用用户答案后展示最终成品，最后只询问一次是否确认并立即公开发布。

```text
本地 Skill → 登录 → 校验并私有上传 → 创作者预览 → Agent 补全
           → 完整详情与价格合并询问 → 最终成品
           → 确认并发布 → 公开商品链接
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
viceme skill publish --path ./my-skill --edition-key my-skill --edition-order 0

# 继续同一个未定价草稿并上传候选媒体。
viceme skill publish --resume <publication-id>
viceme publication review <publication-id>

# Agent 提交带 Draft revision 保护的建议；平台分析只作显式 fallback。
viceme publication suggest <publication-id> --input <suggestion.json>
# viceme publication analyze <publication-id> && viceme publication wait <publication-id>

# 查看完整作品详情后，在同一个草稿上设置人民币 1 元。
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
| `creator-tools` | 安装 ViceMe、通过浏览器登录、管理 Profile、更新、诊断或修复本地环境。 |
| `become-a-creator` | 为所有创作者玩法申请或检查创作者资格。 |
| `customize-your-page` | 用 AI 创建、预览、发布、更新或回滚作者页和作品页，并按目标调用平台接口。 |
| `sell-a-skill` | 发布或更新可下载的付费或免费 Skill；网站、服务与普通商品不属于此 Skill。 |
| `use-a-skill` | 解析免费、已购买或待购买的访问状态，安装所选 Skill，并继续原任务。 |
| `charge-for-your-work` | 为现有网站配置关注或付费解锁并接入宿主代码；复用统一创作者资格检查，平台资源保持为内部实现。 |
| `let-people-interact` | 分支处理仅弹幕、开放赞赏或两者；包含弹幕的路线要求已发布且 canonical Origin 精确匹配的 Website Work，Tip 可使用任意合格且已发布的 Merchant Work。默认使用 Mounted UI，仅在用户明确要求自定义 UI 时进入 Headless；三个分支均不要求 DNS 所有权验证。 |
| `let-others-make-a-copy` | 发布包含根级 `VICEME-REPLICA.md` 的完整网站源码 ZIP，并把平台返回的“做同款”提示词接入创作者原站；不处理买家结算与安装。 |
| `let-me-make-a-copy` | 接受网站做同款邀请；直接读取区域化 S3 托管的官方 Skill 与 Python 3.9+ 标准库脚本，有 CLI 时复用账号或匿名购买，并优先保留既有 standalone 恢复。 |

买家侧 `use-a-skill` 与 `let-me-make-a-copy` 不属于创作者玩法，因此保持独立名称。

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

每个 Profile 把 API Base URL、Web Base URL、市场区域和浏览器授权账户绑定为一个完整
authority；单独的分发区域只选择 CLI 与官方 Skills 的下载来源。

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
  --web-base-url https://www.example.com \
  --market-region cn \
  --use
viceme auth login
```

`VICEME_API_BASE_URL` 只是 API-only 的单进程 CI / 调试覆盖，不是 Profile 状态；
设置时禁止持久化登录。远程自定义
Endpoint 必须使用 HTTPS；只有 localhost 和 loopback 本地开发可以使用 HTTP。
凭据按完整 Profile authority 隔离；Agent 不能因为另一个 Profile 已登录就擅自切换。

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
| `viceme profile list` | 显示 Profile 的 API、Web 与市场 authority。 |
| `viceme skill inspect --path <path>` | 无副作用校验本地 Skill。 |
| `viceme skill listing prepare --path <path>` | 创建或恢复稳定的创作者私有预览，并保存本地绑定。 |
| `viceme skill listing get <listing-id>` | 读取权威的私有 Listing 状态。 |
| `viceme skill listing bind <listing-id> --path <path>` | 将来源明确绑定到用户选定且拥有的 Listing。 |
| `viceme skill publish --path <path> --edition-key <key> --edition-order <order>` | 明确新增或更新的 Skill 条目，定价前上传真实私有包并返回创作者预览。 |
| `viceme skill publish --resume <id>` | 继续同一个未定价 Draft 并上传媒体候选，不启动平台模型。 |
| `viceme publication review <id>` | 读取权威双语文案、价格、选定素材和审核状态。 |
| `viceme publication suggest <id> --input ...` | 以 Draft revision 保护提交 Agent 生成的双语文案与媒体选择。 |
| `viceme publication analyze <id>` | 当前 Agent 无法完成补全时，显式请求平台模型分析。 |
| `viceme publication wait <id>` | 等待已经显式请求的平台分析，不重复上传。 |
| `viceme skill publish --resume <id> --price-minor <fen>` | 在完整详情审核后，为同一个 Draft 写入人民币价格，不新建 Listing。 |
| `viceme publication asset upload ...` | 替换用户明确选择的媒体；加 `--candidate-only` 可暂存由 Agent 提供、随后交给 `publication suggest` 选择的媒体。 |
| `viceme publication update ...` | 用严格 JSON 文件替换完整 Listing Draft。 |
| `viceme publication confirm ...` | 确认当前精确 Review Digest。 |
| `viceme publication publish ...` | 公开已经确认的 Listing。 |
| `viceme replica preview [--path <项目> \| --url <回环地址>]` | 登录或上传前，在匿名 ViceMe 预览壳中打开本地网页；CLI 启动的 dev server 仅存活到命令退出，视觉验证以打开的浏览器为准。 |
| `viceme replica publish ...` | 冻结项目工作树（或校验已有 ZIP）、生成根级 `VICEME-REPLICA.md`，发布不可变版本并返回源码摘要、稳定口令与自站提示词。 |
| `viceme replica install <口令> [--target <新目录>]` | 已登录 Profile 先展示真实 Quote，明确确认后追加 `--confirm`；匿名购买则追加 `--accept-price-cents <分>`，仅在托管支付页打开后使用 `--payment-presented`。两条路径都会原子安装已支付源码。 |
| `viceme update` | 显式更新 CLI；官方 Skills 使用 `viceme install --agent auto` 单独刷新。 |
| `viceme merchant accounts` | 列出当前 User 通过 OWNER 成员关系经营的普通 MerchantAccount。 |
| `viceme merchant work ...` | 创建、查看、更新和发布 Merchant Work，包括 Website Work。 |
| `viceme merchant page ...` | 校验、预览、发布、查看和回滚不可变的作者页/作品页自定义页面包。 |
| `viceme merchant work website-verification ...` | 创建、查看、验证或撤销一个 Website Work 的 DNS 所有权。 |
| `viceme merchant work sdk-access ...` | 在同一 Work 上管理托管 `danmaku`/`tip` 与关注/付费 access；两类配置相互保留，创建会返回永久公开、并非凭据的 `keys.test` 与 `keys.live`。 |
| `viceme merchant commerce-application ...` | 管理 Commerce Application；对于开放赞赏，匹配的应用只是可选可信来源归因，不是授权门禁。 |
| `viceme merchant product ...` | 列出、暂停或归档 Product（可下载 Skill 版本与网站访问权益）。 |
| `viceme commerce ...` | 安装并运行签名购买 Skill，完成会话、报价、支付订单与同会话状态查询。 |

运行 `viceme <command> --help` 查看完整参数和 JSON 字段。

承载赞赏 UI 的页面与被赞赏 Work 是两个独立资源。三个互动分支均不要求 Website
ownership 或 DNS 验证。仅接入赞赏不会把宿主页登记为 Website Work；仅弹幕和组合
仍使用 canonical Origin 与部署 Origin 精确匹配的已发布 Website Work。当前版本的网站关注/付费
解锁同样不要求 DNS TXT 或登记嵌入 Origin。

组合路线只因弹幕要求而共用一个已发布 Website Work，并在同一 target 内只显示一个含赞赏入口的
底部互动栏，不再生成正文 Tip 卡片或 Headless Tip 控件。Tip 本身不增加域名或
Commerce Application 门禁，互动流程也不创建或修改 Website Widget。网站关注/付费
访问及其平台托管 `WEBSITE_WIDGET` 资源由收费接入流程和 Shop 在内部完成，但暂不执行 Origin/DNS
或 checkout iframe 宿主门禁。另行配置的域名绑定 `HOSTED_CHECKOUT` 与签名 Webhook 仍保留所有权
验证规则。

没有匹配可选可信来源归因时，ViceMe 仍会把实际浏览器来源记录为未验证 Origin，
不会因此拒绝开放赞赏。

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
启动恢复恰好需要把已经启动的旧命令交给新版本时，响应中可能出现
`meta.autoUpdate`；它不再是每次自动更新的正常结果。

普通命令输出响应后，CLI 才拉起独立后台进程检查稳定发布渠道，并且只更新 CLI。
前台命令不等待版本发现或激活；后台进程启动失败、断网、权限不足或安装失败都不会
替换本次 JSON 响应。成功替换后，下一次 `viceme` 调用会使用新版本，无需重启整个
Codex、Claude Code 或 WorkBuddy。成功检查会合并 24 小时，失败一小时后可再次尝试。

官方 Skills 使用独立生命周期，因为 Agent 通常在任务启动时加载 Skill。CLI 自动更新
不会改写 Skill 目录；需要刷新时运行 `viceme install --agent auto`，宿主需要重新发现
文件时再新开一个 Agent 任务。

```bash
viceme update --check
viceme update
viceme install --agent auto
```

`viceme update` 保留为显式 CLI 修复命令，并继续使用权限预检与持久化恢复。
为兼容旧调用，`viceme update --agent <target>` 仍可请求 CLI 与 Skill 联合修复；新的
自动化应把 CLI 更新和 Skill 安装拆成两条命令。

CLI 会在创建更新事务前检查当前安装方式需要修改的全部路径。npm 安装通过 Node 检查
实际缓存、全局包及命令入口目录，保留宿主原有的文件权限代理；独立 CLI-only 更新
只检查可执行文件目录，单独安装 Skill 时才检查所选 Agent 目录。显式更新预检发现权限
不足时保留原有完整版本并申请宿主授权；后台权限失败只记录到本地，不影响前台命令。
预检之后权限仍可能变化；一旦已经尝试安装，失败时会保留恢复记录，等待启动恢复。

显式更新或未完成事务的恢复遇到权限不足时，返回 `UPDATE_PERMISSION_REQUIRED`
（退出码 6，不可自动重试）。Agent 必须通过宿主提供的正式审批机制申请权限，真正
获批后才重试 `viceme update`。聊天里的口头同意不等于系统权限已经开放。宿主拒绝、
没有授权入口，或授权后仍不能操作时，停止更新；不得删恢复日志、卸载 CLI 或绕过限制。
`error.details.recovery_required` 或目标状态 `recovery_pending` 表示必须先完成恢复，
不能假定旧安装完整可用。错误响应不包含 npm 原始输出或凭据。

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

需要 Go 1.23+ 与 Python 3.9+；npm 包检查还需要 Node.js 22+。

```bash
make check
make npm-package-check
make release-manifest
```

CLI 与官方 Agent Skills 同版本发布。GitHub、npm、`s3.viceme.cn` 和
`s3.viceme.ai` 的产物来自同一个已评审 Commit。
