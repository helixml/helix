# Fix credit checkout so completed purchases credit the org wallet exactly once

## Summary

A completed credit purchase could leave the intended organization wallet at zero. Observed 2026-09-15: a fully discounted coupon purchase ($5 subtotal discounted to $0) completed on Stripe with a $0 receipt, but the Helix balance stayed at zero.

Root causes, both in the shared webhook path (`api/pkg/stripe/`), found by tracing the full flow (checkout session creation → Stripe → `ProcessWebhook` → org attribution → wallet mutation → idempotency) and reproducing with real signed Stripe wire payloads against a real Postgres store:

- `ProcessWebhook` answered failed top-up handlers with 200, so Stripe never retried. Any transient failure on the single crediting event permanently lost a paid purchase's credits.
- `handleTopUpEvent` hard-errored on zero-amount payment intents without amount metadata ("topup amount must be greater than 0") — exactly the shape of a fully discounted coupon purchase's $0 PaymentIntent (the $0 receipt). Combined with the swallowed errors, the coupon purchase credited nothing.
- Foreign payment intents (no topup metadata) errored instead of skipping, which would become a permanent retry storm once failures surfaced as non-2xx.

Changes:

- Top-up webhook handlers (`checkout.session.completed`, `payment_intent.succeeded`) now return 500 on failure so Stripe retries the delivery. The wallet store already deduplicates retries by checkout session / payment intent ID (`topUpAlreadyProcessed`), so each purchase credits the wallet exactly once — including retries and either event order.
- `handleTopUpEvent` skips cleanly (200 + log line) instead of erroring for payment intents without topup metadata and for zero-amount topups without the requested-amount metadata (the session event owns those purchases; it carries the session metadata).
- Crediting still uses the org ID from checkout metadata (`createTopUp` resolves it via `lookupOrg` to the canonical ID), so credits land on the organization selected at checkout.

The return URL (`?success=true&session_id=…`) triggers no Helix-side mutation, so refreshing or revisiting it cannot duplicate credits; re-delivered webhook events are deduplicated by the store. Failed or incomplete payments (`unpaid`) never grant credits.

One operational note (documented in `design/2026-09-16-credit-checkout-exactly-once.md`): the Stripe webhook endpoint must deliver both `checkout.session.completed` and `payment_intent.succeeded` — either alone credits every realistic purchase shape, but neither arriving means nothing can credit.

## Testing

Tested in the dev sandbox (the environment has no Stripe keys, so a live hosted-checkout run was not possible; the deepest permitted verification is real signed webhook deliveries against the real store, which is what the new integration suite does):

- New `api/pkg/stripe/stripe_topups_integration_test.go` delivers real signed Stripe wire-format payloads through `ProcessWebhook` into a real Postgres store (skips without `POSTGRES_HOST`; CI runs it against the postgres sidecar):
  - Coupon purchase with $0 receipt ($5 subtotal, $0 collected, metadata carrying the requested amount): checkout.session.completed + payment_intent.succeeded in both event orders, each with repeated deliveries → org wallet credited $5.00 exactly once, single top_ups row.
  - Coupon purchase without a PaymentIntent: session event alone, redelivered → exactly once.
  - Normal card payment (no coupon): both events, both orders, retries → org wallet credited $5.00 exactly once.
  - Incomplete payment (`payment_status: unpaid`) → no credits, no top-up rows.
  - $0 payment intent without amount metadata → clean 200 skip, no credits.
  - $0 payment intent with metadata → credits the requested $5 (the $0 receipt case) exactly once.
  - Foreign payment intent → 200, no credits.
- New unit tests in `api/pkg/stripe/stripe_topups_test.go`: `ProcessWebhook` returns 500 on a failed wallet mutation (so Stripe retries) and 200 for foreign payment intents.
- Verified store-layer idempotency separately: `TestUpdateWalletBalance_TopUpIdempotency` and the full `TestWalletTestSuite` against a real Postgres instance — pass.
- Full `pkg/stripe` suite (unit + integration) green with Postgres; `pkg/server` wallet handler tests green; `go build ./...` clean; integration suite verified to skip cleanly when no Postgres is configured.
