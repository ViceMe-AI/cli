---
name: viceme-publish
description: Validate, upload, review, and publish a local AI Agent Skill as a paid ViceMe listing. Use when a user asks to publish, list, sell, upload, or update a Skill directory or ZIP on ViceMe, including choosing a CNY price, confirming a cover and gallery, resuming an interrupted publication, or retrieving its public share link.
---

# Publish a Skill to ViceMe

Use the CLI for every deterministic action. Read [workflow.md](references/workflow.md) before the first write and [errors.md](references/errors.md) when a command fails.

## Workflow

1. Run `viceme profile list` and capture the active Profile without switching it.
2. Run `viceme --profile <publication-profile> skill inspect --path <dir-or-zip>` before authentication or any remote write. Explain local validation failures without asking for price.
3. Run `viceme --profile <publication-profile> auth status`. If unauthenticated, run `viceme --profile <publication-profile> auth login`, show the verification URL, and wait for successful authorization. Never switch Profiles merely because another already has credentials.
4. Pin every remaining command to that Profile. If it becomes invalid or logged out, repair/login to the same Profile unless the user explicitly chooses another.
5. Before `skill listing prepare`, ensure the process can write both the source binding location (`.viceme/skill.json` for a workspace or adjacent `<zip-name>.viceme.json` for ZIP) and the CLI config directory. Then run `viceme --profile <publication-profile> skill listing prepare --path <dir-or-zip>`.
6. Immediately present the returned owner preview before asking for price:
   - Always show `presentation.fallbackUrl` as the stable “Owner Preview” link. Keep showing this stable link in later preview updates even after publication.
   - When `presentation.mode` is `ONE_TIME_LAUNCH`, use `presentation.openUrl` immediately. It is a short-lived, single-use handoff into the private preview; do not save it in files, repeat it in later messages, or treat it as the stable preview address.
   - In Codex Desktop, call `open_in_codex` with `placement: "right"` and `target: {type: "browser", url: presentation.openUrl}`.
   - In WorkBuddy, call `present_files` with `files: [presentation.openUrl]` so its embedded Browser/Preview panel opens the remote URL. Do not generate or download a local HTML copy.
   - In Claude Code Desktop, do not start a local static server. Render `presentation.openUrl` once as a clickable “Open preview” link and keep `presentation.fallbackUrl` visible as the stable fallback.
   - In another Agent host, use an embedded URL-opening tool when one exists; otherwise render the one-time link once as clickable text. Never guess the host in the CLI or generate local HTML.
   - If `presentation.mode` is `FALLBACK_URL`, show and use the stable fallback link and explain only that automatic side-panel handoff is temporarily unavailable.
   This is the first remote business result. Do not ask for price before the preview exists.
7. Ask the user for the exact CNY price in fen.
8. Run `viceme --profile <publication-profile> skill publish --path <dir-or-zip> --price-minor <fen> --dry-run`.
9. Show the canonical package digest, discovered image candidates, title, source description, and price. Ask permission to upload.
10. Before the first non-dry-run `skill publish` or any `skill publish --resume`, ensure that the command process can write the ViceMe CLI configuration directory and the source binding location. In a sandboxed Agent, request the required filesystem permission for this exact command before running it. Do not deliberately run once without access to discover a permission failure, and never delete lock files.
11. Run the same command without `--dry-run` using that permission. The CLI must persist its local intent before any remote Publication is created and must attach it to the prepared `listingId`.
12. Run `viceme --profile <publication-profile> publication wait <id>` after
    upload. `PENDING` is background work, not a new authorization boundary:
    keep waiting automatically and never ask the user to reply “continue”.
    Analysis proposes `summaryZhCn`, `summaryEnUs`, `usageInstructionsZhCn`,
    `usageInstructionsEnUs`, cover, and gallery; do not present any proposal as
    a user decision.
13. Run `viceme --profile <publication-profile> publication review <id>` and show both exact summaries, both exact usage instructions, the exact price, cover, and ordered gallery. Render every selected image inline in the conversation instead of showing only filenames or links:
    - Match `draft.coverUploadId` and each `draft.galleryUploadIds` entry to its exact `uploads[].viewUrl`.
    - For Codex, download each selected `viewUrl` to a unique temporary file that keeps the response image extension. Verify a successful response, an `image/*` content type, and a non-empty file, then render it with Markdown image syntax using the absolute local path, for example `![Cover](/private/tmp/viceme-review-<id>/cover.png)`. This avoids remote-image authentication, redirect, and host-policy failures.
    - If a download fails or the client cannot render local images, fall back to `![Image](viewUrl)` and include the clickable `viewUrl`; never replace the visual review with filenames alone.
    ASCII counts as width 1 and Chinese/non-ASCII counts as width 2; each summary must have total width at most 30. Capture `reviewDigest` from the JSON response for the later CLI commands, but treat it as an opaque integrity token: never label it “review digest” and do not show it to the user unless troubleshooting requires the exact value.
14. If the user wants different media, run `viceme --profile <publication-profile> publication asset upload <id> --role <cover|gallery> --path <image>`. For other edits, prepare a complete strict JSON draft and run `publication update` with the same Profile.
15. Ask exactly once whether the user confirms the displayed review and wants to publish it publicly now. State that public publication is immediate and irreversible. An unambiguous affirmative response to this combined question, such as “confirm and publish” or “publish”, authorizes both following commands; a request to change anything does not.
16. After that combined authorization, run `viceme --profile <publication-profile> publication confirm <id> --review-digest <captured-review-digest>` and then, without asking again, run `viceme --profile <publication-profile> publication publish <id> --review-digest <captured-review-digest>`. Never run `publish` if `confirm` did not succeed.
17. Return the authoritative `product.detailUrl` from the result as the public release URL. Also run `skill listing get <listing-id>` and present its latest Owner Preview using step 6; publication does not replace the private editing/preview entrance.

Never infer this combined authorization from the user's initial request to publish before the final review was displayed. A changed draft produces a new digest and requires displaying the new review and obtaining a new combined confirmation.

## Recovery

- After interruption, first run `skill listing prepare --path <same-source>` to recover the stable `listingId`, then run `skill publish --resume <publication-id>` when a local pending Publication exists.
- Re-running the same ZIP, a moved ZIP with its adjacent sidecar, or a modified workspace must reuse the returned `listingId`. Never identify a work from filename or title.
- Use `--new-listing` only after the user explicitly requests a separate work. Use `skill listing bind <listing-id> --path <zip-or-workspace>` only after the user explicitly selects an owned candidate.
- If `publication wait` reaches its wait deadline, run the same wait command
  again with the same publication ID. Do not re-upload and do not ask for a new
  user confirmation merely to continue waiting.
- If local source bytes changed, do not bypass the digest check. Restore the original source or start a new publication.
- If review confirmation succeeded but public publication returned a retryable or unknown result, query the same publication ID first. When it is still `READY` with the same review digest, continue the already-authorized publish operation without asking a duplicate confirmation; never re-upload.
- `PUBLISHED` is final. Never create a second publication merely because a response was lost; query the existing ID first.
- Do not retry non-retryable errors without changing the invalid input or state.
