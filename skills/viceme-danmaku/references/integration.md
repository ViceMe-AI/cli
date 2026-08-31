# ViceMe 弹幕宿主接入契约

## Publish 交接

`$viceme-publish` 必须先确认 Website Work 已发布、Origin 已验证，并且单一 SDK access 的 hosted features 包含 `danmaku`。本 Skill 只消费 publish 返回的公开 `workKey`、Web base URL、区域、canonical Origin 和完整 hosted feature set。

## Loader

一个页面、一个 `workKey` 只保留一个 loader。允许的 hosted feature 值是：

- `danmaku`
- `tip`
- `danmaku,tip`

同时接入时固定使用 `danmaku,tip`。不要传 endpoint、token、用户身份、价格、provider、application ID 或支付状态。

## 运行时

外部 SDK 在 Shadow DOM 中挂载 Shop 托管 iframe，验证 bridge 的 Origin 和 source，并负责清理节点、监听器与定时器。Shop 拥有消息持久化、限流、键盘和减少动画行为。loader 根据页面 URL 和滚动位置产生不透明 anchor；宿主不得自行生成用户、session 或支付身份。

## CSP

保留现有指令。只把 publish 返回的精确 Shop Origin 加入实际需要的 `script-src`、`connect-src` 和 `frame-src`。保留 `object-src 'none'`；不得增加通配符、宽泛子域或 `unsafe-eval`。
