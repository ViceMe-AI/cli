# ViceMe CLI

ViceMe CLI 是官方 ViceMe Agent Skills 的确定性本地执行器。Codex、Claude
Code 和 WorkBuddy 通过 Skills 引导用户；CLI 负责安装、设备授权、本地检查、
确定性打包、上传、审核和发布。

[English](./README.md)

## 安装

官方 Bootstrap 从同一个不可变 Release 一次安装原生 CLI 和两个官方 Skills。

中国区 macOS / Linux：

```bash
curl -fsSL https://s3.viceme.cn/install.sh | sh
```

海外 macOS / Linux：

```bash
VICEME_REGION=global sh -c "$(curl -fsSL https://s3.viceme.ai/install.sh)"
```

中国区 Windows PowerShell：

```powershell
irm https://s3.viceme.cn/install.ps1 | iex
```

海外 Windows PowerShell：

```powershell
$env:VICEME_REGION="global"; irm https://s3.viceme.ai/install.ps1 | iex
```

不能使用 Bootstrap 时，以 npm 作为备用入口：

```bash
npx --yes @viceme-ai/cli@latest install
```

安装器始终写入兼容 fallback `~/.agents/skills`，并为检测到的 Agent 写入
各自的用户级目录：

| Agent | 原生目录 |
| --- | --- |
| Codex | `~/.codex/skills` |
| Claude Code | `~/.claude/skills` |
| WorkBuddy | `~/.workbuddy/skills` |

可通过 `viceme install --agent codex`、`claude`、`workbuddy` 或 `agents`
指定目标。安装或修复后运行 `viceme doctor`。

## 登录与 Profile

每个 Profile 绑定一个官方区域和一个通过设备码授权的账户。

```bash
viceme auth login
viceme auth status
viceme auth logout

viceme profile list
viceme profile use default
```

Agent 无法在同一个回合等待浏览器授权时使用分段流程：

```bash
viceme auth login --no-wait
viceme auth login --device-code <device-code>
```

用户在浏览器完成授权。不要在对话中复制 Access Token。凭据按 Profile 和 API
Origin 隔离。

## 发布 Skill

第一版接受包含 `SKILL.md` 的本地目录或本地 ZIP，不接受 GitHub URL、远程下载和
多 Skill 批量包。

只读检查：

```bash
viceme skill inspect --path ./my-skill
```

查看确定性发布包和价格计划：

```bash
viceme skill publish --path ./my-skill --price-minor 100 --dry-run
```

开始可恢复的上传与 Listing 分析：

```bash
viceme skill publish --path ./my-skill --price-minor 100
```

之后以服务端 Publication 状态为准：

```bash
viceme publication get <publication-id>
viceme publication review <publication-id>
viceme publication asset upload <publication-id> --role cover --path ./cover.png
viceme publication asset upload <publication-id> --role gallery --path ./demo.png
viceme publication update <publication-id> --input ./listing-draft.json
viceme publication confirm <publication-id> --review-digest <digest>
viceme publication publish <publication-id> --review-digest <digest>
```

模型会建议 `summaryZhCn`、`summaryEnUs`、根据已验证 `SKILL.md` 生成的
`usageInstructions` 和包内图片，但不能替用户确认，也不能决定价格。
每版短简介的最大显示宽度为 30：ASCII 计 1，中文及其他非 ASCII 计 2。Agent 必须把双语
短简介、使用说明、价格、封面和有序展示素材的完整 Draft 展示给用户，在 `confirm` 前获得明确确认，
并在真正公开的 `publish` 前再次确认。

网络中断后继续同一个发布操作：

```bash
viceme skill publish --resume <publication-id>
```

响应未知时不能创建第二个 Publication，应先查询或恢复原 ID。

## 输出契约

业务结果默认使用 JSON。成功时 stdout 只包含最终结果；进度和诊断只写 stderr。

```json
{
  "ok": true,
  "data": {},
  "meta": {
    "cliVersion": "0.10.1",
    "requestId": "optional"
  }
}
```

失败使用非零退出码和稳定的 `error.code`。Agent Skills 只能依据退出码、`ok`、
`error.code` 和 `retryable` 分支，不能解析 message 文本。

## 更新

```bash
viceme update --check
viceme update
```

Bootstrap 安装会读取当前 Profile 所在区域的官方 S3 Release 索引，校验精确二进制
Checksum，用新二进制修复同版本官方 Skills 后原子激活。npm 安装通过精确 npm 包版本
更新。更新子进程不会继承 `VICEME_ACCESS_TOKEN`。

二进制或 npm launcher、两份官方 Skills 和 Profile 配置属于同一个可恢复的本地版本。
Standalone 与 npm 激活共用外层激活锁、委托成员提交锁，并持久化包含语义版本、安装方式和不可变身份的
active-generation。唯一的启动协调器不区分当前入口，始终检查 Standalone 和 npm 两类 Journal。
所有普通命令必须先恢复未完成的外层 Journal；如果恢复后当前进程的版本、安装方式或不可变身份
不再等于 active generation，本次命令会停止并要求重新执行。锁内 generation fence 会拒绝迟到
的旧版本更新。每个真实写入口拿到 activation lock 后、开始暂存或联网安装前，都会再次对两类
Journal 做相同仲裁，避免通过启动检查后暂停的旧进程引入第二套恢复协议。第一阶段也会在任何
文件变更前拒绝 Standalone 与 npm 的原地切换，不能把两套
恢复协议混合使用。每个 Skills/配置事务都必须持有同一代际权限，或在最终提交前重新验证。
npm 内部安装子进程必须匹配 Journal 中的一次性 nonce、目标版本和 Skill 目标；成员提交锁会阻止
父进程崩溃后，新一代在旧子进程尚未提交完毕时进入。目标一旦越过语义提交点，崩溃恢复只能完成本地清理，不能重新联网安装或回滚。私有
Journal 因此只能完整恢复上一代或完整完成目标版本；安装提交前，`viceme doctor` 同时校验
Skill/版本完整性和不携带凭据的 API readiness。

## 第一阶段实施状态

安装、设备码授权、确定性包上传、建议或人工展示素材、Review 确认、发布、取消和终态恢复均已
实现。本地验收使用真实 Shop API、PostgreSQL、Redis 和 S3 兼容存储；`make check`、npm
打包与冷启动、race test，以及 Darwin/Linux/Windows 的 amd64/arm64 构建均通过。真实 LLM
Provider sandbox 仍属于部署环境验收项；没有凭据时分析 fail closed，人工素材流程仍可完成发布。

## 安全边界

- 本地打包拒绝路径穿越、绝对路径、符号链接、特殊文件、超限内容、敏感文件和常见
  Secret 模式。
- API 独立校验不可变 ZIP 与对象元数据，不能信任 CLI 自报结果。
- Pending operation 不保存预签名上传 URL。
- LLM 只接收经过筛选的文本、元数据和图片缩略图。
- 未确认当前 Review Digest、价格、封面和有序展示素材时，Publication 不能公开。

## 开发

需要 Go 1.23+；npm 包检查还需要 Node.js 22+。

```bash
make check
make npm-package-check
make release-manifest
```

CLI、`viceme-shared` 和 `viceme-publish` 同版本发布。GitHub、npm、
`s3.viceme.cn` 与 `s3.viceme.ai` 的产物都来自同一个已评审 Commit。
