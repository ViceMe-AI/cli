# Runtime integration

## Trust boundary

Only a trusted backend calls ViceMe runtime endpoints. Browser code calls the application's backend and receives only a Hosted Checkout URL. The CLI uses the environment Payment API Key from secure storage; application deployment uses its own authorized secret provider.

Never grant access from the browser return URL. Fulfill only after either:

- `payment order get` returns `PAID`; or
- a Webhook with a valid signature and a previously unseen event ID reports the paid state.

## Deliver the backend credential

After issuing the selected environment's Payment API Key, deliver it directly from CLI secure storage to the backend's project-local dotenv file:

```bash
viceme --profile <profile> payment api-key deliver --dir <project> --env-file .env.local
```

The default variable is `VICEME_PAYMENT_API_KEY`; use `--env-var` only when the backend already has a different server-only convention. The command never prints the value. It restricts the dotenv file to the current user, adds an exact project `.gitignore` rule, and refuses an existing Git-tracked file, path escape, symlink, `.env.example`/template file, or duplicate variable definition. Application code reads the variable only on the server and sends it as the bearer credential for `/v1/checkout/v1/*`.

Rerun delivery after API Key rotation so the backend receives the successor key during the overlap window. Local dotenv delivery does not configure a remote deployment; use the deployment platform's authorized secret provider for that boundary.

## Create a checkout

Request file:

```json
{
  "externalOrderNo": "order_20260812_0001",
  "productCode": "pro-plan",
  "priceCode": "default-price",
  "customerReference": "user_123",
  "returnTargetKey": "app",
  "metadata": {"tenant":"tenant_123"}
}
```

```bash
viceme --profile <profile> payment checkout products --dir <project>
viceme --profile <profile> payment checkout create --dir <project> --input checkout.json --idempotency-key checkout-order_20260812_0001
```

Retry the same logical operation with the same idempotency key and identical body. Use a new external order number and key for a new purchase. Persist the returned checkout session ID and `checkoutUrl` with the application's order.

`templateCode` is optional. Omit it to use the Product default when one exists, otherwise the persisted ViceMe platform default. Send `templateCode` only for an existing custom template bound to the selected Product; an explicit unknown code is an error and does not silently fall back.

For a caller-selected amount, include `amountCents` within the active Price Version bounds. Omit it for fixed prices and payer-selected input prices.

For LIVE, confirm `payment context` reports `mode: LIVE`, `marketRegion: CN`, and use only an active `WECHAT_PAY` Price Version. The Hosted Checkout returns a real WeChat payment action; payment truth still comes only from an authoritative order query or verified Webhook, never from browser navigation.

## Query and close

```bash
viceme --profile <profile> payment order get <payment-no> --dir <project>
viceme --profile <profile> payment order close <payment-no> --dir <project> --yes
```

Close only an abandoned open order after user confirmation. A paid order is final for this capability; refunds are not implemented.

## Webhook verification contract

Verify the exact raw request bytes before JSON parsing. Use the endpoint's Webhook Signing Secret, not the Payment API Key. Validate timestamp tolerance, HMAC-SHA256 signature, event schema, environment ID/mode/market, and unique event ID before mutating application state. Return success idempotently for an already processed event.

During signing-secret rotation, accept either advertised key ID only within the overlap. Remove the old secret after the overlap ends. Browser return and Webhook delivery can arrive in either order.
