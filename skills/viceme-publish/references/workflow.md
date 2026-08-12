# Publication command contract

All commands emit one JSON envelope on stdout. Progress belongs on stderr.

## Required inputs

- Root `SKILL.md` with non-empty `name` and `description` frontmatter.
- Explicit `priceMinor` in CNY fen before any upload.
- Two confirmed listing summaries and two confirmed usage instructions (`zh-CN` and `en-US`), a verified package, one confirmed cover, and at least one confirmed gallery item before review confirmation.

## State sequence

`DRAFT -> REVIEW_REQUIRED -> READY -> PUBLISHED`

`FAILED` can return to review after correcting inputs. `CANCELLED` and `PUBLISHED` are terminal.

## Local recovery permission

Every non-dry-run publish and resume writes an idempotent intent under the
ViceMe CLI configuration directory before creating or continuing a remote
publication. A sandboxed Agent must request write access for the exact publish
command before its first execution. Do not use an expected permission failure
as a probe, do not delete zero-byte lock files, and do not start a replacement
publication when the required action is to retry the same command with access.

## Media

Images discovered inside the package are uploaded as candidates. The platform uses an LLM to propose a Chinese summary, an English summary, semantically equivalent Chinese and English usage instructions derived from the validated `SKILL.md`, a cover, and a gallery. Suggestions are non-authoritative. Each summary has a maximum display width of 30: ASCII counts as 1 and Chinese/non-ASCII counts as 2. The user may edit either summary or either usage instruction, or upload PNG, JPEG, GIF, WebP, or AVIF replacements before confirmation.

After the upload command returns a publication ID, run
`viceme --profile <publication-profile> publication wait <id>`. A `PENDING`
analysis means the platform is working in the background; it does not authorize
another write and must not interrupt the workflow with a “continue” question.
If the CLI wait deadline is reached, repeat only the wait command with the same
ID. Never upload the same package again to resume analysis.

## Update draft file

`publication update --input` accepts a complete strict JSON object:

```json
{
  "title": "Skill title",
  "summaryZhCn": "生成专业网页演示",
  "summaryEnUs": "Build polished web slides",
  "usageInstructionsZhCn": "按 SKILL.md 准备素材，然后运行 Skill 生成网页演示文稿。",
  "usageInstructionsEnUs": "Prepare the assets described in SKILL.md, then run the Skill to generate the web presentation.",
  "currency": "CNY",
  "priceMinor": 100,
  "coverUploadId": "uuid",
  "galleryUploadIds": ["uuid"]
}
```

Use IDs returned by `publication review`. Do not invent upload IDs.
