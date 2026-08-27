---
name: viceme-engagement
description: Add ViceMe hosted danmaku and tips to one public website through the complete CLI-owned flow. Use when a user asks to integrate, test, or repair danmaku and tipping together with one engagement embed script.
---

# Integrate ViceMe danmaku and tips

Create creator-owned configuration through the CLI and install exactly one
CLI-generated combined script. Never reuse a fixture, shared `workKey`, or
another creator's app.

## Workflow

1. Inspect the target page, exact HTTPS hostname, deployment command, CSP, and
   browser test setup. Preserve the host's existing layout and deployment path.
2. Run `viceme profile list`. Pin every later command to one Profile and require
   its `apiBaseUrl`, `webBaseUrl`, and `marketRegion`.
3. Run `viceme --profile <profile> auth status`. If it does not report an
   authenticated user, run `viceme --profile <profile> auth login` on that same
   Profile and wait for completion.
4. If `.viceme/access.yaml` is absent, run:

   ```bash
   viceme --profile <profile> access init --name "<website name>" --danmaku
   ```

   Otherwise run `access inspect`. Run `access apply` only when the local config
   intentionally needs reconciliation. Record `data.workKey`; never create a
   second Work merely because an apply failed.
5. Run `creator-app list` on the same Profile. Reuse an app only when it belongs
   to the authenticated creator and includes the exact hostname. Otherwise run
   `creator-app create --name "<website name>"`.
6. When the hostname is not verified, run `creator-app domain add`, serve its
   token verbatim at the returned verification path, deploy, fetch that exact
   HTTPS URL, and run `creator-app domain verify`. Do not print or commit the
   token.
7. Read the target page's actual locale (`zh-CN` or `en-US`) without inferring
   it from the Profile's market, then generate the authoritative combined snippet:

   ```bash
   viceme --profile <profile> creator-app show <app-id> --work-key <work-key> --locale <zh-CN-or-en-US>
   ```

   Insert `data.engagementEmbedSnippet` exactly once before `</body>` or through
   the framework's equivalent script facility. Do not hand-build the tag,
   derive its origin, substitute its region, or keep standalone `tip-embed.js`
   or `viceme.min.js` tags beside it.
8. Preserve existing CSP directives and add only the exact generated script and
   frame origin. Never add wildcards or `unsafe-eval`.
9. Run repository checks, deploy, and verify the real hostname at desktop and
   320px widths. Confirm one script loads, danmaku persists after refresh, the
   tip dialog opens, Escape closes it, and focus returns. State real payment as
   unverified unless a provider transaction was performed.

## Hard constraints

- Do not directly call Shop APIs, write the database, fabricate IDs, or read
  credentials from browser storage.
- Do not copy a React blueprint, add host styling for the widget, or install a
  second runtime.
- Keep every command pinned to the Profile selected at the start.
- Stop on a structured CLI error. Never switch environments as a fallback.
