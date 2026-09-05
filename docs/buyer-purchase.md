# 工具内购买 Skill 与订阅

没有本机试用凭证时，`skill install` 的账号付费购买与 `subscription subscribe` 使用设备登录凭证调用
Shop `/v1/cli/product-quotes`、`/v1/cli/orders` 和
`/v1/cli/creator-subscription-orders`。匿名 Commerce Runtime 的原接口保持独立。

新登录请求 `buyer-commerce:read` 和 `buyer-commerce:write`。已购下载仍只要求
`skill-use:read`；旧登录只有在尝试新购买时才返回 `BUYER_PURCHASE_SCOPE_REQUIRED`，
提示重新授权一次。命令固定使用本次验证的用户与凭证，避免等待期间切换登录导致串单。

`buyer-purchases` 下的记录按 Profile、API、用户和目标隔离，文件权限为 `0600`。
发请求前保存请求正文及幂等 ID；网络错误或认证失败保留它们，重试接续同一订单。
并发创建由文件锁串行化，支付等待不持有创建锁。订阅超时后的重跑不会重复购买一期。
旧 `skill-purchases` 记录缺少账号与环境归属，不能作为新流程的购买凭证。

必须先部署支持上述 scope 和入口的 Shop API，再发布此 CLI。回滚保留服务端订单和
本地恢复记录。验收需覆盖购买、超时重跑、响应丢失、切换账号、订阅及到账自动安装。

## 试用安装与转正

可下载 Skill 的 frontmatter 接受发布端允许的扩展字段，如 `title`、`metadata`
和 `allowed-tools`，安装时保留作者内容。`name` 必须是非空字符串且与安装目录
匹配；`description` 可以省略，存在时必须是字符串。官方内置 Skill 必须填写
description 的内容规范在构建测试中单独检查，不再作为所有商品的安装门槛。

Go CLI 与免 CLI 的 `trial.py` 都在试用安装时生成两处内容：`SKILL.md` 的
frontmatter 后保留“使用前必读”入口，完整检查命令和次数、付费指引写入
`references/viceme-runtime.md`。Agent 按 `use-a-skill/references/trial-usage.md`
判断新的独立使用单元，执行前明确放行才继续；同一授权任务的内部步骤、澄清、修改
与失败重试不再重复申请。历史其他任务的放行、安装成功、检查失败都不能代替本次使用资格。
正文只是提到门禁标记不代表已完成注入。缺少内部规则时重新生成；不完整的生成段
或与作者同名的引用文件发生冲突时明确失败，不覆盖无关内容。

Go 由安装事务校验落盘摘要；Python 在报告试用安装成功前回读主文件与规则文件。
安装、重装不消费试用次数，服务端仍是计次权威，同机 Go/Python 共用凭证与未确认
请求的幂等键。付费后必须通过已购权限检查并重装正式包，替换试用内容和内部规则，
不能仅移除标记就宣称转正。这是面向正常使用流程的软门禁，不保证阻止手动修改。

当预检明确返回 `allowed=false`、`reason=EXHAUSTED` 和购买链接时，Go 和 Python
自动停用对应商品的试用入口：保留 `SKILL.md` 的原始 frontmatter，仅把正文
替换成“试用已结束”、购买链接及原路线免登录购买的恢复指引；账号已购入口仍为
`viceme skill install <product-id> --owned`。脚本、参考资料、用户产物、安装溯源和计次凭证均不删除。
最后一次 `allowed=true / remainingUses=0` 仍完整放行；网络错误、缺字段或格式
异常的响应不触发停用，并保留未确认请求以便重试。

停用只匹配同 Product 的托管试用标记，不改正式版、其他商品、未托管目录或链接文件。
Go/Python 的商品安装与停用共用 Product 锁，停用同时遵守原生目标目录锁及未完成
安装日志的归属，防止覆盖并发安装的正式版。正文以原子替换写入并回读验证；权限
拒绝时不退回截断写入，必须停止使用并通过宿主审批后重试。错误输出中的
`disabledSkillCount` 表示已停用入口数；部分目标失败时不会宣称全部成功。
旧对话已加载的内容无法从模型上下文撤回，本机制不代替使用前预检。

## 本机凭证购买与通用 Widget

`viceme skill trial-status <product-id>` / `trial.py status` 只查询余量，不扣次。
安装成功后已有原任务就继续；否则读取真实 Skill，生成 2～3 条示例，按响应中的
`onboardingGuideUrl` / `onboardingTemplateUrl` 展示通用模板。示例按钮使用
`sendPrompt`，无宿主能力时退回普通文本，不依赖剪贴板。

耗尽后 `viceme skill trial-purchase <product-id> --wait 0` 或
`trial.py purchase --product <product-id> --market cn --wait 0` 创建或恢复订单。
先展示 `paymentPresentation.widgetPath`，再用同一命令有界等待（Go `--wait 60s`，Python `--wait 60`）。
即使首次传了等待参数，也必须先返回二维码。等待超时和二维码过期都不等于订单关闭；
只查询原订单，不自动重开。确认付款后还需 ACTIVE 已购权益和正式制品校验才能恢复任务。

本机购买使用 `~/.viceme/trial/<product-id>.json` 的私有凭证和购买状态，无需安装 CLI
或强制登录。Python 初装的凭证可由 CLI 收编；Go 开始购买时也写入共享购买状态。
两端在同一 Product 锁内保存请求键与订单号，已开始的购买可由另一端接续。
不会为接续购买变更账号已购路由，不承诺匿名凭证丢失或跨设备后的权益找回。

通用支付模板属于 CLI 仓库 `widgets/`，与 Skill、订阅、复制等调用业务解耦。
只有订单、金额、支付方式、内联 SVG、绝对到期时间倒计时和状态；没有查询按钮或业务动作。
详细宿主接口、Python 资源摘要和降级规则见 [Widget 指引](../widgets/README.md)。
本地验收可运行 `node quality/widget-preview.cjs`，测试码不用于付款，示例仅在页面回显。

本轮新增 Shop `/skills/:productId/trial-purchase`、`/status`、`/download` 专用 POST
接口和 grant 到购买身份的持久关联 migration。必须先部署 API/migration，再发布 CLI、
脚本及稳定/摘要 Widget 托管物，最后更新 Web 引导。回滚不删除已购关联或历史摘要资源。
旧安装通过原入口重装获得新门禁；不会在后台自动改写用户已安装的 Skill。
