---
name: become-a-creator
description: 申请或检查 ViceMe 创作者资格，并把有效商家返回给付费 Skill、网站访问和赞赏等玩法。用户直接要求成为创作者时自动申请；被玩法调用时只在用户同意后申请。
---

# 成为 ViceMe 创作者

本 Skill 是所有创作者玩法唯一的资格守卫。它负责登录、资格检查、普通申请与人工审核状态；普通申请提交后还负责询问一次是否设置个人名片，并把已确认的作者页目标交给 `$customize-your-page`。它不发布作品，也不复制其他调用玩法的后续流程。

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

1. 第一条进程命令运行一次 `viceme merchant qualification`。之前不运行 Merchant 命令、
   环境检查或帮助命令，也不并行执行。该命令一次返回登录状态、权限与当前用户拥有的
   **全部**有效商家，并带 `next` 分发字段；流程中不拆开单独运行 `viceme auth status`
   或 `viceme merchant accounts`。
2. `next=LOGIN`（未登录，或缺少 `merchant-commerce:read`、`merchant-commerce:write`、
   `skill-publication:read`、`skill-publication:write` 中任一权限）时，只启动一个等待式登录：
   - 直接申请运行 `viceme auth login --purpose creator-onboarding`，并告诉用户
     “需要登录，我现在为你打开‘登录并申请成为创作者’页面。”
   - 玩法守卫运行普通 `viceme auth login`，并告诉用户
     “需要重新登录，我现在为你打开登录页面。”
   - 用 Bash 后台启动一次 `viceme auth login` 并保存返回的 `task_id`；用一次短时 `TaskOutput` 读取输出，
     立即用内置 `present_files` 打开本次返回的完整链接。页面打开后立即说：
     “请在右侧完成登录，完成后我会自动继续。”如果右侧不方便，也可以在外部浏览器打开下面这个链接。
     同时另起一行用 Markdown 链接格式输出当前命令实际返回的完整链接：`[打开登录页面](https://…)`。
     提示后必须立刻再次调用 `TaskOutput(task_id=<同一个任务>, timeout=180000)`；只要任务仍在运行，就不得结束当前回合、给出最终答复。
     应继续对同一个 `task_id` 调用 `TaskOutput`，且不得启动第二次登录。`present_files` 返回也不代表登录完成。
     不能把一次 `TaskOutput` 的读取超时当成登录流程完成，也不得要求用户回复“已登录”。
   - 只有登录命令成功返回后，才说“登录完成，我继续确认创作者资格。”并重跑一次
     `viceme merchant qualification`，确认 `next` 不再是 `LOGIN`。
3. `next=APPLY_CREATOR`（登录有效但没有有效 OWNER 商家）时，运行一次
   `viceme merchant onboarding status`。已有申请就按状态处理，
   不创建平行申请；没有申请才进入普通申请。直接申请模式遇到审核中的普通申请时，
   可以进入「申请后的个人名片」；玩法守卫仍然停止原玩法。
4. `next=SELECT_MERCHANT`（返回多个有效商家）时，展示返回列表中的名称让用户选择，
   不得猜测、不得取第一个。
5. `next=OK` 表示当前用户拥有唯一有效商家，直接以返回的商家继续并交回调用玩法。

只有当前用户通过 `MerchantAccountMember(role=OWNER)` 拥有的有效商家才代表创作者资格；
`qualification` 返回的商家列表已按此过滤。

`CreatorAccount` 或 `creatorIdentity.status=DRAFT` 只是稳定身份与路由，不授予经营权限，
也不能单独证明已经提交申请。

商家列表读取的“可重试内部失败立即原样重试一次”已内建在 `viceme merchant qualification`
中；第二次失败命令自行报错，用白话说“暂时没能读取你的创作者信息，请稍后再试。”
流程不得自行 sleep、轮询或额外重试。

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

申请成功后立即运行一次 `viceme merchant onboarding status`，读取真实状态、
`merchant.id` 与 `creatorIdentity.profileUrl` / `markdownUrl`。这次读回用于确认刚才的写入，
不是审核轮询。不得编造路径或用户名，也不得编造商家 ID。`merchant.status=SUSPENDED` 只是申请期间
保存作者页的内部租户，不是创作者资格，不得交给付费、网站访问或赞赏等玩法。

随后按「申请后的个人名片」继续；用户跳过或个人名片完成后，再打开平台返回的准确主页
链接并说明：审核期间只有登录着 ViceMe 的本人可见，审核通过后同一地址自动公开，创作者
申请本身仍需工作人员审核且不会即时完成。

## 申请后的个人名片

只在**直接申请模式**且普通申请状态为 `SUBMITTED`、`UNDER_REVIEW` 或
`NEEDS_MORE_EVIDENCE` 时执行。询问一次：

“要现在设置个人名片吗？可以用 Bonjour 风格模板，也可以导入你已有的页面；也可以暂时跳过。”

- 用户跳过：用 `present_files` 打开 `creatorIdentity.markdownUrl`，并提供同一 Markdown
  备用链接后，按人工审核边界结束。
- 用户同意：把本次 status 中的 `creatorIdentity.profileUrl`、`merchant.id` 和“创作者入驻模式”
  原样交给 `$customize-your-page`。这个特例只授权作者页自定义，不表示商家已激活；不得改用
  `.md` 地址，不得创建在线 preview，也不得配置作品页或经营功能。
- 个人名片发布后，用 `present_files` 打开 `creatorIdentity.profileUrl`，另起一行输出
  `[打开我的个人名片](https://…)`，其中链接必须是原 `profileUrl`。审核期间这个 HTML 页面只有本人可见；审核通过后
  同一地址、同一 release 自动公开，不再单独审核个人名片。

## Admin 代注册账号

Admin 已经创建账号、创作者资格与商家归属时，直接按有效 OWNER 商家继续发布，不再提交申请。
作者接手账号后，旧邮箱登录、浏览器会话和 CLI Token 会失效。遇到凭据失效时按固定登录流程重新授权，
由作者使用微信登录；重新读取商家归属后继续原发布任务，不创建新账号或新商家。

## 补充审核材料

状态为「需要补充材料」且用户同意继续补材料时，按固定循环引导，不跳步：

1. 用白话转述工作人员的要求（来自 status 返回的审核备注；没有备注时说明需要
   更多能证明账号归属的材料），不展示内部原因码。
2. 邀请用户给材料：能证明账号归属的截图，和/或一段文字说明（例如主页链接、
   背景说明，最多 2000 字，每轮只能一条）。两者至少要有一样，不得替用户编造。
3. 截图运行
   `viceme merchant onboarding evidence <申请编号> --path <截图路径> --lock-version <当前版本>`；
   文字说明运行
   `viceme merchant onboarding evidence <申请编号> --text "<说明>" --lock-version <当前版本>`。
   一条命令只传一种材料。
4. 每次上传成功后读取返回的新 `lockVersion`，下一条命令必须使用新版本；不得
   用旧版本号重试。
5. 材料齐后只运行一次 `viceme merchant onboarding submit <申请编号> --lock-version <最新版本>`。
   提交后到工作人员处理前不能再补充、修改或撤回；随后按「人工审核边界」结束本轮。

## 人工审核边界

普通申请和小红书渠道验证需要工作人员审核。除直接申请模式可以完成一次可选个人名片外，状态仍在审核、要求补充资料或拒绝时立即结束；
同一回合不得再次查询、sleep、轮询或暗示会自动通过。只有用户之后再次要求继续时才重新
从资格检查开始；只有用户明确要求持续监控时才创建监控。

审核要求补充证明时按「补充审核材料」一节引导用户提交截图与文字说明。审核拒绝时展示
可操作原因；用户明确要求重新申请后才继续。

## 交回调用玩法

仅当 CLI 确认当前用户拥有有效商家时，返回用户选择的 Merchant 并让原玩法继续。任何其他
状态都返回清楚的用户结果并停止原玩法，不把 DRAFT 创作者身份误当成资格。

如果调用方是 `$customize-your-page` 的普通模式，只交回同一个有效 Merchant，由它继续读取页面能力、
制作预览并在用户确认后发布；本 Skill 不重复实现作者页或作品页自定义流程。
