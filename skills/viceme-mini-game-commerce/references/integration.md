# 作品级小游戏接线合同

## 唯一运行库来源

`templates/mini-game-commerce.js` 是 Shop 仓库 `packages/mini-game-license` 的真实浏览器构建产物，运行库版本固定为 `1.0.0`，不是本 Skill 另写的许可证或付款实现。升级必须在该权威源码中修改，运行 `pnpm --filter @viceme-shop/mini-game-license build`，将生成的 `dist/mini-game-commerce.js` 原样复制到本模板，再重新生成 CLI release manifest；禁止手改此模板或从不明 CDN 替换。

CLI 从当前可执行文件的内嵌 Skill FS 读取模板，不依赖用户当前安装目录的可变脚本。运行库暴露 `ViceMeMiniGameCommerce`；受管配置通过 `createMiniGameRuntime(configuration, createXiaohongshuPlatform(window))` 创建 `window.ViceMeMiniGame`。宿主不能再创建另一个实例，也不能重复加载配置。

## 宿主入口

以实际目录为准，顺序保持为：

```html
<script src="viceme/mini-game-commerce.js"></script>
<script src="viceme/mini-game-config.js"></script>
<script src="game.js"></script>
```

以上是接线位置示意，不覆盖原 HTML。不加 async、defer、type=module 或 nomodule 到两份受管脚本。原游戏可以继续使用自己的模块；本地静态 import 会被检查器跟进。

`ViceMeMiniGame` 的五个接口均返回 Promise。成功结果为 `{ ok: true, ... }`，失败为 `{ ok: false, code, message }`。错误信息应展示到原游戏的购买区域，不能吞掉或当成功；初始化每游戏会话做一次，其他操作等初始化成功后才允许执行。

| 接口 | 成功结果 | 接线行为 |
| --- | --- | --- |
| `initialize()` | `installationId` | 恢复安全存储中的当前安装身份与已购权益；不得清空数据来绕过失败 |
| `createPurchaseCard(alias)` | `checkoutUrl` | 保存并呈现运行库生成的付款卡，不把 URL 当已付款证明 |
| `redeemLicense(alias, token)` | `unlocked: true, licenseId` | 接收文本许可证，仅成功后刷新原游戏权益 |
| `importLicenseImage(alias)` | `unlocked: true, licenseId` | 在用户点击的同步调用栈直接触发；取消不等于失败支付，也不解锁 |
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
  imageButton.addEventListener('click', () => {
    const pending = ViceMeMiniGame.importLicenseImage('hero-sword');
    pending.then(async result => {
      if (!result.ok) return showPurchaseMessage(result.message);
      const access = await ViceMeMiniGame.isUnlocked('hero-sword');
      if (access.ok && access.unlocked) refreshOriginalGameInventory();
    });
  });
}
```

`showPurchaseMessage`、按钮变量和刷新函数必须复用宿主已有实现，不把示意占位符原样塞进项目。根据原代码选择合适的 async 启动函数，不把顶层 await 放进普通 script。同时连接文本兑换入口，使用用户输入的 token，不能用假许可证。

## 配置与永久权益

manifest 严格字段为 `schemaVersion`、`runtimeVersion`、`workId`、`workTitle`、`environment`、`publicClientId`、`publicKey`、`checkoutOrigin`、`items`。每项道具仅含 `id`、不可变 `alias`、展示 `title`、`status`。配置投影去掉两种版本字段与道具 status，匹配运行库 exactKeys 校验，不含价格或支付凭据。

`ACTIVE` 要求真实购买调用；下架、归档、草稿道具不要求新购买入口，但都保留在配置以验证原许可证，并至少保留实际加载的 `isUnlocked`、`redeemLicense` 或 `importLicenseImage` 别名引用。缺少时检查返回 `ITEM_ALIAS_MISSING`，无效的旧购买按钮不能替代权益恢复入口。不要删 ID 或根据标题生成新 alias；不缓存价格、不自行构造结算参数、不把购买响应直接当解锁凭据。

受管文件、项目 API 站点、作品、商家与环境通过管理状态绑定。中断可重跑 integrate，只接受已记录的旧/目标哈希；人工修改、错绑及未知数据停止覆盖。`check` 不修改项目，也不会执行游戏脚本；通过不代表运行时或小红书平台验收通过。
