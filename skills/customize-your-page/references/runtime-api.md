# ViceMe 自定义页面接口

始终先运行 `viceme merchant page describe --target <URL> --merchant <商家ID>`。它只返回
目标页面能用的 capability 分组；作者页不会返回作品专属接口。页面入口加载时，平台会注入
`window.viceme`。manifest 只声明页面实际调用的 capability。

## 页面包

ZIP 根目录必须包含 `viceme-page.json`，其余目录自由组织。HTML 入口可以位于任意安全的
相对路径，也允许额外 HTML 和未知扩展名资源。

```json
{
  "apiVersion": "page.viceme.ai/v1alpha1",
  "kind": "CreatorPage",
  "metadata": { "name": "My page" },
  "spec": {
    "entry": "index.html",
    "sdkVersion": "1",
    "capabilities": ["context.read", "navigation.open"]
  }
}
```

作者页用 `CreatorPage`，作品页用 `WorkPage`。ZIP 最大 100 MiB，解压后最大 500 MiB，
单文件最大 100 MiB，最多 10000 个文件。结构检查还会拒绝路径穿越、加密条目、符号链接、
不支持的压缩格式、重复路径和明显的压缩炸弹；不会扫描业务源码或疑似密钥。

## DATA

### `context.read`

```js
const context = await window.viceme.context.get();
```

- 作者页返回 `{ type: "CREATOR", creator, works }`。
- 作品页返回 `{ type: "WORK", creator, work }`。
- `creator` 包含公开的作者资料；作者页 `works` 是公开作品摘要列表。
- 作品页 `work` 包含当前 revision、products、service、websiteAction、metrics 和公开时间字段。

## NAVIGATION

### `navigation.open`

```js
await window.viceme.navigation.openCreator();
await window.viceme.navigation.openWork(workSlug);
await window.viceme.navigation.openMarkdown();
```

`openWork` 只能打开当前上下文中真实存在的作品；路径由宿主生成。

## IDENTITY

### `auth.request-login`

```js
await window.viceme.auth.requestLogin();
```

打开平台登录页并在成功后返回当前页面。页面不能直接读取登录凭证。

## SOCIAL（作品页）

### `work.like`

```js
const state = await window.viceme.work.getLike();
await window.viceme.work.setLiked(true);
await window.viceme.work.setLiked(false);
```

读取可匿名调用；写入由宿主确认当前访客登录并校验真实作品。

### `comments.read`

```js
const page = await window.viceme.comments.list({ limit: 30, cursor });
```

### `comments.write`

```js
await window.viceme.comments.create({ content: "很棒" });
await window.viceme.comments.reply({ commentId, content: "谢谢" });
```

写评论和回复要求访客登录。文本长度为 1 到 5000 字；当前自定义页面接口不直接上传评论图片。

## COMMERCE

### `checkout.open`（作品页）

```js
await window.viceme.checkout.open({ productId });
```

`productId` 必须来自当前 `context.work.products`，购买资格与金额由平台结算页重新确认。

### `creator.subscribe`（作者页和作品页）

```js
await window.viceme.creator.subscribe();
```

打开当前作者的订阅购买流程；可售状态和访客权限由平台确认。

## 错误处理

所有接口都返回 Promise。至少捕获并处理：能力未在 manifest 声明、访客未登录、资源不属于
当前上下文、参数无效、网络失败和调用超时。需要登录时给出明确按钮，再调用
`auth.requestLogin()`；不要伪造成功状态。
