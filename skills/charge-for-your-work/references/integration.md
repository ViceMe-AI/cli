# 接入参考

## 网站身份内部配置

CLI 使用独立 `.viceme/website-access.v1.json`，包含市场／API authority、稳定项目绑定和 requestId。旧 Replica 绑定只读导入 Work／Merchant，不改旧恢复文件。复制项目或跨市场发生绑定冲突时不删除状态规避检查。高层 API 验证归属并负责新 Work 的幂等创建，不要求 Origin。

```bash
viceme website access configure --project /absolute/project --input /private/access-input.json
viceme website access status --project /absolute/project
viceme website access resume --project /absolute/project
```

不要预先执行登录或资格检查；收到 AUTHENTICATE_CREATOR 才使用统一登录流程，收到 COMPLETE_CREATOR_QUALIFICATION／APPLY_CREATOR／SELECT_MERCHANT 等动作才补资格。申请仍需已有授权，永久用户名缺失时由作者确定。缺少命令或参数时仅升级一次当前市场 CLI；服务端不支持新路由则等待服务端升级，不回退容易覆盖配置的手工 CRUD。

## Access 内部配置

输入是本次 featureKey 差量，服务端事务合并，保留旧 hosted/access 功能与公开 key。已有 featureKey 不能切换策略；新增键明确切换入口。金额使用精确的整数最小单位，不用浮点计算或自动换汇；GLOBAL 未接通渠道时保持 PENDING_CHANNEL。

```json
{
  "work": {"title": "图片导出工具", "summary": "编辑图片并导出高清版本"},
  "accessFeatures": [
    {"featureKey": "export", "title": "高清导出", "policyType": "WORK_ENTITLEMENT", "availability": "ACTIVE", "pricingIntent": {"currency": "CNY", "amountMinor": 990}, "status": "ACTIVE"}
  ]
}
```

关注规则用 FOLLOW_OWNER，pricingIntent 为 null。GLOBAL USD 示例将 currency 改 USD、availability 改 PENDING_CHANNEL，金额由作者确定。work.canonicalOrigin、workId 和 merchantAccountId 可省略；已绑定时不能换成其他目标。价格和 availability 以服务端实际结果为准，不从总渠道枚举判断可收款。

响应含 nextAction、completedSteps、phase、requestId、resumeArgs、Work 目标、access、availability。API 写入成功后，CLI 用现有 sdk-access get 写后重读。PLATFORM_CONFIGURED 不代表宿主已修改或验证；权限、网络或配置冲突时保存原请求恢复，不能为了 retry 新建 Work。代码失败保留可恢复平台配置并说明，不能盲目把整份配置 disable 或覆盖他人的新版本。

## 浏览器 SDK

安装 `@viceme-ai/sdk`，生产用 keys.live，region 跟随配置市场：

```ts
import { createViceMe } from "@viceme-ai/sdk";
const viceme = createViceMe({workKey: "wrk_live_...", region: "cn"});
await viceme.ready();
async function exportImage() {
  const decision = await viceme.access.require("export");
  if (!decision.allowed) return;
  await originalExportImage();
}
```

显示名称和价格使用 access.getFeatures()。宿主只有原动作外层门控，匿名识别／恢复、登录默认关注、结账和账号领取均由平台及 SDK 处理。桌面结账保留在 SDK Access Layer，移动 H5/WAP 可进入支付渠道页面；窗口创建、拦截和失败反馈属于 Shop。仅根据新的服务端访问决定解锁，取消或支付窗口失败时宿主不解锁。

## 宿主回执

完成代码与测试后生成回执。configVersion 使用当前平台版本，checks 写真实执行的检查；没有验证不得填 verified=true。

```json
{
  "requestId": "配置返回的 requestId",
  "workId": "配置返回的 Work ID",
  "configVersion": 3,
  "modifiedFiles": ["src/export.ts"],
  "checks": ["取消解锁不执行导出；已拥有权益时原动作执行一次"],
  "verified": true
}
```

```bash
viceme website access resume --project /absolute/project --receipt /private/host-receipt.json
```

至少验证未购买／已购买、匿名恢复、关注／取消、登录绑定、键盘和焦点；GLOBAL 验证待渠道不生成支付动作，不能把预览当实际收款测试。主 Agent 仍需核验回执证据。

## 补充同一作品资料

```bash
viceme website work enrich --project /absolute/project --input /private/work-content.json
```

内容差量支持 summary、bodyMarkdown、tags、media、usageInstructions；例如 `{"summary":"作者已采用的新介绍"}`。summary 1–500 字，媒体仅传经过平台上传校验的引用。命令复用 Work get/update，把差量合并到完整 activeRevision 内容，使用 Work 顶层 revision 作 expectedRevision，显式 status=PUBLISHED 并读回。既有 draft 不覆盖。接口失败仍保留本次 pending 内容；不带 input 重跑 enrich 可恢复，结果不明确先读回，不重复写。

用户已提供用于作品展示的素材和文案无需再问用途；AI 草稿先展示取得采用授权，跳过则保留现有资料。成功反馈说作品资料已保存，失败单独报告，不把资料失败说成支付接入失败。
