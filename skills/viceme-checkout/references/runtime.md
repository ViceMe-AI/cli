# Runtime integration

## Trust boundary

Only a trusted backend calls ViceMe runtime endpoints. Browser code calls the application's backend and receives only a Hosted Checkout URL. The CLI uses the environment Payment API Key from secure storage; application deployment uses its own authorized secret provider.

Never grant access from the browser return URL. Fulfill only after either:

- `payment order get` returns `PAID`; or
- a Webhook with a valid signature and a previously unseen event ID reports the paid state.

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

For a caller-selected amount, include `amountCents` within the active Price Version bounds. Omit it for fixed prices and payer-selected input prices.

## Query and close

```bash
viceme --profile <profile> payment order get <payment-no> --dir <project>
viceme --profile <profile> payment order close <payment-no> --dir <project> --yes
```

Close only an abandoned open order after user confirmation. A paid order is final for this capability; refunds are not implemented.

## Webhook verification contract

Verify the exact raw request bytes before JSON parsing. Use the endpoint's Webhook Signing Secret, not the Payment API Key. Validate timestamp tolerance, HMAC-SHA256 signature, event schema, environment ID/mode/market, and unique event ID before mutating application state. Return success idempotently for an already processed event.

During signing-secret rotation, accept either advertised key ID only within the overlap. Remove the old secret after the overlap ends. Browser return and Webhook delivery can arrive in either order.
