# ViceMe 弹幕和赞赏接入契约

## 必需资源图

一次互动接入恰好包含这些权威资源：

```text
PUBLISHED + VERIFIED Website Work
├── ACTIVE SDK 访问（features 同时含 danmaku、tip）
└── ACTIVE PRODUCTION Website Widget 应用
    ├── workId = Website Work ID
    ├── origins = [Website canonical Origin]
    ├── returnUrls = []
    └── products = []
```

Work 是身份。公开 `workKey` 定位托管运行时访问。Website Widget 应用授权弹幕、支付入口与精确父 Origin；它不替代 Work，也不作为页面身份嵌入。

## Origin

规范 Origin 必须是小写规范 HTTPS，不含凭证、路径、查询、片段或尾部斜杠。Tip iframe 发送 `strict-origin` Referer，商店只在该精确 Origin 已注册时签发嵌入上下文。预览域名与生产域名是不同 Origin。

## 运行时边界

宿主只通过加载器传递 Work key、区域、特性集、目标容器和主题。不传弹幕写入凭据、金额、支付渠道、应用 ID、访问令牌或支付状态。商店拥有弹幕持久化、登录、下单、渠道协议、状态、结算与签名能力。

框架在可信 resize 握手完成前不可交互。商店拥有支付面板，Escape 时先重置为初始金额表单再发出关闭。SDK 只接受预期商店 Origin 与 iframe 窗口的消息，绑定首个有效 Work UUID，并转发脱敏的关闭与支付通知。宿主无需为默认关闭行为添加监听器。界面能打开不代表支付已成交。

宿主不得自行保存弹幕、复制结账页面、调用支付渠道 API 或接收支付秘密；脚本在 Shadow DOM 中渲染互动入口并打开 ViceMe 托管的界面。

## CSP

保留现有指令。只把精确 Profile Web Origin 加入确实需要的 script、connection 和 frame 指令。绝不使用通配符、宽泛 ViceMe 子域、`unsafe-eval` 或宿主自提供的支付脚本。
