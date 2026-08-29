# ViceMe Tip Integration Contract

## Required Resource Graph

One tip integration has exactly these authoritative resources:

```text
PUBLISHED + VERIFIED Website Work
├── ACTIVE SDK access whose features contain tip
└── ACTIVE PRODUCTION Website Widget application
    ├── workId = Website Work ID
    ├── origins = [Website canonical Origin]
    ├── returnUrls = []
    └── products = []
```

The Work is the identity. The public `workKey` locates hosted runtime access.
The Website Widget application authorizes the payment entry and exact parent
Origin; it does not replace the Work and is not embedded as page identity.

## Origin

The canonical Origin must be lowercase canonical HTTPS with no credentials,
path, query, fragment, or trailing slash. The Tip iframe sends a
`strict-origin` Referer, and Shop issues an embed context only when that exact
Origin is registered. A preview domain and production domain are distinct.

## Runtime Boundary

The host sends only Work key, region, feature set, target, and theme through the
loader. It sends no amount, provider, application ID, access token, or payment
state. Shop owns login, order creation, provider protocol, status, settlement,
and signed capabilities.

The frame stays non-interactive until a trusted resize handshake. Shop owns the
payment surface and resets it to the initial amount form on Escape before
emitting close. The SDK accepts messages only from the expected Shop Origin and
iframe window, binds the first valid Work UUID, and redispatches sanitized close
and paid notifications. The host needs no listener for the default close
behavior. Opening the UI is not proof that a payment settled.

## CSP

Keep existing directives. Add only the exact Profile Web Origin to the required
script, connection, and frame directives. Never use a wildcard, broad ViceMe
subdomain, `unsafe-eval`, or host-provided payment script.
