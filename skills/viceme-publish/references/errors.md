# Publication error handling

- `SKILL_PRICE_CONFIRMATION_REQUIRED`: ask for the exact CNY price in fen.
- `SKILL_SECRET_DETECTED` or `SKILL_SENSITIVE_FILE`: stop and remove credentials or sensitive files from the package. Never print their contents.
- `PUBLICATION_SOURCE_CHANGED`: the recovery package differs from the started publication; restore it or start a new publication.
- `SKILL_PUBLICATION_REVIEW_CHANGED`: fetch and show the latest review, then obtain confirmation for its new digest.
- `SKILL_LISTING_MEDIA_REQUIRED`: upload a cover or gallery image, then retry analysis.
- `AUTHORIZATION_PENDING`: wait for the instructed interval and retry before the device code expires.
- `NOT_LOGGED_IN` or `token_expired`: use the `viceme-shared` login workflow.

If `retryable` is true, retry with bounded backoff and the same publication or client request identity. Otherwise change the input or state first.
