---
name: viceme-engagement
description: Add ViceMe hosted danmaku and tips to one public website through the complete CLI-owned flow. Use when a user asks to integrate, test, or repair danmaku and tipping together with one engagement embed script.
---

# Integrate ViceMe danmaku and tips

Complete the real self-service flow. Every tester creates a creator-owned website Work and Creator App; never reuse a shared `workKey`, fixture, or another creator's app.

## Workflow

1. Inspect repository instructions and locate the single public HTML or root layout entry, its source directory, exact HTTPS hostname, deployment command, and Content Security Policy. Preserve the host page and existing deployment mechanism.
2. Run `viceme profile list`. Keep the active Profile fixed for the entire workflow. Require its `apiBaseUrl` and `webBaseUrl`; a custom API Profile without a matching Web origin is invalid and must be recreated explicitly.
3. Run `viceme --profile <profile> auth status`. If unauthenticated or missing `sdk-work:read`, `sdk-work:write`, `creator-app:read`, or `creator-app:write`, run `auth login` on the same Profile and wait for completion.
4. Publish the website to create its creator-owned Work if `.viceme/website.json` is absent:

   ```bash
   viceme --profile <profile> website publish \
     --path <website-dir> --name "<website name>" --url "https://<hostname>"
   ```

   Reuse the existing binding when present. Record the returned public `workKey`; never replace it with a fixture or a key supplied by another creator.
5. Enable hosted danmaku for that Work. If `.viceme/access.yaml` is absent, run:

   ```bash
   viceme --profile <profile> access init \
     --website <website-dir> --config <website-dir>/.viceme/access.yaml \
     --name "<website name>" --danmaku
   ```

   If the config exists, preserve unrelated features, add an active `danmaku` feature with `PUBLIC` policy, then run `access apply` once. Completion requires the response capabilities to contain `danmaku`.
6. Run `creator-app list` on the same Profile. Reuse an app only when it belongs to the authenticated user and contains the exact hostname. Otherwise create one with `creator-app create --name "<website name>"`.
7. If the hostname is not verified, run `creator-app domain add <appId> <hostname>`, serve the returned token verbatim at its `verificationPath`, deploy, fetch the public HTTPS verification URL, and run `creator-app domain verify <appId> <hostname>`. Do not print or commit the token. Completion requires `verified: true`.
8. Replace separate ViceMe danmaku or tip scripts with exactly one entry before `</body>`, using the selected Profile's exact `webBaseUrl`, the Work created in step 4, and the Creator App from step 6:

   ```html
   <script
     defer src="<webBaseUrl>/widget/engagement-embed.js" data-viceme-work="<workKey>" data-creator-app-id="<creatorAppId>"
     data-viceme-region="cn" data-viceme-target="body" data-viceme-theme="auto"
     data-locale="zh-CN"></script>
   ```

   Use `global` only when the active Profile is explicitly global. Do not derive region from locale.
9. Preserve every existing CSP directive. Add only the exact `webBaseUrl` required by `script-src` and `frame-src`; never add wildcards or `unsafe-eval`.
10. Run repository checks, deploy, and verify the real hostname at desktop and 320px widths. Confirm the script loads once, the server advertises `danmaku`, the bar keeps comment/more/tip controls reachable, a message persists after reload, the tip dialog opens, Escape closes it, and focus returns to the tip control. Report real payment completion as unverified unless a provider transaction was performed.

## Hard constraints

- Do not use a shared or pre-provisioned `workKey`; website publication is the authoritative creation path.
- Do not directly call Shop APIs, write the database, fabricate IDs, or read credentials from local storage.
- Do not copy the React blueprint, add host CSS for the widget, or install a second runtime.
- Do not keep standalone `tip-embed.js` or `viceme.min.js` tags beside the combined wrapper.
- Keep every CLI command pinned to the Profile selected at the start.
- Stop on a structured CLI error. Never switch to production or another Profile as a fallback.
