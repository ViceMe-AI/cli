# Hosted danmaku SDK contract

Use this when the target is a static site, blog, or product page that should
receive ViceMe-hosted danmaku without copying React component code into the
host repository.

## Embed snippet

The host integration is a single script tag. Format it across four lines when
showing it to a user:

```html
<script
  src="https://cdn.viceme.cn/danmaku/v1/widget.js"
  data-creator-id="CREATOR_ID"
  data-work-id="WORK_ID"></script>
```

For local acceptance against a running ViceMe Web dev server, keep the same
four-line shape but use the local widget script origin:

```html
<script
  src="http://localhost:4300/danmaku/v1/widget.js"
  data-creator-id="CREATOR_ID"
  data-work-id="WORK_ID"></script>
```

Do not use the production CDN URL for local-only verification unless the
production deployment is intentionally part of the test.

Optional attributes:

- `data-name="ViceMe-Danmaku"` for explicit loader discovery.
- `data-theme="auto|light|dark"`.
- `data-locale="zh-CN|en-US|..."`.
- `data-widget-origin="https://viceme.cn"` for private or regional previews.
- `data-widget-url="https://viceme.cn/embed/danmaku"` for full widget app
  overrides in development.

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

## Loader responsibilities

`assets/cdn/viceme-danmaku-widget.js` is the CDN loader. It is intentionally
small and framework-free. It must:

- read configuration from its own `data-*` attributes;
- fail closed when `data-creator-id` or `data-work-id` is absent;
- mount at most once for the same creator/work pair;
- avoid mutating host page content outside one fixed root element;
- keep host page clicks usable by using a non-interactive stage iframe and a
  separate bottom controls iframe;
- lazy-load modal or login surfaces only after widget messages request them;
- pass `creatorId`, `workId`, `theme`, `locale`, `host`, `sdk`, and the
  automatically detected page-position anchor to the ViceMe-hosted widget app.

## Runtime boundary

The loader owns only mounting, iframe placement, sizing, and modal visibility.
The hosted widget app owns danmaku rendering, authentication prompts,
persistence, comment synchronization, fan/follow prompts, reward flows, and
rate limits.

Do not copy the React Tailwind blueprint into the host page when this hosted
SDK path is requested. The host page should only receive the script snippet.
