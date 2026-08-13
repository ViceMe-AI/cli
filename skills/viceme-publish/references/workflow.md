# Publication command contract

All commands emit one JSON envelope on stdout. Progress belongs on stderr.

## Required inputs

- Root `SKILL.md` with non-empty `name` and `description` frontmatter.
- Explicit `priceMinor` in CNY fen before any upload.
- Two listing summaries and two usage instructions (`zh-CN` and `en-US`), a verified package, one cover, and at least one gallery item must be displayed in the final review before the combined confirm-and-publish authorization.

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

For visual review, map the selected cover and gallery upload IDs to the exact
verified uploads returned by `publication review`. In Codex, download their
`viewUrl` values to a unique temporary directory and embed the absolute local
paths with Markdown image syntax after verifying a successful response, an
`image/*` content type, and a non-empty file. Preserve the server order, label
the cover and gallery positions, and keep the original URLs only as fallbacks.
Do not ask the user to approve media represented only by filenames.

`reviewDigest` is an opaque concurrency and integrity token, not a human
summary. Keep it internally for `publication confirm` and `publication publish`.
The user-facing review consists of the bilingual summaries, bilingual usage
instructions, price, inline cover, and inline ordered gallery.

## Combined confirmation

After displaying the complete final review, ask one question that combines
review confirmation and immediate public publication. Clearly state that the
publication is public and irreversible. An unambiguous affirmative answer to
that question authorizes the Agent to run `publication confirm` followed by
`publication publish` without another user prompt. Keep the two backend state
transitions so failures remain recoverable, but do not expose `READY` as a
second approval step.

The initial request to publish, given before the final review exists, is not
this authorization. Any draft change produces a new `reviewDigest`; display the
new review and obtain a new combined authorization before either command.

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
