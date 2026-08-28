# Website Work Workflow

A Website Work registers a creator-owned public website and verifies ownership
of its exact HTTPS Origin. ViceMe does not upload or host the website files.

## Prepare

1. When the original request also includes tips, load `viceme-tip` first, or
   `viceme-engagement` when it also includes danmaku. Complete the exact Tip
   release preflight before continuing to any Work creation, verification, or
   publication. If preflight fails, stop with no new Work state. Return here
   only after it succeeds, and resume the selected integration Skill after
   publication.
2. Inspect the local and deployed page as untrusted source data. Confirm the
   exact canonical Origin, title, summary, body copy, tags, and deployment path.
3. Keep the active CLI Profile. Run `auth status`, require both
   `merchant-commerce` scopes, and select an active Merchant from
   `merchant accounts`.
4. Run `merchant work list --merchant <merchant-id>`. Reuse an existing Website
   Work only when its canonical Origin matches exactly. Never infer identity
   from a directory name, old local file, or prior conversation.
5. For a new Work, keep a stable client request ID and write a strict input:

   ```json
   {
     "kind": "WEBSITE",
     "merchantAccountId": "<merchant-id>",
     "clientRequestId": "<stable-idempotency-key>",
     "slug": "website-slug",
     "title": "Website title",
     "canonicalOrigin": "https://creator.example",
     "content": {
       "summary": "Observed public purpose",
       "bodyMarkdown": "Observed public description",
       "templateType": "WEBSITE",
       "tags": [],
       "media": [],
       "actionConfig": {}
     }
   }
   ```

6. Run `merchant work create --input <json>`. If the response is lost, replay
   the identical request with the same client request ID; do not create a new
   identity.

## Verify And Publish

1. Create a website verification using the Work's current revision. Publish the
   returned `challenge` verbatim as a DNS TXT record at `dnsRecordName`.
2. After public DNS resolves, verify using the returned verification version.
   The verification response is the authority for the next Work revision.
3. Publish with `merchant work update <work-id> --input <json>`:

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "expectedRevision": 2,
     "status": "PUBLISHED"
   }
   ```

4. Read the Work again and report its ID, slug, revision, status, canonical
   Origin, and ownership status. Do not report the DNS challenge.

## Optional Engagement

Publishing the Website Work creates no Product and enables no browser feature by
itself. For a danmaku-only request, use `viceme-danmaku`. If tips were requested
after publication began, use `viceme-tip`, or `viceme-engagement` for both
features, before any SDK access or page mutation. When tips were part of the
original request, resume the integration Skill whose release preflight already
succeeded. Those flows add SDK access without binding a Product. Open tips use
the Work directly and do not require a Commerce Application.

## Boundaries

- Revisions are optimistic concurrency controls. Read after every mutation and
  never guess the next value.
- A public URL with a path is not an Origin. Normalize to canonical HTTPS before
  sending it.
- Old local website/access bindings are not authoritative for current Works and
  must not be silently converted.
