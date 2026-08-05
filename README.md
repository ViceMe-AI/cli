# ViceMe CLI

ViceMe CLI connects creator projects to ViceMe platform capabilities. It is a deterministic Go client designed for direct use and for AI coding hosts such as Codex and Claude Code.

Version 1.0 starts a new command contract. The old Skill Agent publication, Target, and `job` state machine are intentionally removed.

## Install

```bash
npm install -g @viceme-ai/cli
viceme install --target auto
```

The npm package installs a small launcher. `viceme install` downloads the checksum-verified platform binary, installs the matching `viceme` Agent Skill atomically, and initializes the selected CN or GLOBAL profile. The package has no npm install lifecycle script.

## Authenticate

Interactive use:

```bash
viceme auth login
```

AI hosts use the cross-turn Device Authorization flow:

```bash
viceme auth login --no-wait --json
# Run the exact continue_args returned above, including --profile.
viceme --profile <original-profile> auth login --device-code <opaque-device-code> --json
```

The browser reuses the normal ViceMe login page. Access and rotating refresh credentials are stored only in the operating-system credential store (or the CLI's encrypted fallback). Project files never contain tokens.

## Bind an existing project

```bash
viceme app link --dir . --origin http://localhost:3000
```

This creates or selects an EXTERNAL Creator App, binds its TEST environment, registers the exact browser Origin, and writes `.viceme/app.json`:

```json
{
  "schemaVersion": 1,
  "appId": "550e8400-e29b-41d4-a716-446655440000",
  "hostingMode": "EXTERNAL",
  "environment": "TEST",
  "publishableKey": "app_pk_test_...",
  "origin": "http://localhost:3000",
  "capabilities": {}
}
```

The manifest contains public identifiers and may be committed. It must never contain a CLI token, Server Credential, Webhook Secret, provider key, storage credential, or signed URL.

## Add and diagnose a capability

```bash
viceme capability catalog
viceme capability add commerce --dir .
viceme app doctor --dir .
viceme capability doctor commerce --dir .
```

Slice A exposes the Commerce foundation as DRAFT. Checkout is delivered by the next platform Slice; the CLI does not simulate unavailable capabilities.

The App Context shell can be mounted from the public browser SDK without exposing a credential:

```ts
import { mountAppContextWidget } from "@viceme/web-sdk";

mountAppContextWidget(document.querySelector("#viceme-app"), {
  apiBaseUrl: "https://api.viceme.cn/v1",
  publishableKey: "app_pk_test_...",
});
```

The API resolves the public key, validates the registered Origin, and returns only public App context.

## Command surface

```text
viceme install | update | version
viceme auth login | status | logout
viceme profile list | add | use | configure | rename | remove
viceme skills list | read | install | doctor
viceme app link | get | list | doctor
viceme capability catalog | add | get | doctor
```

The checked-in machine surface is `skills/viceme/references/command-manifest.json`. CI compares it with the Cobra command tree. The bundled Skill and CLI are versioned and digested together.

## Development

```bash
go test ./...
npm test
make quality-check
```

See `docs/releasing.md` for the checksum, npm provenance, and release process.
