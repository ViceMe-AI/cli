---
name: let-people-interact
description: 为创作者作品接入或修复 ViceMe 托管互动。先复用 $become-a-creator 完成资格守卫，再区分仅弹幕、仅赞赏或弹幕加赞赏，并按对应的 Work、市场、验证与官方 Mounted/Headless SDK 边界完成接入。
---

# 接入 ViceMe 互动

先判断用户要的是仅弹幕、仅赞赏，还是弹幕加赞赏；不得把三种请求统一绑定到 Website Work。修改前完整阅读 [integration-contract.md](references/integration-contract.md)。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示挑战值、登录凭据、支付详情或内部 handoff 字段。

## 先分支

| 分支 | 合格 Work | 市场 | 宿主 UI |
| --- | --- | --- | --- |
| 仅弹幕 | 与部署 Origin 精确匹配的 `PUBLISHED + VERIFIED Website Work` | 保留所选 Profile 既有 `cn` 或 `global` 支持 | `createViceMe(...)` + `mountDanmaku(...)` |
| 仅赞赏 | 当前 OWNER Merchant 拥有、公开且 `PUBLISHED` 的任意 kind Work | 仅 `marketRegion: cn` 与 CNY | 先选官方 Mounted 或 Headless |
| 弹幕加赞赏 | 因 Danmaku 要求而使用同一个 `PUBLISHED + VERIFIED Website Work` | 仅 `marketRegion: cn` 与 CNY | Danmaku Mounted；Tip 先选官方 Mounted 或 Headless |

Standalone Tip 的 Work 与承载 UI 的宿主页彼此独立。仅赞赏不要求 Website kind、仓库、HTTPS Origin、DNS、域名、Website verification 或 Commerce Application。组合分支的 Website 与 DNS 门禁只来自 Danmaku；Tip 本身不增加 Origin 或 Application 门禁。

## 共同起点与写入前预检

1. 第一项业务动作以资格守卫模式调用 `$become-a-creator`。只有它确认当前用户通过 `MerchantAccountMember(role=OWNER)` 拥有并选定有效 Merchant 后才继续；本 Skill 不自行运行登录、申请或商家选择命令。
2. 运行 `viceme profile list`，只记录并固定当前 Profile、API/Web base URL 和精确 `marketRegion`。页面 locale 不选择市场；不得切换 Profile，也不得从 hostname、记忆或其他 Profile 推导市场。仅弹幕不受 CN/CNY 限制，保留当前 Profile 既有 `cn` 或 `global` 支持。
3. 仅弹幕分支记录精确部署 HTTPS Origin、目标页面、部署命令、CSP 和浏览器测试，然后按 [Website Work 与安全迁移](#website-work-与安全迁移) 继续。不得因为页面语言是中文就改成 CN，也不得因为页面语言是英文就改成 GLOBAL。
4. 任意包含 Tip 的分支，在任何 Work 创建、更新或发布、Website verification、SDK access 或宿主页写入前，先确认 `marketRegion: cn`；GLOBAL 必须立即停止，且不得留下业务写入。随后请用户选择官方 Mounted UI 或 Headless。Headless 还必须选择 npm 或 CDN ESM。
5. 选定 Tip UI 后，先证明精确 `0.5.0` 的 CN/GLOBAL `index.js` 与 `tip.js` 全部直接可用：

   ```bash
   for asset_url in \
     https://s3.viceme.cn/viceme-sdk/0.5.0/index.js \
     https://s3.viceme.cn/viceme-sdk/0.5.0/tip.js \
     https://s3.viceme.ai/viceme-sdk/0.5.0/index.js \
     https://s3.viceme.ai/viceme-sdk/0.5.0/tip.js
   do
     http_code="$(curl --silent --show-error --connect-timeout 5 --max-time 15 --output /dev/null --write-out '%{http_code}' "$asset_url")" || exit 1
     test "$http_code" = "200" || exit 1
   done
   ```

   `curl` 不使用 `--location`；每个请求必须在 15 秒内直接返回精确 `200`，不得跟随或接受重定向。组合分支还要用完全相同的超时、无重定向和精确 `200` 规则检查：

   ```bash
   asset_url="https://s3.viceme.cn/viceme-sdk/0.5.0/danmaku.js"
   http_code="$(curl --silent --show-error --connect-timeout 5 --max-time 15 --output /dev/null --write-out '%{http_code}' "$asset_url")" || exit 1
   test "$http_code" = "200" || exit 1
   ```

   Headless npm 路线还必须从官方 registry 精确确认该版本：

   ```bash
   npm view @viceme-ai/sdk@0.5.0 version --json \
     --fetch-timeout=15000 --fetch-retries=0 \
     --registry=https://registry.npmjs.org \
     --@viceme-ai:registry=https://registry.npmjs.org
   ```

   结果必须精确为 `0.5.0`。任一必需检查失败就停止，不创建、验证或发布 Work，不写 SDK access，也不编辑宿主页。不得改用 `latest`、alias、声明式或全局 loader、Git 依赖、私有镜像同名包或复制 SDK 源码。

## 仅赞赏的 Work 选择

1. 使用资格守卫返回的 Merchant 运行：

   ```bash
   viceme merchant work list --merchant <merchant-id>
   ```

2. 只保留权威响应证明 `owner.kind: MERCHANT`、`owner.merchantAccountId` 等于所选 Merchant、公开且 `status: PUBLISHED` 的 Work。Work kind 不受限制。逐项展示合格 Work 的 ID、kind 和标题，让用户选择；不得从宿主页、仓库、标题、域名或旧 loader 推断收款目标。
3. 没有合格 Work 时，询问哪项真实 Skill、服务、商品或网站作品会收到赞赏，并要求用户先通过适用于该作品的最终创作者发布流程发布真实作品，再带着同一 Work ID 恢复本流程。只有真实作品是可下载 Skill 时才转交 `$sell-a-skill`；服务、商品或网站不得转给该 Skill。只承载 Tip UI 的宿主页不是作品证据，不能据此创建或推断 Website Work。
4. 用户选定后重新读取该 Work，并复核 Merchant 归属与 `PUBLISHED` 状态。仅赞赏不读取页面 loader 来确认 Work 身份，不执行 Website ownership verification，也不创建、更新、暂停或激活 Commerce Application。已有可选 Commerce Application 只能提供来源归因，不是 Tip 门禁。
5. 进入 [SDK access 完整更新](#sdk-access-完整更新)，请求 hosted feature 为 `tip`。

## Website Work 与安全接入

本节只适用于仅弹幕和组合分支。

1. 规范 Origin 必须是精确小写 HTTPS `scheme + host`，不含凭据、路径、查询、片段或尾部斜杠。预览域名与生产域名是不同 Origin。
2. 写入前只读检查页面。只接受本 Skill 定义的精确 `0.5.0` ESM imports；发现声明式或全局 loader、旧 embed 标签、`data-viceme-*`、非精确版本或多套 ViceMe 运行时时立即停止，要求用户先移除，不把它们作为 Work 身份或迁移输入。
3. 页面已有唯一的精确 `0.5.0` ESM 接入时，只接受其中公开的 `wrk_test_...` 或 `wrk_live_...`。运行 `viceme merchant work sdk-access list --merchant <merchant-id>`，按 `keys.test` 或 `keys.live` 精确定位 Work，再读取 Work 并确认属于当前 Merchant；缺少可验证 Work 身份时，在任何 Work、Website verification、SDK access 或页面写入前停止。
4. 页面没有当前精确 ESM 接入时，运行 `viceme merchant work list --merchant <merchant-id>`，只保留 `kind: WEBSITE` 且 `website.canonicalOrigin` 与部署 Origin 完全一致的候选：

   - 0 个候选时才创建新 Work。
   - 恰好 1 个候选时复用。
   - 多个候选时，在任何 Work、Website Verification、DNS、SDK access 或页面写入前停止，展示每项 `id`、`title`、`status` 与 `website.ownershipStatus`，请用户按 Work ID 选择。不得默认选择第一项、最新项、`PUBLISHED` 项或 `VERIFIED` 项。

5. 用户选择后重新读取 Work，复核 Merchant、Website kind 与 canonical Origin。确实不存在时才用稳定 `clientRequestId` 创建：

   ```json
   {
     "kind": "WEBSITE",
     "merchantAccountId": "<merchant-id>",
     "clientRequestId": "<stable-idempotency-key>",
     "slug": "website-slug",
     "title": "Website title",
     "canonicalOrigin": "https://creator.example",
     "content": {
       "summary": "Observed public purpose",
       "bodyMarkdown": "Observed public description",
       "templateType": "WEBSITE",
       "tags": [],
       "media": [],
       "actionConfig": {}
     }
   }
   ```

   运行 `viceme merchant work create --input <json>`。响应丢失时用同一个 `clientRequestId` 原样重放，不创建平行身份。
6. 运行 `viceme merchant work get <work-id> --merchant <merchant-id>`。`SUSPENDED` 或 `ARCHIVED` 时停止；只有 `DRAFT` 或 `PUBLISHED` 继续。ownership 不是 `VERIFIED` 时先读取最新 verification。`PENDING` 挑战只有当前执行仍持有 create 刚返回且未过期的明文 `challenge` 才能复用；GET 不能找回 challenge。否则从最新 Work revision 创建替代挑战：

   ```bash
   viceme merchant work website-verification create <work-id> \
     --merchant <merchant-id> --expected-revision <work-revision>
   ```

   把 `challenge` 原样发布到 `dnsRecordName`。公共 DNS 精确解析后运行：

   ```bash
   viceme merchant work website-verification verify <work-id> \
     --merchant <merchant-id> \
     --expected-verification-version <verification-version>
   ```

   验证后重新读取 Work；不得为了恢复 `PENDING` verification 创建第二个 Work。
7. `DRAFT` Work 按最新 revision 更新为 `PUBLISHED`，随后重读；已经 `PUBLISHED` 时不重复更新。只有最终同时满足 `status: PUBLISHED`、`website.ownershipStatus: VERIFIED` 与精确 canonical Origin 才继续。
8. 仅弹幕进入 SDK access，加入 `danmaku`；组合加入 `danmaku` 与 `tip`。组合中的 Tip 不触发任何 `WEBSITE_WIDGET` 创建、更新、暂停或激活。

## SDK access 完整更新

1. 对已确认 Work 先只读完整快照：

   ```bash
   viceme merchant work sdk-access get <work-id> \
     --merchant <merchant-id>
   ```

   资源存在时记录完整 hosted `features`、完整 `accessFeatures`、`status`、永久公开的 `keys.test` 与 `keys.live`，以及精确 `configVersion`；不存在时明确记录。仅赞赏不要求宿主页已有 loader 身份。
2. 页面目标 hosted feature set 是现有 `features` 与本次分支请求的并集；只有用户明确要求移除某项时才从完整集合减去。组合分支一次写入 `danmaku,tip`，不得拆成两次更新。资源不存在时创建：

   ```bash
   viceme merchant work sdk-access create <work-id> \
     --merchant <merchant-id> [--feature danmaku] [--feature tip]
   ```

3. 已存在时从刚读取的精确 `configVersion` 做一次完整 hosted replacement；不传 `--follow`、`--purchase` 或 `--clear-access`，让 CLI 把快照中的完整 `accessFeatures` 原样写回：

   ```bash
   viceme merchant work sdk-access update <work-id> \
     --merchant <merchant-id> --expected-config-version <config-version> \
     [--feature danmaku] [--feature tip]
   ```

   `DISABLED` 资源也通过同一次完整 update 恢复，不删除 Website paid/follow access。最终 hosted 集合为空但 `accessFeatures` 非空时使用 `--clear-hosted`；只有 hosted 与 access 都明确应停止时才运行 `sdk-access disable`。
4. create 一次返回 `keys.test` 与 `keys.live`；update、disable、重新启用和恢复不得轮换。写后重读并确认：两个 key 未变、hosted feature set 完整、`accessFeatures` 与写前逐项一致、状态正确，且 `configVersion` 单调增加。
5. 写入冲突时重新读取完整快照，合并本次请求后请用户确认，再用新精确版本重试。响应丢失时先读取同一资源，不创建第二份 access。

## 宿主页接入

新接入只使用精确 `0.5.0` ESM 公共面：`createViceMe(...)`、`mountDanmaku(...)`、`mountTip(...)`，或 Headless 的 `createTip(client).getConfig()/.open()/.destroy()`。完整 Mounted、Headless、仅弹幕与组合示例都在 [integration-contract.md](references/integration-contract.md)；单 HTML Mounted Tip 起点见 [single-html.html](templates/single-html.html)。

- 仅弹幕根据当前 Profile 选择 CN 或 GLOBAL 精确 CDN，不收窄既有 market 支持。写 SDK access 或页面前，对所选区域的 `index.js` 与 `danmaku.js` 执行相同的 5 秒连接、15 秒总时限、无 redirect、精确 `200` 预检。
- 任意 Tip 路线使用已预检的 CN ESM 或已精确安装的 npm 包，并从 `keys.test` 开始。Headless 的金额和 provider 只来自 `getConfig()`，`open()` 结果只按 `PAID | CANCELLED | UNKNOWN` 处理。
- 组合只创建一个 client。官方路线可以分别 `mountDanmaku` 与 `mountTip`；Headless 路线只 `mountDanmaku`，Tip 使用 `createTip`，不得同时挂载两种 Tip UI。一个 mount 失败不得销毁另一个成功能力。
- SPA、组件或路由真实卸载时先销毁全部 mount/controller，再调用 `client.destroy()`；不要使用会在 bfcache 时触发的 `pagehide`。
- 页面只能保留一个选定的精确 `0.5.0` ESM 接入；不得生成或保留第二套运行时。

## Tip 安全与验证

1. Headless 配置读取必须保持 credentialless、无重定向、精确官方 Origin/状态/JSON、8 秒、16 KiB、可取消和凭据错误合同；不得由宿主自行 `fetch` 内部订单或支付 API。
2. `sourceOrigin` 只能由官方服务端从浏览器 Referer 规范化。宿主页使用 `Referrer-Policy: no-referrer` 或 Referer 缺失时必须 fail closed；不得从 query、init 或消息 fallback。
3. Headless 双方只接受精确官方 Origin、直接 window、每次随机 channel、匹配 workKey 与严格 schema。事件名固定为 `viceme:tip-headless-ready`、`viceme:tip-headless-init`、`viceme:tip-headless-result`。外部宿主不接收 token、订单号、订单 capability 或支付 action。
4. 先运行 Local Fake，覆盖 `PAID`、`CANCELLED`、`UNKNOWN` 与配置失败；再用 `keys.test` 完成真实 SANDBOX、桌面、320px、键盘、减少动画、弹窗阻止和 Referer fail-closed 验证。组合还验证一个能力失败不移除另一个。
5. 展示 SANDBOX 证据并取得用户明确确认后，才把所选路线唯一 `createViceMe` 中的 `keys.test` 替换为 `keys.live`。不改变 imports、UI 路线或 Work。production key 不能模拟支付；真实支付必须另行明确确认。

## 完成与恢复

- 报告所选分支、Work ID/kind、公开 `keys.test`/`keys.live`、Profile market、接入路线、变更文件、检查结果，以及是否执行 SANDBOX 或真实支付。不要把 Work key 称为 secret。
- 不报告登录 token、cookie、DNS challenge、支付详情、handoff token、订单 capability 或签名 URL。
- SDK access 写入后无法完成宿主接入时，从最新版本用一次 update 恢复写前完整 hosted features 与 `accessFeatures`；原资源为 `DISABLED` 时恢复后再次 disable，原资源不存在时 disable 新资源。永久 key 不轮换。
- Website Work 或 verification 冲突时重新读取权威 revision/version，不猜测下一个值，也不创建替代 Work。
- Standalone Tip 不因来源归因缺失而补建 Commerce Application。Website 的 follow/paid `WEBSITE_WIDGET` 与 `HOSTED_CHECKOUT` 始终由拥有平台资源的发布流程和 Shop 管理，本 Skill 只原样保留对应 `accessFeatures`。
