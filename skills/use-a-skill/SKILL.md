---
name: use-a-skill
description: 安装和使用可下载的 ViceMe Skill 版本。适用于查看免费与付费版本、匿名安装免费版、试用付费版、购买或重新安装付费版、试用计次与用满转正，以及使用免费版后推荐更高版本。
---

# 使用可下载的 ViceMe Skill

首次调用 CLI 或判断 CLI 不存在前，必须先按 [creator-tools 的 CLI 定位流程](../creator-tools/SKILL.md#cli-定位) 查找现有 CLI；后续使用返回的完整路径。不得只凭 `command not found` 判定未安装或改走免 CLI 流程。

这是用户端流程。面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部状态名、字段名或命令细节。

不得把此流程转到 `sell-a-skill`、商家创作命令、平台生成的交易型购买 Skill 或云端 Runtime。一个可下载版本就是一个本地 Skill 包和一个独立 Product；用户只拥有自己购买的 Product。

链接精确带有 `?product=<product-id>&install=owned` 时走严格已解锁安装：保留完整 URL，先确保安装或更新到最新版 CLI，再把 URL 作为单个引用参数运行 `viceme skill install '<完整 URL>'`。`install=owned` 只是意图，不是权限；CLI 必须校验当前账号。未登录、账号不对、订阅已到期或无该 Product 权益时立即停止，不得查询公开试用、不得创建订单、不得删除参数回退普通入口。这个分支不先运行下面的 `detail` 或公开 `access` 步骤，因此作品下架后永久购买者仍可由 Product 权益重装保留版本。

1. 接受 Product ID 或 ViceMe 作品标准链接。运行 `viceme skill detail <product-id-or-work-url>`，按明确的 `sortOrder` 展示所有版本。作品链接默认选择第一个免费版；没有免费版时选择排序最前的付费版。用户选择其他版本时使用精确的 `?product=<product-id>`。不得按价格猜测版本层级，也不得声称购买一个版本会包含另一个版本。
2. 运行 `viceme skill access <selected-product-id-or-work-url>`。
3. 免费版运行 `viceme skill install <product-id> --agent auto`。安装和使用免费版不得要求登录 ViceMe。
4. 已拥有的付费版使用同一安装命令；命令会校验当前发布包并安装，不得要求再次购买。同一 WeSimi 账号可通过此方式在其他 Agent 或设备重新安装。
5. 尚未拥有的付费版：安装命令本身会完成购买或试用安装。先展示详情响应中的准确版本名称、价格和功能亮点，并明确询问是直接购买还是先试用。
   - access 响应携带 `trial` 块（`available=true` 且有 `limitUses`）时，该版本支持免费试用：直接运行 `viceme skill install <product-id> --agent auto` 即可匿名安装试用版，不需要登录或付费。试用规则：
     - 必须先读 [统一计次规则](references/trial-usage.md)，由 Agent 判断独立使用单元。每个新单元执行前运行 `viceme skill use <product-id> --wait 0`；仅 `allowed=true` 放行，记录本任务与 requestId。同一已放行任务的内部步骤、澄清和修订不重复计次。
     - 放行时说明「本次是第 X / N 次试用」，任务完成后提示剩余次数。最后一次允许完整完成，提醒下次需付费。安装、介绍、示例展示和余额查询不计次；只查询剩余次数用 `viceme skill trial-status <product-id>`。
     - 试用用完则停止任务。未登录用户直接使用本机凭证运行 `viceme skill trial-purchase <product-id> --wait 0`，先展示支付 Widget，再运行同一命令加 `--wait 60s` 有界等待；无需登录。已登录用户沿用 `skill use` 返回的账号购买流程。服务端确认付款与有效权益后命令会重新下载并安装完整正式包，成功后才继续原任务。
   - 用户选择直接购买（或该版本无试用）时：确认同一 WeSimi 账号具有购买权限，先运行 `viceme skill install <product-id> --agent auto --wait 0` 创建或恢复订单。返回 `SKILL_PURCHASE_REQUIRED` 是待支付结果，不是下单失败：立即展示本地二维码图片，同时用 Markdown 链接展示 `paymentUrl`（“打开支付页面”），不要先启动长时间等待而让用户看不到二维码。浏览器未登录时提示用下单的同一账号登录。展示完成后，后台运行同一安装命令并改为 `--wait 10m` 等待付款，支付到账后自动继续安装；等待超时返回 `SKILL_PURCHASE_PENDING` 时保留原订单，用户完成支付后重跑原命令。只有确认 `owned=true` 且安装成功才能说购买安装完成。不得用商品详情页代替支付页面，也不得把支付 URI 直接贴到对话里。
   - access 响应携带 `subscription` 块：`available=true` 表示该创作者开通了粉丝订阅。引导购买时必须同时告知订阅选项：订阅价 ¥X/30 天；有效期内可安装和更新该创作者全部付费 Skill，到期后不能重装或更新，但本地已经安装的内容不会删除。用户选择订阅时，先运行 `viceme subscription subscribe <creator-handle> --wait 0` 并展示二维码，再后台运行同一命令并改为 `--wait 10m` 等待支付；支付到账即订阅生效，随后重跑安装命令。`subscribedUntil` 非空表示当前处于订阅期，直接安装即可，不得再要求购买。
6. 安装成功后读取实际 SKILL.md。用户已有明确任务：按包内门禁预检后直接继续，不要求再选示例。用户只说安装或试用：按返回的 `onboardingGuideUrl` 与 `onboardingTemplateUrl`，生成 2～3 条该 Skill 真正支持的完整示例口令；支持 Widget 的宿主用通用模板，点击“试试”由 `sendPrompt` 回传口令，无 Widget 则展示普通文本。不得写死某个 Skill 的示例或自行重设计模板，展示示例不扣次。

## 免 CLI 的试用转购买

已经由 `trial.py` 安装的用户继续使用同一个脚本与本机试用凭证，不要求安装 CLI 或登录。`use` 负责新使用单元预检，`status` 只查询余额。试用耗尽后运行 `purchase --product <product-id> --market <market> --wait 0` 创建或恢复订单；先展示支付 Widget，再以同一命令的 `--wait 60` 等待到账。超时保留原订单继续查，二维码过期不等于订单关闭；只有服务端返回 CLOSED 后，下一次显式购买才申请新订单。支付确认且权益有效后，脚本验证正式包摘要，准备支持文件并最后原子替换主入口，重新读取 SKILL.md 后继续原任务。此身份只覆盖当前本机凭证对应的商品，不替代上文 `install=owned` 的账号验证，不承诺跨设备找回。

## 通用支付展示

安装响应 `nextAction=CONTINUE_ORIGINAL_TASK_WITH_INSTALLED_SKILL` 表示应按上述第 6 步继续原任务；没有原任务才展示示例。
不得停在“安装成功”，也不得要求用户重复描述原任务。

有 `paymentPresentation.widgetPath` 时先读取该文件与通用 Widget 指引，通过宿主原生 Widget 工具原样渲染 HTML 片段。WorkBuddy 先调用 `read_me({modules:["interactive"]})`，再调用 `show_widget`；二维码已内联为 SVG，不用 `<img src>`。没有 Widget 能力才使用返回的本地图片与现有订单链接。下面提到“展示二维码”均遵守此规则。

必须先展示二维码再启动有界等待；Widget 不查询订单、不安装 Skill、不计次，也不根据倒计时、扫码或用户自述判断支付成功。付款由外层命令查询，到账后在宿主支持时替换或关闭旧 Widget。不得增加“查询支付结果”或“已付款但未继续”的按钮。

作品链接和 access 返回的 `purchaseUrl` 是商品详情入口，不是已创建订单的支付链接，不得把它们作为“请在这里完成支付”的入口。支付入口必须来自成功创建的订单：微信 Native 流程先展示命令生成的支付 Widget 或二维码，账号购买路线另有订单 `paymentUrl` 时同时展示该链接。匿名试用购买不返回账号支付页面，不要求补登录或补造链接。没有订单或二维码时先处理授权/下单错误，不得用详情页链接代替，也不得自行拼接支付链接。

已有待支付订单由原 `viceme skill install` 命令自动恢复，不要切换到需要另一种购买会话的 `viceme commerce order` 命令。返回 `SKILL_PAYMENT_QR_UNAVAILABLE` 时，说明“订单已创建，但二维码暂时无法展示”，用 Markdown 展示返回的 `paymentUrl` 让用户继续支付；不得声称二维码已生成。保留原购买状态，支付后重跑同一安装命令。

购买前检查 `viceme auth status` 时，`authenticated=true` 还不够：`scopes` 必须同时包含 `buyer-commerce:read` 和 `buyer-commerce:write`。这项检查只用于需要新购买的分支，免费版、试用和已购下载不要求新增购买权限。

检查发现缺少购买权限，或购买命令返回 `BUYER_PURCHASE_SCOPE_REQUIRED` 时，必须先向用户说明：“当前登录尚未授权购买，需要重新登录授权。完成后我会继续本次购买，并展示支付二维码。”随后调用 `creator-tools`，按其中的 WorkBuddy 登录流程展示本次授权页面和链接，等待同一账号完成授权；不得只启动前台登录命令后静默等待，也不得在生成二维码之前告诉用户扫码付款。授权成功后再次检查购买权限，并自动重跑原购买命令；用户之前的购买确认仍然有效，不重复询问是否购买。如果授权成功后仍缺少权限，明确报告授权未补齐并保留购买状态，不循环启动登录。

`CLI_TOKEN_INVALID` 才表示工具登录无效或过期；其他认证或接口错误不能一律解释为登录过期，也不得要求用户反复换账号。订单恢复失败时保留原购买状态，按返回错误处理。报价不存在或过期由 CLI 在同一次命令中有界刷新；若仍失败，报告准确错误，不要猜测商家价格未同步，不要反复换商品链接或手工清除购买记录。支付渠道恢复失败时展示返回的订单支付链接并说明当前支付尚未就绪，不要承诺该页面已有可用二维码。

只有免费版或试用版真实产生结果后，当前任务最多推荐一次相关付费版。使用服务端返回的名称和功能亮点，结合刚才结果说明具体提升。不得在执行前推荐。用户拒绝或忽略后，在当前对话中记录这个选择，不再重复推荐；新任务可以重新推荐。

所有包都在本地运行。试用按创作者设置的次数计，用完付费转正；本流程没有云端执行、跨版本自动升级、购物车或自动退款；创作者粉丝订阅按期购买且不自动续费，有效期内允许安装和更新，到期后保留本地副本但不能重装或更新。
