---
name: viceme-shared
description: Set up, authenticate, update, and diagnose the ViceMe creator CLI and official Skills. Use when a user needs to install ViceMe for Codex, Claude Code, or WorkBuddy; sign in with a device code; inspect profiles; check versions; or repair a ViceMe CLI or Skill installation.
---

# ViceMe shared operations

Use `viceme` as the only executable. Never collect, print, or persist access tokens yourself.

## Setup

1. Run `viceme install --agent auto` after the bootstrap installer completes.
2. Run `viceme doctor` and resolve failed checks before business commands.
3. Run `viceme auth status`.
4. If unauthenticated, run `viceme auth login`. Show the verification URL to the user and wait for the command result.

For split browser flows, use `viceme auth login --no-wait`, then continue with
`viceme auth login --device-code <deviceCode>` before expiry.

## Safety

- Treat stdout as the final JSON protocol response. Send progress explanations separately.
- Branch on process exit code and `error.code`, never message text.
- Do not pass `VICEME_ACCESS_TOKEN` unless the user explicitly supplied a scoped automation credential.
- Use `--profile` only to select an existing profile. Use `viceme profile add`, `list`, `use`, or `remove` for profile changes.
- Before changing files or publishing, summarize the intended operation and obtain any confirmation required by the domain Skill.

## Maintenance

- Use `viceme version` to report CLI and bundled Skill versions.
- Use `viceme update` to update the CLI release.
- Re-run `viceme install --agent auto` after an update so every detected Agent and the `~/.agents/skills` fallback receive the matching official Skills.
