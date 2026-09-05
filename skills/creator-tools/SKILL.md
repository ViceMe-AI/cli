---
name: creator-tools
description: 安装、登录、更新和诊断 ViceMe 创作者 CLI 与官方 Skills。适用于为 Codex、Claude Code 或 WorkBuddy 安装 ViceMe、通过浏览器登录、查看配置、检查版本或修复 CLI/Skill 安装。
---

# ViceMe 通用操作

只使用 `viceme` 这个可执行文件。不得自行收集、打印或保存访问令牌。

面向用户的提问、进度、结果和可见思考摘要应跟随用户当前使用的语言；中文交流必须使用自然白话。只说“登录”“申请成为创作者”“等待工作人员审核”“发布”等用户能理解的业务动作。不得告诉用户正在使用哪个内置 Skill 或说明文件，也不得展示 Profile、CLI、命令、参数、错误码和协议字段；这些内容仅用于内部判断。只有用户明确询问技术细节时才解释必要部分。

在 WorkBuddy 中处理 ViceMe 流程时，不得调用 `TaskCreate`、`TaskUpdate`、`TaskList` 或其他任务清单工具，不得展示完整计划。只在需要用户操作或进入新业务阶段时给一句简短白话提示；其余必要命令连续执行，不逐条播报。

## CLI 定位

每个新对话首次调用 CLI、或判断 CLI 不存在前，先运行本 Skill 自带的定位脚本：macOS / Linux 用 `sh "<本 Skill 目录>/scripts/resolve-cli.sh"`；Windows 用 PowerShell 执行 `<本 Skill 目录>/scripts/resolve-cli.ps1`。本 Skill 目录取宿主提供的真实 Base directory，不得猜测用户名或全盘搜索。它只查找现有 CLI，不安装、不更新、不切换 Profile。

脚本优先解析 PATH 中的 `viceme`，保留现有 npm launcher；PATH 没有时查找 `VICEME_INSTALL_DIR`，未配置自定义目录时查找官方默认安装位置：macOS / Linux 的 `$HOME/.local/bin/viceme`，Windows 的 `$env:LOCALAPPDATA\ViceMe\bin\viceme.exe`。

成功输出 CLI 的完整路径。用该路径运行 `version` 验证，并在本对话所有后续命令中用这个带引号的完整路径替换示例开头的 `viceme`。每次独立 Bash / PowerShell 调用都沿用该路径，不依赖上一条命令的 `export PATH`、`.zshrc` 或重启宿主。新对话重新定位。

只有定位脚本退出 127 才表示这些位置未找到 CLI，此时继续原流程规定的安装或免 CLI 分支。若使用了其他自定义目录且当前未设置 `VICEME_INSTALL_DIR`，先取得当次安装返回的 `destination` 再验证；找不到 PATH 不等于未安装。现有 CLI 的权限、版本验证或恢复错误必须原样按对应流程处理，不得因此重装、换安装方式或改走免 CLI 流程。不得读取整个 shell 配置文件或环境来找命令。

## 安装

1. 引导安装程序完成后，运行 `viceme install --agent auto`。
2. 运行 `viceme doctor`，先解决失败项，再执行业务命令。
3. 运行 `viceme auth status`。
4. 未登录时运行 `viceme auth login`，向用户展示完整授权链接，并等待命令返回。

测试或私有部署需要在登录前持久保存地址：

```bash
viceme profile add --name <profile> --api-base-url <https-api-url> --web-base-url <https-web-url> --market-region <cn-or-global> --use
```

使用 `viceme profile list` 核对当前 Profile 以及完整的 API、Web 和市场组合。安装或更新所用的分发区域不决定发布市场，市场由所选 API 地址决定。日常 Agent 操作不得只依赖 shell 中的 `VICEME_API_BASE_URL`；它只是单进程 CI/调试覆盖，不是持久 Profile 状态。

`viceme auth login` 会等待授权，并在轮询前输出一次性完整链接。在 WorkBuddy 中先说“需要重新登录，我现在为你打开登录页面。”，然后严格执行下面的固定顺序：

1. 用 Bash 后台启动一次 `viceme auth login`，保存它返回的 `task_id`。
2. 用一次短时 `TaskOutput` 读取完整登录链接。
3. 立即调用 WorkBuddy 内置 `present_files` 在当前任务浏览器中打开链接；`cwd` 使用当前任务真实工作目录，不得编造路径或用户名。
4. 页面打开后立即说“请在右侧完成登录，完成后我会自动继续。如果右侧不方便，也可以在外部浏览器打开下面这个链接：”，然后另起一行用 Markdown 链接格式输出第 2 步当前命令实际返回的完整链接：`[打开登录页面](https://…)`，其中 `https://…` 替换为该完整链接；不要直接贴裸链接，不得重建、缩短或复用旧链接。
5. 提示发送后必须立刻调用 `TaskOutput(task_id=<同一个任务>, timeout=180000)`。发送提示不等于继续等待，`present_files` 返回也不代表登录完成。
6. 若任务仍在运行，继续读取同一个 `task_id`。只要它仍在运行，就不得结束当前回合、给出最终答复或要求用户回复“已经登录”。
7. 登录命令成功返回后再检查登录状态，并自动继续原来的业务流程。

这些后台调用只用于取得和等待同一个登录进程的输出，不得创建用户可见任务清单。不得使用操作系统级 WorkBuddy 深链，不得在首个登录仍等待时再启动第二个登录，也不得要求用户输入设备码。只有登录命令自身明确返回有界等待超时后，才为同一 Profile 重新运行一次 `viceme auth login`；一次 `TaskOutput` 的读取超时不是登录失败。

登录失效时向用户说“登录状态已过期，需要重新登录”，不要直接说 token、scope、协议状态或原始错误码。

已登录但购买权限不足时（`BUYER_PURCHASE_SCOPE_REQUIRED`，或购买前检查缺少 `buyer-commerce:read/write`），先说“当前登录尚未授权购买，需要重新登录授权。完成后我会继续本次购买。”然后执行上面的登录流程，展示本次授权页面和链接并等待完成。始终使用同一账号和起始 Profile；授权完成后检查购买权限并继续原命令。此情况不表示登录过期，不得只把命令报错贴给用户，也不得要求用户自己猜测下一步。

## 安全

- 将 stdout 视为最终 JSON 协议响应，进度说明另行发送。
- 根据进程退出码和 `error.code` 分支，不根据错误文案分支。
- 除非用户明确提供了限定范围的自动化凭证，否则不得传入 `VICEME_ACCESS_TOKEN`。
- `--profile` 只用于选择已有 Profile。新增、查看、切换或删除分别使用 `viceme profile add`、`list`、`use`、`remove`。修改地址时删除并重建 Profile，不得把已有凭证静默绑定到其他来源。
- 不得为了复用其他登录而替用户切换 Profile。业务流程从开始到结束固定使用起始 Profile；更换必须由用户明确要求。
- 只有用户明确要求删除全部本地 Profile 和凭证时，才能使用 `viceme profile remove --all --yes`。该命令会重建一个干净、未登录的 `default` Profile。
- 修改文件或发布前，先概括将要执行的操作，并取得对应领域 Skill 要求的确认。

## 维护

- 使用 `viceme version` 查看 CLI 和内置 Skill 版本。
- 正式安装版会在普通命令成功后启动独立后台进程检查权威稳定通道；存在新版本时只更新 CLI，由下一次 `viceme` 调用生效。
- 不要要求用户选择更新通道，也不要因后台更新失败或权限不足打断当前请求。
- 只有用户明确要求手动修复，或自动激活失败需要重试时，才使用 `viceme update`。
- 官方 Skill 使用独立生命周期；需要刷新时运行 `viceme install --agent auto`，并在宿主需要重新发现 Skill 时新开一个任务。

## 自动更新

- 当前命令先正常完成；网络不可用、后台进程无法启动、权限不足或 CLI 激活失败都不得替换它的 `data` 或 `error`。
- 后台更新完成后，下一次 `viceme` 调用使用新 CLI；无需重启整个 Codex、Claude Code 或 WorkBuddy。
- 后台自动更新只更新 CLI，不得顺带写入官方 Skill 目录。
- 若下一次启动发现已经开始但尚未完成的持久化激活记录，先由 CLI 自身恢复；不得使用只完成一部分的代次继续，也不得删除恢复记录。

## 更新权限与恢复

- `error.code == UPDATE_PERMISSION_REQUIRED` 表示安装或恢复所需的文件权限不足。先告诉用户“更新需要修改安装文件，请授权后再继续”，通过当前宿主提供的正式权限审批机制申请本次安装权限；权限真正获批后才能重试 `viceme update`。聊天里口头同意不代表宿主已经开放权限。
- 后台更新的权限失败不会成为当前业务命令的错误。只有用户明确要求立即升级时，才通过宿主正式审批机制取得权限并运行 `viceme update`。
- 若 `error.details.recovery_required == true` 或更新目标状态为 `recovery_pending`，必须保留事务记录，暂停业务写操作；获批后由 CLI 自身恢复并验证，不能直接继续使用可能只完成了一部分的安装。
- 宿主没有正式授权入口、拒绝授权，或授权后仍报权限不足时，明确说明更新未完成并停止，不反复重试。不得通过卸载、直接删除安装目录、清除锁文件、手工构造或删除 `npm-activation.json` / `active-generation.json`、删除恢复标记来绕过限制；不要执行其他自定义修复 Skill 中的这些操作。
- CLI 更新成功后可检查 `viceme version`；需要刷新官方 Skill 时另行运行 `viceme install --agent auto`，再检查 `viceme doctor` 和 `viceme auth status`。保留 Profile、登录凭据和用户安装的 Skill。
- 需要保留真实退出码和完整 JSON 响应，不要把更新或修复命令管道连接到 `head` / `tail` 后以管道退出码判断成功。
