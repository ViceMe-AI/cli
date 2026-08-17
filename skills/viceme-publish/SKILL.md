---
name: viceme-publish
description: Validate, upload, review, and publish a local AI Agent Skill as a paid ViceMe listing. Use when a user asks to publish, list, sell, upload, or update a Skill directory or ZIP on ViceMe, including choosing a CNY price, confirming a cover and gallery, resuming an interrupted publication, or retrieving its public share link.
---

# Publish a Skill to ViceMe

Use the CLI for every deterministic action. Read [workflow.md](references/workflow.md) before the first write and [errors.md](references/errors.md) when a command fails.

## Workflow

1. Run `viceme profile list`. If the user's current request explicitly names a Profile, use that exact Profile without changing the active setting; otherwise capture the active Profile. Memory, prior conversations, old publication records, source filenames, package digests, sidecars, and credentials in another Profile are historical hints only: never use them to select or switch the publication Profile.
2. Run `viceme --profile <publication-profile> auth status`. If unauthenticated, run `viceme --profile <publication-profile> auth login`, show the verification URL, and wait for successful authorization. Never switch Profiles merely because another already has credentials.
3. Pin every remaining command to that Profile. Resolve bindings and resume publications only within that Profile's API endpoint, market, and authenticated user. Do not probe or fall back to another Profile to recover a historical publication. If the selected Profile becomes invalid or logged out, repair or log in to the same Profile unless the user explicitly chooses another.
4. Before the first `skill publish`, ensure the process can write both the source binding location (`.viceme/skill.json` for a workspace or adjacent `<zip-name>.viceme.json` for ZIP) and the CLI config directory. In a sandboxed Agent, request filesystem permission for this exact command before running it. This is host filesystem permission, not a separate business confirmation. Do not ask whether ViceMe may upload the private draft.
5. Immediately run `viceme --profile <publication-profile> skill publish --path <dir-or-zip>`. Do not run a separate `skill inspect`, `skill listing prepare`, or `--dry-run` first. The command performs deterministic local validation, creates or recovers the Listing and Publication, uploads and verifies the real private package, then returns its presentation. If validation fails, explain and correct the source; never upload invalid bytes.
6. Present the returned Owner Preview immediately, before asking for price or any other listing detail:
   - Always show `presentation.fallbackUrl` as the stable “Owner Preview” link. Keep showing it after later updates and after publication.
   - When `presentation.mode` is `ONE_TIME_LAUNCH`, use `presentation.openUrl` immediately. It is a short-lived, prefetch-safe launch document whose rendered authorization handoff is single-use; do not save or repeat it.
   - In Codex Desktop, call `open_in_codex` with `placement: "right"` and `target: {type: "browser", url: presentation.openUrl}`.
   - In WorkBuddy, call `present_files` once with `files: [presentation.openUrl]`. The launch document tolerates the host's metadata prefetch before its rendered handoff. Do not pre-open the URL yourself, and do not generate or download a local HTML copy.
   - In Claude Code Desktop, do not start a local static server. Render `presentation.openUrl` once as a clickable “Open preview” link and keep the stable fallback visible.
   - In another Agent host, use an embedded URL-opening tool when one exists; otherwise render the one-time link once. Never guess the host in the CLI.
   - If `presentation.mode` is `FALLBACK_URL`, show and use the stable fallback and explain only that automatic side-panel handoff is temporarily unavailable.
   This uploaded private draft is the first remote business result. Aim to run and present this fast path within ten seconds for a normal package.
7. After every successful command that changes or completes Draft content, uploads, analysis, review readiness, or publication state, immediately present its fresh `presentation` using step 6. Re-focus or reload the existing preview panel when the host supports it; never create another Listing or stable preview URL. Background changes remain visible through the page's revision polling.
8. Ask for the exact CNY price in fen. Run `viceme --profile <publication-profile> skill publish --resume <publication-id> --price-minor <fen>`. This updates the same Draft, uploads media candidates, and starts analysis without another upload confirmation. Present its preview immediately.
   - If the package has no usable image, keep the preview open and obtain a real cover and gallery image after the preview exists. Upload both with `publication asset upload`; once both selections exist, the CLI queues analysis on the same Publication. Never invent an upload ID or call a missing-media Draft complete.
9. Run `viceme --profile <publication-profile> publication wait <id>`. `PENDING` is background work, not a new authorization boundary: keep waiting automatically and never ask the user to reply “continue”. Analysis proposes both summaries, both usage instructions, cover, and gallery; do not present a proposal as a user decision. Present the returned preview again when the wait completes.
10. Run `viceme --profile <publication-profile> publication review <id>` only to read the authoritative final Draft and integrity token. Show both exact summaries, both exact usage instructions, the exact price, cover, and ordered gallery. Render every selected image inline:
    - Match `draft.coverUploadId` and `draft.galleryUploadIds` to exact `uploads[].viewUrl` values.
    - In Codex, download each selected `viewUrl` to a unique temporary file preserving its extension. Verify a successful response, an `image/*` content type, and non-empty bytes, then render the absolute local path.
    - If download or local rendering fails, use `![Image](viewUrl)` and include the clickable URL. Never substitute filenames for visual review.
    - ASCII counts as width 1 and non-ASCII as width 2; each summary must have width at most 30.
    Capture `reviewDigest` for later commands but keep it opaque and hidden unless exact troubleshooting requires it.
11. If the user requests media changes, run `publication asset upload`. For other edits, prepare a complete strict JSON Draft and run `publication update`. Present every successful update before continuing.
12. Ask exactly once whether the user confirms the displayed final Draft and wants to publish it publicly now. State that public publication is immediate and irreversible. An unambiguous affirmative response authorizes both following commands; a requested change does not.
13. After that authorization, run `publication confirm <id> --review-digest <digest>` and then `publication publish <id> --review-digest <digest>` without asking again. Never publish if confirmation failed. Present the latest Owner Preview returned by each command.
14. Return `product.detailUrl` as the public release URL and keep the stable Owner Preview visible as the private editing entrance.

The initial publish request is permission to create and upload a private Draft, not permission to make it public. A changed Draft produces a new digest and requires redisplaying the final Draft and obtaining a new public-publication confirmation.

## Recovery

- After interruption, run `skill publish --path <same-source>` when no local Publication ID is available; it recovers the stable Listing and private upload. Run `skill publish --resume <publication-id>` when a local pending Publication exists.
- Historical publications in another Profile do not change this workflow. Mention them only when useful context is needed; continue in the selected Profile unless the user explicitly asks to resume that other environment.
- Re-running the same ZIP, a moved ZIP with its sidecar, or a modified workspace must reuse the returned `listingId`. Never identify a work from filename or title.
- Use `--new-listing` only after the user explicitly requests a separate work. Use `skill listing bind <listing-id> --path <source>` only after the user selects an owned candidate.
- If `publication wait` reaches its deadline, repeat only that wait with the same Publication ID. Do not re-upload or ask for another confirmation.
- If local source bytes changed, restore the original source or start a new Publication; never bypass the digest check.
- If final confirmation succeeded but public publication returned an unknown result, query the same Publication first. If it remains `READY` with the same digest, continue the already-authorized publish without asking again.
- `PUBLISHED` is final. Never create a second Publication because a response was lost; query the existing ID first.
- Do not retry non-retryable errors without changing the invalid input or state.
