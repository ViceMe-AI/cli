# Publication error handling

- `CREATOR_DISPLAY_NAME_REQUIRED`: repeat the same website publication with `--creator-display-name "<creator name>"`; the existing local website binding preserves the same `clientWorkId`.
- `WEBSITE_BINDING_SCOPE_MISMATCH` or `WEBSITE_REGION_MISMATCH`: restore the intended active CLI context. Do not overwrite the binding or create another work merely to bypass the mismatch.
- `WEBSITE_IDENTITY_CONFLICT`: stop and inspect the existing binding and authoritative work response. Never replace the local `clientWorkId`, `workId`, or `workKey` with a newly invented identity.
- `SKILL_PUBLICATION_PRICE_REQUIRED`: fetch and display the complete current listing details, then ask for the exact CNY price in fen together with any desired title, copy, or media changes. Never ask for price as a standalone question. Resume the same private Publication.
- `SKILL_SECRET_DETECTED` or `SKILL_SENSITIVE_FILE`: stop and remove credentials or sensitive files from the package. Never print their contents.
- `PUBLICATION_SOURCE_CHANGED`: the recovery package differs from the started publication; restore it or start a new publication.
- `SKILL_PUBLICATION_REVIEW_CHANGED`: fetch and show the latest review, then obtain a new combined confirm-and-publish authorization for its new digest.
- `SKILL_LISTING_MEDIA_REQUIRED`: upload a cover or gallery image, then retry analysis.
- `AUTHORIZATION_PENDING`: run a fresh `viceme auth login` in the active CLI context and keep that command active until browser authorization finishes.
- `NOT_LOGGED_IN` or `token_expired`: use the `viceme-shared` login workflow in the active CLI context. Never inspect or switch to another stored environment during publishing.
- `PUBLICATION_SCOPE_REQUIRED`: sign in again in the active CLI context to grant the required publication scopes.
- `PUBLICATION_RECOVERY_PERMISSION_REQUIRED`: the command process cannot write the local ViceMe publication recovery directory. Request filesystem permission for the exact same command and retry it with the same source and price. Do not delete lock files and do not create another publication.
- `SKILL_BINDING_PERMISSION_REQUIRED`: allow the exact command to write `.viceme/skill.json`, the adjacent ZIP sidecar, or the reported endpoint-scoped index directory, then retry with the same source identity.
- `SKILL_BINDING_SCOPE_MISMATCH` or `SKILL_LISTING_BINDING_OWNER_MISMATCH`: do not inspect or switch environments and do not overwrite the original owner's Listing. Ask whether the user explicitly wants `--new-listing` for a separate work in the active CLI context.
- `SKILL_LISTING_SOURCE_AMBIGUOUS`: show the candidate Listings, ask the user to choose, then use `skill listing bind`; never guess from title or filename.

If `retryable` is true, retry with bounded backoff and the same publication or client request identity. Otherwise change the input or state first.
