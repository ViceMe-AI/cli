---
name: viceme-access
description: Integrate the ViceMe browser SDK into a creator website for WeChat login, user-to-user following, feature access checks, and creator-product checkout. Use when Codex needs to add or repair ViceMe workKey setup, `.viceme/access.yaml`, follow-gated UI, purchase-gated UI, or the lightweight `viceme access` workflow in static HTML, React, Next.js, or similar browser projects.
---

# ViceMe Creator Access

Implement a browser-only integration backed by a creator-owned `workKey`. Keep identity, follow, and purchase decisions server-authoritative.

## Workflow

1. Inspect the project framework, package manager, existing auth/payment code, and the exact UI elements to gate. Preserve existing conventions.
2. Run `viceme auth status`. If the token lacks `sdk-work:read` or `sdk-work:write`, ask the user to run `viceme auth login` again.
3. If `.viceme/access.yaml` does not exist, run:

   ```bash
   viceme access init --name "<website name>" --origin "<website origin>" [--product "<owned product slug>"]
   ```

4. Edit `.viceme/access.yaml`. Configure one feature per independently gated UI function. Do not use one shared unlock flag for different policies.
5. Run `viceme access apply`, then `viceme access inspect`. Treat the returned capabilities as authoritative.
6. Install `@viceme-ai/sdk` with the project's existing package manager. Create one client per `workKey`, await `ready()`, and destroy it when the owning application lifecycle ends.
7. Use `access.checkMany()` to render initial states. Use `access.require()` only from a user gesture. Re-check after returning from checkout.
8. Test anonymous, signed-in/unfollowed, followed, unpaid, paid, cancellation, popup-blocked, and disallowed-Origin states.

Read [references/integration.md](references/integration.md) for configuration and code examples.

## Hard constraints

- Treat `workKey` as public and opaque. Never replace it with a product slug or use it as a secret.
- Never add a Payment API key, webhook, amount, currency, product ID, creator ID, or price to browser code.
- Let the server resolve `workKey → owner → bound SkillProduct`.
- Do not store work-session tokens or access decisions in cookies, localStorage, IndexedDB, URLs, analytics, or logs.
- Never unlock from checkout return parameters or browser state. Only `access.check()` can grant access.
- Use `FOLLOW_OWNER` for following. Do not model follow as a subscription.
- Do not configure `ACTIVE_CREATOR_SUBSCRIPTION`; it is reserved and unsupported in this version.
- Do not claim static assets embedded in a public bundle are protected. Gate behavior and fetch protected resources from a trusted backend when hard protection is required.
- Do not use the heavier ViceMe Hosted Checkout/Application flow for this integration.

## Completion checks

- Confirm `viceme access inspect` shows the expected work, product binding, Origins, features, and capabilities.
- Confirm follow-gated and purchase-gated functions have separate state.
- Confirm all public SDK requests omit browser credentials and use the in-memory work session.
- Report any untested WeChat or payment-provider boundary explicitly.
