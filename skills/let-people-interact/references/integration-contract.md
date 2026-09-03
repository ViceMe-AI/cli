# ViceMe 互动公共资源边界

本文件只定义三个分支共享的平台资源关系。执行步骤、代码和路线专属安全要求分别放在相应参考中，未选择的路线不得加载。

## 三种分支

仅弹幕：

```text
PUBLISHED Website Work（canonical Origin 精确匹配部署 Origin）
└── ACTIVE SDK access
    ├── keys.test / keys.live
    └── hosted features 包含 danmaku
```

仅赞赏：

```text
公开 PUBLISHED Merchant Work（任意 kind）
└── ACTIVE SDK access
    ├── keys.test / keys.live
    └── hosted features 包含 tip
```

弹幕加赞赏：

```text
PUBLISHED Website Work（canonical Origin 精确匹配部署 Origin）
└── ACTIVE SDK access
    ├── keys.test / keys.live
    └── hosted features 包含 danmaku,tip
```

宿主页与被赞赏 Work 是独立资源。Standalone Tip 不要求 Work 是 Website，不要求仓库、HTTPS Origin、与宿主页同域或存在 Commerce Application；来源没有匹配归因时是 unverified Origin，不是拒绝来源。包含 Danmaku 的路线才要求 Website Work 与精确 canonical Origin。

三个分支均不创建、读取、验证或撤销 Website ownership verification，也不要求 DNS 或域名所有权验证。`website.ownershipStatus` 不参与门禁。Tip 本身不增加 Origin 或 Application 门禁。组合中的 Tip 不创建、更新、暂停或激活 `WEBSITE_WIDGET`；Website follow/paid access 由拥有它的平台流程管理。

## 不变量

- 当前用户必须通过 `MerchantAccountMember(role=OWNER)` 拥有选定 Merchant。
- 页面 locale 不选择市场。仅弹幕保留当前 Profile 的 CN 或 GLOBAL；任意 Tip 路线只支持 `marketRegion: cn` 与 CNY。
- 两个 Work key 是永久公开标识，不是凭据。create 一次签发 `keys.test` 与 `keys.live`；update、disable、重新启用和恢复不得轮换。
- hosted features 更新使用精确 `configVersion`，发送完整 hosted feature set，并原样保留完整 `accessFeatures`。
- SANDBOX 只使用 `keys.test`；展示证据并取得用户明确确认后才切换同一资源的 `keys.live`。
- production key 不能模拟支付；真实支付是独立且明确的用户决定。

## 按需参考

- SDK 版本和不可变资源：[发布物预检](release-preflight.md)
- Work 选择、发布、SDK access 与恢复：[Work 与 SDK access](work-and-access.md)
- 仅弹幕、仅赞赏或组合的官方组件：[Mounted 接入](mounted.md)
- 仅赞赏或组合的自定义 Tip UI：[Headless Tip 接入](headless.md)

路线实现只能使用这些参考中的公开 SDK surface。公共参考没有定义的行为不是宿主需要反向推断的合同。
