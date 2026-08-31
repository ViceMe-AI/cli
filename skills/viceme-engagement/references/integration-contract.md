# ViceMe 网站互动接入契约

## 官方加载器

当前固定 CLI Profile 是环境权威。其 `webBaseUrl` 提供稳定的 `/viceme-sdk/v1/viceme.min.js` 加载器，`marketRegion` 只取 `cn` 或 `global`。加载器只接受以下属性：

```text
data-viceme-work
data-viceme-region
data-viceme-features
data-viceme-target
data-viceme-theme
data-viceme-loader
```

特性只能是 `danmaku`、`tip`，或去重并规范化后的 `danmaku,tip`。不得通过属性传递 endpoint、token、金额、渠道、application ID 或支付状态。

## 权威资源图

所有分支共用：

```text
PUBLISHED + VERIFIED Website Work
└── ACTIVE SDK access
    └── features 包含本页面请求的特性
```

包含赞赏的分支额外复用：

```text
ACTIVE PRODUCTION Website Widget application（Work 级共享）
├── workId = Website Work ID
├── origins 包含 Website canonical Origin
├── returnUrls = Shop/SDK access 管理的现有值，可为空
└── products = Shop 管理的现有绑定，可为空
```

Work 是身份，公开 `workKey` 只来自 SDK access。Website Widget 授权赞赏入口与精确父 Origin，不替代 Work。Engagement 不管理 Product 绑定或 return URL；平台管理的共享配置可能非空。

## Origin 与运行时

规范 Origin 必须是小写 HTTPS，不含凭证、路径、查询、片段或尾部斜杠。预览域名与生产域名是不同 Origin。

SDK 在本地初始化，通过 Shadow DOM 挂载 Shop 托管 iframe，校验消息的 Origin 与窗口，并负责清理节点、监听器和计时器。Shop 拥有弹幕持久化、限流、键盘与减少动画行为，也拥有登录、下单、渠道协议、支付状态、结算和签名能力。

宿主只传 Work key、区域、特性集、目标容器和主题，不传用户、session、金额、渠道、application ID、访问令牌或支付状态。界面打开不是消息持久化或支付结算的证明。

## CSP

保留现有指令。只把精确 Profile Web Origin 加入确实需要的 script、connection 和 frame 指令。绝不使用通配符、宽泛 ViceMe 子域、`unsafe-eval` 或宿主自提供的支付脚本。
