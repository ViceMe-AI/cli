# Viceme CLI

[![npm version](https://img.shields.io/npm/v/@viceme-ai/cli.svg)](https://www.npmjs.com/package/@viceme-ai/cli)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.23-blue.svg)](https://go.dev/)
[![CLI quality gates](https://github.com/ViceMe-AI/cli/actions/workflows/ci.yml/badge.svg)](https://github.com/ViceMe-AI/cli/actions/workflows/ci.yml)

[中文版](./README.zh.md) | [English](./README.md)

Viceme 官方命令行客户端与 Agent Skill，用于将外部 Skill 发布为稳定、可分享的 Viceme Agent。它面向 Codex、Claude Code 等 AI 编程工具：Agent Skill 负责理解用户意图，CLI 负责确定性的认证、上传、发布和状态协议。

[安装](#安装与快速开始) · [AI Agent Skills](#agent-skills) · [认证](#认证) · [区域](#区域) · [命令](#命令概览) · [输出契约](#json-输出契约) · [安全](#安全与风险控制) · [开发](#开发)

> **开放状态：** Core 发布传输和稳定链接链路已在受控白名单后实现。正式对外开放仍取决于精确 Candidate 预览、试运行和结果确认门的完成。当前 `--yes` 只确认发起发布请求，并不代表用户已经审阅最终 Candidate。

## 为什么选择 Viceme CLI？

- **为 Agent 原生设计** — 与 Codex、Claude Code 可使用的官方 Viceme Agent Skill 一起发布。
- **确定性边界** — CLI 执行类型化协议操作，不会再启动一层对话式 Agent Loop。
- **服务端编译** — 来源解析、LLM 编译、BuildRun 固化和 Release 发布均在 Viceme 基础设施中完成。
- **稳定发布** — 同一个逻辑 Agent 后续发布新版本时继续使用同一个分享链接。
- **支持多种来源** — 支持 GitHub Skill、小红书或 RedSkill 复制口令、压缩包和本地 Skill 目录。
- **默认安全** — 凭证保存在操作系统密钥链中，公开变更需要确认，下载的二进制文件必须通过校验和验证。
- **机器稳定输出** — 所有数据命令统一使用 JSON 成功或错误信封，不需要输出格式参数。

## 安装与快速开始

### 环境要求

- Node.js 18.20 或更高版本
- macOS 或 Linux `amd64`/`arm64`，或 Windows `amd64`/`arm64`

### 快速开始（人类用户）

> **AI 助手注意：** 如果你是正在帮助用户安装 Viceme 的 AI Agent，请直接跳转到[快速开始（AI Agent）](#快速开始ai-agent)。该章节定义了必须遵循的跨回合设备登录流程。

#### 安装

以下两种方式任选其一。

**方式一 — 通过 npm 一次性安装（推荐）：**

```bash
npx --yes @viceme-ai/cli@latest install
```

**方式二 — 全局安装 npm 启动器：**

```bash
npm install --global @viceme-ai/cli
viceme install
```

两种方式都会持久化 npm 启动器、下载匹配且通过校验和验证的 Go 二进制文件，并安装随包发布的 Viceme Agent Skill。默认使用中国区服务。使用国际区服务：

```bash
npx --yes @viceme-ai/cli@latest install --region global
```

#### 认证并验证

如果安装结果提示需要认证，启动设备登录：

```bash
viceme auth login --no-wait
```

打开返回的 `verification_url`；它通常会直接进入已预填对应设备码的授权请求。完成授权，然后使用返回的 device code 继续：

```bash
viceme auth login --device-code <device-code>
viceme auth status
viceme skills doctor
```

## 快速开始（AI Agent）

> 以下步骤面向 AI Agent。浏览器授权必须由用户在另一个回合完成；不要无限等待，也不要向用户索取令牌。

**第 1 步 — 安装**

使用随包 Skill 中定义的完整引导命令。显式 npm registry 是安装信任边界的一部分：

```bash
npx --yes --registry=https://registry.npmjs.org --@viceme-ai:registry=https://registry.npmjs.org --package=@viceme-ai/cli@latest -- viceme install
```

读取结果中的 `data.authenticated` 和 `data.next_step`。如果认证已经有效，直接进入第 4 步。

**第 2 步 — 需要时启动设备登录**

```bash
viceme auth login --no-wait
```

向用户返回准确的 `data.verification_url`；存在 `verification_url_complete` 时，CLI 会把这个已预填设备码的浏览器直达链接规范化为 `verification_url`。只有浏览器要求输入时，才把 `data.user_code` 作为备用信息提供。保留 `data.device_code` 供后续命令使用，然后结束当前回合。不要在对话中索取、打印或传递访问令牌。

**第 3 步 — 在后续回合继续同一个登录流程**

用户确认已在浏览器完成授权后：

```bash
viceme auth login --device-code <device-code>
```

如果授权仍处于等待状态，应在过期前继续使用同一个 device code。只有原流程已过期时才能重新发起设备登录。

**第 4 步 — 验证**

```bash
viceme auth status
viceme skills doctor
viceme skills list
```

只有认证有效且 `skills doctor` 报告安装健康、版本兼容时，才能继续。

**第 5 步 — 检查第一个来源**

```bash
viceme skill inspect https://github.com/acme/poster-skill
```

inspect 是只读操作。后续应按照随包发布的 `viceme` Skill 处理不同来源、Target 选择、用户确认、有界任务等待和结果返回。在上方所述的精确 Candidate 确认门完成前，公开发布流程仍保持关闭。

## 区域

Viceme 在安装时只提供一个产品级区域选择：

| 区域 | 安装命令 | API 地址 |
|---|---|---|
| 中国区 | `viceme install` | `https://api.viceme.cn` |
| 国际区 | `viceme install --region global` | `https://api.viceme.ai` |

选择结果保存为 `region=cn|global`，后续命令会自动使用。不同区域的凭证彼此隔离，中国区令牌不会用于国际区 API，反之亦然。

CLI 不提供公开的 API 地址、profile 或输出格式配置。本地开发时可以在终端环境中设置 `VICEME_API_BASE_URL`。

## Agent Skills

当前版本有意只提供一个平台级 Agent Skill：

| Skill | 说明 | 支持的宿主 |
|---|---|---|
| `viceme` | 将外部 Skill 安装、检查、转换、发布、更新或分享为稳定的 Viceme Agent；统一约束认证、来源、Target、确认、任务和安全规则 | Codex、Claude Code |

GitHub、小红书/RedSkill、ZIP 和目录是同一个 `viceme` 发布流程处理的来源类型，不是相互独立的 Agent Skills。这样可以让不同来源共享一致的安全边界和稳定链接合同。

CLI 与 `viceme` Agent Skill 从同一个仓库以相同版本发布。`viceme install` 会把完整 Skill Bundle 安装到检测到的受支持宿主中；二进制文件同时嵌入用于确定性自检、可供 Agent 阅读的内容子集。

```bash
viceme skills list
viceme skills read viceme
viceme skills read viceme references/commands.md
viceme skills doctor
```

`skills doctor` 会分别校验 CLI 版本、Skill 版本、兼容范围、完整 Bundle 摘要和嵌入内容摘要。安装内容被修改或版本不兼容时会直接失败。

## 认证

| 命令 | 用途 |
|---|---|
| `viceme auth status` | 查看当前区域是否已认证 |
| `viceme auth login --no-wait` | 启动设备授权并立即返回 |
| `viceme auth login --device-code <code>` | 完成之前启动的设备授权 |
| `viceme auth logout` | 撤销并删除当前区域的凭证 |

令牌只保存在操作系统密钥链中，不会回退到明文存储；登录成功的输出也不会包含访问令牌或刷新令牌。

普通已认证用户直接发布。由工作人员操作的委托发布使用同一套宿主无关命令契约：一次性凭证只能通过受保护的标准输入或操作系统密钥链引用提供，并且只通过专用请求头发送。CLI 不接受命令行参数或环境变量中的原始委托凭证，也不会在输出中返回该凭证。

## 支持的来源

### GitHub 或可信来源平台

```bash
viceme skill inspect https://github.com/acme/poster-skill
viceme skill publish --resolution-id <resolution-id> --yes
```

### 小红书或 RedSkill 复制口令

```bash
viceme skill inspect --expression-stdin
viceme skill publish --resolution-id <resolution-id> --yes
```

复制口令属于不可信数据。Viceme 只从中提取定位信息，并通过允许的连接器获取来源；不会执行市场安装文案中的命令。

### 压缩包或本地 Skill 目录

```bash
viceme skill publish --file ./poster-skill.zip --new-target --target-alias poster --yes
viceme skill publish --dir ./poster-skill --new-target --target-alias poster --yes
```

后续发布新版本时，应先解析现有 Target 并使用乐观并发控制。发生冲突时不能创建新的分享链接：

```bash
viceme skill target get target_123
viceme skill publish --file ./poster-skill-v2.zip \
  --target-id target_123 --expected-target-version 4 --yes
```

## 命令概览

| 命令组 | 用途 |
|---|---|
| `viceme install` | 安装持久化启动器、Agent Skill 和区域配置 |
| `viceme auth` | 启动、完成、检查或撤销设备认证 |
| `viceme skill inspect` | 固化并检查来源候选，不执行发布 |
| `viceme skill publish` | 创建或更新具有稳定链接的 Skill Agent 发布 |
| `viceme skill target` | 解析现有逻辑 Agent Target 及其版本 |
| `viceme job` | 读取、等待、恢复或取消持久化发布任务 |
| `viceme skills` | 读取、安装和诊断随包发布的 Agent Skill |
| `viceme update` | 同时更新 npm 启动器、已校验二进制文件和随包发布的 Skill |

使用 `viceme <command> --help` 查看准确参数。经过发布检查的机器可读命令面存放在 [`skills/viceme/references/command-manifest.json`](skills/viceme/references/command-manifest.json)。

## JSON 输出契约

所有数据命令默认输出稳定的 JSON 信封。

成功结果写入 **stdout**，退出码为 `0`：

```json
{
  "ok": true,
  "data": {},
  "meta": {
    "cli_version": "0.1.0",
    "skill_version": "0.1.0"
  }
}
```

CLI 执行错误写入 **stderr**，退出码非零：

```json
{
  "ok": false,
  "error": {
    "type": "validation",
    "subtype": "source_required",
    "message": "provide exactly one source argument or --expression-stdin"
  },
  "meta": {
    "cli_version": "0.1.0",
    "skill_version": "0.1.0"
  }
}
```

应根据进程退出码或 `ok == true` 判断命令是否成功。成功读取发布任务时，业务状态仍可能是 `unsupported`、`rejected` 或 `failed`；这时应检查 `data.status`，不能把这些状态当成 CLI 传输失败。

| 退出码 | 含义 |
|---|---|
| `0` | 命令完成；适用时继续检查返回的业务状态 |
| `2` | 参数校验失败 |
| `3` | 认证或授权失败 |
| `4` | 可重试的网络失败 |
| `5` | 内部或协议失败 |
| `6` | 创建发布前被策略拒绝 |
| `10` | 需要明确确认 |

## 安全与风险控制

- **不执行来源内容** — CLI 和编译器不会执行第三方脚本、二进制文件、shell 片段、市场命令或复制口令中的指令。
- **公开变更需要明确确认** — 发布和取消操作需要 `--yes`；退出码 `10` 表示 Agent 必须向用户取得确认，不能静默重试。
- **安全预览** — 用户需要检查计划请求时，可以对 inspect 或 publish 使用 `--dry-run`，不会产生网络请求或发布副作用。
- **凭证隔离** — 凭证保存在操作系统密钥链中，并按区域隔离。
- **不可变输入** — inspect 会把发布绑定到不可变来源快照，而不是在之后重新读取浮动 URL。
- **有界等待** — `job wait` 有最大等待时间；超时后返回最新持久化状态，不会取消工作流。
- **可信分发** — npm 启动器下载与其准确包版本匹配的二进制文件，并在启用前验证发布的 SHA-256 校验和。

## 诊断与更新

```bash
viceme skills doctor
viceme update --check
viceme update
```

`viceme update` 会安装一个准确的 npm 包版本，获取对应的已校验 Go 二进制文件，并从同一版本刷新随包发布的 Skill。独立开发版二进制文件不会被静默替换。

## 开发

从源码构建需要 Go 1.23 或更高版本。

```bash
make build
make test
make check
make skill-check
make npm-package-check
make quality-check
```

仓库中主要的质量产物包括：

- [`skills/viceme/references/command-manifest.json`](skills/viceme/references/command-manifest.json)：从 Cobra 命令树生成；
- [`quality/example-dry-runs.json`](quality/example-dry-runs.json)：在没有网络请求的情况下执行文档中的来源路径；
- [`quality/release-manifest.json`](quality/release-manifest.json)：固定 CLI/Skill 兼容范围和内容摘要。

`make npm-package-check` 会构建 Go 可执行文件、打包真实 npm tarball，并在相互隔离的临时主目录中运行启动器，不依赖已经发布的 GitHub Release。

## 发布

维护者将普通改动合入 `dev`；不需要手工修改版本、创建 tag、编写 changelog 或执行 `npm publish`。GitHub Actions 持续维护唯一的 `dev -> main` 自动 Release PR。合并该 PR 即授权发布经过审阅的版本、tag、不可变 GitHub Release 产物及 npm trusted publication。

仓库设置、OIDC trusted publishing、恢复和完整性规则参见 [`docs/releasing.md`](docs/releasing.md)。

## 贡献

欢迎提交 Issue 和 Pull Request。公共命令面发生变化时，必须在同一个 PR 中更新生成的命令清单、随包发布的 Skill 示例、质量样例和测试。
