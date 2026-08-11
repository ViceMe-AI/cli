# Publication command contract

All commands emit one JSON envelope on stdout. Progress belongs on stderr.

## Required inputs

- Root `SKILL.md` with non-empty `name` and `description` frontmatter.
- Explicit `priceMinor` in CNY fen before any upload.
- Two confirmed listing summaries (`zh-CN` and `en-US`), a verified package, one confirmed cover, and at least one confirmed gallery item before review confirmation.

## State sequence

`DRAFT -> REVIEW_REQUIRED -> READY -> PUBLISHED`

`FAILED` can return to review after correcting inputs. `CANCELLED` and `PUBLISHED` are terminal.

## Media

Images discovered inside the package are uploaded as candidates. The platform uses an LLM to propose a Chinese summary, an English summary, a cover, and a gallery. Suggestions are non-authoritative. Each summary has a maximum display width of 30: ASCII counts as 1 and Chinese/non-ASCII counts as 2. The user may edit either summary or upload PNG, JPEG, GIF, WebP, or AVIF replacements before confirmation.

## Update draft file

`publication update --input` accepts a complete strict JSON object:

```json
{
  "title": "Skill title",
  "summaryZhCn": "生成专业网页演示",
  "summaryEnUs": "Build polished web slides",
  "currency": "CNY",
  "priceMinor": 100,
  "coverUploadId": "uuid",
  "galleryUploadIds": ["uuid"]
}
```

Use IDs returned by `publication review`. Do not invent upload IDs.
