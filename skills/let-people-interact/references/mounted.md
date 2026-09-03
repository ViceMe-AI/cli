# Mounted 接入

本参考只适用于官方 Mounted UI。模板和公开 API 是权威边界；不要读取 SDK 构建产物推断内部 DOM、iframe 或样式。

## 选择模板

| 分支 | 模板 |
| --- | --- |
| 仅弹幕 | [`mounted-danmaku.html`](../templates/mounted-danmaku.html) |
| 仅赞赏单 HTML | [`single-html.html`](../templates/single-html.html) |
| 弹幕加赞赏 | [`mounted-combination.html`](../templates/mounted-combination.html) |

模板是可嵌入现有页面的起点，不授权重做宿主页面。保留宿主布局，只增加必要 target、状态提示和一套精确 ESM runtime。

## Target 与主题

- Danmaku 会在 target 内创建 fixed 全屏浮层；target 自身保持空容器，不要再设置 fixed、全屏尺寸或高 z-index。
- Tip target 参与宿主正常文档流，应放在创作者选择的支持区域；只调整宿主外层布局，不修改 SDK 内部 DOM。
- `theme` 可取 `"auto"`、`"light"` 或 `"dark"`。默认 `auto`；已有明确单色主题时可以选择对应值，不需要检查 SDK 源码。
- 页面 locale 由 SDK/浏览器处理，不用它推导 CN/GLOBAL market。

## 生命周期与失败隔离

- 组合只创建一个 `createViceMe` client，并用 `Promise.allSettled` 独立调用 `mountDanmaku` 与 `mountTip`。
- 一个 mount 失败时呈现该能力的简短用户可见状态，同时保留另一个成功 handle；不要销毁成功能力。
- SPA、组件或路由真实卸载时先销毁全部成功 handle，再调用 `client.destroy()`。
- 静态文档没有页面内卸载，不绑定 `pagehide`；它会在 bfcache 时触发。

## CSP 与 Referrer Policy

先保留现有 CSP 和 nonce，只加入浏览器错误证明真正缺少的精确 SDK CDN 与官方 Web Origin。不得加入 `*`、宽泛 ViceMe 子域或 `unsafe-eval`。Tip 需要浏览器向官方页面发送来源 Origin；`Referrer-Policy: no-referrer` 会关闭 handoff，不通过伪造 query 或消息补救。

## 一次 SANDBOX 验收

在一个浏览器会话中批量完成并保存一份证据：

| 检查 | 仅弹幕 | 仅赞赏 | 组合 |
| --- | --- | --- | --- |
| 页面只有一套精确版本 ESM | 必须 | 必须 | 必须 |
| target 成功挂载、请求只到所选官方 Origin | 必须 | 必须 | 两者都必须 |
| 控制台无 CSP/frame/script 错误 | 必须 | 必须 | 必须 |
| 桌面、320px、键盘可达 | 控件 | 金额与确认入口 | 两者 |
| 一个能力失败不移除另一能力 | 不适用 | 不适用 | 由 `Promise.allSettled` 静态结构和可见错误状态证明 |

Mounted 的金额/provider、确认层、跨窗口协议和公开结果由官方组件负责；宿主验收不重复运行 Headless Local Fake、弹窗阻止或私有消息协议测试。打开 UI 不代表弹幕消息已持久化或支付已结算；未执行的真实业务动作必须报告。

SANDBOX 证据得到用户确认后，只替换同一个 `createViceMe` 的 test key 为 live key，再部署并在一个浏览器会话中复核版本、挂载、官方请求和控制台。
