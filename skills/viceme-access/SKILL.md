---
name: viceme-access
description: Integrate the ViceMe browser SDK into a creator website for WeChat login, user-to-user following, feature access checks, and creator-product checkout. Use when Codex needs to add or repair ViceMe workKey setup, `.viceme/access.yaml`, follow-gated UI, purchase-gated UI, or the lightweight `viceme access` workflow in static HTML, React, Next.js, or similar browser projects.
---

# ViceMe Creator Access

Implement a browser-only integration backed by a creator-owned `workKey`. Keep identity, follow, and purchase decisions server-authoritative.

## Workflow

1. Inspect the project framework, package manager, existing auth/payment code, and the exact UI elements to gate. Preserve existing conventions.
2. Run `viceme auth status`. If the token lacks `sdk-work:read` or `sdk-work:write`, ask the user to run `viceme auth login` again.
3. If `.viceme/access.yaml` does not exist, create and apply the complete
   access config in one command. Repeat feature flags as needed; use
   `key=title` only when the display title differs from the key:

   ```bash
   viceme access init --name "<website name>" \
     [--follow "<feature-key>[=<title>]"] \
     [--product "<owned product slug>" --purchase "<feature-key>[=<title>]"]
   ```

4. Edit `.viceme/access.yaml` and run `viceme access apply` only for later
   configuration changes. Use `viceme access inspect` for diagnosis, not as
   a mandatory second request after a successful quick init.
5. Install `@viceme-ai/sdk` with the project's existing package manager.
   Create one client per `workKey`, await `ready()`, and call
   `access.require()` from the existing gated button. This is the default
   fast path; do not add an integration wrapper unless the framework requires it.
6. Use the SDK's `<viceme-access-layer>`. The ViceMe-owned component keeps login
   and checkout inside a mobile bottom sheet or desktop in-page layer. Do not
   infer, copy, or generate styles from the creator page in this version.
7. Test anonymous, signed-in/unfollowed, followed, unpaid, paid, cancellation,
   embedded completion, keyboard interaction, and reduced-motion states.

Read [references/integration.md](references/integration.md) for configuration and code examples.

## Hard constraints

- Treat `workKey` as public and opaque. Never replace it with a product slug or use it as a secret.
- Never add a Payment API key, webhook, amount, currency, product ID, creator ID, or price to browser code.
- Let the server resolve `workKey → owner → bound SkillProduct`.
- Do not store work-session tokens or access decisions in cookies, localStorage, IndexedDB, URLs, analytics, or logs.
- Never unlock from checkout return parameters or browser state. Only `access.check()` can grant access.
- Access checks must never silently sign in, follow, or pay. The SDK may show the required interface; the user must activate the action inside it.
- Do not call `follow.follow()` from the creator site's gate handler. Following belongs to the creator-follow interface opened by `access.require()`.
- Never use `window.open`, `window.location`, `confirm`, or `alert` for
  SDK login or checkout. Their complete flows stay in the bottom sheet or
  in-page layer and return through a validated SDK message channel.
- Never inject global feature CSS or alter the ViceMe layer from creator-site styles. The Web Component stays isolated and ViceMe-owned.
- Use `FOLLOW_OWNER` for following. Do not model follow as a subscription.
- Do not configure `ACTIVE_CREATOR_SUBSCRIPTION`; it is reserved and unsupported in this version.
- Do not claim static assets embedded in a public bundle are protected. Gate behavior and fetch protected resources from a trusted backend when hard protection is required.
- Do not use the heavier ViceMe Hosted Checkout/Application flow for this integration.

## Completion checks

- Confirm `viceme access inspect` shows the expected work, product binding, features, and capabilities.
- Confirm follow-gated and purchase-gated functions have separate state.
- Confirm all public SDK requests omit browser credentials and use the in-memory work session.
- Confirm an access check cannot mutate follow state before the user activates the follow action.
- Confirm no browser popup or page navigation is used and login/checkout remain
  inside the bottom sheet or in-page layer.
- Report any untested WeChat or payment-provider boundary explicitly.
