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

For a test or private ViceMe deployment, persist its endpoint before login:

```bash
viceme profile add --name <profile> --region <cn|global> --api-base-url <https-url> --use
```

Use `viceme profile list` to verify the active profile and effective endpoint.
Do not rely on a shell-only `VICEME_API_BASE_URL` export for normal Agent use;
that variable is a one-process CI/debug override and is not profile state.

For split browser flows, use `viceme auth login --no-wait`, then continue with
`viceme auth login --device-code <deviceCode>` before expiry.

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
