# Website publication workflow

Website publication registers a local directory as a stable ViceMe work. It does not upload, deploy, or host the website. A public domain is optional.

## Publish

1. Run `viceme auth status` in the active CLI context. The token must include `sdk-work:read` and `sdk-work:write`. If it does not, run `viceme auth login` again for that same context.
2. Ensure the process can write `<website-dir>/.viceme/website.json`. This binding contains no credential and is the durable local work identity.
3. Publish the directory:

   ```bash
   viceme website publish --path <website-dir> --name "<website name>" \
     [--creator-display-name "<creator name>"] [--url "<published URL>"]
   ```

   Omit `--url` before the website has a public address. If the user profile already has a display name, omit `--creator-display-name` as well.
4. If the command returns `CREATOR_DISPLAY_NAME_REQUIRED`, repeat the same command and source path with `--creator-display-name`. Do not delete the binding or create another work. A successful first publication creates and claims the user's `VICEME` creator identity using the same fields and ownership rules as Skill Publish.
5. Return the `workKey`, `creatorWorkId`, release version, `unchanged` state, and binding path from the authoritative command response. Use `$viceme-access` only after publication when the user also asks to add login, follow, purchase, or feature gates.

## Stable identity and repeat publication

- `.viceme/website.json` persists `clientWorkId`, `workId`, `workKey`, region, and the latest release state.
- `(owner, market, clientWorkId)` identifies the work. The directory name, display name, URL, and Digest do not.
- Repeating publication from the same binding updates the existing work. Content, title, or optional URL changes create another website release, not another work.
- An unchanged Digest, title, and URL returns `unchanged: true` without creating another release.
- Never delete or rewrite the binding to resolve an ownership, region, or identity error.

## Boundaries

- Website publication is immediate registration; it has no Skill package upload, private listing preview, media review, price review, or irreversible marketplace-confirmation step.
- Publishing a website does not configure access or a sale offer. Use `$viceme-access` for those later steps.
- ViceMe does not host the site in this version. Static files in a public website bundle are not protected assets.
