# 托管 SDK 契约

## Loader

所选 CLI Profile 是环境权威来源。其 `webBaseUrl` 提供稳定的 `/viceme-sdk/v1/viceme.min.js` loader，`marketRegion` 选择 `cn` 或 `global`。loader 只接受以下属性：

```text
data-viceme-work
data-viceme-region
data-viceme-features
data-viceme-target
data-viceme-theme
data-viceme-loader
```

Danmaku 独立流程只传一次 `danmaku`。不要通过属性传 endpoint、token、价格、provider、application ID 或支付状态。

面向用户的文字跟随用户当前语言；页面 locale 不得反向选择市场或 Profile。

## 资源门禁

弹幕页面只有在 Website Work 为 `PUBLISHED`、website ownership 为 `VERIFIED`、ACTIVE SDK access 的 hosted features 包含 `danmaku` 时可用。公开 production Work key 只能来自该 access 的 `keys.live`；`keys.test` 和 `keys.live` 都是永久公开标识，不是凭据。

同一 SDK access 还可能包含 `accessFeatures`。只更新 hosted Danmaku 时必须从精确 `configVersion` 出发，不传 follow/purchase flags，并在写后确认 `accessFeatures` 与两个 key 完整保留。

## 运行时

外部 SDK 在本地初始化，不直接发业务 API 请求。它在 Shadow DOM 下挂载 Shop 托管 iframe，验证 bridge 的 Origin 与 source，并清理自身节点、监听器和定时器。Shop 负责消息持久化、限流、键盘行为和减少动画。

loader 从规范页面 URL 与当前滚动区间派生不透明的位置锚点。宿主不得派生或发送自己的用户、session 或支付身份。

## CSP

只为 `script-src`、`connect-src` 与 `frame-src` 加入精确区域 Shop Origin，保留 `object-src 'none'`。带 `strict-dynamic` 的 request nonce 可以授权脚本，但 frame 和 manifest connection 仍需显式 Origin。不得使用通配符或弱化无关指令。
