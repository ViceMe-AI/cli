# Generic merchant Product workflow

Use this workflow for services and physical/custom-made goods. It produces two
separate but linked results:

1. the creator's authored Work and sellable Product in ViceMe; and
2. a platform-generated, signed purchase Skill bound to exactly that Product.

The generated Skill is an entry into ViceMe quoting, payment, same-session
order status, and fulfillment. It is not the creator's service implementation.

## 1. Resolve the approved merchant

Run:

```text
viceme merchant accounts
viceme merchant product blueprints --merchant <merchant-account-id>
```

Only an active MerchantAccount for which the current User is the unique
`MerchantAccountMember(role=OWNER)` can author Products. If there is no
account, stop and explain that Staff must create or activate the Merchant and
bind the current User as OWNER. If multiple accounts are ever returned, show
them and ask which one owns this Work. Do not guess from display names or from
an optional CreatorChannel profile.

Verify that the returned active blueprint is `GENERIC_MERCHANT`. Merchant
Products may use only `MANUAL_PROCESSING` and `SHIPMENT`; never request
`PLATFORM_ADAPTER`, `DIGITAL_ENTITLEMENT`, or `RECHARGE_PHONE` through this
flow. Locked automatic Products are Staff-provisioned for an ordinary Merchant
with its own OWNER; there is no special platform merchant authority.

## 2. Collect the complete sale definition

Use natural language, but obtain concrete values for every required fact:

- Work title and stable kebab-case slug;
- Product title, slug, market, bilingual summaries/instructions where useful,
  description, tags, category/type label, and `PUBLIC`, `UNLISTED`, or
  `PRIVATE` visibility;
- each option, SKU, selected option combination, currency, unit price in cents,
  inventory policy, and allowed quantity range;
- pricing rule: `FIXED_SKU`, `UNIT_QUANTITY`, or `UNIT_PLUS_SHIPPING`, including
  default and regional shipping prices when applicable;
- every buyer-provided field: stable key, user-facing label, type, required
  flag, sensitivity, authorized audience, retention, and semantic role;
- fulfillment order: manual processing, shipment, or both.

Do not collect credentials, passwords, OTPs, tokens, or a buyer-supplied amount.
Price is always server-owned SKU data. Shipment requires required recipient
name, contact phone, and shipping address fields. Ask only for missing business
facts; a merchant who already supplied them does not need to answer them again.

## 3. Create one Work and one draft Product

Write a private strict JSON file and run:

```text
viceme merchant work create --input <work.json>
```

For a service-style purchase Skill, create a `SKILL` Work shell:

```json
{
  "kind": "SKILL",
  "merchantAccountId": "merchant-uuid",
  "clientRequestId": "uuid",
  "market": "CN",
  "slug": "photo-printing-service",
  "title": "照片打印服务"
}
```

Use the returned `id` as `subjectWorkId`. Then write the complete Product JSON
and run:

```text
viceme merchant product create --input <product.json>
```

The outer object is:

```json
{
  "merchantAccountId": "merchant-uuid",
  "subjectWorkId": "work-uuid",
  "clientRequestId": "uuid",
  "blueprintCode": "GENERIC_MERCHANT",
  "authoringInput": {}
}
```

Delete private temporary request files after the command. Keep returned IDs and
revisions in the active task state; do not save buyer data or access tokens in
the Work directory.

## 4. Compile and review the exact candidate

Run:

```text
viceme merchant product compile <product-id> \
  --merchant <merchant-account-id> \
  --expected-revision <revision>
```

Compilation must return no validation errors, one
`candidateSalesSpecVersionId`, one `candidateDigest`, and one
`candidatePurchaseSkill`. Show the user:

- Product title, visibility, every SKU and exact currency/price;
- quantity and shipping rule;
- all buyer fields, marking required and sensitive fields;
- ordered fulfillment steps;
- generated purchase Skill `stableName`, release ID, manifest digest, and
  distribution expectations;
- that payment success and fulfillment success are separate states.

Do not activate a stale candidate. Any Product edit increments the revision and
requires another compile and review.

## 5. Confirm once, publish the Work, and activate the candidate

Ask exactly once whether the user confirms the displayed definition and wants
to make this Product available now. State that activation enables real orders
and payment. A requested change is not approval.

After an unambiguous confirmation, first publish the subject Work with its exact
current revision:

```json
{
  "merchantAccountId": "merchant-uuid",
  "expectedRevision": 1,
  "status": "PUBLISHED"
}
```

```text
viceme merchant work update <work-id> --input <work-update.json>
```

Then build the activation JSON entirely from the same compile response:

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

Run:

```text
viceme merchant product activate <product-id> --input <activation.json>
```

Return its `productDetailUrl`, `purchaseSkillStableName`, Product ID, and these
usable commands. The detail URL must be the platform response's existing
`/{locale}/share/{product-slug}` route; never derive a separate
`/products/{purchase-skill-stable-name}` page in the Skill:

```text
viceme commerce skill install <stable-name> --agent auto
viceme commerce session start --skill <stable-name>
viceme commerce product describe --skill <stable-name> --session-context <local-context-id>
```

Keep the returned `localContextId` only in the current Agent task and pass it
to every subsequent `commerce product`, `asset`, `quote`, and `order` command.
Never reuse it in another conversation or create a replacement context to look
up an existing order.

`DIRECT=PUBLISHED` means the signed package can be installed directly.
`WORKBUDDY=READY` means a human may submit it for WorkBuddy review; it is not a
public WorkBuddy-store listing yet.

## Photo-printing reference shape

This abbreviated `authoringInput` demonstrates a ¥10 print plus ¥5 default
shipping. Preserve the user's actual names, sizes, prices, and policies:

```json
{
  "title": "照片打印",
  "slug": "photo-printing",
  "market": "CN",
  "summaryZhCn": "上传照片，商家打印后发货",
  "summaryEnUs": "Upload photos for printing and delivery",
  "description": "按所选规格打印照片并通过快递发货。",
  "usageInstructionsZhCn": "选择规格、上传照片并填写收件信息。",
  "usageInstructionsEnUs": "Choose a size, upload photos, and provide shipping details.",
  "tags": ["照片", "打印"],
  "visibility": "PUBLIC",
  "merchantTypeName": "照片打印服务",
  "options": [
    {
      "code": "size",
      "name": "尺寸",
      "values": [{ "code": "six-inch", "label": "6 英寸" }]
    }
  ],
  "skus": [
    {
      "code": "six-inch",
      "title": "6 英寸照片",
      "currency": "CNY",
      "priceCents": 1000,
      "inventoryPolicy": "MADE_TO_ORDER",
      "selectedOptions": { "size": "six-inch" },
      "attributes": {}
    }
  ],
  "quantity": { "min": 1, "max": 100 },
  "buyerContract": {
    "fields": [
      {
        "key": "photos",
        "label": "待打印照片",
        "kind": "image",
        "source": "BUYER_INPUT",
        "required": true,
        "sensitivity": "PERSONAL",
        "allowedAudiences": ["MERCHANT"],
        "retentionPolicy": "FULFILLMENT_WINDOW",
        "minItems": 1,
        "maxItems": 20,
        "maxSizeBytes": 10485760,
        "contentTypes": ["image/jpeg", "image/png"]
      },
      {
        "key": "recipient_name",
        "label": "收件人",
        "kind": "text",
        "source": "BUYER_INPUT",
        "required": true,
        "sensitivity": "PERSONAL",
        "allowedAudiences": ["MERCHANT", "SHIPPING"],
        "retentionPolicy": "FULFILLMENT_WINDOW",
        "semanticRole": "RECIPIENT_NAME",
        "maxLength": 100,
        "multiline": false
      },
      {
        "key": "contact_phone",
        "label": "联系电话",
        "kind": "phone",
        "source": "BUYER_INPUT",
        "required": true,
        "sensitivity": "PERSONAL",
        "allowedAudiences": ["MERCHANT", "SHIPPING"],
        "retentionPolicy": "FULFILLMENT_WINDOW",
        "semanticRole": "CONTACT_PHONE",
        "allowedCountryCodes": ["+86"]
      },
      {
        "key": "shipping_address",
        "label": "收货地址",
        "kind": "address",
        "source": "BUYER_INPUT",
        "required": true,
        "sensitivity": "PERSONAL",
        "allowedAudiences": ["MERCHANT", "SHIPPING"],
        "retentionPolicy": "FULFILLMENT_WINDOW",
        "semanticRole": "SHIPPING_ADDRESS",
        "allowedRegions": ["CN"]
      }
    ]
  },
  "fulfillment": [
    { "capabilityCode": "MANUAL_PROCESSING", "configuration": {} },
    { "capabilityCode": "SHIPMENT", "configuration": {} }
  ],
  "pricingPolicyCode": "UNIT_PLUS_SHIPPING",
  "shippingPricing": {
    "defaultAmountCents": 500,
    "regionAmounts": {}
  }
}
```

## Recovery and lifecycle

- If create/compile/activate returns an unknown result, list the same merchant's
  Works or Products and recover by client request ID/Product ID. Never create a
  replacement just to escape uncertainty.
- `viceme merchant product suspend` stops new sales while preserving existing
  session/order recovery. `archive` is terminal and may be rejected while a
  payable order or recoverable session remains.
- No cross-session order lookup exists. A buyer must keep using the exact local
  Commerce Session created for that purchase Skill.
