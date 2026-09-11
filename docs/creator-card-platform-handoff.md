# 创作者个人名片：平台接入与云端发布交接

## 交接目的

本分支完成了 CLI / Agent Skill 侧的模板优先个人名片流程、云端模板册的构建与校验能力、
导入和更新名片的安全状态机，以及通过 CLI 同步账户展示名称和头像的接口边界。

接手开发需要先审查本分支的实际 diff 和测试，再在独立分支实现剩余平台 Contract、部署和
端到端验收；不要仅凭本文件直接合并或假定接口已经可用。

## 已实现并应保留的内容

### 模板册与 CLI

- `templates/creator-pages/production.json` 是正式模板的唯一来源；当前只有 `Bonjour Card`
  (`bonjour-card@1.0.0`) 可下载、创建和发布。
- `templates/creator-pages/demo.json` 中的 Maker Portfolio、Editorial Profile、Studio Practice、
  Minimal Resume、Visual Journal 仅为本地展示 Mock。它们没有生产 `status`、源码 ZIP 或下载
  权限，绝不能出现在正式 manifest。
- `cmd/template-catalog` 构建静态 `index.html`、签名 `manifest.json` / `manifest.sig`、不可变
  预览以及每个正式模板的确定性 `source.zip`。
- `viceme template list` 只信任由嵌入式 Ed25519 公钥环验证的 manifest；`viceme template fetch`
  按 `id + version` 下载、校验 SHA-256 并安全解压。Agent 不得从本地目录、开发 Mock 或任意 URL
  替代这些结果。
- `skills/customize-your-profile-page/SKILL.md` 规定：先展示“查看所有模板”及正式模板链接；用户在对话中
  选择后才下载源码。右侧只用于打开模板、当前页面和本机预览。
- `viceme account profile update --display-name <name>` 与 `viceme account avatar upload --path <image>`
  是名片资料写回平台账户的唯一入口；CLI 用当前 Profile 的令牌调用 Shop 的 CLI 账户接口，后端将头像
  同时写入 User 与 CreatorAccount。模板、右侧编辑器和浏览器 `localStorage` 不能替代该同步。

### 创作者对话与发布闸门

- 已是创作者或申请中的创作者不会在资格提示后停住，而是主动进入个人名片流程。
- 用户有且只有两类创作来源：正式模板，或导入用户有权使用的主页；参考他人主页只可借鉴结构
  与视觉，不复制对方资料。没有空白“完全自定义”路径。
- 无论选模板、原样导入自己的主页还是参考主页，均要先分析已有内容，再输出“可直接使用 /
  待确认公开 / 缺失”。原样导入必须保留已有内容、结构和视觉，不能要求用户重填已有资料。
- 公开范围确认直接授权创建本机草稿；本机最终预览是唯一发布确认。不得增加“确认创建文件”或
  “再确认发布”的循环。
- 导入并能本机打开不等于可发布：在三栏资料整理和最终预览之前，不得询问“这版名片可以发布吗？”。
  当前线上个人页始终保持不变，直至用户确认更新。
- 发布继续使用同一个 `profileUrl`。申请中只对 owner 私有可见；审核通过后，当前 active release
  自动公开。

### 当前“更新名片”行为

用户说“更新我的名片”或“修改个人名片”时，Skill 会让其选择：

1. 修改当前版本；
2. 重新制作一版。

当前 CLI/API 没有可恢复源稿接口，因此“修改当前版本”只能接收用户提供的原始项目/ZIP；否则
诚实地退回“重新制作一版”。平台已托管发布后的页面资产，但“托管可访问产物”不等于“owner 可
取回的完整可编辑项目”。Skill 禁止从线上 HTML、截图或 Profile Blocks 反推源码。

## 开发必须补齐的平台工作

### 1. 部署正式云端模板册

本仓库已有 `.github/workflows/template-catalog.yml`：上游 `dev` 推送发布到
独立公开 `templates` 桶的 `dev/` 前缀，`main` 推送发布到该桶根，并构建 CN 和 Global 两套字节一致的静态产物。
个人 fork 没有上游环境 secret，不能把 fork 的 Actions 成功当作正式部署成功。

在上游仓库配置并验证以下 GitHub Environment secret：

- `VICEME_TEMPLATE_CATALOG_SIGNING_KEY`；
- `VICEME_RELEASE_S3_ENDPOINT_CN`、`VICEME_RELEASE_S3_ACCESS_KEY_ID_CN`、
  `VICEME_RELEASE_S3_SECRET_ACCESS_KEY_CN`、
  `CN_S3_HTTPS_PROXY`（需要时）；
- `VICEME_RELEASE_S3_ENDPOINT_GLOBAL`、`VICEME_RELEASE_S3_ACCESS_KEY_ID_GLOBAL`、
  `VICEME_RELEASE_S3_SECRET_ACCESS_KEY_GLOBAL`。

模板册不复用 `start` 桶：公网 URL 的首段 `templates` 对应独立同名桶，工作流使用上述凭据创建或核对该桶，并只授予匿名 `GetObject`，不授予 `ListBucket`。桶中不得存放私有对象。

发布后必须从真实公开域验证：

```text
https://s3.viceme.cn/templates/dev/index.html
https://s3.viceme.cn/templates/dev/manifest.json
https://s3.viceme.ai/templates/dev/index.html
https://s3.viceme.ai/templates/dev/manifest.json
```

还要在实际 dev CLI 中运行 `viceme template list` 和 `viceme template fetch bonjour-card --version 1.0.0`；
验证 manifest 签名、ZIP 哈希、预览链接和解压路径。不得手工上传无签名 manifest、私钥或 ZIP。

正式 CLI release 必须配置上游 repository variable `TEMPLATE_CATALOG_TRUST_KEYS`，使
`.github/workflows/release.yml` 将对应公钥编入发行 CLI；公钥与模板册私钥必须是一对，但私钥永不
进入仓库、二进制或日志。

### 2. 实现个人名片可恢复源稿 Contract

对每一次个人名片发布，页面服务除保存用于渲染的发布产物外，还应保存仅 owner 可访问的、不可变
源稿快照。快照至少包括：源稿 archive、模板 ID/版本（如有）、SHA-256、release ID、创建时间和
当前 active-release 并发标记。

CLI/API 可以采用不同命名，但必须提供等价语义：

```text
source status(target, merchant)
  -> activeRelease, concurrencyToken,
     availability: RESTORABLE | LOCAL_SOURCE_REQUIRED | UNAVAILABLE,
     template metadata, source digest, createdAt

source restore(releaseId, destination, merchant)
  -> owner-validated, checksum-verified local source directory and revision
```

安全与一致性要求：

- 只能由该个人页 owner 读取或恢复；不向 Agent 返回任意 URL、签名下载 URL、其他用户 archive 或凭证。
- 发布新版本必须携带恢复时读取的 active release / 并发 token。若 Profile Blocks 或另一终端先更新，
  拒绝覆盖，Agent 重新读取后提示用户合并。
- 源稿在原子发布确认前不能覆盖当前公开页；失败保留当前 active release。
- 历史页面只有静态发布产物或无源稿时返回 `LOCAL_SOURCE_REQUIRED` 或 `UNAVAILABLE`，不能声称
  已恢复编辑项目。
- 新模板名片、从用户原项目导入的名片均应在发布后产生可恢复源稿；源稿保留/删除策略必须使
  `source status` 返回可信状态。

页面服务/CLI Contract 完成后，再扩展 `merchant page` 命令和 `customize-your-profile-page` Skill：只有
`RESTORABLE` 才自动恢复本机项目；缺少该状态时保留当前安全降级。不要先在 Skill 中添加不存在的
命令。

### 3. 不做的事情

- 不把 Agent 与右侧 Profile Blocks 编辑器做成共享草稿；本期没有双向同步。
- 不通过抓取公开页面、还原压缩 JS 或复制截图来“恢复源码”。
- 不让 Mock 模板进入 production manifest、生成 source ZIP 或被用户发布。
- 不让模板详情页的“使用此模板”在未有受限宿主回传协议时改变 Agent 状态；用户仍在对话中选择。

## 开发验收矩阵

| 场景 | 应有结果 |
| --- | --- |
| `template list` | 仅出现签名 manifest 内的 production 模板；不显示本地路径或 Mock |
| 模板下载 | 只下载所选 `id + version`；签名或 SHA-256 失败时零解压、零创建 |
| 原样导入主页 | 先三栏资料整理和本机预览，线上页不变；最终确认后才更新同一路由 |
| 已发布且 `RESTORABLE` | 恢复 owner 源稿，局部修改、本机预览、一次确认后同路由更新 |
| 已发布且不可恢复 | 要求原项目/ZIP 或重新制作，不宣称找回源码 |
| 并发更新 | 返回冲突而不是覆盖手动 Block 编辑 |
| 申请中创作者 | 发布版本仅 owner 可见；批准后不重传即可公开 |
| 模板册 CI | CN/Global manifest、签名、预览和 ZIP 均可读取且哈希相同 |

## 审查与合并顺序

1. 从本分支审查云端模板册、Skill、测试和文档；运行仓库验证命令。
2. 在独立开发分支实现页面服务、CLI Contract 和测试；不要直接修改或合并上游 `dev`。
3. 合并本分支的 CLI/Skill 改动后，在上游 `dev` 配置环境并触发模板册发布。
4. 用真实 dev 身份完成验收矩阵，再向 `dev` 发起审查 PR；生产模板册只随审查后的 `main` 发布。
