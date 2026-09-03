# SDK 发布物预检

每次接入只运行一次 Skill 自带的统一预检：

```bash
node "<skill-dir>/scripts/preflight-sdk-release.mjs" \
  --route <danmaku|tip|combined> \
  --region <cn|global>
```

`<skill-dir>` 是承载当前 `SKILL.md` 的可信安装目录，不是用户项目目录。路由映射固定为：

| 业务分支 | `--route` | `--region` |
| --- | --- | --- |
| 仅弹幕 | `danmaku` | 当前 Profile 的 `cn` 或 `global` |
| 仅赞赏 | `tip` | `cn` |
| 弹幕加赞赏 | `combined` | `cn` |

命令成功时只从其 JSON 结果读取并固定：

- 纯 `major.minor.patch` 的 `sdk_version`；
- 当前宿主应使用的精确 `sdk_origin`；
- 本次已检查的 manifest 和不可变入口。

## 命令保证

统一脚本在一次有界操作中：

1. 从官方 npm registry 的稳定 `latest` 元数据解析一次版本，并验证包仍暴露 `createViceMe`、Danmaku、Mounted Tip、Headless Tip 与测试适配器所在的公共入口；
2. 只验证当前 Profile 实际使用区域的 manifest 与入口：仅弹幕检查 `index.js`、`danmaku.js`，仅赞赏检查 `index.js`、`tip.js`，组合检查三者；
3. 跨区域发布完整性由 SDK 发布流程负责，单个站点不重复探测未使用区域；
4. 要求 manifest 版本一致、`apiMajor` 受支持且公共文件完整；
5. 每个 HTTP 请求使用 15 秒上限、拒绝 redirect，并要求直接返回精确 `200`；npm 请求不重试且固定官方 registry。

任何一步失败都以非零状态结束。此时不得创建、更新或发布 Work，不得写 SDK access，也不得编辑宿主页。Agent 不再手工重放其中的 npm/curl，不得再次解析版本，不下载构建产物，也不改用旧版或其他发布源。

## 使用固定版本

- CDN ESM import 必须把全部 `REPLACE_WITH_RESOLVED_SDK_VERSION` 替换为本次 `sdk_version`；仍有占位符就停止。
- `latest` 只用于脚本内部的一次发现；不得把 `latest` 写入 CDN URL、安装规格或宿主页。
- Headless npm 路线使用项目既有包管理器 exact-save 安装 `@viceme-ai/sdk@<sdk_version>`；安装失败就停止，不重新解析版本。
- 不使用 tag、版本范围、alias、Git 依赖、私有镜像同名包、浏览器全局 loader 或复制的 SDK 源码。
