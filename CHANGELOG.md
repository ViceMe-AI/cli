# Changelog

## [0.13.3] - 2026-08-13

### Other Changes

- 重构 CLI 快速开始与能力说明 (`77292b7`)

## [0.13.2] - 2026-08-13

### Fixes

- 规范 Agent 更新提示行为 (`c401fb3`)
- 为独立安装补充版本更新提示 (`7dc007a`)
- 合并审核与公开发布确认 (`6bba5e8`)

## [0.13.1] - 2026-08-13

### Fixes

- 在发布审核中内嵌展示图片 (`a4cf97b`)
- 修复弹幕 Skill 运行契约 (`dd3698b`)

## [0.13.0] - 2026-08-12

### Features

- 增加弹幕组件 Skill (`cf5edb0`)
- 自动等待发布分析完成 (`506a14a`)

### Fixes

- 修复 0.13 弹幕 Skill 兼容契约 (`a20ae0e`)
- 发布前识别本地恢复目录权限 (`4e3d5fb`)

## [0.12.4] - 2026-08-12

### Fixes

- 避免旧 Profile 阻断安装升级 (`05f9d3d`)

## [0.12.3] - 2026-08-12

### Fixes

- 前置发布登录并支持批量清理 Profile (`73c7e85`)

## [0.12.2] - 2026-08-12

### Fixes

- 使用 start Bucket 分发安装资源 (`b5e8a69`)

## [0.12.1] - 2026-08-12

### Other Changes

- CN S3 发布接入 HTTPS 代理 (`2202b99`)

## [0.12.0] - 2026-08-12

### Features

- 持久化自定义 API 地址 (`4c527fe`)
- 审核双语 Skill 使用说明 (`8849a7f`)
- 审核 Skill 使用说明 (`e91f5c7`)

### Fixes

- 兼容 S3 发布校验协议 (`7e9d13b`)

## [0.11.1] - 2026-08-12

### Fixes

- 修复 S3 发布产物被 checkout 清理 (`4fcc6fe`)

## [0.11.0] - 2026-08-11

### Features

- 展示并确认双语 Skill 短简介 (`f063212`)
- rebuild skill publication workflow (`64f5fd2`)

### Fixes

- 修复发版准备后的版本测试 (`a84d1c6`)
- 兼容单层目录包装的 Skill ZIP (`1e63183`)
- keep install independent of credential status (`b74f06b`)
- arbitrate outer activation journals (`01ab265`)
- fence installation generation commits (`5d4a3ad`)
- unify activation recovery state (`cf882a1`)
- fence recoverable activation generations (`7a47561`)
- make installation recovery atomic (`755ae8e`)
- make publication and install recovery durable (`308deab`)

### Other Changes

- document unified activation recovery (`cfeea2c`)
- record first-phase completion (`46ab65e`)

## [0.10.1] - 2026-07-31

### Fixes

- 阻止隐式 profile 与 endpoint 覆盖 (`602c16f`)

## [0.10.0] - 2026-07-30

### Features

- 支持过期发布动作原地续签 (`1d3269a`)
- 更新 Skill 稳定分享链接预览引导 (`f72f45d`)

### Fixes

- 对齐私有预览链接合同 (`02b1f89`)
- 从 inspect 恢复已有发布任务 (`02d165b`)

## [0.9.4] - 2026-07-29

### Fixes

- 避免规范化 Skill 二进制资源 (`c30d422`)
- 修复 Windows CI 平台差异 (`bf8c3d0`)
- 加固 Windows 配置 ACL 继承 (`ab929c1`)
- 修正 Windows 更新目录提示 (`f99bf6c`)
- 收紧 Windows 配置加载校验 (`6f42059`)
- 修复 Windows 配置加载 (`3760b07`)

## [0.9.3] - 2026-07-29

### Fixes

- 允许终态后重新发布相同来源 (`4ad71ba`)
- 登录时自动回退本地加密凭证 (`6e02a08`)
- 支持开发预览环境发布凭证 (`ee42fab`)

## [0.9.2] - 2026-07-27

### Fixes

- restore profile publication credentials (`f50c397`)
- bind T1 process credentials to API origins (`801b3ba`)

### Other Changes

- 更新 T1 权威来源追溯 (`ca9b059`)
- restore profile token rotation example (`eac514d`)
- 中文化 T1 复审交接 (`f549459`)
- add T1 review remediation handoff (`33ddc8b`)

## [0.9.1] - 2026-07-27

### Fixes

- 修复 npm OIDC 发布与恢复流程 (`1f7567b`)

### Other Changes

- 更新飞书卡片 Action 版本 (`bab230a`)

## [0.9.0] - 2026-07-27

### Features

- 新增结构化版本更新提醒 (`be3e975`)

### Fixes

- 修正 Profile 授权与编译等待指引 (`a62d199`)

## [0.8.2] - 2026-07-25

### Fixes

- accept typed source specs (`167656c`)

## [0.8.1] - 2026-07-24

### Fixes

- 修正 Skill 编辑与等待契约 (`e5b92d9`)
- 修正 ViceMe Skill 发布命令契约 (`23f140b`)
- 明确步骤确认摘要路径 (`adc70a3`)
- 六轮复审——confirm_steps 指引消除 preview 死锁 (`3b33b91`)
- T2 复审修复——自然语言走 stdin 结构化传输/超时保留 edit-run ID/resume 对齐确认端点 (`852c234`)

### Other Changes

- separate business and protocol results (`f1f9f9d`)
- 内置 Agent Skill 对齐交互步骤确认门(产品 3427) (`1992801`)
- 中文化 T2 复审交接 (`175e838`)
- add T2 review remediation handoff (`e689dff`)

## [0.8.0] - 2026-07-22

### Features

- surface channel binding actions (`18c1ca1`)

### Fixes

- validate channel binding contract (`629eaf9`)

## [0.7.0] - 2026-07-22

### Features

- skills 安装新增通用 agents target（~/.agents/skills/viceme） (`ed6be01`)

### Fixes

- align secure store contract (`9dcaf0c`)
- support credentials across macOS sandboxes (`e79ceb0`)
- standardize ViceMe display name (`dbca4fa`)

### Other Changes

- 同步 release manifest 摘要以匹配 agents target 改动 (`03c909d`)

## [0.6.0] - 2026-07-22

### Features

- persist explicit local profile overrides (`ba33f0b`)

### Other Changes

- configure profile token explicitly (`c183ec5`)

## [0.5.0] - 2026-07-21

### Features

- pass explicit source root (`e4b4a78`)
- job metadata 新增 --author 透传来源作者修改 (`bab2399`)
- job resume 新增 --expected-public-summary-digest 绑定摘要回执 (`1cf648c`)
- job accept 强制 --inputs-digest 绑定试跑输入集(PRE-04) (`841b4eb`)
- add explicit compiler retry command (`efcaa0e`)
- Host typed-action 闭环(job preview/edit/run/accept + META) (`ec6479a`)
- job metadata 支持信息确认检查点(产品 3098) (`56cf083`)
- add delegated skill publication credentials (`91fee90`)
- job resume 支持 confirm_publish 精确候选确认 (`5707afa`)

### Fixes

- narrow delegated publication client boundary (`3a3647c`)
- make delegated publishing recovery-safe (`a15b6bd`)

### Other Changes

- 重新生成命令清单与发布清单 (`b72c634`)
- 同步 T2 发布门三项强制契约的 Host 指引 (`bc8e170`)
- docs+test: cancel decision 契约测试与确认门试跑引导 (`73e8ad6`)
- 发布流程补充 T2 确认门与试跑门禁说明 (`764dbd8`)

## [0.4.0] - 2026-07-20

### Features

- add guided human login flow (`b883736`)

## [0.3.1] - 2026-07-20

### Fixes

- isolate npm cache and classify failures (`b778aa1`)

### Other Changes

- clarify workflow check names (`cf7da33`)

## [0.3.0] - 2026-07-20

### Features

- add profile management (`fec286e`)

## [0.2.1] - 2026-07-20

### Fixes

- add verified binary mirror fallback (`efb0d83`)

### Other Changes

- add Feishu pull request notifications (`b6174b1`)

## [0.2.0] - 2026-07-19

### Features

- notify Feishu after CLI releases (`1584c09`)
- simplify CLI region and output contract (`021704e`)

### Fixes

- publish through repository GitHub App (`7db30af`)
- prepare only on release intent (`a28c0b5`)
- use scoped deploy key for dev (`fcc7a0b`)
- support protected dev automation (`5f490fd`)
- make npm tests version agnostic (`01ef51f`)
- return direct CLI device authorization link (`6f125f3`)
- default CLI API to viceme.cn (`8ac5172`)
- retry npm registry reads after publish (`2c757af`)

### Other Changes

- explain direct browser device authorization (`07a1cd9`)
- clarify Agent Skills and AI quick start (`60672b2`)
- add Chinese CLI guide (`aa892e0`)
- improve CLI quick start and safety guide (`3f5e9e3`)

## [0.1.0] - 2026-07-18

### Features

- automate CLI releases (`b1c27a5`)
- publish the Viceme CLI through npm (`80a45d3`)
- add skill agent publishing CLI (`00f173c`)

### Fixes

- harden release and API transport (`c45db91`)
- satisfy release workflow shellcheck (`96e8c4a`)
- record publication admission confirmation (`ed27923`)
