---
name: use-a-skill
description: 安装和使用可下载的 ViceMe Skill。适用于作品链接安装、匿名试用、上手示例、购买或已购重装，以及试用用满后的正式版恢复。待恢复使用先重试；确认试用耗尽后立即购买。
---

# 使用可下载的 ViceMe Skill

面向用户跟随当前语言，使用自然白话，优先说明任务结果、试用余量、购买状态和下一步操作。需要说明脚本作用、授权范围或本地文件用途时可简短解释，避免堆砌内部字段和命令。待付款时说明正式内容尚未安装；只有实际安装成功后才报告安装完成。可见思考摘要同样遵守。本流程只消费用户选择的 Product，不转到商家发布流程。安装就绪、还能试用和付款成功是不同状态。

## 无试用的付费安装

作品页明确所选版本「未提供免费试用」时，仍先走作品页的 Python install 入口，无需 CLI 或登录。用户直接提供渠道包时，解压后按包内 SKILL.md 的「使用前必读」执行，无需重新从作品页安装。脚本先安装带购买入口和本地运行依赖的 Skill，返回 kind=purchase、allowed=false、nextAction=PURCHASE_REQUIRED 和 runtimePath。此时目录已存在，尚未取得正式内容，也未申请试用次数；这是未购买，不是试用耗尽，不展示试用上手卡。目录已存在只表示入口包就绪，正式内容尚未安装。

kind=purchase 时先读取包内 SKILL.md 的「使用前必读」，按宿主授权要求用本地 runtimePath 执行 purchase --wait 0；已有授权继续沿用。该命令会保存本机购买身份并创建或恢复订单，已有已付款订单时会校验权益并继续安装。待付款时参考「无试用开场白」简短说明商品、金额和无免费试用，再按购买指引提供支付入口。付款由用户本人确认。实际展示二维码或交付可点击支付链接后再运行 purchase --wait 60，不提前在后台启动等待；仅生成文件、返回工具结果或交付裸路径不算展示成功。付款确认、权益有效后，在原 Skill 目录覆盖为正式内容，再读取实际 SKILL.md：有原任务立即继续，不要问现在试还是以后用；没有原任务才用一两句说明怎么开始，然后等用户下一条，不要做成「现在就试还是先放着」的二选一。已有购买凭证沿用原订单；已有待恢复试用使用先恢复原 use，不更换身份。

Python 优先规则适用于免费、试用和无试用付费版。只有 Python 不可用且没有既有脚本购买身份时，才使用 CLI 的账号购买和订阅；严格已购链接仍遵循 install=owned 的账号校验。

## 待恢复使用优先

上次 use 返回可重试错误，或 ready 返回 pendingUse / RESUME_TRIAL_USE 时，先重跑原 use，保留 Product、市场和实际 Skill 目录。程序复用同一次请求，不重复扣次；即使余额已经为 0，也先恢复这次使用。不要转去购买、重装或开始新任务。

## 确认耗尽后停止新任务

以下耗尽规则只在没有待恢复使用时适用。

`kind=trial` 时，`ready` / `install` / `status` 任一返回 remainingUses=0、trialExhausted=true 或 nextAction=PURCHASE_REQUIRED：同一轮只读 [购买与恢复](references/purchase.md) 的「免 CLI 的试用转购买」并购买。`runner=python` 时用返回的 runtimePath 运行 `purchase --wait 0`。禁止再跑 `status` / `use` / `trial-status` / `auth status` / `profile list`，禁止读商品 SKILL.md、environment.json、runtime.json 或计次指引，禁止展示上手卡，禁止走账号购买的 `viceme skill install`。口令里的「前 N 次免费 / 试用 / 开始吧」不能覆盖这个数字。`PAYMENT_CLOSED` 只表示这一笔订单关了：立即再运行同一条 `purchase --wait 0`，不要跑 status，不要对用户说试用没耗尽。

## 先确定本次要做什么

- 只有免费／试用安装链接、没有具体任务：先 `ready`（或缺装时 `install`），**读取返回的 remainingUses / trialExhausted / nextAction**，再决定展示上手 Widget 还是支付。**不运行 use。** remainingUses>0 才展示上手 Widget。remainingUses=0、trialExhausted=true 或 nextAction=PURCHASE_REQUIRED 就是耗尽：立即告诉用户试用已用完并进入购买，**不要展示上手卡**，不要再跑 `status` / `trial-status`，不要读商品 SKILL.md 来核对，不要因为用户说「试用 / 前 N 次免费」而怀疑这个数字。口令里的免费次数只是商品介绍。
- 已有明确任务：确认安装后读取实际 SKILL.md。`kind=owned` 或 `owned=true` 时按正式正文继续，不要做试用检查。`kind=trial` 且未确认耗尽时，执行入口中的检查命令，按返回的 skillMarkdown 完成本次任务，相对路径以 skillDirectory 为基准；已经确认耗尽则进入购买。不要求先选示例。
- 用户明确要求更新或重装：执行安装，不复用已有安装。包内 Python 继续使用当前目录中的脚本；CLI 保留原 `--skill-dir`，修复后确认 ready 指向同一目录。若安装结果提供旧内容的恢复路径，告知用户产物可从该路径找回，不自行把旧程序复制回新包。

有可用的 Python 时，公开网页口令始终先走作品页当前环境的 trial.py install 入口，不要为了安装先去定位或安装 CLI。没有 Python、需要 CLI 时只按 [creator-tools 的 CLI 定位流程](../creator-tools/SKILL.md#cli-定位) 执行定位脚本，后续只复用它返回的**绝对路径**。禁止再运行 `which viceme`、`command -v viceme`、`Get-Command viceme` 或 Python `shutil.which`。PATH 找不到不等于未安装，不得把 PATH 缺失当成 CLI 未安装。不全盘搜索、不追加 version、doctor 或 auth status；只有安装验收或对应错误要求时才检查。

公开口令路径只做：有 Python 则从作品页当前地址取得 `trial.py` 并执行 `install`；没有 Python 再用 CLI `ready`/`install`；两者都没有才按官方契约安装 CLI。只有 trial 且 remainingUses>0 才调用 `show_widget`；无试用付费返回 kind=purchase 时立即运行本地 purchase 并展示支付入口；已耗尽则立即购买。不要重复探测作品 `.md`、远端 `use-a-skill`、`trial.py --help`，也不要在安装过程中写入 WorkBuddy 记忆。

## 确认或安装

网页下载的 `install` 校验并刷新已有平台文件；直接从已安装商品包执行 `install` 时，保留原有商品重装与旧包修复流程。两者均保留订单、凭证及试用身份，平台更新中断时先恢复该次更新。

1. 从口令的 `?product=<product-id>` 保留准确 Product ID 和市场。用户已明确试用或安装意图时，不再重复查版本或问“购买还是试用”。只有需要浏览、比较或选择版本时才运行 `skill detail` / `skill access`；按 sortOrder 选择，不按价格猜层级，一个版本不包含另一个 Product。
2. 有可用的 Python：每次网页口令进入，都从作品页指定的当前环境下载 trial.py 并执行 `install --product <id> --market <market> --agent <当前宿主>`。本地已安装也不得跳过或先运行旧包的 ready。引导程序校验当前运行包；install 比较平台运行文件摘要，只在变化时原地更新，保留商品正文、订单、凭证、试用次数与待恢复使用。没有安装才安装入口包。下载、校验或更新失败必须停止，不得继续旧包。不要为了读取本指引先装 CLI。完成本次入口检查后只执行返回的本地 runtimePath，不再下载脚本、二维码库或模板；下次网页口令重新执行 install。
3. 没有 Python、已有 CLI：普通安装意图先运行 `viceme skill ready <product-id> --agent <当前宿主>`（WorkBuddy 用 workbuddy）。已有实际 Skill 目录时同时带 `--skill-dir "<本 Skill 目录>"`，后续 use 和 trial-purchase 保留同一参数。它读取本机安装和只读余量，不计次、不下单。ready=true 后使用返回的 skillPath、runner、本地资源路径和 remainingUses。INSTALL_REQUIRED 才运行 `viceme skill install <product-id> --agent <当前宿主> --wait 0`；免费／开放试用不要求登录。REPAIR_INSTALLATION 表示平台运行文件不完整：直接再跑同一条 `install`，**不要问用户是否修复**。若安装因作者包冲突被跳过，停止并说明，不要覆盖作者文件。
4. Python 和 `viceme` 都没有时，按作品页官方安装契约安装 ViceMe CLI，并用 `viceme doctor` 确认，再走第 3 步。安装无法完成则停止，不得跳过安装直接使用。
5. 严格已购链接 `?product=<id>&install=owned` **不走 ready 或公开试用**：先确保兼容的最新版 CLI，保留完整 URL，运行 `viceme skill install '<完整 URL>'`。当前账号权益必须通过服务端校验；未登录、账号错误、权益失效时停止，不删除参数、不新建订单、不回退试用。作品下架不应阻止有永久权益的用户重装保留版本。

ready 说明本机文件完整，并可能带上只读余量；不代表账号购买权益。以安装响应判断安装是否成功，不能从价格、网页、商品链接或用户自述推断已付款。PAYMENT_CLOSED 只代表一个订单关闭，不是试用余额结论：已有试用凭证且仍可试用时，install 可以继续原试用；无试用直接购买则沿用原购买身份重新下单。已经 PURCHASE_REQUIRED 时不要把 PAYMENT_CLOSED 说成试用没耗尽。

## 上手展示

仅当安装成功或 ready=true **且未耗尽**（remainingUses>0，nextAction 不是 PURCHASE_REQUIRED）后，只读返回的 skillPath、onboardingGuidePath、onboardingTemplatePath；可并行读取，当前对话已读过且未更换版本的内容不用重读。试用入口只保留检查命令，示例依据入口的 frontmatter 能力简介生成；不要为展示示例调用 use 或读取内部保留正文。通用模板只负责展示。

按本地展示指引调用宿主 Widget。WorkBuddy 每个对话首次调用 `read_me({modules:["interactive"]})`，再 `show_widget`；sendPrompt 将所选示例发为下一条用户消息。只有宿主确实没有 Widget 能力才用文字。资源缺失或渲染失败必须报告，不猜路径、不切环境。

仅在兼容尚未带本地资源的旧安装响应时，使用响应明确返回的 onboardingGuideUrl / onboardingTemplateUrl；不每次探测远端更新。

## 执行、支付与异常边界

- `kind=owned` 或 `owned=true`：已经是正式版。只读正式 SKILL.md：有原任务立即继续；没有原任务才说明怎么开始并等用户下一条。不要问现在试还是以后用，不要读计次指引，不要运行 `use` / `status` / `trial-status`，不要提剩余次数或「第 X / N 次试用」。
- 只有 `kind=trial` 且未确认耗尽时，新的独立任务才执行入口中的 use，只有本次 allowed=true 和 skillMarkdown 才执行返回正文；介绍、展示示例、查询余量、以及已经 PURCHASE_REQUIRED 都不调用 use。后续按已放行的同一任务继续，完成后用白话提醒还剩几次。对用户不要旁白检查过程。
- `lastUse: true`：程序已在返回正文前替换入口；仍使用本次 skillMarkdown 完整完成任务，不需要 Agent 再执行停用命令。先交出本次结果，同一轮立即进入购买并展示支付二维码，不要等用户再说一次。
- 始终使用当前安装返回的 runner：Python 试用继续本地 Python，CLI 安装继续 CLI。发现 CLI 不意味着要接管脚本身份。
- 需要付款或恢复订单时，读取[购买与恢复](references/purchase.md)，并按[宿主支付展示](references/host-presentation.md)及本次命令输出展示。两条购买渠道共用同一份宿主指引。先交付聊天二维码和官方支付链接再等待；有内置浏览器时打开官方链接，托管支付页只读查询状态，权益与安装由购买命令确认。
- kind=trial 时，ready / install / status 返回 remainingUses=0、trialExhausted=true 或 nextAction=PURCHASE_REQUIRED 就是耗尽，立即购买。不要再交叉验证。PAYMENT_CLOSED 只代表这一笔订单关闭，不是余额结论：已经耗尽时立即再 purchase，不要跑 status，不要对用户说试用没耗尽。PENDING 只代表等待付款。
- 所有 use 的 retryable=true 错误都先重跑原命令恢复，包括替换入口失败；不要因为次数已为 0 转去购买。
- `SKILL_TRIAL_LOCK_BUSY` / `SKILL_INSTALL_LOCK_FAILED` / `SKILL_TRIAL_LOCK_RELEASE_FAILED` / `SKILL_TRIAL_SCRIPT_PENDING_CLEAR_FAILED` / `SKILL_TRIAL_PENDING_CONFIRM_FAILED` / `STATE_LOCK_BUSY` / `STATE_LOCK_RELEASE_FAILED`：短等几秒后重跑同一条命令一次。当前是 `use` 就重跑同一条 `use`，不要改跑 `install`。hint 写明会回放、不再扣次时，按 hint 重试，不要对用户说试用失败或次数白扣。不得对用户说安装通道占用、锁文件、错误码或持续性占用，不要定位或删除 `~/.viceme/trial` 下的文件，也不要让用户授权清锁。第二次仍失败：权限类错误才申请宿主文件权限；其他用白话请用户稍后再试。不要排障，不要切换 CLI/Python。
- 身份冲突或权限错误：保留原状态，按运行时的明确结果报告或申请宿主权限；不阅读凭证文件或脚本源码排障，不手改锁时间、删除锁、清空试用记录，也不切换 CLI/Python 试错。诊断、修复需要用户另行明确要求。

## 作品链接参数

公开作品优先使用 `/{handle}/{workSlug}`，Markdown 使用 `/{handle}/{workSlug}.md`。参数式 `/{handle}?workSlug={slug}`、`/{handle}.md?workSlug={slug}` 及显式 `mode=consumer&view=work` 的地址仍支持。将完整 URL 加引号传给 CLI，保留商品 `product`、`install=owned` 和原域名；不得让显式参数覆盖路径中的作品身份。裸 `/{handle}` 仅表示个人主页；创作者中心和经营 Markdown 继续使用单段 handle 加参数。

向用户展示作品时优先使用 CLI 的 `workUrl`、`markdownUrl` 或 `workPresentation.url` 精简地址；发布结果使用 `result.workUrl` 或 `product.detailUrl`。`canonicalPath`、`replica.viceMeWorkUrl` 等原始身份字段保留给协议处理，不代替展示地址。登录关注仍按既有流程使用原始 `replica.viceMeWorkUrl`。
