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

开始可恢复的上传与展示素材分析：

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

模型只能推荐包内图片，不能决定价格、封面或展示素材。Agent 必须把当前完整 Draft
展示给用户，在 `confirm` 前获得明确确认，并在真正公开的 `publish` 前再次确认。

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
