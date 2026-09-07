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
- Automatic update calls the updater with `RefreshSkills: true` and target
  `auto`, activating the CLI and matching official Skills together. When the
  CLI is current, check installed official Skills with Doctor and repair stale
  or missing copies; skip activation when all are healthy. A new CLI becomes
  observable on the next `viceme` process.
- Windows cannot replace the currently running executable. Schedule the same
  verified CLI and Skill activation for after process exit and preserve the same
  journal and recovery semantics.

Do not reintroduce foreground self-update or automatic command re-execution.
Skill synchronization belongs to the existing detached update and activation
protocol; do not add a separate foreground or uncoordinated file-copy path.

## CLI and Skill lifecycles

CLI updates synchronize the official Skill bundle:

- `viceme update --check` performs release discovery only.
- `viceme update` updates or repairs the CLI and official Skills for `auto`.
- `viceme install --agent <target>` installs or refreshes official Skills.
- `viceme update --agent <target>` selects the Agent destination for the
  combined update or repair.

Agent hosts commonly load Skill files when a task starts. After a
Skill refresh, a new Agent task may be needed for rediscovery. This is different
from a CLI update: the next CLI invocation already uses the new executable.

Refresh only the official bundle through its owning installer and preserve
other user-installed Skills, existing Profile authority, and credentials.
Keep the combined activation journal and rollback semantics so a failed Skill
refresh cannot be reported as a complete CLI and Skill generation.

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
- normal manual and automatic updates refresh matching official Skills;
- a current CLI repairs stale Skills but skips healthy installations;
- explicit low-level CLI-only activation still leaves Skill/config paths alone;
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

- Normal feature and fix PRs target `dev`. Production promotion is the reviewed
  repository-owned `dev` to `main` Release PR described in
  `docs/releasing.md`.
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
