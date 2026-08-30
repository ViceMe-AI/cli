# 托管弹幕 SDK 合同

适用于希望接入 ViceMe 托管弹幕、但不把组件源码复制到宿主仓库的静态站点、博客或产品页面。

## 嵌入片段

所选 CLI Profile 是唯一的环境依据，绑定 API 地址、Web 地址和市场区域。修改宿主页面前先创建并启用 Work：

```bash
viceme --profile <profile> merchant work sdk-access create <work-id> \
  --merchant <merchant-id> --feature danmaku
```

SDK 访问资源由 `merchant work sdk-access` 命令族管理；Work 身份与已验证 Origin 归 Website Work 所有。所选 Profile 是唯一权威来源，不能复用于另一组 API、Web 或市场环境。

远端 Work 已启用公开弹幕能力时，成功的 `init`、`inspect` 和 `apply` 响应都会包含 `data.workKey`、`data.scriptUrl` 和 `data.embedSnippet`。必须原样插入 `data.embedSnippet`，不得自行推导来源、追加 SDK 路径或退回其他 Profile。

`workKey` 是公开且不透明的标识。不得替换为内部 creator ID、product ID、media ID、slug、API token 或支付凭证。不得向生成片段加入 API、iframe、creator、work ID 或宿主 URL 覆盖。

## 页面位置锚点

加载器根据标准页面 URL（包括 hash 路由）和当前 10% 滚动区间确定活动锚点。消息保存在对应页面位置锚点下，并展示给同一 Work、同一锚点的访问者。

## Runtime 边界

浏览器 Runtime 会校验加载器属性、解析公开 Work、确认 `danmaku` 能力已启用、最多挂载一次、隔离样式、保持宿主控件可点击、按需加载弹层，并在销毁时清除所有节点、监听器和计时器。它只调用公开弹幕接口，不建立通用 SDK Session，也不暴露关注、购买或权益接口。

托管 Web 应用负责渲染和交互；Shop API 负责公开 `workKey` 映射、持久化、保留上限和限流。匿名弹幕不强制登录。独立弹幕片段不包含打赏。
