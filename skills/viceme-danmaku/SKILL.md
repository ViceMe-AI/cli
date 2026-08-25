---
name: viceme-danmaku
description: Install or repair the ViceMe-hosted danmaku SDK in a website. Use for script-tag integration, workKey setup, scrolling messages, reactions, comment entry, responsive behavior, keyboard access, and reduced-motion verification.
---

# Install ViceMe Danmaku

The hosted SDK is the default and authoritative integration. Add the CLI-generated
script tag to the host website; the host must not copy ViceMe React, Tailwind,
iframe, API client, or persistence code.

## Required inputs

Before editing, inspect:

1. The target repository's instructions, page entry point, deployment model,
   Content Security Policy, and browser test setup.
2. `viceme profile list`, including the selected Profile's `apiBaseUrl`,
   `webBaseUrl`, and `marketRegion`.
3. [cdn-sdk.md](references/cdn-sdk.md) in full.

## Workflow

1. Run `viceme profile list` and pin every later command to the selected Profile.
   Do not switch Profiles merely to reuse another login.
2. Run `viceme --profile <profile> auth status`. If unauthenticated or missing
   `sdk-work:read` or `sdk-work:write`, run
   `viceme --profile <profile> auth login` and wait for completion.
3. If `.viceme/access.yaml` is absent, run:

   ```bash
   viceme --profile <profile> access init --name "<website name>" --danmaku
   ```

   If the file exists, run `viceme --profile <profile> access inspect`. If its
   authoritative response has no `data.embedSnippet` while the local config is
   active, run `viceme --profile <profile> access apply` once to reconcile it.
   Do not hand-edit the access config or create a second Work.
4. Read `data.embedSnippet` from the successful CLI response and insert it
   exactly once before `</body>` or through the framework's equivalent script
   facility. The CLI derives its URL from the selected Profile's `webBaseUrl`;
   do not guess, concatenate, replace, or fall back to a production origin.
5. Preserve an existing per-response CSP nonce on the inserted script. Add only
   the exact script and frame origins required by `data.embedSnippet`; never add
   `*`, `'unsafe-eval'`, or a broad ViceMe subdomain wildcard.
6. Run the target's format, lint, typecheck, tests, and production build.
7. Verify the rendered page on desktop and mobile. Confirm one SDK root mounts,
   host controls remain clickable, keyboard controls work, reduced motion is
   honored, a sent message survives refresh, and no duplicate script is loaded.

Completion means the target contains one CLI-generated script integration and
the browser checks pass against the same pinned Profile.

## Boundaries

- `workKey` is public and opaque. Never substitute a creator ID, product ID,
  slug, token, or credential.
- The hosted SDK owns rendering, iframe placement, public API calls,
  persistence, and cleanup.
- The host owns only script placement and its own page content.
- Tipping is handled by `viceme-tip`, not this Skill.
- Self-hosting or copying the component source is outside this Skill.

## Handoff

Report changed files, the selected Profile and public `workKey`, checks run,
responsive and keyboard coverage, and any unverified CSP or hosted-runtime
boundary. Never report credentials.
