---
name: viceme-shared
description: Set up, authenticate, update, and diagnose the ViceMe creator CLI and official Skills. Use when a user needs to install ViceMe for Codex, Claude Code, or WorkBuddy; sign in through the browser; inspect profiles; check versions; or repair a ViceMe CLI or Skill installation.
---

Before login, run `viceme auth status` in the active Profile. If it reports an
authenticated, unexpired credential with the scopes required by the requested
operation, do not run `viceme auth login` and do not open an authorization page.
When login is required, keep `viceme auth login` running and immediately handle
the streamed `VICEME_PRESENTATION` event whose intent is `OPEN_AUTHORIZATION`
with the host's embedded-browser capability. If the capability is unavailable,
show its `fallbackUrl` as a clickable link. Never claim the page opened without
a successful host action. In WorkBuddy, call
`present_files(files=[presentation.resultPaneUrl])` immediately with that URL
as the only item, and accept success only when the result includes it in
`previewed`. Do not mix local files or other URLs into the same call.

# ViceMe shared operations

Use `viceme` as the only executable. Never collect, print, or persist access tokens yourself.

## Setup

Use this setup workflow only when the user explicitly asks to install,
authenticate, update, diagnose, or repair ViceMe. Do not run setup, Doctor,
Version, or Install as preflight for another business Skill. A business Skill
must run only its required command and stop on its structured error unless the
user separately asks for diagnosis.

1. Run `viceme install --agent auto` after the bootstrap installer completes.
2. Run `viceme doctor` and resolve failed checks before business commands.
3. Run `viceme auth status`.
4. If unauthenticated, run `viceme auth login`. Show the verification URL to the user and wait for the command result.

For a test or private ViceMe deployment, persist its endpoint before login:

```bash
viceme profile add --name <profile> --api-base-url <https-url> --use
```

Use `viceme profile list` to verify the active profile and effective endpoint.
The install/update distribution region never selects a publication market;
the selected API endpoint owns that market.
Do not rely on a shell-only `VICEME_API_BASE_URL` export for normal Agent use;
that variable is a one-process CI/debug override and is not profile state.

`viceme auth login` is intentionally blocking. Show its one-time complete URL to
the user and keep waiting for the command result while the page signs in when
necessary and authorizes automatically. Never ask the user to enter a device
code, and do not split or background the login. If the bounded wait expires,
run a fresh `viceme auth login` for the same Profile.

## Safety

- Treat stdout as the final JSON protocol response. Send progress explanations separately.
- Branch on process exit code and `error.code`, never message text.
- Do not pass `VICEME_ACCESS_TOKEN` unless the user explicitly supplied a scoped automation credential.
- Use `--profile` only to select an existing profile. Use `viceme profile add`, `list`, `use`, or `remove` for profile changes. Remove and recreate a profile to change its endpoint; do not silently rebind stored credentials to another origin.
- Never switch Profiles on the user's behalf merely to reuse an existing login. A business workflow stays pinned to the Profile selected at its start; changing that Profile requires an explicit user request.
- Use `viceme profile remove --all --yes` only when the user explicitly asks to remove every local Profile and credential. It recreates one clean, unauthenticated `default` Profile so the CLI configuration remains valid.
- Before changing files or publishing, summarize the intended operation and obtain any confirmation required by the domain Skill.

## Maintenance

- Use `viceme version` to report CLI and bundled Skill versions.
- Released installations automatically check the authoritative stable channel before ordinary commands. When a newer generation exists, the CLI and every matching official Skill are activated together, then the original command continues under the new CLI.
- Do not ask the user to choose an update channel, interrupt the requested operation for a routine update, or run a second Skill installation after an automatic update.
- Use `viceme update` only when the user explicitly requests a manual repair or a failed automatic activation must be retried.

## Automatic updates

- Automatic release discovery is fail-open when the network is unavailable, so the last complete verified generation remains usable offline.
- After an automatic update, `meta.autoUpdate` records the previous and executing versions. This is informational; continue processing the command's normal `data` or `error`.
- A standalone Windows binary may return retryable `AUTO_UPDATE_RESTART_REQUIRED` while the operating system releases the old executable. Wait briefly and rerun the exact same command without asking the user for another decision.
- If activation itself fails, branch on the stable error code and stop the requested mutation. Never continue with a partially activated generation.
