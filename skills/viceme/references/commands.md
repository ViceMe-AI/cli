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

Refresh recovery is automatic and local to the secure store. Before contacting the API, the CLI persists a request UUID beside the current refresh credential. A lost response is retried with the same old credential and request UUID; a different UUID is token reuse, not a second recovery attempt. `auth logout` first completes and persists any pending refresh recovery, then keeps the current local access credential until the idempotent remote revocation is confirmed.

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

`app link` persists a non-secret `.viceme/app-link-pending.json` idempotency intent before remote creation, then writes `.viceme/app.json` atomically and removes the intent only after the manifest commits. Retrying after a lost response therefore resolves the same App. The manifest is public project metadata. `app doctor` verifies App ownership, environment/key drift, server-returned Contract/SDK versions, Origin registration, and the public Widget context.

## Capability foundation

```bash
viceme capability catalog
viceme capability add commerce --dir .
viceme capability get commerce --dir .
viceme capability doctor commerce --dir .
```

In Slice A, Commerce is a DRAFT foundation. Checkout commands do not exist yet. Identity, Comments, Storage, and Runtime are returned as planned and `capability add` rejects them until their implementation Slice lands.

## App Context shell

Install the public browser package using the project's existing package manager, then mount it from client-side code:

```ts
import { mountAppContextWidget } from "@viceme/web-sdk";

mountAppContextWidget(document.querySelector("#viceme-app")!, {
  apiBaseUrl: "<api_base_url from app link>",
  publishableKey: "<publishableKey from .viceme/app.json>",
});
```

The browser package sends no CLI token. The API validates the request Origin against the App environment and returns only the public App name, environment, and capability projection.

For static HTML, use the self-contained CDN entry documented by `@viceme/web-sdk`; it auto-mounts declared `data-viceme-app-context` targets and has no external module dependency.
