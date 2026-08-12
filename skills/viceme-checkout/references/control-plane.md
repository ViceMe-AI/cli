# Payment control plane

All commands return one JSON envelope on stdout. Pin the authenticated profile and project directory:

```bash
viceme --profile <profile> payment context --dir <project>
```

## Initialize

```bash
viceme --profile <profile> payment eligibility
viceme --profile <profile> payment init --dir <project> --slug <app-slug> --name <display-name>
```

Initialization creates the user's capability space if needed, one application, a SANDBOX environment, and the Payment v1 installation. It writes only IDs and market context to `.viceme/payment.yaml`.

## Product and one-time fixed price

Product request:

```json
{
  "code": "pro-plan",
  "name": "Pro Plan",
  "description": "One-time access",
  "supportUrl": "https://example.com/support",
  "supportEmail": "support@example.com",
  "defaultLocale": "en",
  "status": "ACTIVE"
}
```

```bash
viceme --profile <profile> payment product create --dir <project> --input product.json
```

Price request and command:

```json
{"code":"default-price"}
```

```bash
viceme --profile <profile> payment price create <product-id> --dir <project> --input price.json
```

Fixed one-time Price Version request:

```json
{
  "billingMode": "ONE_TIME",
  "amountMode": "FIXED",
  "fixedAmountCents": 990,
  "currency": "CNY",
  "channels": [
    {"channelCode":"WECHAT_PAY","allowedScenes":["NATIVE"]},
    {"channelCode":"ALIPAY","allowedScenes":["PAGE"]}
  ]
}
```

```bash
viceme --profile <profile> payment price-version create <price-id> --dir <project> --input price-version.json
viceme --profile <profile> payment price-version activate <price-version-id> --dir <project>
```

For caller-selected input amounts, use `amountMode: "INPUT"`, `amountInputSource: "CALLER"`, and bounded `minAmountCents`, `maxAmountCents`, optional `suggestedAmountCents`, and `stepAmountCents`. Do not create subscription Price Versions; subscription runtime is not delivered.

## Checkout template

Create the exact strict configuration described by the API. Required groups are `layoutPreset`, `themeTokens`, `localizedCopy` for `zh-CN` and `en`, `fieldVisibility`, `channelDisplayPolicy`, `successContent`, and `cancelContent`.

```bash
viceme --profile <profile> payment template create --dir <project> --input template.json
```

Set `productId` and `isDefault: true` to bind a default template to a Product.

## Origins and return targets

```bash
viceme --profile <profile> payment origin create --dir <project> --input origin.json
viceme --profile <profile> payment origin verify <origin-id> --dir <project> --input challenge.json
viceme --profile <profile> payment return-target create --dir <project> --input return-target.json
```

`origin.json` is `{"origin":"https://example.com"}`. `challenge.json` contains the challenge returned by origin creation after the required DNS TXT record exists. A return target contains `key`, `applicationOriginId`, `successUrl`, and optional `cancelUrl`; both URLs must use the verified origin.

## API Key

```bash
viceme --profile <profile> payment api-key create --dir <project> --name sandbox-backend
viceme --profile <profile> payment api-key rotate --dir <project>
viceme --profile <profile> payment api-key revoke --dir <project> --reason '<reason>' --yes
```

Default scopes cover product reads, checkout creation, order reads, and order close. Pass `--scopes` for a smaller comma-separated set. The response never contains the raw key.

## Webhook

```bash
viceme --profile <profile> payment webhook create --dir <project> --input webhook.json
viceme --profile <profile> payment webhook verify <endpoint-id> --dir <project>
viceme --profile <profile> payment webhook list --dir <project>
viceme --profile <profile> payment webhook rotate-secret <endpoint-id> --dir <project>
```

`webhook.json` contains `url` and a unique `eventTypes` list. The CLI stores the signing secret securely and outputs only endpoint/signing-key metadata. Use `abort-rotation <rotation-id> --yes` if a new secret cannot be deployed during its overlap. Use `revoke <endpoint-id> --yes` only for immediate shutdown.
