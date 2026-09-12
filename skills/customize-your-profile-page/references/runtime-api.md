# ViceMe 个人主页接口

始终先运行 `viceme profile page describe`。服务端根据当前登录用户解析唯一的 `CreatorPage` 目标，并返回准确的 `profileUrl`、`markdownUrl`、SDK 版本和 capability 分组；客户端不提交 handle 或 Merchant。

## CLI 生命周期

```sh
viceme profile page describe
viceme profile page status
viceme profile page source status
viceme profile page source restore --destination <新目录>
viceme profile page inspect --path <静态ZIP>
viceme profile page upload --path <静态ZIP> --source <项目根目录>
viceme profile page publish <release-id> --expected-active <id或none> --expected-concurrency <token>
viceme profile page activate <历史release-id> --expected-active <当前id> --expected-concurrency <token>
```

个人主页不使用线上 preview。每次 upload 都必须带可编辑项目目录；平台在同一 release 保存公开运行包与 owner-only 源码快照。恢复只能写入不存在的新目录。发布使用 status 返回的 concurrency token，防止覆盖并发更新。

## 页面包

ZIP 根目录必须包含 `viceme-page.json`，入口可位于任意安全相对路径：

```json
{
  "apiVersion": "page.viceme.ai/v1alpha1",
  "kind": "CreatorPage",
  "metadata": { "name": "My profile" },
  "spec": {
    "entry": "index.html",
    "sdkVersion": "1",
    "capabilities": ["context.read", "navigation.open"]
  }
}
```

ZIP 最大 100 MiB，解压后最大 500 MiB，单文件最大 100 MiB，最多 10000 个文件。校验拒绝路径穿越、加密条目、符号链接、不支持的压缩格式、重复路径和压缩炸弹。

## 浏览器 SDK

平台在入口注入 `window.viceme`。manifest 只声明实际使用的 capability。

- `context.read`: `await window.viceme.context.get()`，返回 `{ type: "CREATOR", creator, works }`。
- `navigation.open`: `openCreator()`、`openWork(workSlug)`、`openMarkdown()`；站内路径由宿主生成。
- `auth.request-login`: `await window.viceme.auth.requestLogin()`；页面不能读取登录凭证。
- `creator.subscribe`: `await window.viceme.creator.subscribe()`；是否可售由平台决定，普通用户主页不应默认声明。

所有调用返回 Promise。处理未声明能力、访客未登录、资源不属于当前上下文、参数无效、网络失败与超时；不得伪造成功状态。
