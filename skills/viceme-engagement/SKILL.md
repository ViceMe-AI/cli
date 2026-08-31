---
name: viceme-engagement
description: 在同一个已验证 Website Work 上接入 ViceMe 托管弹幕与开放赞赏，通过一个 Work SDK access 同时保留 hosted 和 access features，并选择官方或 Headless Tip UI。
---

# 同时接入 ViceMe 弹幕与开放赞赏

配置一个权威资源图和一条用户选定的 Tip UI 路线。弹幕与赞赏不得创建两个 Work。Website 与 DNS 要求来自 Danmaku 的独立安全契约；开放赞赏本身不增加宿主页所有权或应用门禁。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示挑战值、handoff 上下文或敏感字段。

## 流程

1. 查看精确生产 HTTPS Origin、目标页面、部署命令、CSP、响应式布局和浏览器测试。
2. 先交给 `$viceme-creator-onboarding` 在当前 CLI 上下文完成登录、创作者资格检查和 Merchant 选择。只有它确认当前用户以 OWNER 身份拥有有效 Merchant 后才继续；本 Skill 不自行运行登录、申请或商家账户选择命令。随后运行 `viceme profile list`，只记录并固定当前 Profile 与精确 `marketRegion`，不得从页面 locale、hostname 或其他 Profile 推导市场。本版开放赞赏只支持 `marketRegion: cn` 与 CNY；否则停止并说明边界。
3. 在任何 Work 创建/更新、Website verification、SDK access 或页面写入前，请用户选择官方 Tip UI 或 Headless。首先证明精确 `0.4.0` Tip 发布物在两个区域完整可用，每条组合路线都要求：

   ```bash
   for asset_url in \
     https://s3.viceme.cn/viceme-sdk/0.4.0/index.js \
     https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js \
     https://s3.viceme.ai/viceme-sdk/0.4.0/index.js \
     https://s3.viceme.ai/viceme-sdk/0.4.0/tip.js \
     https://s3.viceme.cn/viceme-sdk/0.4.0/danmaku.js
   do
     http_code="$(curl --silent --show-error --connect-timeout 5 --max-time 15 --output /dev/null --write-out '%{http_code}' "$asset_url")" || exit 1
     test "$http_code" = "200" || exit 1
   done
   ```

   最后一项是官方 UI 与 CDN Headless 额外要求的不可变 CN Danmaku 入口。

   Headless 需要选择 npm 或 CDN ESM。npm 路线还要求官方注册表精确返回 `0.4.0`：

   ```bash
   npm view @viceme-ai/sdk@0.4.0 version --json \
     --fetch-timeout=15000 --fetch-retries=0 \
     --registry=https://registry.npmjs.org \
     --@viceme-ai:registry=https://registry.npmjs.org
   ```

   每个 CDN 请求都必须在 15 秒内直接返回精确 `200`，不得跟随或接受重定向。任一必需检查失败就停止，不创建、验证或发布 Work，不写 SDK access，也不编辑页面。不得替换为 `latest`、可变版本别名、声明式 loader、浏览器全局对象、Git 依赖或复制源码。完整双区域预检和所选 npm/Danmaku 检查成功后才能继续。
4. 运行 `viceme --profile <profile> merchant work list --merchant <merchant-id>`。只有 `website.canonicalOrigin` 与实际部署 Origin 完全一致时才复用 Website Work。不存在时，用稳定 `clientRequestId` 和只含观察事实的严格请求创建：

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

   运行 `viceme --profile <profile> merchant work create --input <json>`。响应丢失时，用同一个 `clientRequestId` 原样重放，不得创建新的 Work 身份。
5. 无论 Work 是复用还是新建，都运行 `viceme --profile <profile> merchant work get <work-id> --merchant <merchant-id>`。如果状态是 `SUSPENDED` 或 `ARCHIVED`，立即停止并报告，不创建挑战、不改 DNS，也不另建 Work。

   `website.ownershipStatus` 不是 `VERIFIED` 时先运行：

   ```bash
   viceme --profile <profile> merchant work website-verification get <work-id> \
     --merchant <merchant-id>
   ```

   最新 verification 状态是 `PENDING` 时，只有当前执行仍持有 create 刚返回且未过期的明文 challenge 才能继续使用；GET 不找回 challenge。否则读取最新 Work revision 创建替代挑战：

   ```bash
   viceme --profile <profile> merchant work website-verification create <work-id> \
     --merchant <merchant-id> --expected-revision <work-revision>
   ```

   把返回的 `challenge` 原样发布到 `dnsRecordName` 指定的记录。公共 DNS 精确解析后运行：

   ```bash
   viceme --profile <profile> merchant work website-verification verify <work-id> \
     --merchant <merchant-id> \
     --expected-verification-version <verification-version>
   ```

   验证后重新读取 Work。不得为了恢复带 `PENDING` verification 的 `DRAFT` Work 创建第二个 Work，也不得猜测 challenge、verification version 或 revision。
6. 当前 Work 状态是 `DRAFT` 时，从最新 revision 发布：

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "expectedRevision": 2,
     "status": "PUBLISHED"
   }
   ```

   运行 `viceme --profile <profile> merchant work update <work-id> --input <json>`，然后再次运行 `viceme --profile <profile> merchant work get <work-id> --merchant <merchant-id>`。Work 已经是 `PUBLISHED` 时跳过更新；变为 `SUSPENDED` 或 `ARCHIVED` 时停止并报告。
7. 用户确认这个 Work 后，先读取 SDK access，不写入。记录原有 `keys.test`、`keys.live`、完整 hosted `features`、完整 `accessFeatures`、状态和精确 `configVersion`；不存在时明确记录。该快照是冲突处理与回滚来源。
8. 用一次 create 或 update 应用完整 hosted feature set `danmaku,tip`。资源不存在时：

   ```bash
   viceme --profile <profile> merchant work sdk-access create <work-id> \
     --merchant <merchant-id> --feature danmaku --feature tip
   ```

   已存在时从精确版本做一次 hosted replacement update，不传 follow/purchase/clear-access flags，从而保留完整 `accessFeatures`：

   ```bash
   viceme --profile <profile> merchant work sdk-access update <work-id> \
     --merchant <merchant-id> --expected-config-version <config-version> \
     --feature danmaku --feature tip
   ```

   不得分开更新两个 feature，否则会相互覆盖。create 一次返回永久公开的 `keys.test` 与 `keys.live`；update、disable 和重新启用都不轮换。写入后重读并确认两个 key 未变、两个 hosted features 都存在、全部 `accessFeatures` 未变、状态为 `ACTIVE` 且 `configVersion` 单调增加。
9. 两条路线都从 `keys.test` 开始，避免 Tip 验收移动真实资金。

   官方 UI 在稳定 target 上从同一个精确 CDN ESM release 挂载两个能力：

   ```html
   <div id="viceme-engagement"></div>
   <script type="module">
     import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.4.0/index.js";
     import { mountDanmaku } from "https://s3.viceme.cn/viceme-sdk/0.4.0/danmaku.js";
     import { mountTip } from "https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js";

     const target = document.querySelector("#viceme-engagement");
     if (!target) throw new Error("ViceMe engagement target is missing");

     const client = createViceMe({ workKey: "wrk_test_...", region: "cn" });
     await client.ready();
     const mountResults = await Promise.allSettled([
       mountDanmaku(client, { target, theme: "auto" }),
       mountTip(client, { target, theme: "auto" }),
     ]);
     const mountHandles = mountResults.flatMap((result) =>
       result.status === "fulfilled" ? [result.value] : [],
     );

     function destroyViceMeEngagement() {
       for (const handle of mountHandles) handle.destroy();
       client.destroy();
     }
   </script>
   ```

   Headless Tip 路线完整遵循 `viceme-tip` 的 [接入契约](../viceme-tip/references/integration-contract.md)，金额/provider 来自配置，公开结果只走其脱敏联合类型。只挂载 Danmaku，并从同一个精确 release 创建宿主 Tip controller；下面是 CDN ESM，只有选择且通过 npm 预检时才改用等价精确 npm subpath：

   ```html
   <div id="viceme-engagement"></div>
   <script type="module">
     import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.4.0/index.js";
     import { mountDanmaku } from "https://s3.viceme.cn/viceme-sdk/0.4.0/danmaku.js";
     import { createTip } from "https://s3.viceme.cn/viceme-sdk/0.4.0/tip.js";

     const target = document.querySelector("#viceme-engagement");
     if (!target) throw new Error("ViceMe engagement target is missing");

     const client = createViceMe({ workKey: "wrk_test_...", region: "cn" });
     await client.ready();
     const danmakuHandle = await mountDanmaku(client, {
       target,
       theme: "auto",
     });
     const tip = createTip(client);
     const config = await tip.getConfig();
     renderTipControls(config, (selection) => tip.open(selection));

     function destroyViceMeEngagement() {
       tip.destroy();
       danmakuHandle.destroy();
       client.destroy();
     }
   </script>
   ```

   所有 handle/controller 都跟随所属实例。在 SPA、组件或路由真实卸载时运行销毁函数，确保 mounts 与 Headless Tip 都在 `client.destroy()` 前销毁。不要使用会在 bfcache 时触发的 `pagehide`。
10. 保留 CSP 与 nonce。`sourceOrigin` 只能由官方服务端从浏览器 Referer 规范化，不能从 query 或消息 fallback；宿主页使用 `Referrer-Policy: no-referrer` 时 Tip handoff 必须 fail closed。Headless 双方只信任官方 Origin、直接窗口、每次随机 channel、匹配 workKey 与严格 schema；同时校验 message origin 与 source。Danmaku 的 verified Website Origin 仍是必需条件，但不是 Tip 支付门禁。
11. 验证桌面与 320px 宽度：两个能力都挂载，单个挂载失败不移除另一个，宿主控件可点击，弹幕可持久化，Tip 键盘可达且 SANDBOX 结果正确。访客仅赞赏时不要求登录 ViceMe。
12. 展示 SANDBOX 证据并取得用户明确确认后，才把所选路线单个 `createViceMe` 调用中的 `keys.test` 换为 `keys.live`。不要改变 imports，也不要在 Headless 路线额外挂载官方 Tip UI。production key 不能模拟支付。

## 约束

- Work ID 是内部资源身份；两个 Work key 是永久公开标识，不是凭据。
- 组合流程中的 verified Website Origin 与 DNS 验证由 Danmaku 要求；开放赞赏不验证宿主页，也不要求 Commerce Application。
- Profile market region 控制运行时；首版开放赞赏只支持 CN/CNY，页面 locale 不选择市场。
- 宿主不调用 Shop 支付 API、不检查原始支付状态，也不在页面放入凭据或 handoff token。
- 报告公开资源 ID/key、检查、响应式覆盖、SANDBOX 证据和未验证真实支付边界，不暴露秘密或 DNS challenge。

## 恢复

- create/update 响应丢失时，先读取同一 SDK access 再重试。
- 冲突时读取最新精确 `configVersion`、完整 hosted features 与 `accessFeatures`；不得猜版本或拆分组合更新。
- 服务端写入后无法完成接入时，读取最新版本，并用一次 update 恢复写入前的完整 hosted 与 access 配置。原资源为 `DISABLED` 时恢复后再次 disable；原资源不存在时 disable 新资源。永久 test/live key 在回滚中不轮换。
