---
name: viceme-publish
description: Validate, upload, review, and publish a local AI Agent Skill as a paid ViceMe listing. Use when a user asks to publish, list, sell, upload, or update a Skill directory or ZIP on ViceMe, including choosing a CNY price, confirming a cover and gallery, resuming an interrupted publication, or retrieving its public share link.
---

# Publish a Skill to ViceMe

Use the CLI for every deterministic action. Read [workflow.md](references/workflow.md) before the first write and [errors.md](references/errors.md) when a command fails.

## Workflow

1. Run `viceme skill inspect --path <dir-or-zip>`.
2. Explain validation findings and ask the user for the exact CNY price in fen.
3. Run `viceme skill publish --path <dir-or-zip> --price-minor <fen> --dry-run`.
4. Show the final package digest, discovered image candidates, title, source description, and price. Ask permission to upload.
5. Run the same command without `--dry-run`.
6. Poll with `viceme publication get <id>` while analysis is `PENDING`. Analysis proposes `summaryZhCn`, `summaryEnUs`, cover, and gallery; do not present any proposal as a user decision.
7. Run `viceme publication review <id>` and show both exact summaries, the exact price, cover, ordered gallery, and `reviewDigest`. ASCII counts as width 1 and Chinese/non-ASCII counts as width 2; each summary must have total width at most 30.
8. If the user wants different media, run `viceme publication asset upload <id> --role <cover|gallery> --path <image>`. For other edits, prepare a complete strict JSON draft and run `publication update`.
9. Ask the user to explicitly confirm both displayed summaries, price, cover, and gallery.
10. Only after confirmation, run `viceme publication confirm <id> --review-digest <digest>`.
11. Ask once more before the irreversible public publication, then run `viceme publication publish <id> --review-digest <digest>`.
12. Return the authoritative `product.detailUrl` from the result.

Never infer confirmation from an earlier general request to “publish”. A changed draft produces a new digest and requires confirmation again.

## Recovery

- After interruption, run `viceme skill publish --resume <publication-id>`.
- If local source bytes changed, do not bypass the digest check. Restore the original source or start a new publication.
- `PUBLISHED` is final. Never create a second publication merely because a response was lost; query the existing ID first.
- Do not retry non-retryable errors without changing the invalid input or state.
