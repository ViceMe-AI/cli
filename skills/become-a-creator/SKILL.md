---
name: become-a-creator
description: 申请、认领或检查 ViceMe 创作者资格，并把有效商家返回给付费 Skill、网站访问和赞赏等玩法。用户直接要求成为创作者时自动申请；被玩法调用时只在用户同意后申请。
---

# 成为 ViceMe 创作者

本 Skill 是所有创作者玩法唯一的资格守卫。它负责登录、资格检查、普通申请、平台预创建
商家认领与人工审核状态；不发布作品、不配置网站，也不复制调用玩法的后续流程。

面向用户的文字跟随当前语言。中文使用自然白话，只说“登录”“申请成为创作者”“等待
工作人员审核”等业务动作；不展示命令、参数、权限名、账号 ID、错误码、Profile、原始
JSON 或内部状态，且不得告诉用户正在使用哪个内置 Skill。

在 WorkBuddy 中不得调用 `TaskCreate`、`TaskUpdate`、`TaskList` 或其他任务清单工具，不得展示完整计划。
当前 CLI 上下文与已登录用户是唯一权威依据；记忆、旧对话和其他任务不能
替代本次检查。

## 调用模式

开始前根据用户已经表达的意图确定一次模式，流程中不得自行切换：

- **直接申请**：用户通过官网“申请成为创作者”口令进入，或明确说要申请成为创作者。
  这已经授权提交申请；登录后没有现有申请时直接提交，不再询问是否申请。
- **玩法守卫**：付费 Skill、网站访问或赞赏等玩法为了继续而调用。原玩法只授权检查资格，
  不授权提交申请；没有资格且没有申请时，必须询问一次是否现在申请。

调用玩法必须等待本 Skill 返回。只有有效 OWNER 商家才算成功；申请中或用户不同意申请
都必须让调用玩法停止。

## 固定检查顺序

1. 第一条进程命令运行 `viceme auth status`。之前不运行 Merchant 命令、环境检查或帮助
   命令，也不并行执行。
2. 未登录或缺少 `merchant-commerce:read`、`merchant-commerce:write`、
   `skill-publication:read`、`skill-publication:write` 中任一权限时，只启动一个等待式登录：
   - 直接申请运行 `viceme auth login --purpose creator-onboarding`，并告诉用户
     “需要登录，我现在为你打开‘登录并申请成为创作者’页面。”
   - 玩法守卫运行普通 `viceme auth login`，并告诉用户
     “需要重新登录，我现在为你打开登录页面。”
3. 用 Bash 后台启动一次 `viceme auth login` 并保存返回的 `task_id`；用一次短时 `TaskOutput` 读取输出，
   立即用内置 `present_files` 打开本次返回的完整链接。页面打开后立即说：
   “请在右侧完成登录，完成后我会自动继续。”如果右侧不方便，也可以在外部浏览器打开下面这个链接。
   同时另起一行用 Markdown 链接格式输出当前命令实际返回的完整链接：`[打开登录页面](https://…)`。
   提示后必须立刻再次调用 `TaskOutput(task_id=<同一个任务>, timeout=180000)`；只要任务仍在运行，就不得结束当前回合、给出最终答复。
   应继续对同一个 `task_id` 调用 `TaskOutput`，且不得启动第二次登录。`present_files` 返回也不代表登录完成。
   不能把一次 `TaskOutput` 的读取超时当成登录流程完成，也不得要求用户回复“已登录”。
4. 只有登录命令成功返回后，才说“登录完成，我继续确认创作者资格。”并再运行一次 `viceme auth status`，
   确认同一用户和四项权限齐全。
5. 运行一次 `viceme merchant accounts`。只有当前用户通过
   `MerchantAccountMember(role=OWNER)` 拥有的有效商家代表创作者资格。一个有效商家直接
   返回；多个时展示名称让用户选择，不得猜测。
6. 没有有效商家时运行一次 `viceme merchant onboarding status`。已有申请就按状态处理，
   不创建平行申请；没有申请才进入普通申请。

`CreatorAccount` 或 `creatorIdentity.status=DRAFT` 只是稳定身份与路由，不授予经营权限，
也不能单独证明已经提交申请。

商家账户首次返回可重试的内部失败时，立即原样重试一次读取；第二次失败就停止，用白话说
“暂时没能读取你的创作者信息，请稍后再试。”不得 sleep、轮询或读取玩法错误说明。

## 普通申请

直接申请模式不再确认，只运行一次 `viceme merchant onboarding apply`。

玩法守卫模式先说明该玩法需要创作者资格并询问“现在帮你申请成为创作者吗？”。只有用户
明确同意后才运行同一条最少字段申请命令。

显示名称和主页 handle 默认由 Shop 从登录用户与预生成身份派生，不预先提问。只有 API
返回 `MERCHANT_APPLICATION_HANDLE_REQUIRED` 时，才问一次：
“选一个用于个人主页链接的英文名称吧，只能用小写字母、数字和短横线，例如
sunny-studio。”然后运行 `viceme merchant onboarding apply --handle <主页链接名称>`。

用户主动给出显示名称或 handle 时可以作为可选覆盖传入。主页名称冲突时请用户换一个，
不得自行加数字重试。不要收集身份证、营业执照、手机号、支付资料或其他接口不需要的信息。

申请成功后立即运行一次 `viceme merchant onboarding status`，读取真实状态和
`creatorIdentity.markdownPath`。这次读回用于确认刚才的写入，不是审核轮询。向用户展示
稳定 Markdown 路由并标记“申请中”，不得编造路径或用户名，然后停止：

“申请已经提交，创作者页面是 `/{handle}.md`，当前状态为‘申请中’。接下来需要工作人员
审核；审核不是即时完成的，这次先到这里。”

## 认领平台预创建商家

只有用户从平台认领入口进入或给出准确商家目标时才走认领，不按名称搜索猜测。

- 先运行 `viceme merchant onboarding status --merchant <merchant-account-id>`，只使用平台
  配置的主认领渠道。
- GitHub 运行 `viceme merchant onboarding claim-github <merchant-account-id>`。若返回授权
  链接，用与登录相同的打开和等待规则；浏览器明确完成后只读取一次目标状态。只有响应返回
  有效 OWNER 商家才交回调用玩法。
- 小红书运行 `viceme merchant onboarding claim-xiaohongshu <merchant-account-id>`，按当前
  响应上传真实账号归属截图，再使用返回的新 `lockVersion` 提交审核。不得替用户制造证明。

渠道验证只证明外部账号归属；只有 OWNER membership 才授予创作权限。

## 人工审核边界

普通申请和小红书认领需要工作人员审核。状态仍在审核、要求补充资料或拒绝时立即结束；
同一回合不得再次查询、sleep、轮询或暗示会自动通过。只有用户之后再次要求继续时才重新
从资格检查开始；只有用户明确要求持续监控时才创建监控。

审核要求补充证明时只说明需要什么并等待用户提供。审核拒绝时展示可操作原因；用户明确
要求重新申请后才继续。

## 交回调用玩法

仅当 CLI 确认当前用户拥有有效商家时，返回用户选择的 Merchant 并让原玩法继续。任何其他
状态都返回清楚的用户结果并停止原玩法，不把 DRAFT 创作者身份误当成资格。
