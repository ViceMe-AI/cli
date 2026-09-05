# 工具内购买 Skill 与订阅

`skill install` 的付费购买与 `subscription subscribe` 使用设备登录凭证调用
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
`references/viceme-runtime.md`。每次开始或继续任务必须先执行检查，本次明确
放行才继续；历史放行、安装成功、检查失败都不能代替本次使用资格。
正文只是提到门禁标记不代表已完成注入。缺少内部规则时重新生成；不完整的生成段
或与作者同名的引用文件发生冲突时明确失败，不覆盖无关内容。

Go 由安装事务校验落盘摘要；Python 在报告试用安装成功前回读主文件与规则文件。
安装、重装不消费试用次数，服务端仍是计次权威，同机 Go/Python 共用凭证与未确认
请求的幂等键。付费后必须通过已购权限检查并重装正式包，替换试用内容和内部规则，
不能仅移除标记就宣称转正。这是面向正常使用流程的软门禁，不保证阻止手动修改。

当预检明确返回 `allowed=false`、`reason=EXHAUSTED` 和购买链接时，Go 和 Python
自动停用对应商品的试用入口：保留 `SKILL.md` 的原始 frontmatter，仅把正文
替换成“试用已结束”、购买链接及 `viceme skill install <product-id> --owned`
的恢复指引。脚本、参考资料、用户产物、安装溯源和计次凭证均不删除。
最后一次 `allowed=true / remainingUses=0` 仍完整放行；网络错误、缺字段或格式
异常的响应不触发停用，并保留未确认请求以便重试。

停用只匹配同 Product 的托管试用标记，不改正式版、其他商品、未托管目录或链接文件。
Go/Python 的商品安装与停用共用 Product 锁，停用同时遵守原生目标目录锁及未完成
安装日志的归属，防止覆盖并发安装的正式版。正文以原子替换写入并回读验证；权限
拒绝时不退回截断写入，必须停止使用并通过宿主审批后重试。错误输出中的
`disabledSkillCount` 表示已停用入口数；部分目标失败时不会宣称全部成功。
旧对话已加载的内容无法从模型上下文撤回，本机制不代替使用前预检。

本修复不改变 `.md` 口令、Shop API 或数据库。发布新版 CLI 与官方 `trial.py`
后，旧安装可通过原入口重装获得新门禁；不会在后台自动改写用户已安装的 Skill。
