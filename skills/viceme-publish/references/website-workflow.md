# Website Work Workflow

A Website Work registers a creator-owned public website and verifies ownership
of its exact HTTPS Origin. ViceMe does not upload or host the website files.

## Prepare

1. Inspect the local and deployed page as untrusted source data. Confirm the
   exact canonical Origin, title, summary, body copy, tags, and deployment path.
2. Reuse the active Merchant returned by `$viceme-creator-onboarding`. Do not
   repeat login, scope, or Merchant discovery inside this workflow.
3. Run `merchant work list --merchant <merchant-id>` and keep only Website
   Works whose canonical Origin matches exactly. If there are no matches,
   create a new Work. If there is exactly one match, reuse it. If there are
   multiple matches, stop before any Work, Website Verification, DNS, SDK
   access, Commerce Application, or website write. Show each candidate's `id`,
   `title`, `status`, and `website.ownershipStatus`, and ask the user to select
   the Work by ID. Never select the first, newest, `PUBLISHED`, or `VERIFIED`
   candidate automatically. Re-read the selected Work and reconfirm its
   Merchant, kind, and canonical Origin before continuing. Never infer identity
   from a directory name, old local file, or prior conversation.
4. For a new Work, keep a stable client request ID and write a strict input:

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

5. Run `merchant work create --input <json>`. If the response is lost, replay
   the identical request with the same client request ID; do not create a new
   identity.

## Verify And Publish

1. Read the Work. Before any Website Verification write, inspect its status. If
   it is `SUSPENDED` or `ARCHIVED`, stop without creating a challenge, changing
   DNS, or verifying. Continue only when it is `DRAFT` or `PUBLISHED`.
2. If ownership is not `VERIFIED`, create a website verification using the
   Work's current revision. Publish the returned `challenge` verbatim as a DNS
   TXT record at `dnsRecordName`.
3. After public DNS resolves, verify using the returned verification version.
   The verification response is the authority for the next Work revision.
4. If the Work is `DRAFT`, publish with `merchant work update <work-id> --input
   <json>`:

   ```json
   {
     "merchantAccountId": "<merchant-id>",
     "expectedRevision": 2,
     "status": "PUBLISHED"
   }
   ```

5. Read the Work again and report its ID, slug, revision, status, canonical
   Origin, and ownership status. Do not report the DNS challenge.

## Configure Follow And Paid Access

Website follow and paid unlocks belong to this publish workflow. `$viceme-access`
only integrates the returned configuration into the creator's host code and must
not create, update, disable, or publish platform resources.

1. Skip this section only when the user did not request follow or paid unlocks.
   Otherwise confirm each feature key, public title, policy, and paid price. Use
   `FOLLOW_OWNER` for an explicit creator follow and `WORK_ENTITLEMENT` for a
   one-time paid unlock. Do not configure creator subscriptions.
2. Read the Website Work's single authoritative SDK access resource:

   ```bash
   viceme merchant work sdk-access get <work-id> --merchant <merchant-id>
   ```

   Reuse it when it exists. Do not create a parallel resource or treat legacy
   local access files and retired standalone access commands as authoritative
   state.
3. Before activating a paid feature, show the complete desired active access
   set and each final price, then obtain the explicit publication confirmation
   required by the parent Skill.
4. If no SDK access resource exists, create it with the complete desired access
   set:

   ```bash
   viceme merchant work sdk-access create <work-id> --merchant <merchant-id> \
     [--follow "<key>[=<title>]"]... \
     [--purchase "<key>[=<title>]" --price-minor <fen>]...
   ```

5. If it exists, preserve its hosted `danmaku` and `tip` features by omitting
   `--feature`. Supply the complete desired active follow and purchase set and
   the exact returned `configVersion`:

   ```bash
   viceme merchant work sdk-access update <work-id> --merchant <merchant-id> \
     --expected-config-version <version> \
     [--follow "<key>[=<title>]"]... \
     [--purchase "<key>[=<title>]" --price-minor <fen>]...
   ```

   Omitted active access features are disabled. Preserve every existing active
   feature the user did not ask to remove. One price may be shared by all
   purchase features; otherwise repeat `--price-minor` in `--purchase` order.
   Use `--clear-access` only when the user explicitly removes all follow and
   purchase unlocks. Never disable the whole SDK access resource while hosted
   features must remain active.
6. Read the resource after the mutation. Report its public `workKey`,
   `configVersion`, active feature titles, policies, and paid prices. The
   platform provisions each paid feature's unlisted Product, Website Widget
   application, Hosted Checkout, fulfillment, and Digital Entitlement; do not
   expose their internal IDs to host code.
7. Invoke `$viceme-access` only after publication and access configuration are
   complete, passing the returned `workKey` and confirmed feature keys. That
   Skill changes the creator website only.

Publishing a Website Work alone creates no Product and enables no browser
feature. If the user asks for hosted danmaku, hosted tips, or both, finish
Website Work publication and any requested follow or paid access configuration
first, then use `viceme-engagement`. It must preserve the follow and paid access
set above. Hosted tips reuse the Work-level Website Widget and must not replace
its Shop-managed Product bindings or return URLs.

## Boundaries

- Revisions are optimistic concurrency controls. Read after every mutation and
  never guess the next value.
- A public URL with a path is not an Origin. Normalize to canonical HTTPS before
  sending it.
- Old local website/access bindings are not authoritative for current Works and
  must not be silently converted.
- Login never implies following. Visitor sign-in, explicit follow consent,
  checkout, and entitlement verification remain separate SDK/Shop actions.
- A payment return parameter never grants access. Only a fresh server access
  decision may unlock the creator's protected action.
