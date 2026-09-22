# ViceMe CLI Repository Rules

## Scope and ownership

This repository owns the ViceMe command-line runtime, its official Skill
bundle, the standalone installers, the npm launcher, and the release contracts
that publish those artifacts. Keep implementation with its semantic owner:

- `cmd/viceme` is the executable entrypoint.
- `internal/command` owns commands, startup ordering, machine output, and the
  orchestration of installation and recovery.
- `internal/update` owns release discovery, installation-method-specific
  mutation, permission classification, activation journals, and recovery.
- `internal/skillcontent` owns official Skill discovery, destination
  preflight, transactions, and verification.
- `npm` owns the persistent npm launcher and verified binary acquisition.
- `installers` and `release` own public bootstrap and release contracts.
- `quality/release-manifest.json` is generated evidence for the embedded
  release. Regenerate it; never repair its digests by hand.

Do not move low-level update behavior into a Skill, an Agent prompt, or a
calling application. Codex, Claude Code, WorkBuddy, and other hosts may invoke
the CLI, but they do not own its installation state or recovery protocol.

## Command and output contract

- Commands return one machine-readable JSON envelope on stdout. Human help may
  use stderr; update diagnostics, child-process output, and library logs must
  not pollute stdout.
- Clients branch on stable error `code` values, never message text. Do not leak
  raw npm output, credentials, signed URLs, filesystem secrets, or internal
  exception details.
- `meta.executingCliVersion` identifies the process producing the response.
  When an update succeeds, `data.cli_version` may identify a newer installed
  generation; the current process does not pretend it changed version in place.
- Startup recovery runs before ordinary business commands. A required recovery
  failure stops the command; it must never be hidden behind a successful
  business response.

## Work URL parameters

- Public Work URLs prefer `/{handle}/{workSlug}` and Markdown prefers
  `/{handle}/{workSlug}.md`. Also accept `/{handle}?workSlug={slug}` and
  `/{handle}.md?workSlug={slug}`, including explicit `mode=consumer&view=work`.
  Missing public mode/view may be inferred, but explicit invalid modes/views,
  duplicate parameters, mismatched path/query identities and locale-prefixed
  paths must be rejected. Bare `/{handle}` is the public profile.
- `internal/workurl.PublicParts` owns shared Work identity parsing. Preserve
  `product`, `install=owned`, origin and the full input passed to owning flows.
  Private owner Markdown remains `/{handle}.md?mode=creator&view=work&workSlug=...`.
- The standalone Replica Python script uses the same contract fixtures in
  `internal/workurl/testdata`. Keep Go and Python acceptance behavior aligned.
- User-facing public Work links use `workurl.Display` at the output boundary:
  publication `result.workUrl`, published Skill `product.detailUrl`, listing
  `publicUrl`, and `workUrl` / `workPresentation.url` display fields prefer the path-shaped URL.
  Work detail exposes `workUrl` and `markdownUrl`; its API canonical fields
  remain unchanged. Do not shorten opaque URLs with unknown query parameters.
- Do not mutate returned canonical URLs or signed/confirmation identities.
  Across representations compare frozen public identities with `workurl.Equivalent`;
  it must retain origin, Work identity and edition/intent, never ignore opaque fields.
  When constructing Markdown, append `.md` to the pathname before the query.
  Owner analytics must verify ownership and explicitly request creator mode.
- Preserve pathname plus query in login `returnTo`. URL mode is presentation,
  never authorization. URL parsing does not change the selected profile or
  the existing official-origin requirements for standalone scripts.


## Supported installation methods

ViceMe supports two installation owners and they must remain distinct:

1. The standalone installation uses the official regional release store and
   replaces a verified executable through the bootstrap activation protocol.
2. The npm installation uses the persistent `@viceme-ai/cli` launcher. The
   launcher owns npm package resolution, binary download, checksum validation,
   and the environment that identifies the installation method.

Never guess the installation method from `PATH`, the executable filename, or
the presence of npm on the machine. Honor the launcher-provided
`VICEME_INSTALL_METHOD` contract. Do not turn a standalone installation into an
npm installation, or the reverse, as an update fallback.

Public installers may download only immutable official release artifacts and
must verify their checksums before activation. Do not add arbitrary mirrors,
unverified URLs, `curl | sh` fallbacks, or a second installer implementation
inside Agent instructions.

## Automatic update lifecycle

- An ordinary command must emit its own response before attempting to start an
  automatic update. Network discovery, download, permission probes, and
  activation happen only in a detached worker.
- Failure to launch the worker, loss of network, permission denial, or
  installation failure is fail-open for the already completed foreground
  command. The worker records bounded local state and emits no second response.
- The detached worker has null standard streams, does not read the foreground
  stdin, and must not inherit `VICEME_ACCESS_TOKEN` or another publication
  credential.
- Automatic checks are coalesced by `automatic-update.lock` and
  `automatic-update.json`. Successful/current checks wait 24 hours; failures
  become eligible after one hour. Do not replace this with a process-local
  timer or one goroutine per invocation.
- Automatic update is disabled for CI, development versions, update/install/
  bootstrap commands, activation children, and re-executed commands. Tests
  that exercise it must explicitly clear the ambient `CI` value and enable the
  test-only development-version path.
- Automatic update always calls the updater with `RefreshSkills: false`. It
  updates the CLI only. A successful replacement becomes observable on the
  next `viceme` process; restarting the surrounding Codex, Claude Code, or
  WorkBuddy application is not required.
- Windows cannot replace the currently running executable. Schedule the same
  verified CLI-only activation for after process exit and preserve the same
  journal and recovery semantics.

Do not reintroduce foreground self-update, automatic command re-execution, or
automatic Skill rewriting as a convenience fallback.

## CLI and Skill lifecycles

The CLI executable and Agent Skills are separate lifecycles:

- `viceme update --check` performs release discovery only.
- `viceme update` is the explicit CLI-only repair/update path.
- `viceme install --agent <target>` installs or refreshes official Skills.
- `viceme update --agent <target>` remains a compatibility path for an
  explicitly requested combined repair; new automation must keep the commands
  separate.

Agent hosts commonly load Skill files when a task starts. After an explicit
Skill refresh, a new Agent task may be needed for rediscovery. This is different
from a CLI update: the next CLI invocation already uses the new executable.

Never make a background CLI update write `.agents/skills`, `.codex/skills`,
Claude configuration, WorkBuddy configuration, or the active ViceMe profile.
Never roll back a successfully installed CLI merely because a separate Skill
destination cannot be updated.

## Permission preflight

Before creating an activation journal or downloading a replacement, preflight
every path the selected operation will mutate:

- standalone executable directory and replacement path;
- ViceMe configuration, lock, staging, backup, and recovery paths;
- npm cache, actual scoped global package, and persistent launcher directories;
- every selected Agent Skill destination for an explicit Skill installation.

Probe the real resolved paths using temporary create/write/rename operations so
host filesystem brokers and mount restrictions are preserved. A parent
directory mode check alone is not evidence that activation can succeed.

If permission is denied before mutation, leave the installed generation intact
and do not create a recovery journal. Explicit commands return
`UPDATE_PERMISSION_REQUIRED` with exit code 6 and instruct the host to use its
official approval mechanism. A chat message granting consent does not grant OS
access.

Never respond to a permission failure by using `sudo`, changing broad file
permissions, uninstalling the CLI, deleting installation directories, removing
activation journals, changing npm prefixes, or bypassing a host broker. If
access remains unavailable, stop safely and keep the old working generation.

Permission can change after preflight. Once mutation has begun, preserve the
durable journal so startup recovery can roll forward or restore the previous
generation. Do not convert a mid-activation failure into a clean preflight
failure.

## Atomic activation and recovery

- `activation.lock` is the single outer coordinator for standalone and npm
  activation. Inspect both outer journal types while holding it.
- `activation-member.lock` protects the Skill/config member of a coordinated
  generation. Respect the lock order already used by command startup,
  bootstrap, npm activation, and explicit installation.
- `bootstrap-activation.json` and `npm-activation.json` are recovery state, not
  disposable cache files. Validate their schema, paths, target identity,
  version, nonce, and generation before acting on them.
- Stage and verify bytes before the atomic boundary. Retain enough information
  to restore the previous executable when commit has not completed.
- Recovery is idempotent: an already complete target coalesces, an incomplete
  preparation rolls back, a committed generation rolls forward, and an older
  generation must never overwrite a newer one.
- When recovery installs a generation newer than the running process, return
  the controlled restart/re-execution signal. The old process must not continue
  business work under the new on-disk identity.
- Never maintain separate standalone and npm recovery coordinators or acquire
  the activation locks in a different order.

## Security boundaries

- Child processes and detached workers receive the minimum environment. Strip
  publication credentials, and never persist them in update state or journals.
- Release metadata is untrusted until its schema, semantic version, platform
  asset name, size, and checksum are validated.
- Keep regional release origins authoritative. Profile locale does not select a
  release region; use the configured distribution region.
- Files containing configuration or recovery state must use private file
  helpers and atomic replacement. Do not replace them with casual direct writes.
- Do not log private paths or tool output when a stable error kind is sufficient.

## Tests required for installer or updater changes

Run focused tests while iterating, then complete all applicable gates before
handoff:

```bash
make check
go test -race ./internal/command ./internal/update
make npm-package-check
```

Changes to installation or update behavior must cover, as applicable:

- foreground response remains successful when background launch/network/access
  fails;
- current, failed, stale, and concurrent automatic-update states;
- CLI-only update does not invoke a Skill child or touch Skill/config paths;
- permission refusal before mutation creates no journal and preserves hashes;
- permission loss after mutation retains recoverable state;
- concurrent activation commits one generation;
- preparing rollback, committing roll-forward, crash recovery, and old-process
  restart behavior;
- standalone, npm, macOS/Linux, and Windows delayed-replacement paths;
- child-process environment filtering and machine-output isolation.

For real-process smoke tests, use an isolated temporary HOME, explicit
`VICEME_CLI_CONFIG_DIR`, temporary executable path, and local cache. Never use
the developer's active CLI, Agent Skill directories, credentials, or recovery
files as test fixtures. A simulated or local-binary activation is not evidence
that an unpublished official release exists; state that boundary explicitly.

## Release and pull-request workflow

- 从最新 `origin/main` 创建功能分支，先 PR 到 `dev` 验收，再用同一来源分支 PR 到
  `main` 独立发布；保留 `dev → main` 整体发布。详见 `docs/releasing.md`。
- 禁止将 `dev` 或临时集成分支反向合入功能分支。集成冲突在从 `dev` 创建的
  `chore(repo)/integrate-*` 分支解决，该分支只能 PR 到 `dev`。
- 独立发布的业务 head 必须已进入最新 `dev`，PR 正文记录验收 SHA 和结果。
  唯一例外是紧随该 head 的一个自动版本提交：必须通过发布工作流证据、文件白名单
  和从父提交重新生成后的完整 Git tree 比对；不能仅信任作者或提交标题。
- Bot 更新原来源分支，最终提交运行完整发布检查。业务代码再变更必须再次进入 dev。
  分支只从 main 同步；不得为满足检查把 dev 合入。已从 dev 创建的存量分支逐一
  检查相对 main 的完整差异，不批量重建。
- PR 标题与正文使用中文（自动发布标题保留 `chore(release): vX.Y.Z`），交付至少
  一个已有语义标签。未经明确授权不合并 PR、不发布。
- Use Conventional Commit subjects. Keep generated version, changelog, package,
  checksum, and manifest changes inside the automated release preparation flow.
- Do not create or move release tags manually, publish npm locally, or edit
  immutable release assets.
- Embedded Skill or release-contract changes must regenerate and validate the
  release manifest and all affected npm/installer consumers.
- Before handoff, synchronize the latest target branch, run the required gates,
  inspect `git diff --check`, and ensure the PR contains only the intended
  semantic change. Do not report merge readiness while required CI is pending
  or failing.
