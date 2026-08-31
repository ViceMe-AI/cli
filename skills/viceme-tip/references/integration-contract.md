# ViceMe 打赏宿主接入契约

## Publish 交接

`$viceme-publish` 必须先确认以下平台资源已经可用：

```text
PUBLISHED + VERIFIED Website Work
├── ACTIVE SDK access（hosted features 含 tip）
└── ACTIVE PRODUCTION Website Widget application
    ├── canonical Origin
    └── products = []
```

本 Skill 只消费 publish 返回的公开 `workKey`、Web base URL、区域、canonical Origin 和完整 hosted feature set，不读取或修改上面的资源。

## Loader

一个页面、一个 `workKey` 只保留一个 loader：

```text
data-viceme-work
data-viceme-region
data-viceme-features
data-viceme-target
data-viceme-theme
```

features 只能是 `tip` 或规范顺序的 `danmaku,tip`。不要传 endpoint、token、金额、渠道、application ID 或支付状态。已有 loader 时更新属性，不插入第二条脚本。

## 运行时边界

Tip iframe 使用规范 Origin 建立可信嵌入上下文。Shop 拥有登录、金额选择、下单、支付渠道、状态和结算；SDK 校验消息来源并只转发脱敏事件。宿主不得复制结账页面、调用支付渠道 API 或接收支付秘密。界面能打开不代表支付已成交。

## CSP

保留现有指令。只把 publish 返回的精确 Shop Origin 加入确实需要的 script、connection 和 frame 指令。绝不使用通配符、宽泛子域或 `unsafe-eval`。
