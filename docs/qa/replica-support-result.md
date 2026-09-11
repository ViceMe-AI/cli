# 做同款到账结果验收

对应飞书「用户端支付流程」，验收日期：2026-09-10。

## 预期行为

用户已确认金额、宿主已展示付款入口后，等待命令携带
`--payment-result-first`。CLI 或 Python 取得原订单的权威 `PAID`
状态，先返回 `PRESENT_SUPPORT_RESULT` 和独立的 `.support.html`。
Skill 立即再次调用平台内文件预览，切换到「创作者已收到你的支持」，
隐藏二维码、金额、倒计时和扫码提示，然后同一轮执行返回的
`RECOVERY_ONLY` 继续参数。继续参数绑定 `--expected-order-no`、原作品、
原身份与目标目录，不重新下单。准备作品失败时保留已付款事实。

CLI 默认每 3 秒查询一次、等待 3 分钟；Python 每 3 秒查询一次、
最多 60 次，网络请求耗时另计。只有一个等待进程。

## 自动化覆盖

- Go 的账号与匿名测试均使用本地 HTTP 服务、签名授权和 ZIP：
  到账阶段尚未请求下载或创建目标目录；执行返回的继续参数后完成安装，
  checkout 总数仍为 1；不匹配的订单参数在下载前被拒绝。
- Python 测试覆盖到账与安装分阶段、实付金额来自不可变订单、
  继续参数的解析和执行、其他目标的共享回执不能覆盖原订单、
  下载或安装失败保留 `PAID`、错误订单和未知状态不显示成功。
- Widget 测试覆盖只在权威 `PAID` 下显示支持卡、隐藏收银区域、
  清理倒计时、不把文本当作 HTML，以及通用支付卡原行为。

以下仓库验证命令全部通过：

```sh
make release-manifest
make check
go test -race ./internal/command ./internal/update
make npm-package-check
git diff --check
```

## 浏览器与宿主核验

运行 `node quality/widget-preview.cjs`，访问终端返回地址的
`/support-check`。使用真实 `widgets/payment.html`，数据是本地模拟，
没有真实付款。已在浏览器检查中文 360px、英文 320px：
结果卡可见，整个收银区域不可见，320px 的文档宽度和滚动宽度均为 320，
无横向溢出；Tab 可经过页面链接进入预览，无键盘陷阱；
文档标题、区域标签和读屏状态均切换为支持结果。

本机 WorkBuddy 5.5.4 的工具结果分发代码显示：新的 `present_files`
调用从返回文件中选择目标文件，发出 `openResultView` 请求，打开对应的
本地文件预览并显示结果面板。独立结果文件配合新的预览调用符合该路径，
不依赖旧 HTML 自动刷新。

尚未完成 WorkBuddy 内真实 `present_files` 工具流及真人扫码付款验收。
浏览器截图与宿主代码核验不能替代这两项；也未验证生产接口的轮询承载能力。
本改动不涉及数据库、服务端配置或通用支付业务文案。
