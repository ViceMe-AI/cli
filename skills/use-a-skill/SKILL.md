---
name: use-a-skill
description: 安装和使用可下载的 ViceMe Skill。适用于作品链接安装、匿名试用、上手示例、购买或已购重装，以及试用用满后的正式版恢复。ready 返回 remainingUses=0 时立即购买，不要再核对「前 N 次免费」。
---

# 使用可下载的 ViceMe Skill

面向用户跟随当前语言，使用自然白话。只说任务结果、还剩几次试用、试用已用完、请扫码付款。不得对用户说 use、trial、trial.py、viceme、CLI、放行、预检、requestId、nextAction、widgetPath 或任何命令行；这些只出现在工具调用里。可见思考摘要同样遵守。本流程只消费用户选择的 Product，不转到商家发布流程。安装就绪、还能试用和付款成功是不同状态。

## 硬停止

`ready` / `install` / `status` 任一返回 remainingUses=0、trialExhausted=true 或 nextAction=PURCHASE_REQUIRED：同一轮只读 [购买与恢复](references/purchase.md) 的「免 CLI 的试用转购买」并购买。`runner=python` 时用返回的 runtimePath 运行 `purchase --wait 0`。禁止再跑 `status` / `use` / `trial-status` / `auth status` / `profile list`，禁止读商品 SKILL.md、environment.json、runtime.json 或计次指引，禁止展示上手卡，禁止走账号购买的 `viceme skill install`。口令里的「前 N 次免费 / 试用 / 开始吧」不能覆盖这个数字。`PAYMENT_CLOSED` 只表示这一笔订单关了：立即再运行同一条 `purchase --wait 0`，不要跑 status，不要对用户说试用没耗尽。

## 先确定本次要做什么

- 只有安装／试用链接、没有具体任务：先 `ready`（或缺装时 `install`），**读取返回的 remainingUses / trialExhausted / nextAction**，再决定展示上手 Widget 还是支付。**不运行 use。** remainingUses>0 才展示上手 Widget。remainingUses=0、trialExhausted=true 或 nextAction=PURCHASE_REQUIRED 就是耗尽：立即告诉用户试用已用完并进入购买，**不要展示上手卡**，不要再跑 `status` / `trial-status`，不要读商品 SKILL.md 来核对，不要因为用户说「试用 / 前 N 次免费」而怀疑这个数字。口令里的免费次数只是商品介绍。
- 已有明确任务：确认安装后读取实际 SKILL.md。`kind=owned` 或 `owned=true` 时按正式正文继续，不要做试用检查。`kind=trial` **且 remainingUses>0** 才沿用包内计次路线并执行一次 use；已经耗尽则走硬停止，不读商品 SKILL.md。不要求先选示例。
- 用户明确要求更新或重装：执行安装，不复用已有安装。

有可用的 Python 时，公开安装先走作品页 trial.py 入口或已装包内脚本，不要为了安装先去定位或安装 CLI。没有 Python、需要 CLI 时只按 [creator-tools 的 CLI 定位流程](../creator-tools/SKILL.md#cli-定位) 执行定位脚本，后续只复用它返回的**绝对路径**。禁止再运行 `which viceme`、`command -v viceme`、`Get-Command viceme` 或 Python `shutil.which`。PATH 找不到不等于未安装，不得把 PATH 缺失当成 CLI 未安装。不全盘搜索、不追加 version、doctor 或 auth status；只有安装验收或对应错误要求时才检查。

口令路径只做：有 Python 则 `trial.py` `ready`/`install`；没有 Python 再用 CLI `ready`/`install`；两者都没有才按官方契约安装 CLI。remainingUses>0 才调用 `show_widget`；已耗尽则立即购买。不要 curl 作品 `.md`、远端 `use-a-skill`、`trial.py --help`，也不要在安装过程中写入 WorkBuddy 记忆。

## 确认或安装

1. 从口令的 `?product=<product-id>` 保留准确 Product ID 和市场。用户已明确试用或安装意图时，不再重复查版本或问“购买还是试用”。只有需要浏览、比较或选择版本时才运行 `skill detail` / `skill access`；按 sortOrder 选择，不按价格猜层级，一个版本不包含另一个 Product。
2. 有可用的 Python：已有商品 Skill 时，执行其包内 `.viceme/scripts/trial.py ready --product <id> --market <market>`，本 Skill 目录来自宿主的实际安装路径。否则按作品页现有 trial.py 安装入口执行一次；引导程序校验完整运行包后随商品安装。不要为了读取本指引先装 CLI。之后只执行返回的本地 runtimePath，不再 curl 脚本、二维码库或模板。
3. 没有 Python、已有 CLI：普通安装意图先运行 `viceme skill ready <product-id> --agent <当前宿主>`（WorkBuddy 用 workbuddy）。它读取本机安装和只读余量，不计次、不下单。ready=true 后使用返回的 skillPath、runner、本地资源路径和 remainingUses。INSTALL_REQUIRED 才运行 `viceme skill install <product-id> --agent <当前宿主> --wait 0`；免费／开放试用不要求登录。REPAIR_INSTALLATION 表示平台运行文件不完整：直接再跑同一条 `install`，**不要问用户是否修复**。若安装因作者包冲突被跳过，停止并说明，不要覆盖作者文件。
4. Python 和 `viceme` 都没有时，按作品页官方安装契约安装 ViceMe CLI，并用 `viceme doctor` 确认，再走第 3 步。安装无法完成则停止，不得跳过安装直接使用。
5. 严格已购链接 `?product=<id>&install=owned` **不走 ready 或公开试用**：先确保兼容的最新版 CLI，保留完整 URL，运行 `viceme skill install '<完整 URL>'`。当前账号权益必须通过服务端校验；未登录、账号错误、权益失效时停止，不删除参数、不新建订单、不回退试用。作品下架不应阻止有永久权益的用户重装保留版本。

ready 说明本机文件完整，并可能带上只读余量；不代表账号购买权益。以安装响应判断安装是否成功，不能从价格、网页、商品链接或用户自述推断已付款。PAYMENT_CLOSED 只代表一个订单关闭，install 应继续试用安装，不要把它当成耗尽；已经 PURCHASE_REQUIRED 时也不要把 PAYMENT_CLOSED 说成试用没耗尽。

## 上手展示

仅当安装成功或 ready=true **且未耗尽**（remainingUses>0，nextAction 不是 PURCHASE_REQUIRED）后，只读返回的 skillPath、onboardingGuidePath、onboardingTemplatePath；可并行读取，当前对话已读过且未更换版本的内容不用重读。示例由实际 SKILL.md 的能力生成，通用模板只负责展示，不能自行重设计卡片。

按本地展示指引调用宿主 Widget。WorkBuddy 每个对话首次调用 `read_me({modules:["interactive"]})`，再 `show_widget`；sendPrompt 将所选示例发为下一条用户消息。只有宿主确实没有 Widget 能力才用文字。资源缺失或渲染失败必须报告，不猜路径、不切环境。

仅在兼容尚未带本地资源的旧安装响应时，使用响应明确返回的 onboardingGuideUrl / onboardingTemplateUrl；不每次探测远端更新。

## 执行、支付与异常边界

- `kind=owned` 或 `owned=true`：已经是正式版。只读正式 SKILL.md 并继续任务或展示上手示例；不要读计次指引，不要运行 `use` / `status` / `trial-status`，不要提剩余次数或「第 X / N 次试用」。
- 只有 `kind=trial` **且 remainingUses>0** 时，新的独立任务才读取包内计次指引并执行一次 use；介绍、展示示例、查询余量、以及已经 PURCHASE_REQUIRED 都不调用 use。后续按已放行的同一任务继续，完成后用白话提醒还剩几次。对用户不要旁白检查过程。
- `lastUse: true` 或完成后 `remainingUses=0`：先交出本次结果，同一轮立即进入购买并展示支付二维码，不要等用户再说一次。
- 始终使用当前安装返回的 runner：Python 试用继续本地 Python，CLI 安装继续 CLI。发现 CLI 不意味着要接管脚本身份。
- 需要付款或恢复订单时再读 [购买与恢复](references/purchase.md)。回复正文单独一行写 Markdown 图片：`![微信支付二维码]` 后紧跟圆括号，括号内填入 imageChatSrc（local-file:// 加上 imagePath）。不要只写裸绝对路径，WorkBuddy 聊天气泡不会显示。同时 `present_files([widgetPath])` **只**打开支付 HTML。不要把 imagePath 或 PNG 交给 `present_files`，不要 Read 支付 HTML 或 PNG，支付不要调用 `show_widget`。支付页不查询订单、不计次、不安装 Skill。
- ready / install / status 返回 remainingUses=0、trialExhausted=true 或 nextAction=PURCHASE_REQUIRED 就是耗尽，立即购买。不要再交叉验证。PAYMENT_CLOSED 只代表这一笔订单关闭，不是余额结论：已经耗尽时立即再 purchase，不要跑 status，不要对用户说试用没耗尽。PENDING 只代表等待付款。
- `SKILL_TRIAL_LOCK_BUSY` / `SKILL_INSTALL_LOCK_FAILED` / `SKILL_TRIAL_LOCK_RELEASE_FAILED` / `SKILL_TRIAL_SCRIPT_PENDING_CLEAR_FAILED` / `SKILL_TRIAL_PENDING_CONFIRM_FAILED` / `STATE_LOCK_BUSY` / `STATE_LOCK_RELEASE_FAILED`：短等几秒后重跑同一条命令一次。当前是 `use` 就重跑同一条 `use`，不要改跑 `install`。hint 写明会回放、不再扣次时，按 hint 重试，不要对用户说试用失败或次数白扣。不得对用户说安装通道占用、锁文件、错误码或持续性占用，不要定位或删除 `~/.viceme/trial` 下的文件，也不要让用户授权清锁。第二次仍失败：权限类错误才申请宿主文件权限；其他用白话请用户稍后再试。不要排障，不要切换 CLI/Python。
- 身份冲突或权限错误：保留原状态，按运行时的明确结果报告或申请宿主权限；不阅读凭证文件或脚本源码排障，不手改锁时间、删除锁、清空试用记录，也不切换 CLI/Python 试错。诊断、修复需要用户另行明确要求。
