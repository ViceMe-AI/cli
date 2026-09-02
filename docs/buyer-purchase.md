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
