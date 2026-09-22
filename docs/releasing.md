# CLI 开发与自动发布

`main` 是生产来源和新分支基线，`dev` 是集成验收分支。两种常规发布方式并存：
同仓库功能分支独立发布，以及原有 `dev → main` 整体发布。维护者不手工修改版本号
或发布清单，不手动移动标签。本规则仅适用于 CLI，SDK 保持自己的发布流程。

## 功能开发与独立发布

1. 从最新 `origin/main` 创建短期功能分支，向 `dev` 提 PR，完成检查和验收。
2. 在原功能分支向 `main` 提 PR，正文记录已验收的准确业务 SHA、测试环境或安装包
   版本、验证命令及结果。CI 检查 head 是否已进入最新拉取的 `dev`；此检查不能
   代替人工验收记录。功能分支不得包含其他未发布功能。
3. Ready 的 main PR 触发 `CLI release preparation`。Bot 基于该 PR 的准确 head，
   按 Conventional Commits 计算版本：breaking 为 major、feat 为 minor、其余为 patch。
   更新 npm/Go/官方 Skill 版本、兼容范围、CHANGELOG 与发布清单。
4. Bot 先运行 `make check` 和 `make npm-package-check`，再把单个生成提交推到
   **原功能分支**，更新现有 PR 标题，保留作者与人工验收正文。分支移动时普通 push
   失败，不强推、不覆盖开发者的新提交。
5. 更新后的 PR 运行完整质量与安装检查。其业务父提交必须在 dev 中；生成提交仅
   允许发布文件，并依据原工作流记录的日期从父提交重新生成、比较整个 Git tree。
   校验 Actions API 中的原工作流、源 SHA 与状态。不能仅凭 Bot 邮箱或 trailer 放行。
   此严格限定的生成提交不要求再人工合入 dev；任何后续业务改动仍须重新进入 dev。
6. 评审后使用 merge commit 合入 main。发布工作流从 main 的合并提交解析唯一的
   同仓库 Release PR，为已包含在 main 历史中的准确源 head 打不可变标签，保留
   六平台二进制、checksum、npm OIDC、CN/Global 安装清单及镜像发布流程。
7. npm 与双区域发布成功后发送飞书总结。独立发布显示原 PR 作者的真实 @ 和
   “单独发布”，不归属给 approve/merge 操作者。映射见 `.github/feishu-users.json`。

多个候选 PR 可以开发并行，但生产发布按顺序处理。准备前功能分支必须包含最新
main；前一个发布合入后，下一个候选需同步 main、解决版本冲突、重新进入 dev 验收
并重新准备。不得强行复用已经发布的版本。不可变标签检查会拒绝版本碰撞。
尚未准备的 main PR 只运行轻量来源检查；准备成功后的准确 head 运行完整矩阵。

## 保留 dev → main 整体发布

把已验收的 dev 向 main 提 Ready PR，仍自动准备版本，并由 Release App 将生成提交
写回 dev。原 Release PR 更新后运行完整检查，评审合并触发相同发布流水线。
手动触发版本准备仅允许在 main 工作流上操作 dev。该模式不显示“单独发布”。
生产发布后，通过 PR 将 main 同步回 dev；仅当两侧 tree 完全一致时才能只记录 ancestry，
不得用丢弃改动的合并策略处理实际差异。

## dev 集成冲突

功能分支保持基于 main，禁止点 Update branch 将 dev 合回功能分支。
如功能分支与 dev 上其他未发布功能冲突：

```bash
git fetch origin
git switch -c 'chore(repo)/integrate-example' origin/dev
git merge 'origin/feat(cli)/example'
# 在临时分支解决冲突，提交并推送，再提 PR 到 dev
```

临时集成分支只能合入 dev，禁止作为 main 发布来源。原功能分支仍用于独立发布。
验收记录写明原功能 SHA 与实际测试的 dev 版本；原分支再更新时，重新集成和验收。
与 main 冲突则将 main 同步到原分支，再经 dev 验收。

## 存量分支与恢复

从 dev 创建的存量分支必须检查相对 main 的全部差异，确认没有夹带未发布功能；
不能只根据 PR 标题判断，也不要求批量 rebase。生产 hotfix 和不可变标签恢复保留
既有边界；hotfix 不走普通自动准备，恢复不能创建新版本或冒充独立发布。

## One-time repository setup

Register a private organization-owned GitHub App named `ViceMe CLI Release Bot`.
Install it only on `ViceMe-AI/cli` with repository `Contents: read and write`;
leave every other optional permission disabled. Webhooks and user authorization
are not required. Configure:

- repository variable `RELEASE_APP_ID`: the numeric App ID;
- repository variable `COMMERCE_SKILL_TRUST_KEYS`: the versioned public
  Commerce Skill trust ring in
  `keyId:base64url-spki[,keyId:base64url-spki]` form. Release and POC workflows
  parse every SPKI, require unique Ed25519 key IDs, freeze the validated ring
  for that workflow execution, and revalidate it in every binary job;
- repository secret `RELEASE_APP_PRIVATE_KEY`: the complete generated PEM key.

A Commerce Skill `keyId` is permanently bound to one Ed25519 public key after
it has signed a Product Skill Release. Key rotation must allocate a new ID and
publish both the old and new public keys in `COMMERCE_SKILL_TRUST_KEYS` before
Shop signs with the new key. Never replace key material under an existing ID;
remove an old public key only after every Release signed by it is no longer
installable.

**Hosted development Commerce signer.**

`https://dev.viceme.cn` has a separate source-pinned Commerce trust entry in
`internal/command/commerce_install.go`. Its `v1` public Ed25519 SPKI SHA-256 is
`20f58035b232eef096699009db4301ad708cf2ae962cdbece04b62d26dd16196`.
The entry matches the normalized HTTPS origin, including its port, and applies
to both Commerce Skill releases and Website Replica licenses. It does not
relax document signatures, artifact checks, or the existing loopback policy.

Do not put this development key in the origin-independent production
`COMMERCE_SKILL_TRUST_KEYS` variable. For development key rotation, verify the
new public key against the intended development deployment, allocate a new
key ID, add another entry scoped to the same origin, and release the CLI before
switching the development signer. Retain old entries while their signed
releases or paid licenses remain recoverable. A remote endpoint response alone
never changes an installed CLI's trust policy.

Keep `main` as the repository default branch, but target normal feature and fix
pull requests explicitly at `dev`. Repository settings allow merge commits only;
squash and rebase merging are disabled so an administrator bypass cannot detach
a reviewed source head from `main` history.

Protect `dev` with its own active branch ruleset that retains the normal pull
request, one approving review, the `PR quality` check, all three `PR npm
installer (<runner>)` checks, deletion
protection, and force push protection. Protect `main` with a separate ruleset
that requires the same checks plus `Release candidate preparation`, but does
not require `dev` to contain the previous release merge commit. Both rulesets
allow merge commits only. The required `PR quality` job rejects `main` pull
requests from forks or dev-only integration branches. Feature PRs additionally
require accepted dev ancestry and reproducible preparation. Disable strict
up-to-date checks for dev and set repository `allow_update_branch=false` so the UI
does not encourage merging dev into independently releasable source branches.
Keep the existing review and required checks; disabling this suggestion cannot
prevent a developer from manually merging dev, so review ancestry and full diff.

Add `ViceMe CLI Release Bot` and the organization-admin role to both bypass
lists with `Always allow`; the latter preserves the legacy rule's existing
`enforce_admins: false` behavior. Do not leave a legacy branch-protection rule
active beside the rulesets because it cannot recognize the ruleset's App bypass.

The App installation token is scoped to the current repository and
`Contents: write`, expires after at most one hour, and is revoked automatically
when the job finishes. The workflow still stages an explicit allowlist of
generated files and validates the complete release before pushing. No
maintainer PAT or Deploy Key is used.

The general `CLI PR checks` workflow runs for pull requests, not branch pushes.
For a repository-owned source to `main` promotion, it classifies the exact head:
an unprepared head runs only target validation, while the marked Release Bot
commit runs the complete required matrix. A Release App push synchronizes the
already-open PR and cancels any older generic run for the same PR. The resulting
full checks therefore cover the exact prepared commit once. The synchronize event reuses preparation metadata; the required PR quality
job independently regenerates and verifies feature-release commits.

The checks from `CLI release publication` are deliberately not required for
merging: that workflow starts only after the release PR has been merged and
performs the tag, binary, GitHub Release, npm, and notification steps.

Create a GitHub Actions Environment named `cdn` and restrict deployments to
protected branches. The S3 publication job is the only release job that uses
this Environment, matching the SDK release boundary; npm Trusted Publisher
remains token-free and does not use a GitHub Environment restriction.

Configure npm trusted publishing for:

- npm package: `@viceme-ai/cli`;
- GitHub organization/repository: `ViceMe-AI/cli`;
- workflow filename: `release.yml`.

Trusted publishing is the only publication credential path and uses GitHub OIDC
plus npm provenance. Do not configure `NPM_TOKEN`; the publication job does not
generate an npm auth file or expose a long-lived token.

The npm tarball contains `checksums.txt`, generated from the six immutable
GitHub Release checksum assets immediately before publication. The launcher
uses that bundled manifest as its trust root whether the matching binary is
transported by GitHub Release, a configured npm registry binary mirror, or the
public npmmirror binary mirror. Registering `viceme-cli` with cnpmcore enables
the public `/-/binary/viceme-cli/` mirror; it does not create another npm
package.

The CN and Global release mirrors are S3-compatible origins rather than Amazon
S3 itself. `cmd/s3-publish` always runs from the GitHub Actions workflow
revision that is executing (so recovering an older tag still uses the current
publisher) and reads the immutable `dist` artifact assembled for that release.
The publisher and workflow set request and response checksum calculation to
`WHEN_REQUIRED`: immutable artifacts are still compared byte-for-byte on
recovery, while optional AWS streaming checksum trailers that the origins do
not implement are not sent.

Every release renders two public Agent contracts with the exact stable version:

- `release/agent-install.md.tmpl` is the narrow CLI Runtime bootstrap. It
  installs or updates one verified CLI plus official-Skill generation, runs
  `viceme doctor`, and then returns control to its caller. Windows Agents verify
  the Manifest-selected executable and invoke the executable's own
  `bootstrap activate` entrypoint directly, so a POSIX-only host such as WorkBuddy does not need to
  escape into PowerShell or npm. It does not know a Product stable name or
  install a merchant Product Skill.
- `release/commerce-skill-install.md.tmpl` is the generic Product Skill
  activation contract. It first uses an already healthy Commerce Runtime and
  delegates to the same-region `agent-install.md` only when the Runtime is
  missing or reports an unsupported version. It then runs the exact
  platform-provided `viceme commerce skill install` command and returns to the
  original service request.

Recovery never rerenders an already-published asset with the current `main`
template and then asks it to match old bytes. It downloads every existing asset
from the immutable GitHub Release and restores those exact bytes over the
locally assembled candidate; only genuinely missing assets, such as the first
`commerce-skill-install.md` added to an older release, remain newly generated
and are uploaded. The final release is downloaded again and compared with the
complete recovered candidate before S3 publication continues.

Historical tags are not required to contain validators or release generators
introduced later. Recovery validates the trusted workflow revision that is
performing the repair and preserves every binary or checksum already present in
the GitHub Release. A missing binary is rebuilt from the exact immutable tag
with the frozen current trust ring; an existing checksum must match that byte,
while a missing checksum is derived from the existing binary. A recovered
signed Agent Manifest must exactly match a freshly reconstructed Manifest over
the final verified binary pairs, so a partial Release can never combine an old
signature with different replacement bytes. Current generators are otherwise
used only for genuinely missing contract assets. The tag, existing release
bytes, and final recovered candidate remain immutable and are verified
independently.

The same bytes are published as immutable
`cli/releases/vX.Y.Z/agent-install.md` and
`cli/releases/vX.Y.Z/commerce-skill-install.md` objects in both regions. Only
the highest stable version updates the public root `agent-install.md`,
`commerce-skill-install.md`, `install.sh`, and `install.ps1` pointers. The
separate `agent-release-manifest.json` contains the six platform asset
digests, bundled Skill digests, installer digests, and Sigstore verification
identity. Its detached `agent-release-manifest.sigstore.json` bundle is created with
the Release Workflow's GitHub OIDC identity and verified before publication;
recovery reuses an existing immutable bundle byte-for-byte. A recovery tag
that predates this contract keeps its original `release-manifest.json`
unchanged and uses the trusted current workflow generator only to add the new
Agent Manifest, signature bundle, and document.

The publication job verifies both public origins after upload. It compares both
versioned documents, the Manifest, and signature bundle with the release
artifacts, checks immutable and root cache policies, requires both public root
Agent documents to use `text/markdown; charset=utf-8`, compares CN and Global
root documents, and proves that an uploaded object outside the installation
allowlist is not anonymously readable. Anonymous bucket listing must also stay
disabled. The allowlist is limited to `agent-install.md`,
`commerce-skill-install.md`, the existing root installers, and the versioned
`cli/releases` installation objects; Skill ZIPs, user uploads, and business
media never belong in this bucket or policy.

ViceMe Product pages point Agents to the root `commerce-skill-install.md`, not
directly to the base installer or a signed Product ZIP. The Commerce contract
accepts only the exact platform-generated install command. The CLI retrieves a
fresh Shop authorization and independently verifies Product Skill identity,
digest, platform signature, and Commerce Runtime compatibility. Release tests
must keep both contracts free of merchant-controlled URLs, API origins, shell
fragments, and login requirements; the base `agent-install.md` must additionally
remain free of Product stable names and Product installation commands.

Configure the repository secret `CN_S3_HTTPS_PROXY` with the authenticated
HTTPS forward-proxy URL used by GitHub Actions to reach the CN S3 endpoint.
The release job applies it only inside the CN publication subshell; Global S3
publication remains a direct connection. Keep the proxy credentials in the
secret value and never print the URL in workflow logs.

`GITHUB_TOKEN` is provided by Actions and is used to maintain the Release PR
and resolve a merged `main` commit back to its reviewed Release PR.
`RELEASE_APP_ID` and `RELEASE_APP_PRIVATE_KEY` authenticate the narrowly scoped
Release App.

The release notification job uses the same repository secrets as ViceMe Web,
API, and Engine:

- `FEISHU_RELEASE_WEBHOOK`: webhook for the release notification group;
- `AI_API_KEY`: API key used to generate the release summary;
- `AI_MODEL`: optional model override, defaulting to `deepseek-chat`;
- `AI_BASE_URL`: optional OpenAI-compatible endpoint override, defaulting to
  `https://api.deepseek.com/v1`.

The notification runs only after the GitHub Release and npm publication have
both succeeded, so a failed or incomplete release is not announced as
successful.

## Recovery

The original `push` publication run is safe to rerun from GitHub Actions.
Existing tags must point to the same reviewed commit. Existing GitHub Release
assets are compared byte-for-byte and never overwritten. Existing npm versions
must have the same registry integrity as the locally packed artifact; otherwise
the workflow fails closed. A rerun of an older version cannot move the npm
`latest` tag behind a newer release.

If a publication failed after creating the immutable tag, a maintainer may
manually dispatch `CLI release publication` with that exact stable tag. This
also covers failures before the GitHub Release was created: recovery may create
the missing Release from regenerated and verified artifacts. If the Release
already exists, it must be non-draft and every existing asset must match
byte-for-byte before a missing asset is uploaded. Recovery still refuses
missing tags, version mismatches, changed release assets, and npm integrity
mismatches. It cannot create a new release identity. Normal production releases
originate from merging an eligible repository-owned Release PR into `main`.

## Shared host payment presentation

`skills/use-a-skill/references/host-presentation.md` owns host image syntax,
page tools, capability checks and fallbacks. The CLI embeds this source directly;
the no-CLI runtime includes the identical bytes as `guides/host-presentation.md`.
Exported purchase entries also include it beside `references/purchase.md` so
relative links resolve. Business purchase sequencing remains in `purchase.md`.
Do not add host policy branches to Go/Python or duplicate them in Widget docs.

After an edit, run `make release-manifest`, `make check` and
`make npm-package-check`; the digest-addressed runtime and official Skill bundle
must ship together. Shop's runtime archive validator must accept
`guides/host-presentation.md` before publishing this generation. Its optional
allowlist entry preserves compatibility with the previous seven-member runtime.
Rollback may use the previous runtime while that Shop validator remains deployed.
A validator rollback must follow a runtime rollback, since the old validator
rejects the additional member. No database or payment-state migration is needed.
Existing exported packages and installed Skills retain their bundled guidance;
re-export/reinstall through their normal explicit update path to obtain changes.
Do not mutate existing purchase credentials or orders to refresh instructions.
