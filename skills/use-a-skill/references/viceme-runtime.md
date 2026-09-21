# 核心保护 Skill 的调用、门禁与恢复

本文仅用于 `deliveryMode=PROTECTED`。本地 SKILL.md 是公开入口，作者私有正文保留在服务端。原附件和脚本在本地执行；不要运行 SOURCE 的 use、读取 trial-body.md，或在购买后下载、解锁私有原文。

从解压包直接开始时先运行 `python3 .viceme/scripts/trial.py ready`（Windows 用 `py`），读取安装和购买恢复动作；不要为了判断购买而虚构指导任务。

安装完成已有用户任务时继续原任务；没有任务且没有立即购买要求时，根据公开名称、能力简介生成 3 个示例任务，使用现有上手展示指引。只展示可以提出的需求，不编造执行结果，不调用指导接口、不扣试用。用户选定或提出任务后才开始授权调用。

## 提交完整任务

把当前完整任务写入 task.json：`requestKey` 为新 UUID，`prompt` 包含全部有效要求，`facts` 只放选择规则所必需的字符串事实。用户业务文件、明细和脚本输出留在本地，不自动上传，不包含凭证。修改语言、补充信息或纠正事实时，由用户 Agent 整理当前完整需求并使用新键。服务端不管理对话历史，不接受 sessionId，也不会补回旧事实。

`facts` 可省略，完整需求可以全部放在 `prompt`。不必按字段拆分来规避长度限制；服务端会将系统指令、私有 Skill 和本次任务合计计数，最多使用 838,860 tokens（1M 输入上限的 80%）。超限不生成、不扣试用，由本地 Agent 精简完整任务后用新键提交。输入文件与 HTTP 提交另有 16 MiB 传输上限。

在 Skill 目录运行 `python3 .viceme/scripts/trial.py guidance --input task.json --wait 60`（Windows 使用 `py`）；CLI 路线为 `viceme skill guidance --input task.json --wait 60s`。安装的 runtime 提供商品、市场、API 与 releaseId；请求版本必须与本地原附件相符，不能自行改成新版本。

## 按返回状态操作

- `ready` 且 `allowed=true`：完整读取 executionPath，遵循前置条件、规则、脚本参数和产物校验，在本地完成这一次任务。成功持久化 ready 才扣一次试用。
- `needs_input`：取得必要澄清，把原需求与补充一起整理成完整输入，用新 requestKey 提交；本次不扣试用。
- `refused`：解释返回的原因并停止；不扣试用。
- 需要购买：读取同目录 [购买与恢复](purchase.md)，按本次 runtime 返回的支付展示指引操作。先提供支付入口，再有界等待；服务端确认支付和权益后继续原任务。已购买或有效订阅不重复购买。
- 输入过长或模型明确报告上下文超限：本地 Agent 整理精简完整需求，使用新 requestKey 重提，不重试同一超长输入。服务端不摘要、压缩或静默截断。
- 超时、网络错误、等待结束或付款恢复：保留原输入、凭据和 requestKey，按 RETRY_SAME_TASK/retryable 恢复。其他不可重试错误停止并报告，不擅自换键循环调用。

本地提示词只说明操作流程；允许使用、试用额度和购买/订阅权益由服务端代码在调用模型之前校验。不要根据本地安装状态、用户自述或支付页面判断已授权，不把支付判断交给云端模型。

安装或 ready 返回 PURCHASE_REQUIRED / CHECK_PURCHASE_ACCESS 时，先按本地 .viceme/scripts/trial.py 运行 `purchase --wait 0`（CLI 为 `viceme skill trial-purchase <productId> --skill-dir <当前Skill目录> --wait 0`），校验并恢复本机已有购买；服务端实际返回待付款才按 purchase.md 展示支付。无需虚构任务、无需先调用指导、无模型费用。支付确认且没有待恢复任务时，按公开简介生成三个任务示例；有原任务则使用原输入恢复。`kind=purchase` 仅说明安装入口类型，不是当前未购买的证据；账号已购沿用 CLI 的账号权益。
