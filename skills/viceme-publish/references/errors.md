# Publication error handling

- `SKILL_PRICE_CONFIRMATION_REQUIRED`: ask for the exact CNY price in fen.
- `SKILL_SECRET_DETECTED` or `SKILL_SENSITIVE_FILE`: stop and remove credentials or sensitive files from the package. Never print their contents.
- `PUBLICATION_SOURCE_CHANGED`: the recovery package differs from the started publication; restore it or start a new publication.
- `SKILL_PUBLICATION_REVIEW_CHANGED`: fetch and show the latest review, then obtain a new combined confirm-and-publish authorization for its new digest.
- `SKILL_LISTING_MEDIA_REQUIRED`: upload a cover or gallery image, then retry analysis.
- `AUTHORIZATION_PENDING`: wait for the instructed interval and retry before the device code expires.
- `NOT_LOGGED_IN` or `token_expired`: use the `viceme-shared` login workflow for the same pinned publication Profile. Never recover by switching to another stored Profile.
- `PUBLICATION_SCOPE_REQUIRED`: sign in again to the same publication Profile to grant the required publication scopes.
- `PUBLICATION_RECOVERY_PERMISSION_REQUIRED`: the command process cannot write the local ViceMe publication recovery directory. Request filesystem permission for the exact same command and retry it with the same Profile, source, and price. Do not delete lock files and do not create another publication.

If `retryable` is true, retry with bounded backoff and the same publication or client request identity. Otherwise change the input or state first.
