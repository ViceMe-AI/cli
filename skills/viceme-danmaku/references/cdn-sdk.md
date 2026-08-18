# Hosted danmaku SDK contract

Use this when the target is a static site, blog, or product page that should
receive ViceMe-hosted danmaku without copying React component code into the
host repository.

This is a surgical integration, not a component-building task. The normal
result is one generated `.viceme/access.yaml` file and one four-line edit to an
existing host entry. Do not inspect or validate unrelated application behavior.

## Embed snippet

The host integration is one script tag and one public `workKey`. Preserve this
four-line form when showing or applying it:

```text
<script
  defer src="https://s3.viceme.cn/viceme-sdk/v1/viceme.min.js" data-viceme-work="WORK_KEY" data-viceme-region="cn"
  data-viceme-features="danmaku" data-viceme-target="body"
  data-viceme-theme="auto"></script>
```

Create and activate the work before editing the host page:

```bash
viceme access init --name "<website name>" --danmaku
```

`access init` performs work creation and activation in one operation. If it
requires authentication, run `viceme auth login` and wait for the user. If the
API is unavailable, stop with the reported error. Never substitute a localhost
script, copied component, inline implementation, placeholder key, or mock API.

The command writes `.viceme/access.yaml` and returns the same `workKey`. Treat
the key as public and opaque. Never replace it with an internal creator ID,
product ID, media ID, slug, API token, or payment credential.

Optional attributes:

- `data-viceme-theme="auto|light|dark"`.
- `data-viceme-region="cn|global"`.

Do not add endpoint, iframe URL, API base URL, creator ID, work ID, or host URL
overrides to production snippets.

## Fast completion check

After inserting the snippet, perform only these checks unless the user asks for
full acceptance testing:

- the hosted script URL occurs exactly once in the edited entry;
- `data-viceme-work` contains the generated public `workKey`;
- `git diff --check` reports no whitespace error in the edited files.

Do not start a server, navigate a browser, inspect iframes, send messages, test
persistence, or modify host styles as part of the default installation.

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
