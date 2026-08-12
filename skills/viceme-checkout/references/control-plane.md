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

Initialization creates the user's capability space if needed, one application, isolated SANDBOX and LIVE environments, and their Payment v1 installations. It selects SANDBOX locally and writes only IDs and market context to `.viceme/payment.yaml`.

To select the default LIVE management context:

```bash
viceme --profile <profile> payment environment use live --dir <project>
viceme --profile <profile> payment context --dir <project>
```

This does not grant real-money access. LIVE API Key creation remains blocked until an authorized Admin enables issuance for the Application. Switch back with `payment environment use sandbox`.

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
    {"channelCode":"WECHAT_PAY","allowedScenes":["NATIVE"]}
  ]
}
```

```bash
viceme --profile <profile> payment price-version create <price-id> --dir <project> --input price-version.json
viceme --profile <profile> payment price-version activate <price-version-id> --dir <project>
```

For caller-selected input amounts, use `amountMode: "INPUT"`, `amountInputSource: "CALLER"`, and bounded `minAmountCents`, `maxAmountCents`, optional `suggestedAmountCents`, and `stepAmountCents`. Do not create subscription Price Versions; subscription runtime is not delivered.

## Optional custom checkout template

Skip this section unless the user needs a custom hosted checkout presentation. Every Payment installation already contains a ViceMe-managed default template based on the Shop design. If no Product default and no `templateCode` are provided, Checkout uses that platform default.

For a custom template, create the exact strict configuration described by the API. Required groups are `layoutPreset`, `themeTokens`, `localizedCopy` for `zh-CN` and `en`, `fieldVisibility`, `channelDisplayPolicy`, `successContent`, and `cancelContent`.

```bash
viceme --profile <profile> payment template create --dir <project> --input template.json
```

Set `productId` and `isDefault: true` to make the custom template the Product default. Custom templates override the platform default only for their bound Product.

## Origins and return targets

```bash
viceme --profile <profile> payment origin create --dir <project> --input origin.json
viceme --profile <profile> payment origin verify <origin-id> --dir <project> --input challenge.json
viceme --profile <profile> payment return-target create --dir <project> --input return-target.json
```

`origin.json` is `{"origin":"https://example.com"}`. `challenge.json` contains the challenge returned by origin creation after the required DNS TXT record exists. A return target contains `key`, `applicationOriginId`, `successUrl`, and optional `cancelUrl`; both URLs must use the verified origin.

## API Key

```bash
viceme --profile <profile> payment api-key create --dir <project> --name payment-backend
viceme --profile <profile> payment api-key deliver --dir <project> --env-file .env.local
viceme --profile <profile> payment api-key rotate --dir <project>
viceme --profile <profile> payment api-key revoke --dir <project> --reason '<reason>' --yes
```

Default scopes cover product reads, checkout creation, order reads, and order close. Pass `--scopes` for a smaller comma-separated set. The response never contains the raw key.

The key is bound to the selected environment. A LIVE key can be issued only after `payment environment use live` selects the default LIVE environment and an Admin has enabled LIVE API Key issuance; its prefix is `vcp_live_` and it remains server-only.

`payment api-key deliver` reads the selected environment's key from CLI secure storage and writes it directly to a protected, project-local dotenv file as `VICEME_PAYMENT_API_KEY` by default. It returns only file/variable/credential metadata. Run it again after rotation. Do not use it for `.env.example`, frontend-exposed variables, or remote deployment configuration.

## Webhook

```bash
viceme --profile <profile> payment webhook create --dir <project> --input webhook.json
viceme --profile <profile> payment webhook verify <endpoint-id> --dir <project>
viceme --profile <profile> payment webhook list --dir <project>
viceme --profile <profile> payment webhook rotate-secret <endpoint-id> --dir <project>
```

`webhook.json` contains `url` and a unique `eventTypes` list. The CLI stores the signing secret securely and outputs only endpoint/signing-key metadata. Use `abort-rotation <rotation-id> --yes` if a new secret cannot be deployed during its overlap. Use `revoke <endpoint-id> --yes` only for immediate shutdown.
