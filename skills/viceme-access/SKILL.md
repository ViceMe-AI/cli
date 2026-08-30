---
name: viceme-access
description: Integrate the ViceMe browser SDK into a published creator website for host-native login, follow, and paid feature entry points backed by the Work's existing SDK access resource.
---

# ViceMe Website Access

Add the smallest browser-only gate around an existing user action. The published
Website Work and its single `WorkSdkAccess` resource are authoritative. Reuse that
resource when danmaku or tip is already enabled; never create a parallel access
resource or local mirror of remote state.

## Workflow

1. Inspect the requested entry point, unchanged business action, component system,
   styling tokens, and tests. If feature key, policy, entry point, and price are
   already confirmed, implement them directly. Otherwise ask only for the missing
   decisions.
2. Confirm the website is already published through `$viceme-publish`, and take the
   Website Work ID from that result or `viceme merchant work list`.
3. Run `viceme auth status`. Reauthenticate if `merchant-commerce:read` or
   `merchant-commerce:write` is missing.
4. Read the one authoritative resource:

   ```bash
   viceme merchant work sdk-access get <work-id> --merchant <merchant-id>
   ```

5. If it does not exist, create it with the complete desired hosted and access
   features. If it exists, update only the access domain: omitted `--feature`
   values preserve existing danmaku and tip configuration.

   ```bash
   viceme merchant work sdk-access create <work-id> --merchant <merchant-id> \
     [--feature danmaku] [--feature tip] \
     [--follow "<key>[=<title>]"] \
     [--purchase "<key>[=<title>]" --price-minor <fen>]...

   viceme merchant work sdk-access update <work-id> --merchant <merchant-id> \
     --expected-config-version <version> \
     [--follow "<key>[=<title>]"] \
     [--purchase "<key>[=<title>]" --price-minor <fen>]...
   ```

   One price may be shared by all purchase features. To use different prices,
   repeat `--price-minor` once per `--purchase` in the same order. Use
   `--clear-access` to remove all follow and purchase features. Never use the
   whole-resource `disable` command when danmaku or tip must remain active.
6. Install `@viceme-ai/sdk`, create one client per `workKey`, and call
   `access.require()` from the existing user action. Fetch titles and prices with
   `access.getFeatures()` instead of hard-coding them.
7. Keep the original allowed action unchanged. Reuse the host's Button, Card,
   Dialog, breakpoints, focus behavior, loading state, and error feedback. The
   ViceMe access layer remains isolated and must not be restyled.
8. Test anonymous, signed-in/unfollowed, followed, unpaid, paid, cancellation,
   popup completion, keyboard interaction, and reduced motion.

Read [references/integration.md](references/integration.md) for examples.

## Hard constraints

- Treat `workKey` as public and opaque.
- Never put payment keys, Product IDs, prices, access decisions, or session tokens
  in browser storage, URLs, logs, or analytics.
- Let the server resolve `workKey → Work → WorkAccessFeature → Product →
  DigitalEntitlement`.
- Login must not automatically follow. Following remains a separate explicit action.
- Checkout must reuse CommerceApplication, Hosted Checkout, Order, fulfillment,
  and DigitalEntitlement. A payment return parameter never grants access;
  `access.check()` is authoritative.
- Static files already shipped in a public bundle cannot be protected retroactively.

## Completion checks

- The original danmaku and tip features remain unchanged after access configuration.
- `merchant work sdk-access get` shows the expected access features and Products.
- The denied path never calls the protected action; the allowed path preserves its
  arguments, result, errors, and side effects.
- Sign-in and checkout open through the SDK flow without navigating the creator
  page, and access unlocks only after a fresh server decision.
- Report untested WeChat and payment-provider boundaries.
