# 创作者模板中心

模板源码和预览都由本仓库维护。正式目录只从
`templates/creator-pages/production.json` 读取 `status: "production"` 的条目；每项必须有唯一的
`id + version`、源码目录、预览文件、名称、场景、说明和授权声明。

本地构建需要对应私钥的 base64url PKCS#8 Ed25519 文件：

```bash
make template-catalog TEMPLATE_CATALOG_ORIGIN=https://s3.viceme.cn/templates/dev TEMPLATE_CATALOG_SIGNING_KEY_FILE=/secure/template-catalog-key
```

构建会生成 `index.html`、签名的 `manifest.json` / `manifest.sig`、每个版本的预览和 `source.zip`。manifest 中的预览与源码位置使用目录内相对 URL，CLI 只在验签后才按所选 CN/Global origin 解析，因此两个区域发布完全相同的字节。版本目录永远不可覆盖；若同一对象已经存在，发布工作流只接受字节完全相同的内容。

推送到 `dev` 时工作流通过现有 `cdn` GitHub Environment 发布到独立公开 `templates` 桶的 `dev/` 前缀，推送到 `main` 时发布到该桶根。工作流使用既有 CN/Global 发布凭据创建或核对专用桶；公开策略写入前必须确认其中没有模板册约定之外的对象，随后才收敛整桶策略为仅允许匿名 `GetObject`。桶内不承载任何私有对象，也不开放 `ListBucket`。工作流会逐项从 CN/Global 公网回读首页、manifest、签名、预览和源码，比较本地产物及跨区域字节，并用实际 dev CLI 执行 `template list/fetch`。删除或下架模板时，只从 source catalog 的稳定清单移除；历史版本化对象保留，但 Agent 不再发现或下载它。

发布前，贡献者必须确认源码和预览拥有可发布授权，并提供适用的 `license` 字段。工作流使用 `cdn` Environment 中的 `VICEME_TEMPLATE_CATALOG_SIGNING_KEY` 签名；CLI 发布版本通过 repository variable `TEMPLATE_CATALOG_TRUST_KEYS` 内置对应公钥。私钥不能提交到仓库或写入模板源码。

五个 Mock 的 metadata 与预览只位于仓库级 `templates/creator-pages/`，不得放入 `skills/`、`production.json`、release manifest 或任何正式下载包。`cmd/template-catalog --allow-insecure-origin` 只允许 loopback HTTP，并且本地 demo 不生成 Mock 源码 ZIP。
