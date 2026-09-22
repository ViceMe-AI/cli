# 作品级小游戏接线合同

## 唯一运行库来源

`templates/mini-game-commerce.js` 是 Shop 仓库 `packages/mini-game-license` 的真实浏览器构建产物，运行库版本固定为 `2.0.0`，不是本 Skill 另写的动态码或付款实现。升级必须在该权威源码中修改，运行 `PNPM_CONFIG_STORE_DIR=<同一绝对store路径> pnpm --filter @viceme-shop/mini-game-license build`，将生成的 `dist/mini-game-commerce.js` 原样复制到本模板，再运行 CLI 的 `make release-manifest`；禁止手改此模板或从不明 CDN 替换。

CLI 从当前可执行文件的内嵌 Skill FS 读取模板，不依赖用户当前安装目录的可变脚本。运行库暴露 `ViceMeMiniGameCommerce`；受管配置通过 `createMiniGameRuntime(configuration, createXiaohongshuPlatform(window))` 创建 `window.ViceMeMiniGame`。宿主不能再创建另一个实例，也不能重复加载配置。

## 宿主入口

以实际目录为准，顺序保持为：

```html
<script src="viceme/mini-game-commerce.js"></script>
<script src="viceme/mini-game-config.js"></script>
<script src="game.js"></script>
```

以上是接线位置示意，不覆盖原 HTML。不加 async、defer、type=module 或 nomodule 到两份受管脚本。原游戏可以继续使用自己的模块；本地静态 import 会被检查器跟进。

`ViceMeMiniGame` 的四个接口均返回 Promise。成功结果为 `{ ok: true, ... }`，失败为 `{ ok: false, code, message }`。错误信息应展示到原游戏的购买区域，不能吞掉或当成功；初始化每游戏会话做一次，其他操作等初始化成功后才允许执行。

| 接口 | 成功结果 | 接线行为 |
| --- | --- | --- |
| `initialize()` | `{ ok: true }` | 自动生成并安全持久化随机 128 位安装身份，恢复同安装已购权益；不返回安装 ID，不得展示、填写、编辑或清空它 |
| `createPurchaseCard(alias)` | `checkoutUrl` | 保存并呈现运行库生成的付款卡，不把 URL 当已付款证明；安装 ID 仅由付款卡内部传递 |
| `redeemCode(alias, code)` | `unlocked: true` | 只接收保留前导零的六位数字字符串；成功后刷新原游戏权益，无图片导入接口 |
| `isUnlocked(alias)` | `unlocked` | 只按 `ok && unlocked` 开放受保护能力 |

所有 alias 必须来自这次响应；把实际字符串写到对应按钮/能力的调用中。以下只是异步错误处理示意，`hero-sword` 不能作为未知作品的默认别名：

```js
const initialized = await ViceMeMiniGame.initialize();
if (!initialized.ok) {
  showPurchaseMessage(initialized.message);
} else {
  buyButton.addEventListener('click', async () => {
    const result = await ViceMeMiniGame.createPurchaseCard('hero-sword');
    showPurchaseMessage(result.ok ? '付款卡已生成，请按卡片指引购买。' : result.message);
  });
  redeemButton.addEventListener('click', async () => {
    const result = await ViceMeMiniGame.redeemCode('hero-sword', codeInput.value);
    if (!result.ok) return showPurchaseMessage(result.message);
    const access = await ViceMeMiniGame.isUnlocked('hero-sword');
    if (access.ok && access.unlocked) refreshOriginalGameInventory();
  });
}
```

`showPurchaseMessage`、按钮变量和刷新函数必须复用宿主已有实现，不把示意占位符原样塞进项目。根据原代码选择合适的 async 启动函数，不把顶层 await 放进普通 script。兑换输入使用 `type="text" inputmode="numeric" pattern="[0-9]{6}" maxlength="6"`，不能使用 `type="number"`、`Number()` 或 `parseInt()` 丢失前导零。动态码按 30 秒窗口变化；失效后去原付款卡对应的订单页面重新取码，不能用假码或旧文本/图片许可证。

## 配置与永久权益

manifest 严格字段为 `schemaVersion: 2`、`runtimeVersion: "2.0.0"`、`workId`、`workTitle`、`environment`、`publicClientId`、`sharedSecret`、`checkoutOrigin`、`items`。`sharedSecret` 为 64 位小写十六进制；每项道具仅含 `id`、不可变 `alias`、展示 `title`、`status`。配置投影去掉两种 manifest 版本字段与道具 status，加入 `protocolVersion: 2`，匹配运行库 exactKeys 校验，不含价格或支付凭据。共享密钥只能用于生成受管游戏配置，不得出现在日志、命令报告、回复或买家界面。

`ACTIVE` 要求真实购买调用；下架、归档、草稿道具不要求新购买入口，但都保留在配置以恢复同安装权益，并至少保留实际加载的 `isUnlocked` 或 `redeemCode` 别名引用。缺少时检查返回 `ITEM_ALIAS_MISSING`，无效的旧购买按钮不能替代权益恢复入口。不要删 ID 或根据标题生成新 alias；不缓存价格、不自行构造结算参数、不把购买响应直接当解锁凭据。

动态码绑定安装、作品、环境、道具和时间窗口，不能跨安装兑换或换绑。同一安装可重新取码，正常重启可恢复已经兑换的本地权益；清数据、卸载重装或换设备会丢失绑定权益，禁止提供安装 ID 编辑、导入备份或“恢复旧安装”入口。购买/兑换区域需说明该代价。游戏内容预置，共享密钥内置后可被提取并伪造动态码，不是强 DRM，不能保证防止内容提取或修改。

受管文件、项目 API 站点、作品、商家与环境通过管理状态绑定。中断可重跑 integrate，只接受已记录的旧/目标哈希；人工修改、错绑及未知数据停止覆盖。`check` 不修改项目，也不会执行游戏脚本；通过不代表运行时或小红书平台验收通过。
