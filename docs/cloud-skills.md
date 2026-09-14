# 云端保护 Skill

`SOURCE` 下载完整作者包，保留现有试用软门禁。`CLOUD` 仅安装服务器生成的短入口及明确公开的文件；新任务均由服务端验证权益和生成本次执行指令。购买不会下载作者私有源码，也不会把新任务转为离线执行。当前 CLOUD 必须为付费 edition，可开放匿名试用。

作者在源包根创建 `viceme-cloud.json`：

```json
{
  "version": 1,
  "purpose": "根据用户提供的事实生成活动海报文案",
  "privateFiles": ["SKILL.md", "references/writing-rules.md"],
  "publicFiles": ["assets/cover.png", "templates/poster.html"]
}
```

清单穷尽全部源文件，控制文件自身不用列入。`SKILL.md` 必须私有；私有文件只允许 UTF-8 `.md` / `.txt`，每个最多 16000 字符。公开范围不能重复或占用 `.viceme` 运行资源。图片只在显式公开后作为展示候选上传。CLOUD 的 `spec.cloud` 与源 ZIP 声明一致，源码及清单都进入不可变包摘要。

```sh
viceme skill publish ./poster-skill --price-minor 990 --trial-use-limit 3
```

安装沿用 `viceme skill install <商品UUID>` 或作品页 Python 安装口令。`deliveryMode` 缺失时兼容 SOURCE；CLOUD 的 `.viceme/runtime.json` 使用 `schemaVersion:2`、`deliveryMode:CLOUD`，旧运行器会要求修复安装。新运行器不注入旧 trial gate，也不在耗尽后覆盖短入口。

把用户确认的任务存为 `task.json`，保留此文件：

```json
{
  "productId": "11111111-1111-4111-8111-111111111111",
  "requestKey": "22222222-2222-4222-8222-222222222222",
  "releaseId": "33333333-3333-4333-8333-333333333333",
  "prompt": "为本周末的读书活动写一份海报文案",
  "facts": {"时间": "周六下午两点", "地点": "社区图书馆"}
}
```

```sh
viceme skill cloud --input task.json --wait 60s
python3 .viceme/scripts/trial.py cloud --input task.json --wait 60
```

包内 Python 可省略 `productId`，从已安装环境取得商品、API 和市场；Go 也可用 `--product <商品UUID>`。两条路线都可从已安装 runtime 补齐 `releaseId`，持久任务始终保留原 releaseId，不从最新 access 自动升级版本。若本地公开文件已换成另一版本，会返回 SKILL_CLOUD_RELEASE_MISMATCH，须先恢复原版本安装；不会生成新的付费任务。`--help` 显示参数。`--wait 0` 仅提交并返回当前状态，最长有界等待为 600 秒。输入支持最多 8000 字符 prompt、30 项 facts，每项键最多 80、值最多 4000 字符。

请求发送前，输入和身份存入私有的 `~/.viceme/cloud/<API与商品摘要>/`。Go 与 Python 共用格式和本机试用凭证；记录不包含账号令牌或试用 secret。网络失败、响应丢失、进程重启、服务端可重试失败和等待结束后，都用同一文件与 UUID requestKey 重试。改输入必须换键；同一键不得换账号或试用安装身份。账号令牌刷新不改变 userId 绑定。返回的 inputPath 可用于之后重试。

返回 `task.outcome=ready` 且 `allowed=true` 时，执行当前任务的 `executionPath`。只有成功 ready 扣一次试用；重复读取同一结果不多扣。该结果不能用于另一个新任务。`needs_input` 只返回缺失信息，补充后保留 sessionId 并新建 requestKey。`refused` 解释 message 并停止。非 ready 与失败不生成执行文件，也不扣次。

额度耗尽会复用现有付款流程。匿名 `trial.py purchase --wait 60` / `viceme skill trial-purchase <商品UUID> --wait 60s` 付款后直接恢复原 blocked task；注册账号付款后重跑原 cloud 命令。匿名已购继续使用同一匿名凭证，之后登录其他账号不会改变原任务身份。Python 遇到账号任务时调用随包附带的官方 CLI 定位脚本，复用现有 CLI/ npm launcher 的登录，不读取或复制凭据。付款后返回的 owned 只描述权益；每个新任务仍需服务端授权。

Python 新任务在本机存在 CLI 时，由 CLI 查询当前账号权利，避免旧匿名试用记录掩盖后来购买的账号权益；无 CLI 的匿名试用可继续直接请求。已有任务保持原身份。付款后的 `resumedTasks` 逐项报告结果；旧版本或过期任务会保留原记录并返回 `allowed:false` 和 `error`，其他待办继续恢复。

验证包括实际 HTTP 响应丢失、同键输入冲突、Go 与已安装 Python 独立进程交叉恢复、非 ready 输出边界、匿名支付后不下载包及短入口保留。服务端计次、并发与持久化约束由 Shop 的真实数据库测试负责。
