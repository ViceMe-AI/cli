# ViceMe 打赏接入合同

## 公开边界

一个创作者接入由宿主拥有的 HTML 页面和一段 ViceMe 脚本组成。脚本在 Shadow DOM 中渲染入口，并打开 ViceMe 托管的结账 iframe。宿主不得复制结账页面、调用支付渠道 API 或接收支付秘密。

## 绑定定义

只有同时满足以下条件，作品才可以使用：

1. 所选 CLI Profile 已以创作者身份登录；
2. 该作品存在 External Creator App；
3. 应用中准确的公开主机名已验证；
4. HTML 使用该应用生成的 Creator App ID 和平台来源。

仅有 HTML 中的 `data-creator-app-id`、本地演示 ID 或未验证域名，都不算完成绑定。

## 域名验证

当前固定 Profile 下，`creator-app domain add` 返回的验证 URL/路径和 token 是权威结果。部署可能要求形如以下路径的公开文件：

```text
https://<creator-host>/.well-known/viceme-app-verification.txt
```

使用目标技术栈的公开文件或路由机制，在 CLI 响应指定的准确路径提供内容。请求部署后的 URL，与同一次 `creator-app domain add` 返回的 token 比较，再运行 `creator-app domain verify`；不得把 token 打印到对话或持久日志。跳转到其他来源以及非公开主机均无效。

## 嵌入合同

```html
<script
  async
  src="<generated-widget-script-url>"
  data-creator-app-id="<creator-app-id>"
  data-locale="<zh-CN-or-en-US>"
></script>
```

每个 Creator App 只保留一个标签。`creatorAppId` 是公开配置，不是凭证。完整脚本 URL、ID 和标签必须来自当前固定 Profile 下 `creator-app show` 返回的 `data.embedSnippet`，不得从 API URL 推导或手工拼接。

## CSP

存在 CSP 时保留原规则，只向浏览器证明需要的指令添加准确的 ViceMe 平台来源。嵌入需要脚本来源和结账 iframe 来源。不得为了省事添加 `*`、`unsafe-eval` 或宽泛的 ViceMe 子域通配符。

## 真实验证

只有在已验证的公开 HTTPS 主机名满足以下条件，接入才算完成：

- HTML 返回 200；
- 嵌入脚本返回 200 且内容为 JavaScript；
- 入口可见并可用键盘访问；
- 入口为预期作品打开托管结账页；
- Escape 关闭弹窗并把焦点返回原处；
- 没有 CSP、frame、script 或组件错误；
- Agent 说明是否真正创建订单或完成支付。

打开界面不代表支付已经结算。必须区分结账界面验证和真实资金支付验证。
