---
name: viceme-access
description: Integrate the ViceMe browser SDK into a creator website for host-native login, follow, and paid feature entry points backed by ViceMe access checks and checkout. Use for `.viceme/access.yaml`, feature-gated UI, or the `viceme access` workflow. Follow an explicit integration plan directly; analyze and propose safe entry points only when the plan is missing or incomplete. If the website is not yet published, use $viceme-publish before applying access configuration.
---

# ViceMe Website Access

Implement a browser-only integration backed by an existing creator-owned `workKey`. Keep identity, follow, purchase, and entitlement decisions server-authoritative. Preserve the website's core behavior and visual language; ViceMe owns the authorization and checkout layer, while the host owns its access entry points.

## Workflow

1. Inspect enough of the project to locate the requested action, existing
   auth/payment code, component system, styling tokens, and tests. Choose the
   shortest applicable path:
   - When the user already supplied the paid/follow feature, entry point,
     behavior, and price, implement that plan directly. Do not require a separate
     analysis or approval round and do not reinterpret confirmed choices.
   - When only part of the plan is missing, analyze and resolve only the missing
     parts.
   - When no concrete plan exists, inspect the core user journey and rendered UI,
     then propose safe feature keys, entry points, policies, host components to
     reuse, protected actions, and prices that still require the user's decision.
     Do not write access configuration or host code until the user selects a plan.
2. Keep the original business action intact. Add the smallest host-side gate at
   its existing user-triggered entry, then call the unchanged action only after
   `access.require()` returns `allowed: true`. If no safe outer seam exists,
   report the coupling and stop instead of refactoring core behavior to force an
   integration.
3. Inspect `<website-dir>/.viceme/website.json`. If it is missing or has no
   `workKey`, stop access setup and invoke `$viceme-publish` first. Follow its
   complete website metadata review and confirmation workflow. Resume this
   workflow only after explicit website publication succeeds. Never let an
   access command publish the website implicitly.
4. Run `viceme auth status`. If it does not report an authenticated user, ask the user to run `viceme auth login` again.
5. If `.viceme/access.yaml` does not exist, create and apply the complete
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

6. Edit `.viceme/access.yaml` and run `viceme access apply` only for later
   configuration changes. Use `viceme access inspect` for diagnosis, not as
   a mandatory second request after a successful init.
7. Install `@viceme-ai/sdk` with the project's existing package manager.
   Create one client per `workKey`, await `ready()`, and call
   `access.require()` from the existing user action. Use `access.getFeatures()`
   when a host-native entry displays the current title or price; never duplicate
   those values in browser code. Do not add an integration abstraction unless
   the framework requires it.
8. Reuse the host site's existing Button, Card, Dialog, icon library, design
   tokens, responsive behavior, loading state, and error feedback for host-owned
   locked states and access entry points. Add only minimal local styling when no
   existing primitive fits. Do not introduce global access CSS or a parallel
   ViceMe visual language.
9. Use the SDK's `<viceme-access-layer>`. The ViceMe-owned component keeps login
   and checkout inside a mobile bottom sheet or desktop in-page layer. Do not
   restyle it, pierce its Shadow DOM, or copy host styles into it.
10. Test anonymous, signed-in/unfollowed, followed, unpaid, paid, cancellation,
   embedded completion, keyboard interaction, and reduced-motion states.

Read [references/integration.md](references/integration.md) for configuration and code examples.

## Hard constraints

- Treat `workKey` as public and opaque. Never replace it with an offer ID or use it as a secret.
- `viceme access init` requires an explicitly published website binding and
  never publishes a website. It must not create a website release or replace
  the `$viceme-publish` review and confirmation workflow.
- Never add a Payment API key, webhook, product ID, creator ID, or hard-coded amount, currency, or price to browser code. Host-native price presentation must come from `access.getFeatures()`.
- Let the server resolve `workKey → CreatorWork → SaleOffer → Entitlement`.
- Do not store work-session tokens or access decisions in cookies, localStorage, IndexedDB, URLs, analytics, or logs.
- Never unlock from checkout return parameters or browser state. Only `access.check()` can grant access.
- Access checks must never mutate identity, follow, or payment state. Call `access.require()` only from an explicit user action. For a follow gate, the login consent layer must show the creator before authorization. Accepting login authorization signs in and automatically follows the creator without a second follow layer. Checkout still requires the user to select and confirm payment.
- Never rewrite, move, rename, or change the parameters, return value, errors, or
  side effects of the website's core business action for access integration.
  Keep the gate as an outer adapter and preserve the original allowed path.
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
- Never inject global feature CSS or alter the ViceMe layer from host-site styles. The Web Component stays isolated and ViceMe-owned; only the host-owned entry uses the site's design system.
- Use `FOLLOW_OWNER` for following. Do not model follow as a subscription.
- Do not configure `ACTIVE_CREATOR_SUBSCRIPTION`; it is reserved and unsupported in this version.
- Do not claim static assets embedded in a public bundle are protected. Gate behavior and fetch protected resources from a trusted backend when hard protection is required.
- Do not use the heavier ViceMe Hosted Checkout/Application flow for this integration.

## Completion checks

- Confirm `viceme access inspect` shows the expected work, feature-specific one-time offers, features, and capabilities.
- Confirm follow-gated and purchase-gated functions have separate state.
- Confirm the allowed path still calls the original core action without changing
  its arguments, result, errors, or side effects, and the denied/cancelled path
  does not call it.
- Confirm host-owned access entry points match the existing component variants,
  focus behavior, breakpoints, theme, loading state, and feedback patterns.
- Confirm all public SDK requests omit browser credentials and use the in-memory work session.
- Confirm follow state cannot change before the user accepts login authorization or activates a standalone follow action.
- Confirm no browser popup or page navigation is used and login/checkout remain
  inside the bottom sheet or in-page layer.
- Confirm WeChat QR and one-tap authorization remain fully visible, and the
  authorization frame keeps the consent layer's height without a visual host-page
  mask or transition jump. Confirm the checkout layer keeps a stable height while
  its content loads.
- Report any untested WeChat or payment-provider boundary explicitly.
