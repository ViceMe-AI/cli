---
name: viceme-tip
description: 为任意合格且已发布的 Merchant Work 启用开放赞赏，并在独立宿主页接入官方 UI 或 Headless UI。Standalone Tip 不验证宿主页域名，也不要求 Commerce Application；适用于添加、安装、配置或修复 ViceMe 赞赏入口。
---

# 接入 ViceMe 开放赞赏

为一个现有 Work 实现最小且真实的开放赞赏接入。修改宿主前必须完整阅读 [integration-contract.md](references/integration-contract.md)。Work 是唯一收款目标；宿主页与被赞赏 Work 相互独立。Standalone Tip 不创建 Website ownership verification，不校验宿主页域名，也不创建 Commerce Application。

面向用户的文字跟随用户当前语言；中文交流使用自然白话，不直接展示内部协议字段、挑战值或敏感值。

## 流程

1. 先交给 `$viceme-creator-onboarding` 在当前 CLI 上下文完成登录、创作者资格检查和 Merchant 选择。只有它确认当前用户通过 `MerchantAccountMember(role=OWNER)` 拥有有效 Merchant 后才继续；本 Skill 不自行运行登录、申请或商家账户选择命令。
2. 运行 `viceme profile list`，固定当前 Profile，并记录其精确 API base URL 与 `marketRegion`。不得切换 Profile，也不得从 DNS、页面语言、记忆或其他 Profile 推导环境。本版开放赞赏只支持 `cn` Profile 与 CNY；其他市场立即停止并说明边界。
3. 使用上一步选定的 Merchant 运行：

   ```bash
   viceme --profile <profile> merchant work list --merchant <merchant-id>
   ```

   候选可以是任意 Work kind，但必须由权威响应证明 `owner.kind: MERCHANT`、归属所选 Merchant 且 `status: PUBLISHED`。展示合格 Work 并请用户确认目标后才能改变其 Tip 能力。宿主页、仓库、标题和域名都不能用来推断 Work kind 或收款目标。
4. 在任何 Work 创建/发布、Website verification、SDK access 写入或宿主页编辑前，请用户选择官方 UI 或 Headless。先证明精确 `0.5.0` Tip 发布物在两个发布区域都完整可用；每条路线都必须成功执行以下四个检查：

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

   官方 UI 与 CDN Headless 使用已检查的 CN ESM。选择 Headless 时再询问 npm 或 CDN ESM；npm 路线还要求官方注册表精确返回 `0.5.0`：

   ```bash
   npm view @viceme-ai/sdk@0.5.0 version --json \
     --fetch-timeout=15000 --fetch-retries=0 \
     --registry=https://registry.npmjs.org \
     --@viceme-ai:registry=https://registry.npmjs.org
   ```

   每个 CDN 请求都必须在 15 秒内直接返回精确 `200`，不得跟随或接受重定向。任一必需检查失败就停止，不得创建、验证或发布 Work，不得创建或更新 SDK access，也不得编辑宿主页。只有完整双区域预检和所选 npm 检查成功后，才能继续任何业务写入。不得替换为 `latest`、可变版本别名、声明式 loader、浏览器全局对象、Git 依赖或复制的 SDK 源码。
5. 没有合格 Work 时，询问哪项真实 Skill、服务、商品或网站作品会收到赞赏，然后停止当前设置并加载 `$viceme-publish`。沿该 Skill 的权威路线发布真实作品，完成后带着 Work ID 返回本流程。只承载 Tip UI 的页面不是待发布作品，不得为了启用 Tip 创建 Website Work。用户独立选择网站本身作为 Work 时，DNS 所有权验证属于网站发布，不属于 Standalone Tip。
6. 对确认的 Work 先只读：

   ```bash
   viceme --profile <profile> merchant work sdk-access get <work-id> \
     --merchant <merchant-id>
   ```

   资源存在时保存完整 feature set 快照：记录完整 hosted `features`、完整 `accessFeatures`、`status`、`keys.test`、`keys.live` 与精确 `configVersion`，作为冲突处理和回滚来源；不存在时明确记录。两个 key 都是永久公开标识，不是凭据。
7. 应用服务端状态：

   - 资源不存在时创建：

     ```bash
     viceme --profile <profile> merchant work sdk-access create <work-id> \
       --merchant <merchant-id> --feature tip
     ```

   - 资源存在但 hosted features 不含 `tip` 时，从刚读取的 `configVersion` 做一次完整 hosted replacement update。命令中传回原有全部 hosted features，再加 `tip`；不要传 `--follow`、`--purchase` 或 `--clear-access`，从而原样保留完整 `accessFeatures`：

     ```bash
     viceme --profile <profile> merchant work sdk-access update <work-id> \
       --merchant <merchant-id> --expected-config-version <config-version> \
       [--feature danmaku] --feature tip
     ```

   - 状态是 `DISABLED` 时，从精确 `configVersion` 用同一完整 hosted feature set 更新并重新启用；不得借机删除访问功能。
   - 状态是 `ACTIVE` 且已有 `tip` 时不要重复写入。

   create 会一次返回 `keys.test` 与 `keys.live`；update、disable 和重新启用都不轮换它们。写入后重新读取，必须同时确认两个 key 未变、hosted features 完整、`accessFeatures` 完整保留、`configVersion` 单调增加且状态正确。
8. 使用下面所选路线接入。Standalone Tip 到这里仍不执行 Website ownership verification，也不创建 Commerce Application。可选 Commerce Application 只做可信来源归因，不是 Tip 授权门禁。

## 官方 UI

先使用 `keys.test` 和已预检的精确 CDN ESM：

```html
<div id="viceme-tip"></div>
<script type="module">
  import { createViceMe } from "https://s3.viceme.cn/viceme-sdk/0.5.0/index.js";
  import { mountTip } from "https://s3.viceme.cn/viceme-sdk/0.5.0/tip.js";

  const target = document.querySelector("#viceme-tip");
  if (!target) throw new Error("ViceMe Tip target is missing");

  const client = createViceMe({ workKey: "wrk_test_...", region: "cn" });
  await client.ready();
  const mountHandle = await mountTip(client, {
    target,
    theme: "auto",
  });

  function destroyViceMeTip() {
    mountHandle.destroy();
    client.destroy();
  }
</script>
```

在桌面、移动端和键盘路径验证 SANDBOX。只有展示证据并取得用户明确确认后，才把公开标识替换为 PRODUCTION 的 `keys.live`；固定 imports 和其他选项保持不变。真实 SPA、组件或路由卸载时，必须先运行 `mountHandle.destroy()` 再运行 `client.destroy()`；静态文档不虚构卸载事件，也不使用会在 bfcache 时触发的 `pagehide`。

## Headless

使用 [integration-contract.md](references/integration-contract.md) 中 npm 或精确 CDN ESM 示例。宿主从 `getConfig()` 渲染金额和渠道控件，并在用户点击或键盘激活调用栈内直接调用 `open()`；宿主不创建、查询或恢复订单。最终确认层由 ViceMe 只读控制，不能被宿主替换、重绘或附加支付结论。

Headless 同样从 `keys.test` 开始，先完成 Local Fake 与 SANDBOX 检查。只有用户确认证据后才能换为 `keys.live`。

## 来源与 handoff 安全边界

- `sourceOrigin` 只能由官方页面服务端从浏览器 Referer 规范化得到；宿主和 SDK 都不得通过 query、初始化参数、消息字段或其他 fallback 自报来源。宿主若设置 `Referrer-Policy: no-referrer`，或浏览器没有发送 Referer，handoff 必须 fail closed 并显示不可用，绝不降级绕过。
- SDK 只信任所选 Profile 的官方 Web Origin、它直接打开的 `window`，以及每次 `open()` 新生成的随机 channel。官方页面只接受 Referer 对应的 `sourceOrigin`、直接 `opener`/`parent`、匹配的 channel 与 workKey，以及严格 schema。双方都必须同时校验 `event.origin`、`event.source`、channel 和消息形状。
- 外部宿主不接收 handoff token、订单 capability、`orderNo` 或 `PaymentAction`，也不直接调用 Tip 建单、状态、取消或支付渠道 API。公开结果只包含约定的 `PAID`、`CANCELLED`、`UNKNOWN` 脱敏联合类型。
- 无匹配可选归因时，来源只是 unverified Origin，不是被拒绝的 Origin。不得把宿主页与被赞赏 Work 是否同域、是否为 Website 或是否完成 DNS 验证当作 Tip 门禁。

## 完成

- 报告 Work ID 与 kind、所选 Profile、公开 `keys.test`/`keys.live`、接入路线、变更文件和检查结果。
- 说明是否执行了 SANDBOX 模拟以及是否执行真实生产支付。生产 key 不能模拟支付。
- 不得报告登录凭据、cookie、支付详情、handoff token、订单 capability 或 DNS challenge；也不要把 Work key 称为 secret。
- 只停止 Tip 且保留 Danmaku 时，从最新 `configVersion` 更新完整 hosted feature set，移除 `tip` 但保留 `danmaku` 和全部 `accessFeatures`。只有所有 hosted 与 access 能力都应停止时才 disable；任何操作都不轮换 key。

## 恢复

- create/update 响应丢失时，先读取同一 SDK access 再决定是否重试。
- 发生配置冲突时，读取最新 `configVersion`、完整 hosted features 与 `accessFeatures`，向用户重新确认后再写；不得猜版本或发送部分 feature list。
- 本流程写入 SDK access 后却无法完成接入时，读取最新状态，并用一次更新恢复完整 feature set，同时保护 `accessFeatures` 和永久 key。原状态是 `DISABLED` 时恢复完整配置后再次 disable；原资源不存在时 disable 新建资源。不得用部分 feature list 伪造回滚。
- 不得因为 Tip 设置结果未知而创建另一个 Work；作品发布只能通过 `viceme-publish` 的权威身份恢复。
