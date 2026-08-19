---
name: viceme-tip
description: Integrate ViceMe tips into one public creator HTML page. Use when a user asks to add, install, embed, configure, or repair a ViceMe tip or appreciation button, including signing in, binding a creator work and domain, and verifying the real hosted checkout.
---

# Integrate ViceMe tips

Build the smallest real creator integration: one host-owned HTML page plus the ViceMe embed script. Read [integration-contract.md](references/integration-contract.md) before editing.

## Workflow

1. Run `viceme profile list` and capture the active Profile without switching it. Pin every later CLI command to that Profile. Completion: one Profile and region are explicit.
2. Run `viceme --profile <profile> auth status`. When unauthenticated, run `viceme --profile <profile> auth login`, show the complete verification URL once, and wait for successful authorization. Completion: the selected Profile reports `authenticated: true`.
3. Inspect the target repository instructions and locate the one public HTML page, its deployed HTTPS origin, deployment mechanism, and any Content Security Policy. Prefer an existing page; use [single-html.html](templates/single-html.html) only when the user needs a new example. Completion: the exact HTML file and public hostname are known.
4. Search that HTML for an existing ViceMe tip script and `data-creator-app-id`. Cross-check with `viceme --profile <profile> creator-app list`: treat the work as bound only when the Creator App ID belongs to the user and the exact public hostname is verified on that app. A script tag alone is not proof of binding.
5. When the work is not bound, create and bind it through the CLI — never ask the user to operate the Creator Center page:
   1. `viceme --profile <profile> creator-app create --name <work-name>`; record `data.app.id`. The first call also establishes the creator identity automatically, so a "not a creator yet" detour is never required.
   2. `viceme --profile <profile> creator-app domain add <appId> <hostname>`; record the domain entry's `verificationToken` and `verificationPath`.
   3. Serve that token verbatim at `<hostname><verificationPath>` over HTTPS and deploy it.
   4. `viceme --profile <profile> creator-app domain verify <appId> <hostname>`; on failure, fetch the verification URL yourself, compare it with the recorded token, fix the hosting, and retry — do not ask the user to click anything.
   Completion: the domain entry reports `verified: true`.
   If the platform returns 404 for `creator-app` commands, stop and report that this exact selected environment has not deployed the binding capability; do not fabricate a work or URL, and do not fall back to a production origin.
6. Build the embed snippet with `viceme --profile <profile> creator-app show <appId>`; use its `data.embedSnippet` exactly — the CLI composes it from the Profile's `webBaseUrl`, so never derive or invent the script URL or Creator App ID. Insert one idempotent script tag before `</body>`:

```html
<script
  async
  src="<generated-widget-script-url>"
  data-creator-app-id="<creator-app-id>"
  data-locale="zh-CN"
></script>
```

   Preserve all host content and inline the host's own CSS/JavaScript when the deliverable must remain one HTML file. Completion: the page contains exactly one matching embed script.
7. If the page has a CSP, minimally allow the exact platform origin required by `script-src` and `frame-src`; preserve every existing directive. Do not add wildcards or weaken unrelated directives. Completion: the browser loads both the embed script and checkout iframe without CSP violations.
8. Run local syntax/static checks, then deploy through the repository's existing path. Verify the public HTTPS page, not only localhost: the script returns 200, the circular tip launcher is visible, keyboard activation opens the ViceMe checkout, Escape closes it, and browser console/network contain no widget errors. Completion: the real verified hostname opens the hosted checkout.
9. Report the HTML file, public URL, Creator App ID, verification URL, checks run, and any unverified payment boundary. Never report login tokens, verification tokens, cookies, payment credentials, or signed URLs.

## Boundaries

- The creator signs in to configure the work; visitors do not need to sign in merely to open the tip checkout.
- ViceMe owns checkout, payment provider calls, order state, and settlement. The host page owns only its content and embed tag.
- `creatorAppId` identifies the payee work. Reusable templates use a placeholder; a creator's own deployed work may use that creator's real ID.
- Domain verification is a prerequisite outside the one-HTML embed surface. The final integration can remain one HTML file even when deployment separately serves the verification response.
- Start with the standalone launcher. Use an integrated custom dock only when the deployed embed script explicitly documents and proves that protocol.

## Recovery

- Login timeout: rerun `auth login` on the same Profile.
- Existing unverified domain: reuse it and re-deploy its current token; do not create another work.
- Existing verified work: reuse its Creator App ID; do not create a duplicate merely because the HTML moved.
- Wrong hostname: bind the exact production hostname. Do not treat localhost, a preview hostname, and production as interchangeable.
- Widget missing: verify script status, Creator App ID, hostname binding, CSP, and duplicate script tags in that order.
