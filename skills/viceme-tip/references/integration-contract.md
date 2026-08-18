# ViceMe tip integration contract

## Public boundary

A creator integration consists of one host-owned HTML page and one ViceMe script tag. The script renders the launcher in Shadow DOM and opens a ViceMe-hosted checkout iframe. The host must not copy checkout markup, call payment-provider APIs, or receive payment secrets.

## Binding definition

A work is ready only when all are true:

1. the selected CLI Profile is authenticated as the creator;
2. an External Creator App exists for the work;
3. the exact public hostname appears as verified on that app;
4. the HTML uses that app's generated Creator App ID and platform origin.

An HTML `data-creator-app-id`, a local demo ID, or an unverified domain is not a binding.

## Domain verification

Creator Center supplies the verification URL/path and token. Its generated instructions are authoritative. A deployment may, for example, require a well-known URL shaped like:

```text
https://<creator-host>/.well-known/viceme-app-verification.txt
```

Use the target stack's public-file or routing mechanism at the exact path Creator Center displays. Fetch the deployed URL and compare it with the value shown in Creator Center without printing the token into chat or persistent logs. Redirects to another origin and non-public hosts are invalid.

## Embed contract

```html
<script
  async
  src="<generated-widget-script-url>"
  data-creator-app-id="<creator-app-id>"
  data-locale="<zh-CN-or-en-US>"
></script>
```

Keep exactly one tag per Creator App. `creatorAppId` is public configuration, not a credential. Obtain the complete script URL and ID from Creator Center rather than deriving them from API URLs.

## CSP

When a CSP exists, preserve it and add only the exact ViceMe platform origin to the directives the browser proves necessary. The embed requires the script origin and checkout iframe origin. Never add `*`, `unsafe-eval`, or a broad ViceMe subdomain wildcard as a shortcut.

## Real verification

The integration is complete only on the verified public HTTPS hostname:

- HTML returns 200;
- embed script returns JavaScript with 200;
- launcher is visible and keyboard reachable;
- launcher opens the hosted checkout for the expected work;
- Escape closes the dialog and returns focus;
- no CSP, frame, script, or widget errors occur;
- the agent states whether real order creation/payment was exercised.

Opening the UI is not proof that a payment settled. Keep checkout verification and real-money payment verification distinct.
