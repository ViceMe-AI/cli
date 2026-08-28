---
name: viceme-shared
description: 安装、登录、更新和诊断 ViceMe 创作者 CLI 与官方 Skills。适用于为 Codex、Claude Code 或 WorkBuddy 安装 ViceMe、通过浏览器登录、查看配置、检查版本或修复 CLI/Skill 安装。
---

# ViceMe 通用操作

只使用 `viceme` 这个可执行文件。不得自行收集、打印或保存访问令牌。

面向用户的提问、进度、结果和可见思考摘要应跟随用户当前使用的语言；中文交流必须使用自然白话。只说“登录”“申请成为创作者”“等待工作人员审核”“发布”等用户能理解的业务动作。不得告诉用户正在使用哪个内置 Skill 或说明文件，也不得展示 Profile、CLI、命令、参数、错误码和协议字段；这些内容仅用于内部判断。只有用户明确询问技术细节时才解释必要部分。

在 WorkBuddy 中处理 ViceMe 流程时，不得调用 `TaskCreate`、`TaskUpdate`、`TaskList` 或其他任务清单工具，不得展示完整计划。只在需要用户操作或进入新业务阶段时给一句简短白话提示；其余必要命令连续执行，不逐条播报。

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
4. 页面打开后立即说“请在右侧完成登录，完成后我会自动继续。如果右侧不方便，也可以在外部浏览器打开下面这个链接：”，然后另起一行原样输出第 2 步当前命令实际返回的完整 `http://` 或 `https://` 链接；不得重建、缩短或复用旧链接。
5. 提示发送后必须立刻调用 `TaskOutput(task_id=<同一个任务>, timeout=180000)`。发送提示不等于继续等待，`present_files` 返回也不代表登录完成。
6. 若任务仍在运行，继续读取同一个 `task_id`。只要它仍在运行，就不得结束当前回合、给出最终答复或要求用户回复“已经登录”。
7. 登录命令成功返回后再检查登录状态，并自动继续原来的业务流程。

这些后台调用只用于取得和等待同一个登录进程的输出，不得创建用户可见任务清单。不得使用操作系统级 WorkBuddy 深链，不得在首个登录仍等待时再启动第二个登录，也不得要求用户输入设备码。只有登录命令自身明确返回有界等待超时后，才为同一 Profile 重新运行一次 `viceme auth login`；一次 `TaskOutput` 的读取超时不是登录失败。

登录失效时向用户说“登录状态已过期，需要重新登录”，不要直接说 token、scope、协议状态或原始错误码。

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
- 正式安装版会在普通命令前自动检查权威稳定通道。存在新代次时，CLI 和所有匹配的官方 Skill 会一起激活，然后原命令继续执行。
- 不要要求用户选择更新通道，不要因例行更新打断当前请求，也不要在自动更新后再次安装 Skill。
- 只有用户明确要求手动修复，或自动激活失败需要重试时，才使用 `viceme update`。

## 自动更新

- 网络不可用时，自动发现更新应当放行，继续使用最后一个完整且验证过的代次。
- 自动更新后，`meta.autoUpdate` 记录更新前后版本；它只用于诊断，应继续处理命令正常返回的 `data` 或 `error`。
- Windows 独立二进制在系统释放旧文件期间可能返回可重试的 `AUTO_UPDATE_RESTART_REQUIRED`。短暂等待后原样重跑同一命令，不再向用户索取决定。
- 激活失败时根据稳定错误码停止本次写操作，不得使用只完成一部分的代次继续。
