---
name: viceme-publish
description: Validate, upload, review, and publish a local AI Agent Skill as a paid ViceMe listing. Use when a user asks to publish, list, sell, upload, or update a Skill directory or ZIP on ViceMe, including choosing a CNY price, confirming a cover and gallery, resuming an interrupted publication, or retrieving its public share link.
---

# Publish a Skill to ViceMe

Use the CLI for every deterministic action. Read [workflow.md](references/workflow.md) before the first write and [errors.md](references/errors.md) when a command fails.

## Workflow

1. Run `viceme auth status` before inspecting or parsing the Skill. Capture the returned active `profile` as the publication profile.
2. If unauthenticated, run `viceme --profile <publication-profile> auth login`, show the verification URL, and wait for successful authorization before continuing. Never switch to another Profile merely because it already has credentials.
3. Pin every remaining command in this workflow with `viceme --profile <publication-profile> ...`. If that Profile becomes invalid or logged out, stop and repair/login to the same Profile unless the user explicitly chooses a different one.
4. Run `viceme --profile <publication-profile> skill inspect --path <dir-or-zip>`.
5. Explain validation findings and ask the user for the exact CNY price in fen.
6. Run `viceme --profile <publication-profile> skill publish --path <dir-or-zip> --price-minor <fen> --dry-run`.
7. Show the final package digest, discovered image candidates, title, source description, and price. Ask permission to upload.
8. Before the first non-dry-run `skill publish` or any `skill publish --resume`, ensure that the command process can write the ViceMe CLI configuration directory (normally `~/.viceme-cli`, or `VICEME_CLI_CONFIG_DIR` when explicitly configured). In a sandboxed Agent, request the required filesystem permission for this exact command before running it. Do not deliberately run once without access to discover a permission failure, and never delete the lock files.
9. Run the same command without `--dry-run` using that permission. The CLI must persist its local intent before any remote publication is created.
10. Run `viceme --profile <publication-profile> publication wait <id>` after
    upload. `PENDING` is background work, not a new authorization boundary:
    keep waiting automatically and never ask the user to reply “continue”.
    Analysis proposes `summaryZhCn`, `summaryEnUs`, `usageInstructionsZhCn`,
    `usageInstructionsEnUs`, cover, and gallery; do not present any proposal as
    a user decision.
11. Run `viceme --profile <publication-profile> publication review <id>` and show both exact summaries, both exact usage instructions, the exact price, cover, ordered gallery, and `reviewDigest`. ASCII counts as width 1 and Chinese/non-ASCII counts as width 2; each summary must have total width at most 30.
12. If the user wants different media, run `viceme --profile <publication-profile> publication asset upload <id> --role <cover|gallery> --path <image>`. For other edits, prepare a complete strict JSON draft and run `publication update` with the same Profile.
13. Ask the user to explicitly confirm both displayed summaries, both usage instructions, price, cover, and gallery.
14. Only after confirmation, run `viceme --profile <publication-profile> publication confirm <id> --review-digest <digest>`.
15. Ask once more before the irreversible public publication, then run `viceme --profile <publication-profile> publication publish <id> --review-digest <digest>`.
16. Return the authoritative `product.detailUrl` from the result.

Never infer confirmation from an earlier general request to “publish”. A changed draft produces a new digest and requires confirmation again.

## Recovery

- After interruption, run `viceme --profile <publication-profile> skill publish --resume <publication-id>`.
- If `publication wait` reaches its wait deadline, run the same wait command
  again with the same publication ID. Do not re-upload and do not ask for a new
  user confirmation merely to continue waiting.
- If local source bytes changed, do not bypass the digest check. Restore the original source or start a new publication.
- `PUBLISHED` is final. Never create a second publication merely because a response was lost; query the existing ID first.
- Do not retry non-retryable errors without changing the invalid input or state.
