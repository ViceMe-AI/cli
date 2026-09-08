# Skill 作品页数据与操作

始终先运行 `viceme merchant page describe --target <URL> --merchant <商家ID>`，以它返回的
capability 分组确定当前作品允许调用的操作，再用本页说明保证一个按钮所需的读取和写入行为完整。
页面入口加载时平台会注入 `window.viceme`，`viceme-page.json` 只声明页面实际调用的 capability。

这些操作属于作品本身。默认作品页和自定义作品页只是两种呈现方式，不得向用户说正在给作品
“增加接口”或“开通能力”。

## 页面包

ZIP 根目录必须包含：

```json
{
  "apiVersion": "page.viceme.ai/v1alpha1",
  "kind": "WorkPage",
  "metadata": { "name": "My work page" },
  "spec": {
    "entry": "index.html",
    "sdkVersion": "1",
    "capabilities": ["context.read", "work.like"]
  }
}
```

ZIP 最大 100 MiB，解压后最大 500 MiB，单文件最大 100 MiB，最多 10000 个文件。结构检查会
拒绝路径穿越、加密条目、符号链接、不支持的压缩格式、重复路径和明显的压缩炸弹。

## 作品与作者信息

声明 `context.read` 后读取：

```js
const context = await window.viceme.context.get();
```

作品页返回 `{ type: "WORK", creator, work }`。`work` 包含当前 revision、products、metrics 和公开
时间等字段。产品 ID、价格、免费状态、试用次数和购买状态只能来自本次 context，不能写死。

## 页面跳转与登录

```js
await window.viceme.navigation.openCreator();
await window.viceme.navigation.openMarkdown();
await window.viceme.auth.requestLogin();
```

对应声明 `navigation.open` 或 `auth.request-login`。登录由平台完成，页面不能读取登录凭证。

## 点赞

声明 `work.like`。按钮必须把三步作为一组实现：

```js
const state = await window.viceme.work.getLike();
await window.viceme.work.setLiked(true);
await window.viceme.work.setLiked(false);
```

加载与刷新时读取状态，操作失败时恢复按钮状态。写入要求登录时调用 `auth.requestLogin()`。

## 评论

读取列表声明 `comments.read`：

```js
const page = await window.viceme.comments.list({ limit: 30, cursor });
```

写评论或回复还要声明 `comments.write`：

```js
await window.viceme.comments.create({ content: "很棒" });
await window.viceme.comments.reply({ commentId, content: "谢谢" });
```

文本长度为 1 到 5000 字，写入要求登录；当前不直接上传评论图片。

## 获取或购买

声明 `checkout.open`：

```js
await window.viceme.checkout.open({ productId });
```

`productId` 必须来自 `context.work.products`。页面先根据产品的真实免费、价格、试用和可购买状态
决定按钮文案与是否可点；最终资格和金额仍由平台页面重新确认。多个版本必须让用户知道当前操作的
是哪一个版本。

## 作者订阅

声明 `creator.subscribe`：

```js
await window.viceme.creator.subscribe();
```

只在当前上下文适用时展示；可售状态和访客权限由平台确认。

## 错误处理

所有方法都返回 Promise。捕获未登录、声明不匹配、资源不属于当前作品、参数错误、网络失败和超时。
需要登录时提供用户可理解的按钮并调用 `auth.requestLogin()`；不得伪造成功状态。
