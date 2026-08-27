# ViceMe Demo Agent 使用指南

本文面向通过 Codex、Claude Code 或 WorkBuddy 使用 ViceMe Demo 的外部用户。安装、更新、环境绑定和发布都交给 Agent 执行，用户不需要切换源码分支、本地构建 CLI 或手工上传 S3。

- Agent 安装契约：`https://s3-demo.viceme.cn/viceme-shop/agent-install.md`
- Demo Web/API：`https://demo.viceme.cn`
- Demo API Base URL：`https://demo.viceme.cn/api`
- CLI 公开发布通道：`https://s3-demo.viceme.cn/viceme-shop/cli/releases`
- 当前公开 Demo CLI：`0.17.101`
- 当前兼容线：`0.17.x`

## 1. 让 Agent 安装或更新

在 Agent 平台中发送：

> 请读取 https://s3-demo.viceme.cn/viceme-shop/agent-install.md，并严格按照其中的安装契约安装或更新 ViceMe Demo CLI。安装完成后绑定 demo Profile，但不要登录、发布或发起交易，除非我随后明确要求。

Agent 会按受信任契约完成：

```text
校验契约域名
  → 检测操作系统与 CPU 架构
  → 选择不可变版本包
  → 校验 SHA-256
  → 安装 CLI 与官方 Skills
  → 创建或选择 demo Profile
  → 执行 doctor 与 auth status
  → 返回可验证结果
```

安装本身不代表授权 Agent 登录、上传、公开发布、购买或提交任何内容。

## 2. 首次使用与已有 CLI

### 从未安装

Agent 会安装适配当前平台的最新公开包，同步官方 Skills，并将业务 API 绑定到 `https://demo.viceme.cn/api`。完成后，Agent 必须报告 CLI 版本、Profile、API Base URL 和健康检查结果。

### 已有较旧的 standalone CLI

Agent 会使用更新的 Demo 版本原子替换现有 standalone 版本，保留原有 Profile 和同 Origin 的登录凭证，然后重新执行健康检查。

### 已有版本相同、更新或由 npm/npx 管理

Agent 不会降级、覆盖不同字节的同版本，也不会破坏 npm 的激活记录。如果遇到 `BOOTSTRAP_DOWNGRADE_REFUSED` 或 `BOOTSTRAP_INSTALL_METHOD_CHANGE_REFUSED`，Agent 应改用独立的 Demo 安装目录和配置目录，并在结果中明确说明隔离方式。

此隔离保证正式 CLI 的版本、Profile、凭证和 Skills 不被 Demo 环境修改。

## 3. 登录 Demo

只有在用户明确要求执行需要身份的操作时，Agent 才应启动登录。可以发送：

> 请登录 ViceMe Demo。确认 Profile 是 demo、API 是 https://demo.viceme.cn/api，然后启动一次阻塞式登录并等待最终结果。

Agent 会打开浏览器授权。用户完成确认后，Agent 必须等待 CLI 返回成功或明确错误，不能把“已打开授权页”当作登录成功。

## 4. 使用 Agent 发布

将待发布的 Skill、网站或服务目录放在 Agent 可访问的工作区，然后用自然语言说明目标。

### 发布可下载 Skill

> 请使用 ViceMe Demo 发布当前目录中的 Skill。先检查包结构和元数据，向我展示将要公开的名称、描述、版本、价格和文件范围；得到我确认后再登录并发布。

Agent 应使用 `viceme-publish` Skill 编排，完成检查、预览、用户确认、上传和结果校验。

### 发布服务型 Work

> 请在 ViceMe Demo 发布一个服务型 Work。先完成场景分析，让我逐项确认收集字段、授权范围、交付流程和页面预览；只有在我最终确认后才激活公开交互。

Agent 的标准流程是：

```text
识别发布类型
  → 检查素材和账户状态
  → 场景分析
  → 逐项收集必要信息
  → 生成页面和交互预览
  → 用户最终确认
  → 创建或更新 Work
  → 上传产物
  → 激活并校验公开页
  → 返回 Work URL、版本和发布状态
```

Agent 不得跳过确认门禁，也不得仅凭安装或登录授权推断用户同意公开发布。

### 中断后继续发布

如果 Agent 返回可恢复的 publication ID，发送：

> 请继续 ViceMe Demo 中 publication ID 为 `<publication-id>` 的发布。先读取当前状态，不要重复已完成的步骤；遇到新的付费、授权或公开激活决策时再请我确认。

## 5. 多版本与更新

Demo S3 保留多个不可变版本，目前包含 `0.17.100` 和 `0.17.101`，每个版本均提供：

- macOS Intel 与 Apple Silicon；
- Linux amd64 与 arm64；
- Windows amd64 与 arm64；
- 每个二进制对应的 SHA-256 文件；
- 对应版本的 macOS/Linux 与 Windows 安装器。

默认安装契约选择 `latest`。如果需要验证指定历史版本，请在提示词中明确版本号，并要求 Agent 使用独立的安装和配置目录。

Demo CLI 内置了独立更新通道，后续更新仍从 `s3-demo.viceme.cn` 读取，不会自动切换到正式 CN Release。

## 6. 故障处理

### 授权页已打开，但发布仍提示未授权

要求 Agent 检查阻塞式登录的最终返回值、当前 Profile 和 API Origin。Demo 凭证必须绑定 `https://demo.viceme.cn/api`，不能复用正式环境凭证。

### `BOOTSTRAP_DOWNGRADE_REFUSED`

目标版本低于已安装版本，或同版本对应了不同二进制。要求 Agent 使用隔离的 Demo 安装和配置目录。

### `BOOTSTRAP_INSTALL_METHOD_CHANGE_REFUSED`

现有 CLI 由 npm/npx 管理，不允许在同一配置目录原地切换安装方式。要求 Agent 使用隔离的 Demo 目录，不要删除原激活文件。

### Agent 没有发现新 Skills

让 Agent 执行健康检查，然后新建 Codex、Claude Code 或 WorkBuddy 会话。Agent 平台通常只在会话启动时发现 Skills。

### 公开包下载失败

要求 Agent 报告完整的版本、平台、对象 URL 和 checksum 结果。不要跳过 SHA-256 校验，也不要从非 `s3-demo.viceme.cn` 域名的镜像安装。
