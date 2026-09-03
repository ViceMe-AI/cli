# Work 与 SDK access

本参考只在源码、分支、Merchant、Profile 和 `sdk_version` 已固定后读取。业务 CLI 输出按完整 JSON 判断 `ok` 与 `error.code`；不得把业务命令管道到 `head`、`grep` 或其他会掩盖退出码的命令。

## 仅赞赏的 Work 选择

1. 使用资格守卫返回的 Merchant 运行：

   ```bash
   viceme merchant work list --merchant <merchant-id>
   ```

2. 只保留权威响应证明 `owner.kind: MERCHANT`、`owner.merchantAccountId` 等于所选 Merchant、公开且 `status: PUBLISHED` 的 Work。Work kind 不受限制。
3. 逐项展示合格 Work 的 ID、kind 和标题，让用户选择；不得从宿主页、仓库、标题、域名或旧 loader 推断收款目标。
4. 没有合格 Work 时，询问哪项真实 Skill、服务、商品或网站作品会收到赞赏，并要求用户先通过适用于该作品的最终创作者发布流程发布真实作品。只有真实作品是可下载 Skill 时才转交 `$sell-a-skill`。只承载 Tip UI 的宿主页不是作品证据。
5. 选择后运行 `viceme merchant work get <work-id> --merchant <merchant-id>`，重新确认 Merchant 归属与 `PUBLISHED`。仅赞赏不执行 Website ownership verification；已有可选 Commerce Application 只能提供来源归因，不是 Tip 门禁。

随后进入 [SDK access 完整更新](#sdk-access-完整更新)，请求 hosted feature `tip`。

## Website Work 与安全接入

本节只适用于仅弹幕和组合分支。

1. 部署 Origin 必须是精确小写 HTTPS `scheme + host`，不含凭据、路径、查询、片段或尾部斜杠。预览域名与生产域名是不同 Origin。
2. 一次性检查页面。只接受本次固定 `sdk_version` 的精确 ESM imports；发现旧 loader、`data-viceme-*`、其他版本或多套 ViceMe runtime 时停止，要求用户先决定保留哪一套，不把它们当作迁移输入。
3. 页面已有唯一同版本 ESM 接入时，从公开 `keys.test` 或 `keys.live` 精确匹配：

   ```bash
   viceme merchant work sdk-access list --merchant <merchant-id>
   ```

   匹配后重读对应 Work，确认属于当前 Merchant。缺少可验证 Work 身份时停止。
4. 页面没有当前 ESM 接入时，运行一次 `viceme merchant work list --merchant <merchant-id>`，只保留 `kind: WEBSITE` 且 `website.canonicalOrigin` 与部署 Origin 完全一致的候选：
   - 0 个才创建；
   - 1 个直接复用；
   - 多个时展示 ID、标题与状态，请用户按 ID 选择，不默认取第一项、最新项或 PUBLISHED 项。
5. 选择后重读 Work，复核 Merchant、Website kind 与 canonical Origin。确实不存在时，使用稳定 `clientRequestId` 创建：

   ```json
   {
     "kind": "WEBSITE",
     "merchantAccountId": "00000000-0000-4000-8000-000000000001",
     "clientRequestId": "stable-idempotency-key",
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

   把示例值替换为本次权威值后运行 `viceme merchant work create --input <create-json>`。响应丢失时只用同一个 `clientRequestId` 原样重放。
6. 运行 `viceme merchant work get <work-id> --merchant <merchant-id>`：`SUSPENDED` 或 `ARCHIVED` 停止；`PUBLISHED` 不重复更新；`DRAFT` 使用刚读取的精确 revision 和下面的最小请求：

   ```json
   {
     "merchantAccountId": "00000000-0000-4000-8000-000000000001",
     "expectedRevision": 1,
     "status": "PUBLISHED"
   }
   ```

   替换 Merchant 与 revision 后运行：

   ```bash
   viceme merchant work update <work-id> --input <publish-json>
   ```

   不传 `--merchant`，也不把 `revision`、`slug` 或 `website` 塞入 update 请求。发布成功后立即重读同一个 Work；只有最终仍属于当前 Merchant、`kind: WEBSITE`、`status: PUBLISHED` 且 canonical Origin 与部署 Origin 精确匹配才继续。`website.ownershipStatus` 不参与门禁。
7. 仅弹幕请求 `danmaku`；组合在同一个 access 中请求 `danmaku,tip`。组合中的 Tip 不创建、更新、暂停或激活 Commerce Application。

## SDK access 完整更新

先读取同一 Work 的完整快照：

```bash
viceme merchant work sdk-access get <work-id> --merchant <merchant-id>
```

资源存在时记录完整 hosted `features`、完整 `accessFeatures`、`status`、永久公开的 `keys.test` 与 `keys.live`，以及精确 `configVersion`；`WORK_SDK_ACCESS_NOT_FOUND` 表示可以创建，不是需要搜索其他命令的异常。create 一次返回 `keys.test` 与 `keys.live`，后续更新不得重新创建 access。

目标 hosted feature set 是现有 `features` 与本次分支请求的并集；只有用户明确要求移除时才做减法。组合一次写入两个 feature，不拆分：

```bash
viceme merchant work sdk-access create <work-id> \
  --merchant <merchant-id> \
  --feature danmaku \
  --feature tip
```

只保留当前分支需要的 `--feature`。已有资源时用刚读取的精确版本做一次完整 replacement：

```bash
viceme merchant work sdk-access update <work-id> \
  --merchant <merchant-id> \
  --expected-config-version <config-version> \
  [--feature danmaku] [--feature tip]
```

不传 `--follow`、`--purchase` 或 `--clear-access`，让 CLI 把快照中的完整 `accessFeatures` 原样写回。`DISABLED` 资源也通过同一次 update 恢复；最终 hosted 为空但 access 非空时才用 `--clear-hosted`，只有两者都明确应停止时才运行 `sdk-access disable`。

写后重读并逐项确认：

- 两个 key 没有发生轮换；
- hosted feature set 完整；
- `accessFeatures` 与写前一致；
- `status` 正确；
- `configVersion` 单调增加。

revision 或 configVersion 冲突时重读完整快照，重新合并后再写，不猜下一个值。响应丢失时先读同一资源，不创建第二份 access。

## 失败恢复

SDK access 已写入但宿主接入失败时，使用最新 `configVersion` 做一次 update，恢复写前完整 hosted `features` 与 `accessFeatures`；原资源为 `DISABLED` 时恢复后再次 disable，原资源不存在时 disable 新资源。永久 key 不轮换，Website Work 不创建替代身份。
