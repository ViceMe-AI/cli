---
name: withdraw-income
description: 通过 ViceMe CLI 自主提取国内站创作者收入，检查可提现余额和收款账户，引导首次实名签约或微信绑定，并恢复原提现、查询到账进度。适用于提现收入、查询提现结果及上层 Agent 安排的提现任务。
---

# 提取创作者收入

首次调用前，按 [creator-tools 的 CLI 定位流程](../creator-tools/SKILL.md#cli-定位) 找到现有 CLI，并使用其完整路径。所有账户、金额、渠道和状态以 CLI 的结构化响应为准；不得读取登录凭据、调用私有接口或自行操作数据库。

## 执行边界

用户或上层 Agent 已交付提现任务时，直接自主执行，不要求逐笔确认。此 Skill 不建立后台调度；定期执行与后续唤醒由上层 Agent 负责。

仅支持当前国内站 Profile。运行 `viceme withdraw context`，读取可提现收入、渠道能力、推荐账户及办理入口。命令不存在时，按 creator-tools 更新 CLI；`WITHDRAWAL_SCOPE_REQUIRED` 表示旧登录缺少提现能力，按 creator-tools 的普通登录流程重新登录后继续。未登录同样复用该流程；不得为提现增加另一套授权页面。

`MERCHANT_OWNER_REQUIRED` 时复用 `$become-a-creator` 检查创作者资格，已有有效资格时不重复申请。账户归属始终由当前登录身份决定，不能用用户提供的 ID 替代服务端身份。

## 发起与首次办理

为每个新提现任务生成一个唯一的 `request-id`（例如 UUID），写入当前 Agent 任务上下文。恢复、重新登录、首次办理返回及网络失败都继续使用这个标识；只有明确的新提现任务使用新标识。

```bash
viceme withdraw create --request-id <本次任务的稳定标识>
viceme withdraw create --request-id <本次任务的稳定标识> --amount 100.00
viceme withdraw create --request-id <本次任务的稳定标识> --payout-method <收款账户ID>
```

- 未指定金额时，由 CLI 提取当前允许的最大金额；指定金额以人民币元传入，最多两位小数。仅可提现收入参与提现。
- 未指定账户时，由平台按最近一次成功提现且仍可用、最早绑定且渠道支持的顺序推荐。指定账户不可用时报告原因，不自行替换。
- `NO_FUNDS`：本次无可提现收入，没有创建提现单。
- `ACTION_REQUIRED / COMPLETE_SETUP`：向用户展示 CLI 返回的办理入口，请本人完成缺失的实名、签约、绑定等操作。不要收集身份证号、完整银行卡号或第三方签名参数。完成后重试原命令，自动接续。
- 微信直连使用平台已绑定的微信身份，不传入收款账户 ID 或 OpenID。
- CLI 在提交前保存原请求；不要删除或编辑恢复文件，不要因结果未知改变原金额、账户或请求标识。

## 查询与完成

```bash
viceme withdraw status --request-id <原提现任务的稳定标识>
```

提交后如为 `PENDING`，按返回的 `nextPollAfterSeconds` 短暂等待并查询一次。仍未结束时，将单号、请求标识和 `nextCommand` 返回上层 Agent，由其继续跟踪，不创建第二笔提现。

微信要求本人确认收款时，展示 `actionUrl`，本人在微信完成官方收款确认后继续查原单。不得把此类操作伪装为可由 Agent 代替的确认。

只有 `SUCCEEDED` 表示全部提现成功。`PARTIALLY_SUCCEEDED` 必须说明成功、退回及未完成金额；`FAILED` 报告实际结果。已受理、处理中、待本人操作均不声称到账成功。

超时或响应未知时，先查询原请求；`WITHDRAWAL_NOT_FOUND` 才重试原 create 命令，且完整保留原参数。不要换新标识盲目重提，不要自动更换渠道。`WITHDRAWAL_RECIPIENT_CHANGED` 表示原单未创建而收款身份已改变，停止原任务；明确的新任务才可使用新请求标识。平台明确拒绝、账户不可用或余额不足时返回准确原因，由上层决定新任务。

## 交付

用用户当前语言说明结果、实际提现金额、脱敏收款账户、剩余可提现收入和单号。未结束时给出后续查询方式；需要本人操作时给出入口。禁止展示 token、完整收款账号、OpenID、签名收款 package 或本地恢复文件内容。
