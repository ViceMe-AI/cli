---
name: viceme-access
description: Integrate the ViceMe browser SDK into an explicitly published creator website for WeChat login, following, feature access checks, and one-time work checkout. Use for `.viceme/access.yaml`, follow-gated UI, purchase-gated UI, or the lightweight `viceme access` workflow. If the website is not yet published, use $viceme-publish first, then resume access setup.
---

# ViceMe Website Access

Implement a browser-only integration backed by an existing creator-owned `workKey`. Keep identity, follow, purchase, and entitlement decisions server-authoritative.

## Workflow

1. Inspect the project framework, package manager, existing auth/payment code, and the exact UI elements to gate. Preserve existing conventions.
2. Inspect `<website-dir>/.viceme/website.json`. If it is missing or has no
   `workKey`, stop access setup and invoke `$viceme-publish` first. Follow its
   complete website metadata review and confirmation workflow. Resume this
   workflow only after explicit website publication succeeds. Never let an
   access command publish the website implicitly.
3. Run `viceme auth status`. If the token lacks `sdk-work:read` or `sdk-work:write`, ask the user to run `viceme auth login` again.
4. If `.viceme/access.yaml` does not exist, create and apply the complete
   access config in one command. Repeat feature flags as needed; use
   `key=title` only when the display title differs from the key:

   ```bash
   viceme access init --website <website-dir> --name "<website name>" \
     [--follow "<feature-key>[=<title>]"] \
     [--purchase "<feature-key>[=<title>]" --price-minor <fen>]...
   ```

   One price may be shared by all purchase features. To use different prices,
   repeat `--price-minor` once per `--purchase` in the same order. Each purchase
   feature creates or updates its own one-time sale offer.
   Creator subscriptions are not supported in this version.

5. Edit `.viceme/access.yaml` and run `viceme access apply` only for later
   configuration changes. Use `viceme access inspect` for diagnosis, not as
   a mandatory second request after a successful init.
6. Install the exact POC SDK `@viceme-ai/sdk-poc@0.1.7-poc.16` with the
   project's existing package manager.
   Create one client per `workKey`, await `ready()`, and call
   `access.require()` from the existing gated button. This is the default
   fast path; do not add an integration wrapper unless the framework requires it.
7. Use the SDK's `<viceme-access-layer>`. The ViceMe-owned component keeps login
   and checkout inside a mobile bottom sheet or desktop in-page layer. Do not
   infer, copy, or generate styles from the host page in this version.
8. Test anonymous, signed-in/unfollowed, followed, unpaid, paid, cancellation,
   embedded completion, keyboard interaction, and reduced-motion states.

Read [references/integration.md](references/integration.md) for configuration and code examples.

## Hard constraints

- Treat `workKey` as public and opaque. Never replace it with an offer ID or use it as a secret.
- `viceme access init` requires an explicitly published website binding and
  never publishes a website. It must not create a website release or replace
  the `$viceme-publish` review and confirmation workflow.
- Never add a Payment API key, webhook, amount, currency, product ID, creator ID, or price to browser code.
- Let the server resolve `workKey → CreatorWork → SaleOffer → Entitlement`.
- Do not store work-session tokens or access decisions in cookies, localStorage, IndexedDB, URLs, analytics, or logs.
- Never unlock from checkout return parameters or browser state. Only `access.check()` can grant access.
- Access checks must never mutate identity, follow, or payment state. Call `access.require()` only from an explicit user action. For a follow gate, the login consent layer must show the creator before authorization. Accepting login authorization signs in and automatically follows the creator without a second follow layer. Checkout still requires the user to select and confirm payment.
- Keep the ViceMe-owned consent UI intact: it shows the creator avatar above the
  display name and published-work count on one line, separated by `·`. The work
  count includes both published Skills and websites; up to two recent covers
  include one of each when both kinds are available.
  The description shows at most 50 characters and exposes the full text on
  hover. Its only primary action is `授权`, which opens the WeChat QR flow
  directly without an intermediate WeChat authorization page.
- Do not call `follow.follow()` from the host site's gate handler. Following belongs to the owner-follow interface opened by `access.require()`.
- Never use `window.open`, `window.location`, `confirm`, or `alert` for
  SDK login or checkout. Their complete flows stay in the bottom sheet or
  in-page layer and return through a validated SDK message channel.
- Never inject global feature CSS or alter the ViceMe layer from host-site styles. The Web Component stays isolated and ViceMe-owned.
- Use `FOLLOW_OWNER` for following. Do not model follow as a subscription.
- Do not configure `ACTIVE_CREATOR_SUBSCRIPTION`; it is reserved and unsupported in this version.
- Do not claim static assets embedded in a public bundle are protected. Gate behavior and fetch protected resources from a trusted backend when hard protection is required.
- Do not use the heavier ViceMe Hosted Checkout/Application flow for this integration.

## Completion checks

- Confirm `viceme access inspect` shows the expected work, feature-specific one-time offers, features, and capabilities.
- Confirm follow-gated and purchase-gated functions have separate state.
- Confirm all public SDK requests omit browser credentials and use the in-memory work session.
- Confirm follow state cannot change before the user accepts login authorization or activates a standalone follow action.
- Confirm no browser popup or page navigation is used and login/checkout remain
  inside the bottom sheet or in-page layer.
- Confirm WeChat QR and one-tap authorization remain fully visible, and the
  authorization frame keeps the consent layer's height without a visual host-page
  mask or transition jump. Confirm the checkout layer keeps a stable height while
  its content loads.
- Report any untested WeChat or payment-provider boundary explicitly.
