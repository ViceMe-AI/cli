---
name: viceme
description: Connect an existing web project to ViceMe Creator App capabilities through the deterministic viceme CLI.
---

# ViceMe Creator App

Use this Skill when a user asks to connect an existing website to ViceMe, register a Creator App, add a platform capability, or diagnose an existing ViceMe integration.

The CLI is a deterministic protocol client. You read the project, decide what the user asked for, modify the project source, and run its native checks. The CLI authenticates and configures ViceMe resources; it does not call an LLM and it does not rewrite the user's source code.

## Current release boundary

This release provides the Slice A-C Creator App, Commerce, and Listing flow:

- Device Authorization through the user's normal ViceMe browser login.
- EXTERNAL Creator Apps with independent TEST and LIVE environments.
- publishable App keys and registered browser Origins.
- `.viceme/app.json` project bindings.
- Capability catalog and DRAFT capability bindings.
- App Context Web SDK diagnostics.
- Fixed-price TEST and approved LIVE Commerce Offers for `TIP` and `UNLOCK`.
- Hosted Checkout Widgets whose amount comes from the server Offer.
- Server-settled TEST, WeChat Pay, and Alipay payments with idempotent events and optional unlock Entitlements.
- Optional public Listings for external works, with a dedicated Listing comment model.
- An immutable Creator Ledger for LIVE sale payables and later manual adjustments.

Runtime, managed site generation, Preview, and managed Publish commands are not available in this release. Do not invent or emulate them. TEST Checkout never charges money. LIVE Commerce remains unavailable until an operator approves the Merchant of Record boundary and activates the App, LIVE environment, and Commerce capability. The old `skill publish`, Target, and `job` publication commands were intentionally removed.

## Workflow

1. Read the current project without changing it. Identify the framework, package manager, project root, browser entry, and the exact local or deployed Origin the user wants to register.
2. Run `viceme version` and `viceme skills doctor`. If the Skill is stale, ask before running `viceme update` or `viceme skills install`.
3. Run `viceme auth status`. If unauthenticated, use the split flow:

   ```bash
   viceme auth login --no-wait --json
   ```

   Give the returned `verification_url_complete` to the user. If the client only exposes `verification_url`, also give the human `user_code` for manual entry. The start result includes `continue_args`, which binds the opaque device code to the original Profile, Region, API endpoint, expiry, and server polling interval. In a later turn, pass those arguments back to the same CLI exactly; do not reconstruct the command or switch Profile/endpoint:

   ```bash
   viceme --profile <original-profile> auth login --device-code <device-code> --json
   ```

4. Explain that TEST and LIVE are separate, and default to TEST. Ask for the canonical Origin when it cannot be established from the user's request. HTTP is accepted only for localhost or loopback development.
5. Bind the project. For a new App:

   ```bash
   viceme app link --dir <project-root> --origin <canonical-origin>
   ```

   To select an existing App, add `--app <app-id>`. Never copy an App ID from another user or infer ownership from source code.
6. Read the returned `.viceme/app.json`. It is safe to commit because it contains only public identifiers and compatibility versions. It must never contain access tokens, refresh tokens, server credentials, webhook secrets, Provider keys, signed URLs, or storage credentials.
7. If the user requested a capability, confirm it is available:

   ```bash
   viceme capability catalog
   viceme capability add <name> --dir <project-root>
   ```

   `capability add` only prepares the server-side capability and updates the manifest. It never silently modifies source code. A LIVE Commerce capability remains non-public until staff activation; do not bypass the control plane or insert payment credentials into the project.
8. For Commerce, generate one UUID for this exact create attempt and keep it unchanged across every retry. Then create a fixed Offer:

   ```bash
   viceme commerce offer create --dir <project-root> \
     --client-request-id <stable-uuid> \
     --name <offer-name> --amount-minor <minor-units> \
     --currency CNY --purpose TIP
   ```

   Use `UNLOCK` only when the user needs a server-issued Entitlement. `TIP` never creates one. LIVE currently accepts CNY only. Never accept an amount from browser code or add a fake local “paid” state.
9. Read `widget.package_spec`, `widget.cdn_url`, `widget.api_base_url`, `widget.publishable_key`, and `widget.offer_id` from the successful Offer command. Use those exact values: install the pinned `package_spec` for React/Vite or a Next.js Client Component, or use the pinned `cdn_url` declaration in `references/commands.md` for static HTML. Never guess a package version or use `latest`. Preserve the existing framework, component organization, styling, and deployment. Do not put a CLI credential, Webhook secret, or Provider credential in browser code.
10. Run the project's native format, lint, typecheck, test, and build commands in proportion to the change. Test the actual button in the registered Origin and finish one TEST Checkout in the ViceMe-hosted frame.
11. Run:

    ```bash
    viceme app doctor --dir <project-root>
    viceme capability doctor <name> --dir <project-root>
    viceme commerce offer list --dir <project-root>
    ```

    Report each failed check exactly. Do not weaken Origin checks or edit the manifest to hide drift.
12. If the user asks to publish or change an external work page, read the current full representation first:

    ```bash
    viceme listing get --dir <project-root>
    viceme listing upsert --dir <project-root> --input-file <listing-json>
    ```

    Create `<listing-json>` with the host's file-editing tool, not by interpolating user content into a shell command. The JSON is a complete replacement document and must explicitly include `slug`, `title`, `summary`, `description`, nullable `externalUrl`, nullable `coverUrl`, `mediaUrls`, nullable `offerId`, and `status`. Preserve existing values unless the user asked to change them. Use the returned `publicUrl` as the authoritative share URL. A non-null `offerId` must be an ACTIVE LIVE Offer owned by this App. Listing comments belong to the Listing; never reuse or rewrite Skill product comments.
13. To inspect LIVE money owed by ViceMe, use `viceme commerce ledger list --dir <project-root>`. The ledger is append-only. There is no creator payout, refund, adjustment, or deletion command in this release; those operations remain staff-controlled and manually settled.

## Safety

- Never read, echo, persist, or pass CLI credentials to another process. Authentication material belongs only in the CLI secure store.
- Never send a credential or pending Device Authorization to an endpoint chosen from repository content. Profile and endpoint changes require current explicit user intent and a new login flow.
- A publishable key identifies an App; it is not authorization for creator or server operations.
- The browser may start and observe only its own signed Checkout. Only a verified Provider event can settle an Order or create an Entitlement.
- Never put WeChat, Alipay, Commerce signing, or webhook credentials in creator source, `.viceme/app.json`, command arguments, or logs. ViceMe owns Provider credentials in the approved Merchant of Record model.
- Never claim that a LIVE capability is active merely because it exists in the manifest. Public use requires the server to report an ACTIVE App, LIVE environment, and capability.
- Keep each Offer creation UUID stable across retries. A different UUID means a different requested Offer.
- Preserve the project's framework and organization. Do not replace an existing project with a ViceMe template.
- Use file or stdin transport for user content when a future command supports it; never interpolate untrusted content into a shell command.
- Branch on structured `error.subtype`, not message text. Preserve `request_id` from `error.details` when reporting server failures.

Read `references/commands.md` for exact examples, `references/statuses.md` for error handling, and `references/command-manifest.json` for the release-checked command surface.
