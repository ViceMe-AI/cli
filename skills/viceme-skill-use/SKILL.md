---
name: viceme-skill-use
description: 安装和使用可下载的 ViceMe Skill 版本。适用于查看免费与付费版本、匿名安装免费版、购买或重新安装付费版、购买后继续原任务，以及使用免费版后推荐更高版本。
---

# 使用可下载的 ViceMe Skill

这是用户端流程。面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部状态名、字段名或命令细节。

不得把此流程转到 `sell-a-skill`、商家创作命令、平台生成的交易型购买 Skill 或云端 Runtime。一个可下载版本就是一个本地 Skill 包和一个独立 Product；用户只拥有自己购买的 Product。

1. 接受 Product ID 或 ViceMe 作品标准链接。运行 `viceme skill detail <product-id-or-work-url>`，按明确的 `sortOrder` 展示所有版本。作品链接默认选择第一个免费版；没有免费版时选择排序最前的付费版。用户选择其他版本时使用精确的 `?product=<product-id>`。不得按价格猜测版本层级，也不得声称购买一个版本会包含另一个版本。
2. 运行 `viceme skill access <selected-product-id-or-work-url>`。
3. 免费版运行 `viceme skill install <product-id> --agent auto`。安装和使用免费版不得要求登录 ViceMe。
4. 已拥有的付费版使用同一安装命令；命令会校验当前发布包并安装，不得要求再次购买。同一 WeSimi 账号可通过此方式在其他 Agent 或设备重新安装。
5. 尚未拥有的付费版：安装命令本身会完成购买。先展示详情响应中的准确版本名称、价格和功能亮点，并明确询问是否购买。用户确认后，确保 `viceme auth status` 显示同一 WeSimi 账号已登录，然后直接运行 `viceme skill install <product-id> --agent auto --wait 10m`：命令会自动创建订单并生成本地微信支付二维码，把二维码图片给用户扫码；支付到账后安装自动继续，只有返回 `owned=true` 后才能说购买成功。`--wait 0` 表示只出二维码不等待；等待超时返回 `SKILL_PURCHASE_PENDING` 时，让用户完成扫码后重跑同一命令——命令会接续原订单，不会重复开单。不得让用户去浏览器打开购买链接，也不得把二维码链接文本或支付 URI 直接贴到对话里。
   - access 响应携带 `subscription` 块：`available=true` 表示该创作者开通了粉丝订阅。引导购买时必须同时告知订阅选项：订阅价 ¥X/30 天，订阅后该创作者全部付费 Skill 都能免费使用；用户选择订阅时，运行 `viceme subscription subscribe <creator-handle> --wait 10m`，同样把生成的支付二维码给用户扫码，支付到账即订阅生效，随后重跑安装命令。`subscribedUntil` 非空表示当前处于订阅期，直接安装即可，不得再要求购买。
6. 安装返回 `nextAction=CONTINUE_ORIGINAL_TASK_WITH_INSTALLED_SKILL` 时，立即调用返回的 `invocation` 并继续用户原来的任务。不得停在“安装成功”，也不得要求用户重复描述任务。

购买返回 `BUYER_PURCHASE_SCOPE_REQUIRED` 时，说明“当前登录还没有授权购买，需要补一次授权”，按 `creator-tools` 完成同一账号的登录授权后重跑原命令。`CLI_TOKEN_INVALID` 才表示工具登录无效或过期；其他认证或接口错误不能一律解释为登录过期，也不得要求用户反复换账号。订单恢复失败时保留原购买状态，按返回错误处理。

只有免费版真实产生结果后，当前任务最多推荐一次相关付费版。使用服务端返回的名称和功能亮点，结合刚才结果说明具体提升。不得在执行前推荐。用户拒绝或忽略后，在当前对话中记录这个选择，不再重复推荐；新任务可以重新推荐。

所有包都在本地运行。本流程没有使用配额、云端执行、跨版本自动升级、购物车或自动退款；创作者粉丝订阅按月买断，到期自然失效，不自动续费。
