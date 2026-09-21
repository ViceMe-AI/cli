# 工具内购买 Skill 与订阅

没有本机试用凭证时，`skill install` 的账号付费购买与 `subscription subscribe` 使用设备登录凭证调用
Shop `/v1/cli/product-quotes`、`/v1/cli/orders` 和
`/v1/cli/creator-subscription-orders`。匿名 Commerce Runtime 的原接口保持独立。

新登录请求 `buyer-commerce:read` 和 `buyer-commerce:write`。已购下载仍只要求
`skill-use:read`；旧登录只有在尝试新购买时才返回 `BUYER_PURCHASE_SCOPE_REQUIRED`，
提示重新授权一次。命令固定使用本次验证的用户与凭证，避免等待期间切换登录导致串单。

`buyer-purchases` 下的记录按 Profile、API、用户和目标隔离，文件权限为 `0600`。
发请求前保存请求正文及幂等 ID；网络错误或认证失败保留它们，重试接续同一订单。
并发创建由文件锁串行化，支付二维码落盘也在同一把锁内完成；支付等待不持有创建锁。订阅超时后的重跑不会重复购买一期。
旧 `skill-purchases` 记录缺少账号与环境归属，不能作为新流程的购买凭证。

必须先部署支持上述 scope 和入口的 Shop API，再发布此 CLI。回滚保留服务端订单和
本地恢复记录。验收需覆盖购买、超时重跑、响应丢失、切换账号、订阅及到账自动安装。

## 试用安装与转正

可下载 Skill 的 frontmatter 接受发布端允许的扩展字段，如 `title`、`metadata`
和 `allowed-tools`，安装时保留作者内容。`name` 必须是非空字符串且与发布包目录
匹配；第三方安装后的目录别名可以不同。`description` 可以省略，存在时必须是字符串。官方内置 Skill 必须填写
description 的内容规范在构建测试中单独检查，不再作为所有商品的安装门槛。

Go CLI 与免 CLI 的 `trial.py` 在试用安装时生成三处内容：

- `SKILL.md` 保留原 frontmatter，正文只放直接可执行的使用检查入口。
- `.viceme/trial-body.md` 保存去掉生成门禁后的作者正文，由 runtime manifest 记录摘要。
- `references/viceme-runtime.md` 保存详细的计次与支付规则。

新独立任务先运行 use。客户端校验实际安装的 Product、市场、API 归属和正文摘要，
再向服务端申请一次使用。只有允许响应才返回 `skillMarkdown` 与 `skillDirectory`；
Agent 按该正文执行，相对资源路径以该目录为基准。拒绝或错误响应不返回任务正文。
安装、介绍、示例和余额查询不扣次；同一已获准任务的澄清与局部修改不重复计次。

最后一次 `allowed=true / remainingUses=0` 按以下顺序处理：读取本次正文 →
原子替换试用入口并回读验证 → 确认未完成请求已交付 → 返回本次正文。
响应带 `lastUse=true`、`entrySuspended=true` 和 `disabledSkillCount`。Agent 仍应
完整交付最后一次任务，再在同一轮展示购买入口；不依赖 Agent 在任务完成后另跑停用命令。

替换失败保留未确认请求，不交付正文；原命令重试由服务端按相同 requestId 回放，
不重复扣次。ready 的 `pendingUse / RESUME_TRIAL_USE` 优先于耗尽判断，purchase
也会拒绝越过待恢复使用。网络错误、缺字段或格式异常不会被当作成功。
Go/Python 用同一跨进程锁保护请求分配、服务端调用、入口替换与请求确认。
正在执行的任务不会被另一个独立任务当作失败重试；后一个调用需等待前者结束后重试。
锁仍属于存活进程时不能因经过五分钟而抢占，未确认请求也不能仅按时间丢弃。
ready 查询到 0、拒绝使用或恢复未支付订单时也会补做停用；创建订单本身不证明耗尽，
提前购买且仍有余量时不替换入口。ready 不再隐藏替换失败。status 始终只读。

停用保留原始 frontmatter，只替换入口正文；脚本、参考资料、用户产物、安装溯源
和计次凭证不删除。Go/Python 共用 Product 锁，停用同时遵守原生目标目录锁和
未完成安装日志。权限拒绝不退回截断写入；部分目标失败不会宣称全部成功。
只处理匹配 Product、API/市场的托管试用，不覆盖正式版、其他商品或链接入口。

工作区安装不要求文件夹名与 frontmatter.name 相同。Python 以当前包内脚本位置
定位实际 Skill，普通更新、修复和付款恢复均写回该目录。CLI 的 install、ready、use、
trial-purchase 使用 `--skill-dir` 指定实际目录，并验证 Product、市场与 API 归属。
不能在一个目录报告修复成功，却继续从另一个目录使用旧版本。

两种安装器共用发布文件归属清单：新包覆盖受管文件，移除新版本已删除的旧发布文件；
未受管的用户产物和本地依赖保留。正常 Python 虚拟环境中的符号链接不会阻断付款恢复，
保留链接时不跟随读取其目标。付款恢复仍需要服务端有效权益和正式包摘要验证。
新包与未受管用户文件发生同路径内容冲突时停止安装并保留原目录，不静默覆盖用户文件。

历史安装没有完整的发布文件清单，不能猜测未知文件属于作者还是用户。迁移时先把旧内容
保全到宿主不会加载的恢复目录，成功响应提供找回路径，活动目录只安装新包。
保全或安装失败保留可恢复的旧内容，不以成功响应掩盖部分失败。

此次修复不变更 Shop API、数据库或试用额度规则。发布需要同时更新 CLI 与独立
Python runtime ZIP/bootstrap。旧 Skill 中的内嵌脚本不会随 CLI 后台升级自动更新：
官方安装需要重新安装/刷新 Skill，第三方平台需要重新导出并分发新包。
旧安装缺少受校验正文时，新 use 返回修复要求且不扣次；购买恢复只要求安装归属可验证，
可以用正式包修复旧运行文件。重装保留同一凭证和服务端余量。

本流程改善正常调用与自动替换的确定性；本地内容泄露或人为篡改不在本次修复范围。

## 本机凭证购买与通用 Widget

`viceme skill trial-status <product-id>` / `trial.py status` 只查询余量，不扣次。
安装成功后已有原任务就继续；否则依据 Skill 的能力简介生成 2～3 条示例，按响应中的
`onboardingGuideUrl` / `onboardingTemplateUrl` 展示通用模板。示例按钮使用
`sendPrompt`，无宿主能力时退回普通文本，不依赖剪贴板。

耗尽后 `viceme skill trial-purchase <product-id> --wait 0` 或
`trial.py purchase --product <product-id> --market cn --wait 0` 创建或恢复订单。
支付展示统一遵循[宿主支付展示](../payments/host-presentation.md)：聊天中优先展示一张本地二维码，始终保留官方支付链接，有内置浏览器时打开该链接。本地付款 HTML 已退役。先展示再使用原购买命令有界等待（Go `--wait 60s`，Python `--wait 60`）。
即使首次传了等待参数，也必须先返回二维码。等待超时和二维码过期都不等于订单关闭；
只查询原订单，不自动重开。确认付款后还需 ACTIVE 已购权益和正式制品校验才能恢复任务。

本机购买使用 `~/.viceme/trial/<product-id>.json` 的私有凭证和购买状态，无需安装 CLI
或强制登录。Python 初装的凭证可由 CLI 收编；Go 开始购买时也写入共享购买状态。
两端在同一 Product 锁内保存请求键与订单号，已开始的购买可由另一端接续。
不会为接续购买变更账号已购路由，不承诺匿名凭证丢失或跨设备后的权益找回。

公共支付展示规则属于 CLI 的 `payments/` 功能层，Skill 购买与网站复制购买共用同一维护源。
官方收银台按服务端订单状态原地显示已支付；聊天二维码保持静态。上手示例 Widget 仍保留，可用 `node quality/widget-preview.cjs` 本地预览，示例仅在页面回显。

本轮新增 Shop `/skills/:productId/trial-purchase`、`/status`、`/download` 专用 POST
接口和 grant 到购买身份的持久关联 migration。必须先部署 API/migration，再发布 CLI、
脚本及稳定/摘要 Widget 托管物，最后更新 Web 引导。回滚不删除已购关联或历史摘要资源。
旧安装通过原入口重装获得新门禁；不会在后台自动改写用户已安装的 Skill。
