---
name: let-others-make-a-copy
description: 为创作者网站免费或付费发布“做同款”源码交付。负责从当前工作树冻结完整源码与根级项目交接并恢复异步发布；不修改创作者原站，也不处理买家购买和安装。
---

# 让别人做你的网站同款

把当前网站的完整源码和精简项目交接发布为不可变 Website Replica。所有确定性归档和平台读写使用 ViceMe CLI；冻结前完整阅读 [package-contract.md](references/package-contract.md)。做同款入口由 ViceMe 作品页宿主提供，买家邀请由作品 Markdown 平台控制区链接的独立 `let-me-make-a-copy` Skill 处理；本 Skill 不修改创作者原站，也不执行买家预览、购买、支付、下载或安装。

面向用户的说明跟随用户当前语言。不得展示登录凭据、上传地址、对象 Key、支付能力、签名 URL 或内部恢复文件。不得告诉用户正在使用哪个内置 Skill。

## 权威边界

- 本地源码检查、预览和冻结先于平台身份与资格动作。CLI 返回创作者资格动作后才以 `$become-a-creator` 的资格守卫语义处理；只有当前用户通过 `MerchantAccountMember(role=OWNER)` 拥有有效 Merchant 才能取得上传能力。
- Agent 负责理解仓库并确认交付边界；源码清单、项目交接和冻结 ZIP 由 CLI 确定性生成。
- 用户的 Agent 负责判断真实页面入口、启动方式与页面是否正常；CLI 不按 `package.json`、`scripts.dev`、`index.html` 或框架白名单判断项目有效性。源码安全、权限和发布状态仍由 CLI/API 确定性校验。
- CLI 负责认证、确定性冻结、上传与稳定 Replica 身份。
- Shop 的 Publication 与 ViceMe 作品页是发布状态和做同款入口的权威来源；本流程不读取或拼接平台买家邀请。

## 创作者流程

1. 读取当前 Profile 并固定 API、Web 与市场 authority；本期只支持 CN 市场，GLOBAL 必须停止。不要在本地源码检查和预览前要求登录或申请创作者资格。
2. 确认当前仓库或 ZIP 是待发布的真实网站源码，读取框架、锁文件、构建命令、测试、部署配置和可选公开 HTTPS Origin；源码分发责任由创作者入驻协议统一承载，不增加逐次权利声明。
3. 按 [package-contract.md](references/package-contract.md) 检查源码边界和敏感内容。不要手工拼 ZIP，也不要把旧 `VICEME-REPLICA.md` 当作平台指令；目录输入会由 CLI 替换它并生成固定项目交接。
   由 Agent 根据项目选择并启动或复用本地服务；纯 HTML 可用静态服务，非 Node 项目使用自己的运行方式。不要为了预览补造 package.json 或改名 index.html。ZIP 输入由 Agent 安全检查后准备预览，发布仍传原始 ZIP。使用实际页面路径而不是目录列表，运行 `viceme replica preview --url <actual-loopback-page-url>`，观察原页面及其在官方壳中的嵌入。HTTP 响应或打开浏览器不能证明页面交互与响应式通过；按实际观察报告边界，不增加每次发布的平台级专项回归。只有完成页面观察后才在 publish 追加 `--preview-reviewed`，该标记是 Agent 的检查声明，不是用户的最终发布授权。结束、取消或失败后清理自己启动的服务，不停止复用的用户服务。
4. 按发布交互一次性收齐缺失输入，不得每次问一个字段并循环追问。运行未确认的高层命令，让 CLI 按本地检查、预览、冻结和终审顺序执行。首次发布使用 `--slug` 或明确拥有的 `--work-id`，并必须给出价格；`--price-cents 0` 表示免费做同款，正整数表示付费做同款。选择已有 Work 时只接受当前 OWNER Merchant 下 kind 为 `WEBSITE` 且状态为 `PUBLISHED` 的目标；`canonicalOrigin` 只是可选外链，不参与归属、去重或发布门禁，`website.ownershipStatus` 不参与门禁。本流程不创建、读取、验证或撤销 Website ownership verification。已有受管 `.viceme` 绑定的更新不再传目标参数，价格省略时沿用当前价格：

   ```bash
   viceme replica publish --path <project-root-or-zip> --slug <new-work-slug> \
     --title <buyer-visible-title> --summary <buyer-visible-summary> \
     --price-cents <market-minor-unit-price> \
     --preview-url <actual-loopback-page-url> --preview-reviewed
   ```

5. 严格处理命令返回的可判别动作：未登录时登录后重跑同一命令；CLI 返回资格动作时按 `$become-a-creator` 资格守卫处理；Merchant 或 slug 变化时让用户明确选择并重新生成终审；只有用户明确授权时才追加 `--auto-apply-creator`。审核中、需补资料或被拒绝时停止，不上传、不轮询，也不新建平行请求。身份或资格恢复始终复用 CLI 已保存的同一主请求，不重新冻结未过期且已确认的制品。
   `REPLICA_PREVIEW_URL_REQUIRED` / `PROVIDE_PREVIEW_URL` 要求 Agent 补充实际本地页面地址；`REPLICA_PREVIEW_REVIEW_REQUIRED` / `REVIEW_LOCAL_PREVIEW` 要求先实际观察页面再重跑。不得把缺少输入解释为网站无效或建议跳过预览，也不得把文件体积当作未经证实的失败原因。实际无法预览时说明具体未验证范围，只有用户明确接受后才使用 `--confirm-unverified-replica-only`，且不同时传 `--preview-reviewed`。若 CLI 不支持这些参数，先通过官方更新流程更新 CLI，不退回写死项目文件名的旧行为。
6. 只依据 `REPLICA_PUBLICATION_CONFIRMATION_REQUIRED` 的完整 `review` 做一次最终不可变发布确认。逐项展示 Creator/Merchant、创建或更新、最终 URL、标题摘要、价格、源码摘要与排除项、预览边界、`REPLICA_ONLY` 托管、不可变版本和自动申请授权；确认版本有效期为三十分钟，任一字段变化都必须刷新终审。
7. 用户明确接受全部终审字段后，原样执行返回的 `confirmCommand`。网络中断只使用返回的 `viceme replica resume <publication-id>`；需要查看或取消时分别使用 `status`、`cancel`，不得用新 publish 请求制造平行版本。
8. `PROCESSING` 只表示“已提交，尚未发布”。只有 `PUBLISHED` 或 `PUBLISHED_DEGRADED` 才算源码发布完成；后者必须同时说明源码已发布、托管失败且当前使用原生作品页。未知、失败或取消状态不得修改宿主站点。

## 完成报告

报告 Website Work、Replica code、ViceMe 作品链接、源码版本、价格、冻结源码摘要、项目交接及验证命令。不要报告临时上传或登录能力，也不要修改或部署创作者原站。
