---
name: viceme-shared
description: Set up, authenticate, update, and diagnose the ViceMe creator CLI and official Skills. Use when a user needs to install ViceMe for Codex, Claude Code, or WorkBuddy; sign in through the browser; inspect profiles; check versions; or repair a ViceMe CLI or Skill installation.
---

# ViceMe shared operations

Use `viceme` as the only executable. Never collect, print, or persist access tokens yourself.

## Setup

1. Run `viceme install --agent auto` after the bootstrap installer completes.
2. Run `viceme doctor` and resolve failed checks before business commands.
3. Run `viceme auth status`.
4. If unauthenticated, run `viceme auth login`. Show the verification URL to the user and wait for the command result.

For a test or private ViceMe deployment, persist its endpoint before login:

```bash
viceme profile add --name <profile> --region <cn|global> --api-base-url <api-origin> --web-base-url <web-origin> --use
```

Use `viceme profile list` to verify the active profile and effective endpoint.
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
- Use `viceme update` to update the CLI release.
- Re-run `viceme install --agent auto` after an update so every detected Agent and the `~/.agents/skills` fallback receive the matching official Skills.

## Notices

- Inspect the top-level `_notice` object after every CLI response; it is advisory and never replaces `data` or `error`.
- When `_notice.update` is present, finish the user's current operation first. Then briefly state that a newer ViceMe CLI is available and show the exact command `viceme update`.
- Do not copy the raw notice JSON into the answer, interrupt the current operation, or run an update without the user's authorization.
