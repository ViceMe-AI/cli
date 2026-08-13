---
name: viceme-checkout
description: Configure, integrate, and test ViceMe Hosted Checkout for an application. Use when a user asks to add payments, create a Payment product or one-time price, build a hosted checkout flow, register return URLs or signed webhooks, issue, rotate, or safely deliver a Payment API Key to a backend dotenv file, test a SANDBOX checkout, connect an Admin-enabled CN WeChat Pay or Alipay LIVE environment, query or close an order, or diagnose an existing .viceme/payment.yaml integration.
---

# ViceMe Hosted Checkout

Use the `viceme payment` CLI for every ViceMe control-plane and runtime operation. Keep application code, local project context, and remote Payment resources consistent without exposing secrets to the Agent.

Read [control-plane.md](references/control-plane.md) before creating or changing remote resources. Read [runtime.md](references/runtime.md) before changing application code or testing a checkout. Read [errors.md](references/errors.md) when a command fails.

## Workflow

1. Run `viceme auth status`. Capture the active `profile`, then pin every later command with `viceme --profile <profile> ...`. Do not silently switch profiles.
2. If unauthenticated, run `viceme --profile <profile> auth login`. Complete the device flow before reading or writing Payment state.
3. Inspect the repository for its backend framework, server-only dotenv convention or deployment secret provider, existing order model, webhook endpoint, and success/cancel routes. Payment API Keys are server-only credentials; never place them in browser code.
4. Run `viceme --profile <profile> payment context --dir <project>`. If the project is not initialized, choose a stable application slug and name, show them to the user, obtain approval for the remote write, then run `payment init`.
5. Run `payment eligibility`. A user without a claimed Creator may configure resources but cannot issue or rotate a Payment API Key. SANDBOX and LIVE environments exist by default. For LIVE, run `payment environment use live`; API Key creation remains blocked until an authorized Admin enables LIVE API Key issuance for the Application.
6. Prepare strict JSON files for Product, Price, and Price Version. Show the exact Product code, Price code, amount, currency, and channels before creating them. Activate a Price Version only after the user accepts those immutable terms. Checkout Template creation is optional: omit it to use the persisted ViceMe platform default, and create a custom template only when the user asks to change the hosted checkout presentation.
7. Register and verify an application origin before creating a return target. Browser return is navigation only; never grant an entitlement from a success URL.
8. Implement the application backend integration from [runtime.md](references/runtime.md). Use a stable idempotency key, persist `externalOrderNo`, and treat API query or a verified Webhook as payment truth.
9. If Webhooks are needed, create the receiving endpoint in application code first. Register it with `payment webhook create`; the CLI stores the one-time signing secret securely and never prints it. For a local backend, run `payment webhook deliver <endpoint-id> --env-file <project-relative-.env-file>` so the CLI copies it directly from secure storage into `VICEME_PAYMENT_WEBHOOK_SECRET`. Run it again after secret rotation. Do not claim deployment is complete unless the application's deployment secret provider has been configured by an authorized path.
10. Ask before issuing a Payment API Key. Run `payment api-key create`; the CLI writes the one-time key to secure storage and returns only non-secret metadata. Creation refuses to replace an existing project/environment credential; use `rotate` or explicitly revoke it first. For a local backend, run `payment api-key deliver --env-file <project-relative-.env-file>` so the CLI copies it directly from secure storage into `VICEME_PAYMENT_API_KEY`. Never read or reconstruct the value. The command refuses tracked, escaping, symlink, example, and duplicate-variable targets, writes an exact `.gitignore` rule, and protects the file for the current user. Use an authorized deployment secret provider instead of a repository dotenv file for remote production deployment.
11. Run `payment checkout products`, then create a checkout with a new external order number and stable idempotency key. Treat only the returned channels as currently available. Omit `templateCode` to use the Product default or ViceMe platform default; send it only for an existing custom template bound to that Product. In SANDBOX, complete the simulator action. In LIVE, require `CN` and an Admin-authorized LIVE API Key, then use a returned `WECHAT_PAY` or `ALIPAY` channel and let the payer complete the real channel action. Return the authoritative `checkoutUrl` and query the order afterward.
12. Report created resource IDs, changed code, validation commands, and any unverified deployment or Webhook boundary.

## Safety boundaries

- LIVE support is limited to CN WeChat Pay and Alipay one-time payments whose API Key issuance was enabled by an Admin and whose platform-owned channel is currently available. Application authorization grants access to all globally enabled LIVE channels; do not invent a per-Application channel allowlist, create LIVE authorization, request provider credentials, or change platform Merchant Routes.
- Never request, display, copy, log, commit, or place a Payment API Key or Webhook Signing Secret in a prompt, JSON input, `.env.example`, Skill file, screenshot, or source file. Use `payment api-key deliver` for a local backend so the secret moves directly from secure storage to the protected dotenv file.
- Keep `.viceme/payment.yaml`; it contains non-secret application and environment IDs. Do not treat it as authorization.
- Use the Payment API Key only for `/v1/checkout/v1/*`. Use the user CLI credential only for control-plane commands.
- Require explicit confirmation for immediate API Key/Webhook revocation and order close. Rotation normally preserves a 24-hour overlap; if secure persistence fails, the CLI aborts the rotation.
- Do not implement refunds, subscriptions, provider credential setup, merchant-route rotation, or financial reconciliation under this Skill. They remain operator-owned boundaries.
