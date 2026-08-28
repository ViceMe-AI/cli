# Hosted SDK Contract

## Loader

The selected CLI Profile is the environment authority. Its `webBaseUrl` serves
the stable `/viceme-sdk/v1/viceme.min.js` loader and its `marketRegion` selects
`cn` or `global`. The loader accepts only these attributes:

```text
data-viceme-work
data-viceme-region
data-viceme-features
data-viceme-target
data-viceme-theme
data-viceme-loader
```

Features may be `danmaku`, `tip`, or both exactly once as `danmaku,tip` (either
input order is normalized). Do not pass endpoints, tokens, prices, providers,
application IDs, or payment state through attributes.

## Resource Gate

A danmaku page is usable only when the Website Work is `PUBLISHED`, website
ownership is `VERIFIED`, and active SDK access contains `danmaku`. The public
production Work key comes only from `keys.live` on that access resource.
Both environment keys are permanent public identifiers, not credentials.

## Runtime

The external SDK initializes locally and makes no business API request. It
mounts Shop-hosted iframes under Shadow DOM, validates bridge origin and source,
and cleans up its nodes, listeners, and timers. Shop owns message persistence,
rate limits, keyboard behavior, and reduced-motion behavior.

The loader derives an opaque page-position anchor from the canonical page URL
and current scroll bucket. The host must not derive or send its own user,
session, or payment identity.

## CSP

Allow only the exact regional Shop Origin required by `script-src`,
`connect-src`, and `frame-src`. Keep `object-src 'none'`. A request nonce with
`strict-dynamic` can authorize scripts, but frame and manifest connections still
need explicit Origins.
