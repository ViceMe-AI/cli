# Publication command contract

All commands emit one JSON envelope on stdout. Progress belongs on stderr.

## Required inputs

- Root `SKILL.md` with non-empty `name` and `description` frontmatter.
- Explicit `priceMinor` in CNY fen before any upload.
- A verified package, one confirmed cover, and at least one confirmed gallery item before review confirmation.

## State sequence

`DRAFT -> REVIEW_REQUIRED -> READY -> PUBLISHED`

`FAILED` can return to review after correcting inputs. `CANCELLED` and `PUBLISHED` are terminal.

## Media

Images discovered inside the package are uploaded as candidates. The platform may suggest a cover and gallery with an LLM. Suggestions are non-authoritative. The user may upload PNG, JPEG, GIF, WebP, or AVIF replacements before confirmation.

## Update draft file

`publication update --input` accepts a complete strict JSON object:

```json
{
  "title": "Skill title",
  "summary": "Short listing summary",
  "currency": "CNY",
  "priceMinor": 100,
  "coverUploadId": "uuid",
  "galleryUploadIds": ["uuid"]
}
```

Use IDs returned by `publication review`. Do not invent upload IDs.
