# ViceMe CLI 1.0 commands

## Install and authentication

```bash
viceme install --target auto
viceme update --check
viceme version
viceme auth status
viceme auth login --no-wait --json
viceme --profile <original-profile> auth login --device-code <opaque-device-code> --json
viceme auth logout
```

The first command returns exact `continue_args`; use them unchanged. The CLI keeps only hashed-key pending metadata in its secure store and rejects a continuation under another Profile, Region, or exact API base URL. API paths are part of the credential namespace, so `/production` and `/staging` never share credentials even on the same Origin. `--device-code` is an opaque one-time authorization value. Do not log it, put it in project files, or confuse it with the human `user_code` shown in the browser.

Refresh recovery is automatic and local to the secure store. Before contacting the API, the CLI persists a request UUID beside the current refresh credential. A lost response is retried with the same old credential and request UUID; a different UUID is token reuse, not a second recovery attempt. The entire refresh transaction is serialized through a stable per-user process lock that does not follow `VICEME_CLI_CONFIG_DIR`, matching the shared OS Keyring identity. `auth logout` first completes and persists any pending refresh recovery, then keeps the current local access credential until the idempotent remote revocation is confirmed.

## Profiles and bundled Skill

```bash
viceme profile list
viceme profile add --name global --region global
viceme profile use global
viceme profile configure global --api-base-url https://api.viceme.ai
viceme profile rename global worldwide
viceme profile remove global

viceme skills list
viceme skills read viceme
viceme skills install --target codex
viceme skills doctor --target codex
```

## Creator App binding

```bash
# Create an EXTERNAL App and bind its TEST environment.
viceme app link --dir . --origin http://localhost:3000

# Bind an existing owned App.
viceme app link --dir . --app <app-id> --environment TEST \
  --origin https://project.example

viceme app get <app-id>
viceme app list
viceme app doctor --dir .
```

`app link` acquires a per-user process lock keyed by the canonical project path, then persists a non-secret `.viceme/app-link-pending.json` idempotency intent before remote creation, writes `.viceme/app.json` atomically, and removes the intent only after the manifest commits. Retrying after a lost response or starting two concurrent first-link commands therefore resolves the same App. The manifest is public project metadata. `app doctor` verifies App ownership, environment/key drift, server-returned Contract/SDK versions, Origin registration, and the public Widget context.

## Capability and Commerce TEST/LIVE

```bash
viceme capability catalog
viceme capability add commerce --dir .
viceme capability get commerce --dir .
viceme capability doctor commerce --dir .

# Generate one UUID in the Agent host and reuse it for retries of this exact operation.
viceme commerce offer create --dir . \
  --client-request-id <stable-uuid> \
  --name "Support this work" \
  --amount-minor 1 \
  --currency CNY \
  --purpose TIP

viceme commerce offer list --dir .
viceme commerce ledger list --dir . --limit 50
```

`amount-minor` is the fixed server-owned amount in the currency's minor unit. `TIP` completes without an Entitlement; `UNLOCK` creates one only after the server processes a verified Provider event. TEST is immediately usable and never charges money. LIVE supports CNY and remains unavailable until ViceMe staff approves the Merchant of Record boundary and activates the App, environment, and capability. `commerce ledger list` returns immutable sale payables and later staff-controlled debit/adjustment entries; it never mutates money. Identity, Storage, and Runtime remain planned and `capability add` rejects them until their implementation Slice lands.

## Public Listing

`listing upsert` accepts a complete replacement document so an omitted flag
cannot silently clear an existing Offer or media item. Run `listing get` first,
then write a JSON file with the host's file-editing tool:

```json
{
  "slug": "my-public-work",
  "title": "My public work",
  "summary": "A short public summary",
  "description": "The full public description",
  "externalUrl": "https://project.example",
  "coverUrl": "https://cdn.example/cover.webp",
  "mediaUrls": ["https://cdn.example/one.webp"],
  "offerId": null,
  "status": "PUBLIC"
}
```

```bash
viceme listing get --dir .
viceme listing upsert --dir . --input-file ./listing.json
# Use --input-file - only when the complete JSON already arrives on stdin.
```

Every key in the example is required; nullable values use explicit JSON `null`
and `mediaUrls` is always an array. Do not construct this document through
shell interpolation. An EXTERNAL App must have a safe HTTPS (or loopback
development) `externalUrl` before it can become PUBLIC. A non-null `offerId`
must identify the same App's ACTIVE LIVE Offer. The server-returned `publicUrl`
is the share URL; do not construct it from the slug or a guessed domain.
Listing comments use a dedicated public API and are not Skill product comments.

## App Context shell

Install the exact `widget.package_spec` returned by `commerce offer create`
using the project's existing package manager. For the current
`.viceme/app.json` binding that is:

```bash
pnpm add @viceme-ai/web-sdk@0.1.0
```

Use the equivalent exact-version command for npm, yarn, or bun when that is the
project's existing package manager. Never install `latest`. Then mount it from
client-side code:

```ts
import { mountAppContextWidget } from "@viceme-ai/web-sdk";

mountAppContextWidget(document.querySelector("#viceme-app")!, {
  apiBaseUrl: "<api_base_url from app link>",
  publishableKey: "<publishableKey from .viceme/app.json>",
});
```

The browser package sends no CLI token. The API validates the request Origin against the App environment and returns only the public App name, environment, and capability projection.

For static HTML, use the exact `widget.cdn_url` returned by the Offer command.
It is derived from the manifest's `sdkPackage` and `sdkVersion`, auto-mounts
declared targets, and has no external module dependency.

## Hosted Checkout Widget

The Widget opens a ViceMe-hosted checkout. It never receives a payment credential and never sends an amount.

Static HTML:

```html
<script
  src="https://cdn.jsdelivr.net/npm/@viceme-ai/web-sdk@0.1.0"
  data-viceme-api-base="<api_base_url from app link>"
  data-viceme-app-key="<publishableKey from .viceme/app.json>"
  defer
></script>
<button type="button" data-viceme-checkout="<offer id>">
  Support this work
</button>
```

React/Vite or a Next.js Client Component:

Install `widget.package_spec` with the project's existing package manager
before adding the component. The example version must equal
`.viceme/app.json -> capabilities.commerce.sdkVersion`.

```tsx
"use client";

import { mountCommerceCheckoutWidget } from "@viceme-ai/web-sdk";
import { useEffect, useRef } from "react";

export function SupportButton() {
  const button = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!button.current) return;
    const widget = mountCommerceCheckoutWidget(button.current, {
      apiBaseUrl: "<api_base_url from app link>",
      publishableKey: "<publishableKey from .viceme/app.json>",
      offerId: "<offer id>",
    });
    return () => widget.destroy();
  }, []);

  return <button ref={button}>Support this work</button>;
}
```

The SDK creates one explicit idempotency UUID per browser attempt, uses a simple Origin-bearing form request, and accepts completion messages only from the exact Hosted Checkout origin and iframe window. Do not replace this with a frontend-only paid flag. A static frontend can improve the interaction, but protected content still requires a server-side Entitlement check in a later supported flow.
