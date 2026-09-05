# 运营身份、来源发布与官方 Skill

状态：2026-09-05 文档先行，CLI 实施和本地完整检查已完成。
本轮实现提交 `053c82d`，随后以 `ef43eb0` 合入最新 dev `a081402`；合并后的
`make check` 和 `make npm-package-check` 均通过，`git diff --check` 无错误。
合并前收尾又以 `6c80c46` 合入 dev `7e9a919`，保留发布恢复及 Windows 权限修复，并从源码重新生成官方 manifest。
随后以 `5f0a831` 同步新增 dev `f3482b1` 的做同款支付流程修复，并再次从源码生成官方 manifest。
代码已推送，当前配套为 [CLI #270](https://github.com/ViceMe-AI/cli/pull/270) 和
[Shop #421](https://github.com/Leizhenpeng/ViceMe-Shop/pull/421)；最新提交完整复核及远端 CI 结果记录在 PR。
本轮只处理合并前交付，数据库部署、官方初始化、CLI release/npm 和真实供应商联调在合并后进行。
延续已合入 dev 的作者代注册 PR #200，Shop 权威设计见
[创作者代注册、官方身份与运营邮箱](https://github.com/Leizhenpeng/ViceMe-Shop/blob/feat%28repo%29/creator-provisioning/docs/creator-provisioning-and-handover.md)。

## 身份与授权

- 运营邮箱在 Admin 完成验证并绑定到唯一目标 User，CLI 复用正常平台登录/授权。
  一个 User 可以有多个运营邮箱；不增加身份选择器、运营 User 或商家成员角色。
- 某邮箱被废弃后，其 CLI Token 和在途授权都失效；另一运营邮箱的授权继续有效。
  作者微信接手则废弃全部临时运营授权，作者重新授权 CLI，User/作品/收入不迁移。
- 官方 User 永久归平台，无个人微信绑定、接手、个人渠道绑定或提现引导。
  待接手作者可以正常发布和销售，但不能授权 GitHub 私有源或办理结算/提现。
- 上述规则由 Shop 后端判定；CLI 正确呈现业务错误，不以隐藏提示代替权限检查。

## 发布来源

| 来源 | CLI 行为 |
| --- | --- |
| 本地目录 / ZIP | 保留预检、归档和发布 |
| GitHub 公开仓库 | 直接读取，无 OAuth 或仓库所有者校验 |
| GitHub 私有仓库 | 按需打开当前 User 的本人 OAuth，校验其拥有的个人仓库 |
| 小红书公开 Skill | 直接读取，无授权或渠道审核 |

删除渠道账号/认证概念、`merchant channel` 命令及官方 Skill 中先绑渠道的步骤。
普通创作者申请审核继续保留；公开资料不是认证后的账号。私有 GitHub 不扩大到协作者
或组织仓库；授权只保存在 Shop 当前 User 下，CLI 不保存或打印 GitHub token。
有效授权复用，多次发布本人私有仓库不重复要求授权；撤销/过期/权限不足时按需重新授权。
来源 repo/path/不可变 commit、Skill/version、digest/size 和已有更新映射继续保留。

## 官方作品与包

- 官方 `viceme` 是普通业务结构中的真实 User / Creator / Merchant / Work 所有者。
  CLI 可以列出/管理官方 Work，页面介绍、封面和布局复用既有页面发布协议。
- 官方作品使用正常 `/viceme/{workSlug}`、`kind=SKILL`，不增加 GUIDE 类型或 `/guides`。
  页面默认模板与正常自定义发布共存，下架/停用在 HTML、Markdown、CLI 都生效。
- CLI 仓库是官方 Skill 源码、版本和发布包的唯一权威。Shop 保存官方安装引用，
  不把内置官方工具再次上传成普通 Shop S3 商品，不维护第二份包。
- 普通创作者 Skill 仍由 Shop S3 托管并校验下载权益。
- 官方显式安装/刷新使用既有 `viceme install --agent <target>` 流程；CLI 自动更新与
  Skill 生命周期分离，后台更新继续 `RefreshSkills: false`。
- 官方销售/打赏/订阅若启用，收入由 Shop 记入平台自营账，不计入官方 User 作者钱包。

## 实施与验证

- [x] 清理命令、客户端契约、错误/nextAction 与内置 Skills 的渠道认证流程。
- [x] 公开 GitHub / 小红书直接发布，私有 GitHub 按需 OAuth、复用及失效恢复。
- [x] 运营凭据废弃/作者接手后正常报告需要重新登录，不把旧授权转换为新邮箱绑定。
- [x] 官方 Work 能按正常接口管理，安装入口指向 CLI 官方发布源；普通下载无回归。
- [x] 官方身份无微信接手、个人来源绑定、提现提示；保留普通作者现有引导。
- [x] 重新生成 `quality/release-manifest.json`，不手改版本/摘要或发布制品。
- [x] 相关真实 HTTP/CLI 集成、`make check`、`make npm-package-check`。

完整检查包含 Go 测试和 vet、官方 Skill 内容/manifest 检查、51 项 npm 测试和打包预检。
`merchant_onboarding_test.go` 覆盖新 `source github` 授权流程的等待、拒绝、超时与取消；
`source_publication_test.go` 覆盖公开源不触发 OAuth、私有源授权后继续、失效重新授权以及
重试次数上限；`publication_vnext_test.go` 覆盖来源发布；`skill_use_test.go` 覆盖官方安装引用、下架拒绝和普通下载。
Shop 侧真实数据库验证运营授权撤销、来源授权复用和私有仓库所有者约束，详见
[Shop 本轮验收记录](https://github.com/Leizhenpeng/ViceMe-Shop/blob/feat%28repo%29/creator-provisioning/docs/creator-operations-verification.md)。

测试仅使用独立临时配置、输出目录及 Shop 测试服务，不触碰用户实际 CLI 登录信息或
已安装 Agent Skills，不执行真实 OAuth、支付、发布 release 或 npm publish。
