# Changelog

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

### Security

- bind process publication credentials to a versioned audience and canonical API origin
- restore explicit local Profile publication credentials for controlled internal operation

## [0.5.0] - 2026-07-21

### Features

- pass explicit source root (`e4b4a78`)
- job metadata 新增 --author 透传来源作者修改 (`bab2399`)
- job resume 新增 --expected-public-summary-digest 绑定摘要回执 (`1cf648c`)
- job accept 强制 --inputs-digest 绑定试跑输入集(PRE-04) (`841b4eb`)
- add explicit compiler retry command (`efcaa0e`)
- Host typed-action 闭环(job preview/edit/run/accept + META) (`ec6479a`)
- job metadata 支持信息确认检查点(产品 3098) (`56cf083`)
- add staff-authorized Skill publication credentials (`91fee90`)
- job resume 支持 confirm_publish 精确候选确认 (`5707afa`)

### Fixes

- narrow staff-authorized publication client boundary (`3a3647c`)
- make staff-authorized publishing recovery-safe (`a15b6bd`)

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
