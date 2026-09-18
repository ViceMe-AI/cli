# 渠道交付：固定分支与 PR

父级 `SKILL.md` 的权限和用户表达规则适用于本文件全部步骤。渠道交付把已发布版本的
试用门禁内容交到作者自己的 Skill 目录、固定 GitHub 分支和 PR；它发生在公开发布成功
之后，不改变任何已发布事实。权威方案见 Shop 仓库
`docs/sell-a-skill-channel-pr-delivery.md`。

## 交付什么

- `viceme publication deliver <publication-id> --skill-dir <绝对路径>` 以服务端保存的
  本次发布准确 Release 为源（下载后校验 artifactDigest），用官方门禁生成器重建试用
  渠道内容，原位应用到原 Skill 路径，并在旁边生成渠道 ZIP（默认
  `<skill-dir>-viceme-trial-channel.zip`，可用 `--zip` 指定，必须在 Skill 目录之外）。
- 本期交付对象是开启试用的付费 Skill 的试用渠道包。免费条目和不开试用的付费条目
  不做渠道交付；不得为了统一形式劝作者加试用。
- 目录与 ZIP 来自同一份构建结果；交付幂等，无变化重跑不会重写文件。
- 命令返回 `branch`（固定分支名，来自作品 slug，展示名、价格、Release 和生成器版本
  变化都不改分支名）、`commitScope`（该目录当前受本命令管理的完整路径清单——不是
  本轮写入差异；响应丢失后重跑同样给出全量清单）、`zip`（路径与 digest）、
  `trialBodyEditPosition`（固定为 `.viceme/trial-body.md`）。
- 目录里的作者业务文件必须与目标 Release 一致：本地改过或新增了未发布内容、或缺少
  已发布文件时，交付报 `SKILL_CHANNEL_LOCAL_CONFLICT` 并在 `conflicts` 里逐条列明。
  此时目录、命令 ZIP 与作者自打包的 PR 分支会分叉——先把改动走发布
  （`viceme skill publish --path <目录>` 自动还原原始包），再重新交付；或明确丢弃
  本地改动后重跑。
- PR 失败不撤销已发布版本；重试交付不重复扣费、不重置试用次数、不创建新商品。

## 作者编辑位置（必须写进 PR 说明）

- 业务正文只在 `.viceme/trial-body.md` 编辑；脚本、图片、参考资料仍在原路径编辑。
- `SKILL.md` 门禁入口和 `.viceme/` 门禁运行文件由官方生成器维护；不得手写或让
  Agent 编造另一套计次、支付逻辑。手改过的生成文件在下次交付时报
  `SKILL_CHANNEL_LOCAL_CONFLICT` 并被原样保留——先发布改动再交付，或明确丢弃
  本地改动后重跑。

## GitHub 编排（本地 Git/GitHub 工具）

GitHub 写入只在用户明确同意交付到具体仓库后进行；仓库可读不等于可写，不自动替
作者创建或公开仓库。无目标仓库时交付到本地目录和 ZIP 即完成。

1. **分支身份**：使用交付结果返回的 `branch`。首次交付从用户指定或当前上下文明确
   的来源提交创建该分支；同名分支已存在时先核对仓库、Skill 相对路径、
   `.viceme/skill.json` 绑定的 listingId 与市场环境，不能直接重置或覆盖。后续交付
   始终基于远端固定分支当前提交，不从默认分支重新复制。
2. **提交范围**：只提交 `commitScope` 列出的路径与必要的说明文件；`commitScope` 是
   受管路径全集（含仍在等待提交的删除——Git 已无该文件时也按删除提交），本轮是否
   新写入或删除要看 `written`/`unchanged`/`removed`。是否创建提交由 Git 实际差异
   决定：`git status` 在这些路径上有未提交变化（含删除）就提交，没有任何差异（包括
   重跑全量 `unchanged` 且 Git 干净）才不创建提交或 PR。不顺手改其他 Skill，不重排
   仓库，不提交渠道 ZIP 和任何 `.viceme` 之外的临时产物。作者本地未提交、未推送的
   内容先确认归属并隔离，不混入自动提交。
3. **推送**：推送前核对远端分支是否前移；有作者新提交时先拉取合并并检查冲突，
   绝不 force push。分支被删除时按交付记录与 GitHub 当前状态恢复可证明的提交，
   无法确定就明确报告缺失信息，不用旧本地快照硬重建。
4. **PR**：创建前按仓库、head=固定分支、base=来源分支查重；同一交付已有开放 PR
   就继续用它。PR 已合并后新版本仍沿用固定分支、再创建下一个 PR。作者关闭未合并
   PR 不等于同意重新打开；有新的明确交付请求时再创建。PR 说明必须包含：来源
   publicationId/releaseId、作品链接（`product.detailUrl`）、修改文件清单、包类型
   （试用渠道包）、作者编辑位置（`.viceme/trial-body.md`）、打包方法（压缩该 Skill
   目录内容，ZIP 根含 `SKILL.md`，不是整个仓库）、验证结果。没有实际验证过的步骤
   不得写成已完成。
5. **合并不是使用前置条件**：作者可以不合并，长期在固定分支继续开发。收尾时如实
   区分「已发布、PR 待恢复」「已完整交付」两种状态。

## 从渠道目录更新（再次发布）

固定分支上的 Skill 目录已被门禁包装。更新流程：

1. `viceme skill publish --path <该目录> ...edition flags...`——CLI 在构建区自动
   还原原始包（`.viceme/trial-body.md` 恢复完整 `SKILL.md`，剔除 runtime.json 记录
   的生成文件，未知作者文件保留），工作区不被修改。直接压缩目录或发布渠道 ZIP
   会被拒绝（`SKILL_CHANNEL_ARCHIVE_NOT_AUTHORABLE`）。
2. 走正常预览、确认、发布主线，同一商品产生新 Release。
3. 重新运行 `viceme publication deliver <新 publication-id> --skill-dir <同一目录>`，
   用准确新版本重建渠道内容（同一商品的新发布会继承上一版交付基线，不会把旧生成
   文件误报为冲突），然后按上文提交、推送、更新 PR。
4. 无变化重跑：交付返回全量 `unchanged` 且 Git 无差异时不创建新提交或新 PR。

## 错误分支

- `SKILL_CHANNEL_LOCAL_CONFLICT`：向用户报告冲突文件清单（白话：哪些生成文件被手改
  过、哪些业务文件与已发布版本不一致），给出两条路：先发布改动再交付，或确认丢弃
  这些改动后重跑。不得静默覆盖。
- `SKILL_CHANNEL_DELIVERY_IN_PROGRESS`：另一条交付正持有该目录的锁。等待其结束后
  重跑同一命令；被挡下的那次不会改动目录或 ZIP。
- `SKILL_CHANNEL_BINDING_MISMATCH` / `SKILL_CHANNEL_DIR_OWNED_BY_OTHER_PRODUCT`：
  目录绑定与本次发布不一致。向用户核实正确目录；不得重置未知渠道目录。
- `SKILL_CHANNEL_PUBLICATION_NOT_PUBLISHED`：先完成发布再交付。
- `SKILL_CHANNEL_ARTIFACT_MISMATCH`：导出字节与发布摘要不符，属服务端或传输异常，
  如实报告，不得换“最新版本”绕过。
- 交付响应丢失：直接重跑同一命令；它是幂等的，服务端以 publicationId 锁定准确
  版本，不会重复副作用。

## 安全边界

- 不打印、不提交签名下载 URL 或任何凭证；渠道 ZIP 和 PR 只含公开商品与版本标识。
- 消费者安装状态（`.viceme/package-files.json`、`install-manifest.json`）不属于作者
  交付物，不写入 PR。
- GitHub 授权由作者本人在宿主完成；发布流程不要求也不存储 GitHub token。
