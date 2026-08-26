# Generic merchant Product workflow

Use this workflow only when the offering needs server-owned price, Quote,
Order, payment, or Commerce fulfillment. For a no-price direct service, use
`interaction-definition.md` and do not create a zero-price Product or Order.
This workflow always produces one real creator Work before a sellable Product:

1. a `SERVICE` Work with permanent creator/slug identity and versioned public
   content;
2. the merchant's Product and immutable sales specification under that Work;
3. a platform-generated, signed purchase Skill bound to exactly that Product.

The generated Skill is an entry into ViceMe quoting, payment, same-session
order status, and fulfillment. It is not the creator's service implementation.

## 1. Resolve the approved merchant

Run:

```text
viceme merchant accounts
viceme merchant product templates --merchant <merchant-account-id>
```

Only an active MerchantAccount for which the current User is the unique
`MerchantAccountMember(role=OWNER)` can author Products. If there is no
account, stop and explain that Staff must create or activate the Merchant and
bind the current User as OWNER. If multiple accounts are ever returned, show
them and ask which one owns this Product. Do not guess from display names or from
the CreatorAccount profile. A public Product also requires that the Merchant is
linked to one active CreatorAccount; its stable handle owns the permanent Work
route. CreatorAccount and verified external identities are attribution, not
write authority.

Verify that the returned active authoring template is `GENERIC_MERCHANT`.
Merchant Products may currently use only `MANUAL_PROCESSING` and `SHIPMENT`;
never claim a provider adapter exists. An official creator is an ordinary
Creator/Merchant/OWNER relation, not a special platform principal.

## 2. Collect the complete sale definition

Use natural language, but obtain concrete values for every required fact:

- Work title, permanent slug, public summary/body, tags, usage or service
  instructions, and whether this is a new Work or a new revision of an owned
  Work;
- Product title, slug, market, bilingual summaries/instructions where useful,
  description, tags, category/type label, and `PUBLIC`, `UNLISTED`, or
  `PRIVATE` visibility;
- each option, SKU, selected option combination, currency, unit price in cents,
  inventory policy, and allowed quantity range;
- pricing rule: `FIXED_SKU`, `UNIT_QUANTITY`, or `UNIT_PLUS_SHIPPING`, including
  default and regional shipping prices when applicable;
- every buyer-provided field: stable key, user-facing label, type, required
  flag, sensitivity, authorized audience, retention, and semantic role;
- fulfillment order: manual processing, shipment, or both;
- execution mode: use `FULFILLMENT_ONLY` when Commerce fulfillment alone owns
  the post-order lifecycle; use `INTERACTION` when participants need a
  persistent staged workflow;
- for `INTERACTION`, compile the Product candidate first, then create and review
  a `PURCHASE` Interaction Definition for that same Work revision. Activate the
  confirmed Definition before activating the already compiled Product
  candidate. Do not declare the offering ready or create any Quote until both
  are active. Progress lookup remains limited to the original Commerce Session
  or a claimed registered user.

Do not collect credentials, passwords, OTPs, tokens, or a buyer-supplied amount.
Price is always server-owned SKU data. Shipment requires required recipient
name, contact phone, and shipping address fields. Ask only for missing business
facts; a merchant who already supplied them does not need to answer them again.

## 3. Create or update the real Service Work

Every public Product requires a real Work. For an existing Work, resolve it
from `viceme merchant work list --merchant <merchant-account-id>` and update a
new draft revision. For a new offering, write a private strict JSON file and
run:

```text
viceme merchant work create --input <work.json>
```

The minimal shape of a transaction-backed Service Work is:

```json
{
  "kind": "SERVICE",
  "merchantAccountId": "merchant-uuid",
  "clientRequestId": "uuid",
  "market": "CN",
  "slug": "service-offering",
  "title": "Service offering",
  "content": {
    "summary": "Public description of the offered outcome.",
    "bodyMarkdown": "## Service\n\nPublic terms and expected outcome.",
    "templateType": "service",
    "tags": [],
    "usageInstructions": "Select the offering and provide checkout-required information.",
    "serviceInstructions": "Commerce fulfillment owns the post-order lifecycle.",
    "media": [],
    "actionConfig": {}
  },
  "service": {
    "intakeSchema": {},
    "stages": [
      { "code": "PROCESSING", "label": "处理中", "terminal": false },
      { "code": "COMPLETED", "label": "已完成", "terminal": true }
    ],
    "policy": {
      "executionMode": "FULFILLMENT_ONLY"
    }
  }
}
```

For `INTERACTION`, read `scenario-analysis.md` and publish a confirmed
`PURCHASE` Interaction Definition for the same Work before creating Quotes.
Keep only transaction-required information in the Product buyer contract.
Progressive service input belongs to Interaction actions, tasks, records, and
artifacts; never duplicate it in checkout. Do not put private participant data
in public Work content or `intakeSchema`.

Use the returned Work `id` as the mandatory `subjectWorkId`. Then write the
complete Product JSON and run:

```text
viceme merchant product create --input <product.json>
```

The outer object is:

```json
{
  "merchantAccountId": "merchant-uuid",
  "clientRequestId": "uuid",
  "subjectWorkId": "work-uuid",
  "authoringTemplateCode": "GENERIC_MERCHANT",
  "authoringInput": {}
}
```

Delete private temporary request files after the command. Keep returned IDs and
revisions in the active task state; do not save buyer data or access tokens in
the source or Skill directory.

## 4. Compile and review the exact candidate

Run:

```text
viceme merchant product compile <product-id> \
  --merchant <merchant-account-id> \
  --expected-revision <revision>
```

Compilation must return no validation errors, one
`candidateSalesSpecVersionId`, one `candidateDigest`, and one
`candidatePurchaseSkill`. Only after that exact candidate exists, create a
private preview grant for the Work revision used by the Product:

```text
viceme merchant work preview create <work-id> \
  --merchant <merchant-account-id> \
  --expected-revision <revision>
```

Display both returned `htmlUrl` and `markdownUrl`. Both URLs must project the
same Work revision and the same compiled Product, price, buyer contract,
service stages, and generated purchase Skill candidate. A preview must never
show an install or purchase action as active before activation. Never treat a
preview URL as the permanent public identity. Revoke an unused preview with
`viceme merchant work preview revoke <work-id> <preview-id> --merchant
<merchant-account-id>`.

Show the user:

- Product title, visibility, every SKU and exact currency/price;
- quantity and shipping rule;
- all buyer fields, marking required and sensitive fields;
- ordered fulfillment steps;
- generated purchase Skill `stableName`, release ID, manifest digest, and
  distribution expectations;
- the exact HTML and Markdown preview URLs for this candidate;
- that payment success and fulfillment success are separate states.

Do not activate a stale candidate. Any Work or Product edit requires another
compile and another HTML/Markdown preview before review.

For `INTERACTION`, also create, preview, and display the exact `PURCHASE`
Interaction Definition described in `interaction-definition.md`. It must target
this Work and the reviewed Work revision. Obtain one confirmation covering the
Product candidate and Interaction Definition candidate; neither candidate may
change after that confirmation.

## 5. Confirm once and activate the complete offering

Ask exactly once whether the user confirms the displayed definition and wants
to make this Product available now. State that activation enables real orders
and payment. A requested change is not approval.

After an unambiguous confirmation, build the activation JSON entirely from the
same compile response:

```json
{
  "merchantAccountId": "merchant-uuid",
  "expectedRevision": 2,
  "candidateSalesSpecVersionId": "uuid",
  "candidateDigest": "64-lowercase-hex",
  "candidateSkillReleaseId": "uuid",
  "candidatePurchaseManifestDigest": "64-lowercase-hex"
}
```

For `INTERACTION`, activate the confirmed Interaction Definition first. Its
activation publishes the reviewed Work revision and freezes the runtime
contract. Then run:

```text
viceme merchant product activate <product-id> --input <activation.json>
```

Return its `productDetailUrl`, `purchaseSkillStableName`, and Product ID. The
activation transaction publishes the current Work revision, Product sales
specification, ServiceOffering version when present, and signed purchase Skill
together. For `FULFILLMENT_ONLY`, never publish the Work separately immediately
before Product activation because that can expose a page whose action is not
yet executable. For `INTERACTION`, Definition-first activation is required;
Product activation rejects a missing, mismatched, or non-`PURCHASE` Definition
with `INTERACTION_DEFINITION_REQUIRED`.

The detail URL must be the platform response's permanent locale-free
`/{creatorHandle}/{workSlug}` route; never derive a `/share`, locale-prefixed,
or `/products/{purchase-skill-stable-name}` page in the Skill. Tell the merchant
that the page's “copy to WorkBuddy / Codex” action emits the platform-owned
`commerce-skill-install.md` contract plus the exact signed-Skill installation
command. Do not expose a temporary ZIP URL, digest, expiry, or API origin, and
do not start a buyer Commerce Session merely to finish publication.

If the merchant explicitly asks for a local buyer-side smoke test after
activation, use only the installed purchase Skill's deterministic
`viceme commerce flow start --skill <stable-name>` entry. Keep its returned
`localContextId` in that one test conversation and never create a replacement
context to look up an existing order.

Every generated stable name must start with `viceme-`.
`DIRECT=PUBLISHED` means the signed package can be installed directly.
`WORKBUDDY=READY` means a human may submit it for WorkBuddy review; it is not a
public WorkBuddy-store listing yet.

## Recovery and lifecycle

- If create/compile/activate returns an unknown result, list the same merchant's
  Works or Products and recover by client request ID/Work ID/Product ID. Never
  create a replacement just to escape uncertainty.
- `viceme merchant product suspend` stops new sales while preserving existing
  session/order recovery. `archive` is terminal and may be rejected while a
  payable order or recoverable session remains.
- No cross-session order lookup exists. A buyer must keep using the exact local
  Commerce Session created for that purchase Skill.
