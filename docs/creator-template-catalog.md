# 创作者模板中心

模板源码和预览都由本仓库维护。正式目录只从
`templates/creator-pages/production.json` 读取 `status: "production"` 的条目；每项必须有唯一的
`id + version`、源码目录、预览文件、名称、场景、说明和授权声明。

本地构建需要对应私钥的 base64url PKCS#8 Ed25519 文件：

```bash
make template-catalog TEMPLATE_CATALOG_ORIGIN=https://s3.viceme.cn/templates/dev TEMPLATE_CATALOG_SIGNING_KEY_FILE=/secure/template-catalog-key
```

构建会生成 `index.html`、签名的 `manifest.json` / `manifest.sig`、每个版本的预览和 `source.zip`。版本目录永远不可覆盖；若同一对象已经存在，发布工作流只接受字节完全相同的内容。

推送到 `dev` 时工作流发布到 `templates/dev/`，推送到 `main` 时发布到 `templates/`。删除或下架模板时，只从 source catalog 的稳定清单移除；历史版本化对象保留，但 Agent 不再发现或下载它。

发布前，贡献者必须确认源码和预览拥有可发布授权，并提供适用的 `license` 字段。工作流使用 `VICEME_TEMPLATE_CATALOG_SIGNING_KEY` 签名；CLI 发布版本通过 `TEMPLATE_CATALOG_TRUST_KEYS` 内置对应公钥。私钥不能提交到仓库或写入模板源码。
