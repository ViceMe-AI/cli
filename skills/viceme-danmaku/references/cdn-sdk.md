# Hosted danmaku SDK contract

Use this when the target is a static site, blog, or product page that should
receive ViceMe-hosted danmaku without copying React component code into the
host repository.

## Embed snippet

The host integration is one script tag and one public `workKey`. The selected
CLI Profile is the sole endpoint authority:

```bash
viceme profile list
viceme --profile <profile> auth status
```

Record that Profile's `webBaseUrl` without switching Profiles. From the website
source directory, publish or reuse its Work and activate danmaku before editing
the host page:

```bash
viceme --profile <profile> access init --name "<website name>" --danmaku
```

The command uses the current POC website-publication contract internally. It
maintains `.viceme/website.json` as the stable source identity and writes
`.viceme/access.yaml` for the same `workKey`. If the first publication requires
a creator display name, rerun once with `--creator-display-name "<creator name>"`.

If `.viceme/access.yaml` already exists, use:

```bash
viceme --profile <profile> access inspect
```

If the authoritative response has no `data.embedSnippet` while the local config
is active, run `viceme --profile <profile> access apply` once. Do not hand-edit
the file or create a replacement Work.

Successful `init`, `inspect`, and `apply` responses include `data.workKey`,
`data.scriptUrl`, and `data.embedSnippet` whenever the authoritative Work has
an active public danmaku capability. The CLI derives the script URL from the
selected Profile's `webBaseUrl` and emits the complete four-line tag. Insert
`data.embedSnippet` exactly; the agent must not guess the origin, concatenate
the SDK path, or fall back to a production origin.

Treat the returned key as public and opaque. Never replace it with an internal
creator ID, product ID, media ID, slug, API token, or payment credential.

Do not add endpoint, iframe URL, API base URL, creator ID, work ID, or host URL
overrides to the generated snippet.

## Page-position anchors

The four-line snippet is the whole installation. The host page does not need to
report business events or call a scene API.

The loader automatically derives the active anchor from:

- the current canonical page URL, including hash routes when the page uses
  them;
- the current scroll bucket, grouped in 10% ranges such as `0-10`, `60-70`, or
  `90-100`.

When a user sends a danmaku, the hosted widget stores it under that page-position
anchor. Other visitors on the same work see it when they are on the same page
and scroll into the same bucket.

## SDK responsibilities

The browser runtime is published by `@viceme-ai/sdk` and is not duplicated in
this Skill. It must:

- validate `data-viceme-work`, region, feature, target, and theme attributes;
- establish a short-lived in-memory Work Session and require the server to
  advertise the `danmaku` capability;
- mount at most once for the same work and target;
- avoid mutating host page content outside one fixed root element;
- isolate SDK styles in Shadow DOM;
- keep host page clicks usable by using a non-interactive stage iframe and a
  separate bottom controls iframe;
- lazy-load modal surfaces only after widget messages request them;
- pass only `workKey`, theme, locale, SDK version, and the opaque
  page-position anchor to the ViceMe-hosted widget app;
- remove all nodes, listeners, timers, and in-memory session state on destroy.

## Runtime boundary

The SDK owns configuration, session establishment, mounting, iframe placement,
sizing, page-position detection, and lifecycle. The ViceMe-hosted Web app owns
danmaku rendering and interaction. The Shop API resolves
`workKey -> creator-owned SdkWork` and owns persistence and rate limits.

Anonymous danmaku does not force login. Tipping and rewards are not part of
this integration.

Do not copy the React Tailwind blueprint into the host page when this hosted
SDK path is requested. The host page should only receive the script snippet.
