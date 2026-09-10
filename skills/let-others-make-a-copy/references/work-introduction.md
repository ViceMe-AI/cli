# 保存发布后的作品介绍

本流程只在主 Skill 的发布成功步骤中使用。正文中的介绍已由作者明确采用或提交后，才能更新；不把关闭卡片、无回复或 AI 草稿本身当作授权。

## 读取当前作品

复用本次成功 Publication 返回的 `merchantAccountId`、`workId` 和 `result.workUrl`，不根据标题猜测目标，不向作者展示这些内部 ID。使用当前已确定的 CLI 完整路径和同一 Profile：

```bash
viceme merchant work get <work-id> --merchant <merchant-account-id>
```

确认响应 `id` 与目标一致、`kind=WEBSITE`、`status=PUBLISHED`、`owner.kind=MERCHANT` 且 `owner.merchantAccountId` 一致，并且存在 `activeRevision`。`draftRevision` 非空时停止本次自动更新并说明已有待处理草稿，不能覆盖或顺带发布其他人的改动。权限、身份、目标或响应异常时停止，不切换账号。

## 构造最小更新

作者确认的介绍写入 `content.summary`。`content` 是完整内容快照，不能只传 `summary`：从刚读取的 `activeRevision` 原样保留 `bodyMarkdown`、`templateType`、`tags`、`media`、`actionConfig`，`usageInstructions` 非 null 时也原样保留。不得把完整 `activeRevision` 对象直接当作 `content`，其中的 ID、版本、digest 和日期不是可写字段。必要字段为空或不符合现有契约时停止，不用 AI 内容补造正文来通过校验。

新介绍必须非空且不超过 500 字；用户提交过长文本时先展示压缩稿再确认，不静默截断。若新介绍与当前 `activeRevision.summary` 一致，直接沿用，跳过更新。

将请求安全序列化为任务临时目录中的 JSON 文件，不用 shell 字符串拼接用户文本。请求结构示例如下；其中内容字段都应替换为本次已确认介绍和刚读取的真实快照：

```json
{
  "merchantAccountId": "00000000-0000-4000-8000-000000000001",
  "expectedRevision": 3,
  "content": {
    "summary": "记录游泳训练距离，查看每天的变化，并按自己的训练目标调整展示。",
    "bodyMarkdown": "保留当前作品正文原文。",
    "templateType": "website-replica",
    "tags": [],
    "media": [],
    "actionConfig": {}
  },
  "status": "PUBLISHED"
}
```

`expectedRevision` 使用刚读取的 Work 顶层 `revision`，不是 `activeRevision.version`。显式传 `status=PUBLISHED`，让介绍直接生效；省略会只生成草稿。不得提交 `title`、价格或其他无关变更，不执行新的 `replica publish`。

```bash
viceme merchant work update <work-id> --input <introduction-json>
viceme merchant work get <work-id> --merchant <merchant-account-id>
```

按顺序执行；更新成功后才执行读回。确认 Work 仍为 `PUBLISHED`，`activeRevision.summary` 等于作者采用的介绍，且正文、标签、图片、交互配置与原快照一致，才能报告保存完成。删除本次创建的临时请求文件；失败时仍在对话中保留作者介绍，不输出私有配置或认证信息。

`WORK_REVISION_CONFLICT` 或 `WEBSITE_REPLICA_PUBLICATION_IN_PROGRESS` 表示作品已变化或另有发布进行中。停止并保留作者介绍，不自动重试、覆盖草稿或沿用旧 revision；用户要求继续时重新读取并核对当前内容。网络结果不明确时，先读回目标一次确认是否已经生效，不直接重发更新。原发布成功与介绍更新失败分别报告。
