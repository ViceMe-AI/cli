# Hosted danmaku SDK contract

Use this for a static site, blog, or product page that should receive
ViceMe-hosted danmaku without copying component code into the host repository.

## Embed snippet

The selected CLI Profile is the sole environment authority. It binds the API
base URL, Web base URL, and market region. Create and activate the Work before
editing the host page:

```bash
viceme --profile <profile> access init --name "<website name>" --danmaku
```

If `.viceme/access.yaml` already exists, use `access inspect`; run `access apply`
only to reconcile an intentional local config. The config records the Profile
authority and cannot be reused under another API, Web, or market environment.

Successful `init`, `inspect`, and `apply` responses include `data.workKey`,
`data.scriptUrl`, and `data.embedSnippet` whenever the remote Work has an active
public danmaku capability. Insert `data.embedSnippet` exactly. Never derive the
origin, append an SDK path, or fall back to another Profile.

`workKey` is public and opaque. Never replace it with an internal creator ID,
product ID, media ID, slug, API token, or payment credential. Do not add API,
iframe, creator, work-ID, or host-URL overrides to the generated snippet.

## Page-position anchors

The loader derives the active anchor from the canonical page URL, including
hash routes, and the current 10% scroll bucket. A message is stored under that
page-position anchor and appears to visitors on the same Work and anchor.

## Runtime boundary

The browser runtime validates loader attributes, resolves the public Work and
requires an active `danmaku` capability, mounts at most once, isolates styles,
keeps host controls clickable, lazy-loads modal surfaces, and removes all nodes,
listeners, and timers on destroy. It calls only public danmaku endpoints; it does
not establish a general-purpose SDK Session or expose follow, purchase, or
entitlement APIs.

The hosted Web app owns rendering and interaction. The Shop API owns the public
`workKey` mapping, persistence, retention limit, and rate limits. Anonymous
danmaku does not force login. Tipping is not part of this standalone snippet.
