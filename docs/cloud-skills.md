# Skill 混合交付与本地执行

本分支先记录协议，再实现安装与运行。Shop 同名分支 `feat(repo)/hybrid-skill-delivery` 的 `docs/cloud-skill-delivery.md` 定义服务端发布、权益与任务协议。

## 发布

`SOURCE` 按现有流程交付完整 Skill。作者选择 `CLOUD` 时，在包根声明 `viceme-cloud.json`：

```json
{
  "version": 1,
  "purpose": "校验输入并按核心规则生成报告",
  "localWorkflow": "WORKFLOW.md",
  "privateFiles": ["SKILL.md", "references/core.md"],
  "publicFiles": ["WORKFLOW.md", "scripts/validate.py", "scripts/report.py"]
}
```

`localWorkflow` 必须为公开 Markdown，描述固定步骤与脚本输入输出。每个文件明确归属，原始 `SKILL.md` 必须私有；服务端替换为公共调用入口。公开文件保持路径、字节和执行权限。CLI 在发布准备时校验声明并传入共享 manifest，服务端重新验证完整包。不能把私有脚本当作远程可执行程序。

手动选择 CLOUD 封面或展示图时，使用 `publication asset upload <id> --role cover --path ./assets/cover.png --relative-path assets/cover.png`。包内路径必须属于 `publicFiles`，服务端重新核对该路径的原始字节；不允许上传额外私有图或仅凭同名文件猜测路径。失败后以相同路径重试会复用上传槽。

沿用现有发布的暂存、核验、激活流程，不另造自动激活入口。暂存成功不代表消费者可以使用该版本。CLOUD 本轮只用于付费版本，可无试用；不要求必须开启买断，可以仅由有效订阅授权。

## 安装

从 API 下载经过摘要校验的公共包，包含生成入口、作者公开流程、脚本、模板和资源。安装 runtime manifest 使用 `schemaVersion: 2`、`deliveryMode: CLOUD` 和发布 ID。

试用、无试用购买入口和购买后恢复都必须安装同一个完整公共包，不能仅合成 `SKILL.md` 而丢失附件。包与元数据的产品、市场、API、交付方式、发布 ID 不一致时停止。CLOUD 不使用 SOURCE 的试用正文覆盖/源码解锁逻辑，购买不会下载私有源文。

Go CLI 与随 Skill 安装的独立 Python runtime 使用同一安装身份和状态锁。无试用入口创建直接购买身份，不请求 trial grant。身份文件使用私有权限和原子替换；已存在的不匹配身份不能被覆盖。

## 一次任务

1. Agent 读取本地公开流程，确定任务范围与所需本地工具。用户原始文件和脚本输出保留在本地。
2. 写 `task.json`，包含新的 UUID `requestKey`、任务范围 `prompt` 和可选字符串值 `facts`；只提供必要的范围事实，不自动上传业务文件、令牌或整段历史对话。
3. 在 Skill 目录运行 `viceme skill cloud --input task.json --wait 60s`，或 `python3 .viceme/scripts/trial.py cloud --input task.json`。运行时补充安装的产品、发布和 API 标识。
4. `ready` 时，读取结果 `execution.md`，按公开流程和本次核心指导在本地调用原脚本、判断业务数据、构建并验证产物。服务端不会代做分析或生成最终报告。
5. `needs_input` 只补充选择规则所需的范围事实，以返回的 `sessionId` 和新 `requestKey` 继续。`refused` 或错误要停止并展示；不能自行猜测缺失规则。
6. 未完成或中断时保留原文件，重复同一 `requestKey` 恢复。不得修改已接受任务的输入或身份。需要付款时沿用现有付款展示/恢复流程，付款后恢复阻塞任务。

安装公开包不等于拥有使用权，每个新任务由服务端重新核验。试用仅成功生成 `ready` 时消耗一次，其他状态及恢复不重复扣次。不限期订阅沿用服务端真实的空截止时间语义。

## 本地状态与恢复

请求、远端标识和指导保存在私有 `.viceme/cloud/<API 与产品的摘要>/` 目录；凭据不写入请求或结果。调用固定安装发布，版本过期提示重新安装，不静默混用新规则和旧脚本。任务在受理时绑定匿名或账号主体，登录/付款后恢复不能接管别人的请求。

Go 与 Python 的结果保持相同机器可读字段，stdout 只输出一个 JSON envelope。脚本错误不输出 traceback、secret 或签名 URL。重试和等待有时间上限，后台模型调用不依赖客户端连接存活。

## 验证与发布边界

本地真实进程测试使用隔离 HOME、CLI 配置、Skill 路径与缓存。覆盖公开附件保真、无试用购买身份、两种 runtime 的安装与恢复、版本冲突、输入不可变、失败/重试及 SOURCE 回归。服务端使用隔离基础设施验证权益和并发；真实模型指导由本地原脚本完成产物构建。

完成 `make check`、规定的 race 与 npm 门禁后，由独立子代理整体评审并修复，再 PR 到 dev。生成的内嵌 runtime ZIP 与 release manifest 通过仓库脚本重建，不手工修补摘要。不发布 npm、正式安装器或生产版本；本地成功不代表官方渠道已分发此能力。

Python 转交账号任务前，通过 CLI `version` 的 `protocols.skillCloud: 1` 协商能力。旧 CLI 不接收任务；独立匿名身份继续由 Python 执行，账号任务提示更新 CLI 后原键重试。

## 运营渠道包

官方 Python runtime 的 `export-package --delivery-mode CLOUD --kind trial|purchase --input public.zip` 是离线打包入口。输入必须为发布生成的公开安装包，根 `viceme-cloud.json` 的协议、交付模式与 `--release-id` 必须匹配，作者私有声明不能作为公开包输入。公开文件保持原始字节和可执行位，仅附加 runtime、文件摘要和无凭据安装标识；不注入 SOURCE 试用正文或计次门禁。试用包首次任务通过服务端建立本机 grant，无试用包建立独立购买身份，完成授权后提交云端任务。原 `SOURCE` 导出参数与行为保留。
