# ViceMe 自定义作者页接口

## 账户资料同步

名片制作时，模板中的本地头像状态不会自动写回平台账户。Agent 只能在用户已明确同意把该资料作为平台资料展示后，通过当前登录 Profile 的 CLI 命令完成同步：

```text
viceme account profile update --display-name <已确认称呼>
viceme account avatar upload --path <已确认头像文件>
```

这不是 `window.viceme` 的页面 capability，页面运行时不能直接调用账户 API、读取凭据或上传对象存储。头像上传由 Shop 后端校验并同时更新账户和创作者公开头像；命令返回成功前，Agent 不得宣称同步完成或发布使用不同资料的最终名片。

始终先运行 `viceme merchant page describe --target <URL> --merchant <商家ID>`。它只返回
作者页能用的 capability 分组。页面入口加载时，平台会注入
`window.viceme`。manifest 只声明页面实际调用的 capability。

创作者入驻模式是唯一例外：审核中的普通申请会返回一个 `SUSPENDED` 页面租户，只有该
申请人自己的 `CreatorPage` 可调用 describe、upload、status 和 publish。该模式先在本地
预览，使用 `viceme merchant page upload` 上传，不调用线上 preview；暂停租户不能调用任何经营接口。
发布后的资产继续私有，直到创作者申请批准后由同一公开路由读取。

## 可编辑源稿

每次作者页 upload/preview 都必须同时传入 `--source <项目根目录>`。平台在同一个 release 内分别保存
公开运行包与 owner-only 的可编辑源码快照；这不是两次用户发布，也不会把源码放进公开静态资源。
模板项目同时传准确的 `--template-id` 和 `--template-version`。

更新已发布作者页前，运行 `merchant page source status`。只有 `RESTORABLE` 可以用
`merchant page source restore` 恢复到一个不存在的新目录；`LOCAL_SOURCE_REQUIRED` 表示旧版本没有
源稿，只能使用用户持有的原项目或重新制作。发布必须把本次 page status 返回的
`concurrencyToken` 作为 `--expected-concurrency` 传回，若页面在此期间已被别人更新，发布会拒绝，
必须重新读取状态并让用户确认变化。

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

作者页必须使用 `CreatorPage`。ZIP 最大 100 MiB，解压后最大 500 MiB，
单文件最大 100 MiB，最多 10000 个文件。结构检查还会拒绝路径穿越、加密条目、符号链接、
不支持的压缩格式、重复路径和明显的压缩炸弹；不会扫描业务源码或疑似密钥。

## DATA

### `context.read`

```js
const context = await window.viceme.context.get();
```

- 作者页返回 `{ type: "CREATOR", creator, works }`。
- `creator` 包含公开的作者资料；已验证的外部身份同时提供可选 `profileUrl`。
- 作者页 `works` 是公开作品摘要列表，每项包含标题、简介、站内路径和可选 `coverUrl`，足够
  渲染汪奕辰模板中的作品 Block。

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

## COMMERCE

### `creator.subscribe`

```js
await window.viceme.creator.subscribe();
```

打开当前作者的订阅购买流程；可售状态和访客权限由平台确认。

## 错误处理

所有接口都返回 Promise。至少捕获并处理：能力未在 manifest 声明、访客未登录、资源不属于
当前上下文、参数无效、网络失败和调用超时。需要登录时给出明确按钮，再调用
`auth.requestLogin()`；不要伪造成功状态。
