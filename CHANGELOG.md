# Changelog

## [0.24.1] - 2026-09-02

### Fixes

- 修复工具购买鉴权并持久恢复同一订单 (`1e07d2a`)
- 隐藏网站收费配置细节 (`6a146a2`)

### Other Changes

- 刷新官方 Skill 清单 (`87ed5f6`)
- 说明付费解锁来源策略 (`dd838be`)

## [0.24.0] - 2026-09-02

### Features

- 添加网站做同款官方技能 (`d0214a8`)
- 实现复刻源码购买安装 (`bb8348a`)
- 添加复刻源码安全原子安装器 (`ab783dd`)
- 实现网站源码复刻发布 (`017d439`)
- 添加网站复刻登录权限 (`babd2bf`)

### Fixes

- 明确发布条目选择与免费改价影响 (`048f616`)
- 加固复刻发布与确认边界 (`4ace44f`)
- 加固复刻安装与支付恢复 (`9935180`)
- 锁定复刻交易响应与恢复状态 (`1ec51ee`)
- 收紧复刻响应关系校验 (`36e35aa`)
- 严格校验复刻交易响应 (`1df96cb`)
- 加固复刻源码安装恢复 (`1d99cf2`)
- 对齐复刻包摘要格式 (`c40641c`)

### Other Changes

- 覆盖复刻安装崩溃恢复边界 (`ca95dea`)

## [0.23.1] - 2026-09-02

### Fixes

- 保留可重试的凭据清理状态 (`23216f3`)
- 移除互动接入 DNS 验证 (`1fb8bb3`)

### Other Changes

- 修正工作流 shellcheck 告警 (`71113ab`)
- 避免发布准备重复运行全量检查 (`89c1b9e`)

## [0.23.0] - 2026-09-01

### Features

- 买家侧交易闭环：install 未购版直接下单扫码支付并新增 subscribe (`0efec34`)

### Fixes

- 发布终态后本地恢复清理失败降级为警告 (`513d7b7`)

### Other Changes

- 约束 main 拉取请求来源 (`bf8e79f`)
- viceme-skill-use 购买引导改为扫码支付闭环 (`2c6cf5c`)
- 钉死 sell-a-skill 预览、改名与来源判定合同 (`374cb95`)

## [0.22.2] - 2026-09-01

### Fixes

- 动态解析互动 SDK 版本 (`8f316dc`)
- 按 Agent 平台选择交互工具 (`4685902`)
- 优先使用交互式接入引导 (`ce7b716`)
- 跟随 SDK 最新版本 (`ae5fbbf`)
- 对齐 SDK 内嵌网站支付 (`8d18a24`)

### Other Changes

- 明确移动支付失败边界 (`5e8b9c1`)

## [0.22.1] - 2026-09-01

### Fixes

- 同步互动 Skill 章节边界与发布清单 (`111eecc`)
- 移除旧版互动接入迁移流程 (`123e4f4`)

## [0.22.0] - 2026-09-01

### Features

- 入驻补材料支持文字说明并锁定单轮单次提交 (`a51e096`)

### Fixes

- 申请成功后主动打开申请人主页 (`4a5f3e0`)
- 沙箱内自动更新优雅放行并降级 Skill 安装事务 (`d964787`)

### Other Changes

- v0.21.1 (#173) (`5665070`)

## [0.21.1] - 2026-09-01

### Fixes

- 沙箱拒 rename 时降级直写并预检完整保存链路 (#172) (`50067b8`)
- 对齐开放赞赏最终入口 (#171) (`0724d9b`)

### Other Changes

- v0.21.0 (#170) (`027f9dd`)

## [0.21.0] - 2026-08-31

### Features

- 创作者申请最少提问与身份路由展示 (#168) (`845846a`)

### Other Changes

- 统一创作者 Skill 英文名称与资格复用 (#169) (`196d4aa`)
- 统一网站互动 Skill (#160) (`7a7c8e6`)
- v0.20.0 (#165) (`36caa44`)

## [0.20.0] - 2026-08-31

### Features

- 统一网站关注与付费发布流程 (`77ee070`)

### Fixes

- 下载 Skill 安装守卫与已下架官方 Skill 退役身份 (#161) (`63f498d`)
- 解决 CLI 发布分支冲突 (#166) (`e7ff4d1`)

### Other Changes

- v0.19.0 (#159) (`17725ab`)

## [0.19.0] - 2026-08-31

### Features

- 实现玩法一发布与购买闭环 (#147) (`7b07940`)
- 适配新版网站关注与付费接入 (`b3d0cd0`)
- 迁移至 Work 互动资源图 (#152) (`f04807e`)
- 适配商品蓝图购买 Skill (#140) (`51fe94c`)
- 恢复创作者访问接入 (`39f0e4a`)

### Fixes

- 修复付费访问配置保留 (`fcdb9c6`)
- 接受网站应用商品绑定 (`552ce61`)
- 更新无语言前缀的授权地址 (`b8d0aad`)

### Other Changes

- 收敛网站访问配置入口 (`7340f57`)

## [0.18.0] - 2026-08-27

### Features

- 恢复创作者访问接入 (`2c89405`)

## [0.17.0] - 2026-08-25

### Features

- 支持公开互动能力自助接入 (#144) (`21ff78b`)
- 支持网站功能独立定价 (`14cf4ae`)
- 上传网站发布封面 (`af88653`)
- 完善网站发布展示信息 (`16f8926`)
- 统一 Skill 与网站发布流程 (`9ec8ca3`)
- 支持网站发布补齐创作者身份 (`602bd70`)
- 支持外部网站发布与作品售卖 (`d2f694b`)
- 限制创作者接入 Work (`3958862`)
- 自动选择唯一创作者作品 (`b6caeac`)
- 一步完成创作者接入 (`832aa4d`)
- 移除接入 Origin 配置 (`0f12281`)
- 提供创作者网页接入命令 (`e3a0e75`)

### Fixes

- 更新创作者访问发布摘要 (`9d42715`)
- 明确发布先于网站能力接入 (`fdf8b61`)
- 接入初始化时引导完成网站发布 (`4820b26`)
- 按发布源类型路由发布流程 (`c5fd6ae`)
- 允许无域名发布网站作品 (`88a15dc`)

### Other Changes

- 同步创作者授权弹层说明 (`3959f1c`)
- 同步创作者授权弹层交互 (`53a2a40`)
- 更新网站授权接入规则 (`d4a038c`)
- 支持普通用户创建 Work (`fc4d210`)
- 同步创作者访问交互 (`4be32a6`)
- 固定创作者接入交互样式 (`cde115d`)
- 更新权限交互集成流程 (`3df2a0a`)

## [0.16.1] - 2026-08-19

### Fixes

- 修复安装重试与启动信号传播 (`cae2f39`)
- unbrick npm activation recovery and anchor launcher cache to the bundled checksum manifest (`acf28b2`)

## [0.16.0] - 2026-08-19

### Features

- 自动更新 CLI 与官方 Skills (`d68d096`)

### Fixes

- 合并并发自动更新并重执行等待命令 (`f48d2f6`)

### Other Changes

- 稳定激活子进程并发测试 (`fa139f9`)
- 注册 POC CLI 手动发布工作流 (`3e265fa`)
- 默认使用 Agent 补全发布内容 (`ee5030e`)

## [0.15.2] - 2026-08-18

### Fixes

- 统一认证失败详情契约 (`934139c`)
- 覆盖官方凭据的 Profile 迁移 (`53cad42`)
- 收口发布流程升级兼容契约 (`0aa469a`)
- 以 API 地址确定运行环境 (`5ee2002`)
- 合并作品详情确认与询价 (`d358466`)
- 修复 Skill 包元数据与占位符误判 (`594c89f`)
- 无价继续媒体与分析 (`e5bcdfb`)
- 隐藏发布流程环境选择 (`997dfd6`)
- 更新安装后的发布入口 (`a78db66`)
- 固定发布环境选择优先级 (`0171135`)
- 同步 Skill 发布摘要 (`6a76f1e`)
- 修正 WorkBuddy 预览打开契约 (`d21330a`)

### Other Changes

- 同步渐进式发布流程 (`a646450`)

## [0.15.1] - 2026-08-17

### Fixes

- 先上传真实草稿并持续拉起预览 (`8cb4a09`)
- 明确执行进程版本元数据 (`8b950d0`)

## [0.15.0] - 2026-08-16

### Features

- 增加 Owner Preview 展示意图 (`0d5225f`)

### Fixes

- 修复 Agent 安装文档 UTF-8 响应头 (`ad1f9d5`)
- 登录流程始终等待浏览器授权 (`3738239`)

### Other Changes

- 统一浏览器登录措辞 (`87e432b`)

## [0.14.2] - 2026-08-15

### Fixes

- 修复发布契约恢复流程 (`1d07412`)

## [0.14.1] - 2026-08-15

### Fixes

- 对齐 CDN 发布环境 (`914dbc5`)

### Other Changes

- 兼容 Windows 工作流换行 (`1b57a43`)

## [0.14.0] - 2026-08-15

### Features

- 发布签名 Agent 安装协议 (`14939ae`)
- 实现稳定 Skill 来源身份与恢复 (`28957ab`)

### Fixes

- 修复发布工作流代理校验 (`37534e6`)
- 保持旧发布合同不可变恢复 (`aaaef54`)

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
