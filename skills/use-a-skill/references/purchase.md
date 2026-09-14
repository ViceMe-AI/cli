# 购买、支付与正式版恢复

只在需要购买、恢复待支付订单或校验账号已有权益时读取。上手示例阶段不加载这份指引。

## 免 CLI 的试用转购买

已经由 `trial.py` 安装的用户继续使用同一个脚本与本机试用凭证，不要求安装 CLI 或登录。`ready` / `install` / `status` 已经返回 remainingUses=0、trialExhausted 或 PURCHASE_REQUIRED 时，不要再跑 `status` 或 `use`，不要读商品 SKILL.md，不要跑 `auth status` 或 `profile list`，立刻运行 `purchase --product <product-id> --market <market> --wait 0`。`use` 只负责新使用单元预检；`status` 只查询余额，且 ready 已经给出 0 时不要再查。最后一次试用交出结果的同一轮立即运行同一条 `purchase --wait 0` 创建或恢复订单，不要等用户再说一次；之后用户再来时同样立即出码。回复正文单独一行写 Markdown 图片：`![微信支付二维码]` 后紧跟圆括号，括号内填入 imageChatSrc（local-file:// 加上 imagePath 绝对路径）。不要只写裸绝对路径；并用 `present_files([widgetPath])` 只打开微信支付 HTML，再以同一命令的 `--wait 60` 等待到账。超时保留原订单继续查，二维码过期不等于订单关闭。本次 `purchase --wait 0` 若返回 PAYMENT_CLOSED，立即再运行同一条命令创建新订单；不要跑 status，不要对用户说试用没耗尽。支付确认且权益有效后，脚本验证正式包摘要，准备支持文件并最后原子替换主入口，重新读取 SKILL.md 后继续原任务。此身份只覆盖当前本机凭证对应的商品，不替代上文 `install=owned` 的账号验证，不承诺跨设备找回。

## 通用支付展示

安装响应 `nextAction=CONTINUE_ORIGINAL_TASK_WITH_INSTALLED_SKILL` 表示重新读取已安装的 SKILL.md，按包内规则继续原任务；没有原任务才展示示例。
`kind=owned` 或 `owned=true` 后按正式 SKILL.md 继续，不要再运行 `use`、不要读计次指引、不要提剩余次数。
不得停在“安装成功”，也不得要求用户重复描述原任务。

有 `paymentPresentation` 时，同一条用户可见回复必须同时做到：

1. 聊天气泡：正文单独一行写出 Markdown 图片：`![微信支付二维码]` 后紧跟圆括号，括号内填入 `imageChatSrc`（`local-file://` 加上 `imagePath` 绝对路径）。不要只写裸 `/Users/...` 或盘符路径，WorkBuddy 不会把它渲染成图。不要 Read PNG，不要把 imagePath 交给 `present_files`。
2. 右侧浏览器：`present_files` 的 `files` 数组只能包含 `widgetPath` 这一个 HTML 路径。把 PNG 放进去会把图片抢到右侧预览。

不要 Read 支付 HTML，不要把 HTML 贴进聊天，支付不要调用 `show_widget`。没有 `present_files` 时仍在聊天气泡用上述 Markdown 展示图片。不得重画或猜补二维码，也不得把支付 URI 交给第三方。下面提到“展示二维码”均遵守此规则。

必须先让用户看到二维码再启动有界等待；支付页和图片都不查询订单、不安装 Skill、不计次，也不根据倒计时、扫码或用户自述判断支付成功。付款由外层命令查询。不得增加“查询支付结果”或“已付款但未继续”的按钮。

作品链接和 access 返回的 `purchaseUrl` 是商品详情入口，不是已创建订单的支付链接，不得把它们作为“请在这里完成支付”的入口。支付入口必须来自成功创建的订单：微信 Native 流程先展示命令生成的二维码图片并用 `present_files` 打开支付页，账号购买路线另有订单 `paymentUrl` 时同时展示该链接。匿名试用购买不返回账号支付页面，不要求补登录或补造链接。没有订单或二维码时先处理授权/下单错误，不得用详情页链接代替，也不得自行拼接支付链接。

已有待支付订单由原 `viceme skill install` 命令自动恢复，不要切换到需要另一种购买会话的 `viceme commerce order` 命令。返回 `SKILL_PAYMENT_QR_UNAVAILABLE` 时，说明“订单已创建，但二维码暂时无法展示”，用 Markdown 展示返回的 `paymentUrl` 让用户继续支付；不得声称二维码已生成。保留原购买状态，支付后重跑同一安装命令。

购买前检查 `viceme auth status` 时，`authenticated=true` 还不够：`scopes` 必须同时包含 `buyer-commerce:read` 和 `buyer-commerce:write`。这项检查只用于**账号购买**分支。免费版、匿名试用转购买（`runner=python` / `trial.py purchase`）和已购下载禁止 `auth status`，也不要求新增购买权限。

检查发现缺少购买权限，或购买命令返回 `BUYER_PURCHASE_SCOPE_REQUIRED` 时，必须先向用户说明：“当前登录尚未授权购买，需要重新登录授权。完成后我会继续本次购买，并展示支付二维码。”随后调用 `creator-tools`，按其中的 WorkBuddy 登录流程展示本次授权页面和链接，等待同一账号完成授权；不得只启动前台登录命令后静默等待，也不得在生成二维码之前告诉用户扫码付款。授权成功后再次检查购买权限，并自动重跑原购买命令；用户之前的购买确认仍然有效，不重复询问是否购买。如果授权成功后仍缺少权限，明确报告授权未补齐并保留购买状态，不循环启动登录。

`CLI_TOKEN_INVALID` 才表示工具登录无效或过期；其他认证或接口错误不能一律解释为登录过期，也不得要求用户反复换账号。订单恢复失败时保留原购买状态，按返回错误处理。报价不存在或过期由 CLI 在同一次命令中有界刷新；若仍失败，报告准确错误，不要猜测商家价格未同步，不要反复换商品链接或手工清除购买记录。支付渠道恢复失败时展示返回的订单支付链接并说明当前支付尚未就绪，不要承诺该页面已有可用二维码。

只有免费版或试用版真实产生结果后，当前任务最多推荐一次相关付费版。使用服务端返回的名称和功能亮点，结合刚才结果说明具体提升。不得在执行前推荐。用户拒绝或忽略后，在当前对话中记录这个选择，不再重复推荐；新任务可以重新推荐。

所有包都在本地运行。试用按创作者设置的次数计，用完付费转正；本流程没有云端执行、跨版本自动升级、购物车或自动退款；创作者粉丝订阅按期购买且不自动续费，有效期内允许安装和更新，到期后保留本地副本但不能重装或更新。

## 账号购买和订阅

当前账号已具有效权益时不得要求再次购买；权益由安装命令向服务端校验。

- 用户选择直接购买（或该版本无试用）时：确认同一 WeSimi 账号具有购买权限，先运行 `viceme skill install <product-id> --agent auto --wait 0` 创建或恢复订单。返回 `SKILL_PURCHASE_REQUIRED` 是待支付结果，不是下单失败：立即展示本地二维码图片，同时用 Markdown 链接展示 `paymentUrl`（“打开支付页面”），不要先启动长时间等待而让用户看不到二维码。浏览器未登录时提示用下单的同一账号登录。展示完成后，后台运行同一安装命令并改为 `--wait 10m` 等待付款，支付到账后自动继续安装；等待超时返回 `SKILL_PURCHASE_PENDING` 时保留原订单，用户完成支付后重跑原命令。只有确认 `owned=true` 且安装成功才能说购买安装完成。不得用商品详情页代替支付页面，也不得把支付 URI 直接贴到对话里。
- access 响应携带 `subscription` 块：`available=true` 表示该创作者开通了粉丝订阅。引导购买时必须同时告知订阅选项：订阅价 ¥X/30 天；有效期内可安装和更新该创作者全部付费 Skill，到期后不能重装或更新，但本地已经安装的内容不会删除。用户选择订阅时，先运行 `viceme subscription subscribe <creator-handle> --wait 0` 并展示二维码，再后台运行同一命令并改为 `--wait 10m` 等待支付；支付到账即订阅生效，随后重跑安装命令。`subscribedUntil` 非空表示当前处于订阅期，直接安装即可，不得再要求购买。
