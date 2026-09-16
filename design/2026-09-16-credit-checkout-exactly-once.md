# Credit checkout: exactly-once wallet crediting (coupon + card paths)

Date: 2026-09-16

## Incident (2026-09-15)

A coupon redemption was active, Stripe completed checkout and sent a $0
receipt for a $5 subtotal discounted to $0, but the Helix organization
balance remained zero.

## Flow (shared path)

```
createTopUp (wallet_handlers.go)        → lookupOrg → canonical org ID in metadata
GetTopUpSessionURL (stripe_topups.go)   → Checkout session (payment mode,
                                          AllowPromotionCodes, metadata:
                                          type/user_id/org_id/topup_amount_cents)
Stripe Checkout                         → coupon redemption
ProcessWebhook (stripe.go)              → checkout.session.completed AND/OR
                                          payment_intent.succeeded
handleTopUp* (stripe_topups.go)         → getTopUpWallet → UpdateWalletBalance
UpdateWalletBalance (store_wallet.go)   → row lock + topUpAlreadyProcessed dedupe
                                          (by stripe_checkout_session_id /
                                          stripe_payment_intent_id)
```

The return URL (`?success=true&session_id=…`) triggers no mutation on the
Helix side; the wallet page only reads. Crediting happens exclusively in the
webhook handlers.

## Root causes (all proven with a real-Postgres webhook harness)

1. **Failed webhook deliveries were answered with 200.** `ProcessWebhook`
   logged handler errors and returned 200, so Stripe never retried. Any
   transient failure on the single crediting event permanently lost a paid
   purchase's credits — the "balance remained zero" class of failure.
2. **`handleTopUpEvent` hard-errored on legitimate zero-amount payment
   intents** (`topUpAmountDollars`: "topup amount must be greater than 0").
   A fully discounted coupon purchase produces exactly that: a $0
   PaymentIntent (the $0 receipt) whose metadata carries the *requested*
   amount. When the metadata was missing the event errored and, combined with
   (1), was lost forever.
3. **Foreign payment intents errored instead of skipping** — harmless today
   (swallowed) but a permanent retry storm once (1) is fixed.

Empirical harness (signed real-format payloads → `ProcessWebhook` → real
Postgres) confirmed:

- Normal card payment (both events, either order, retries): exactly one
  credit, org wallet — already correct.
- Fully discounted coupon with a $0 PaymentIntent carrying the requested
  amount in metadata: exactly one credit via the dedupe — already correct,
  but had zero regression coverage.
- Fully discounted coupon where the session event reports
  `payment_intent: null` and a $0 `payment_intent.succeeded` still arrives:
  the session-completed row cannot cross-dedupe by payment intent and the
  wallet was credited twice. Per Stripe's contract the session references its
  PaymentIntent whenever one exists, so this combination should not occur;
  it is documented here rather than papered over with a heuristic (any
  wallet+amount time-window heuristic would silently drop a legitimate
  second same-amount coupon purchase).

## Fix (shared path, minimal diff)

`api/pkg/stripe/stripe.go` — top-up webhook handlers return 500 on failure so
Stripe retries; the store's `topUpAlreadyProcessed` dedupe keeps retries at
exactly-once. Subscription/invoice handlers unchanged.

`api/pkg/stripe/stripe_topups.go` — `handleTopUpEvent` skips cleanly (200,
log line) instead of erroring for (a) payment intents without topup metadata
and (b) zero-amount topups without the requested-amount metadata (the session
event owns those purchases).

## Operational requirement (not fixable in code)

The Stripe webhook endpoint must deliver **both** `checkout.session.completed`
and `payment_intent.succeeded`. Either one alone credits every realistic
purchase shape (with the store dedupe preventing overlap); if neither
arrives, nothing can credit. Check the endpoint's event subscription in the
Stripe dashboard when investigating missed credits.

## Regression coverage

- `api/pkg/stripe/stripe_topups_integration_test.go` — real signed Stripe
  wire payloads through `ProcessWebhook` into a real Postgres store (skips
  without `POSTGRES_HOST`; CI runs it against the postgres sidecar):
  - coupon purchase (both event orders × retries) → org wallet credited
    exactly once;
  - coupon purchase without a PaymentIntent → exactly once;
  - normal card payment (both event orders × retries) → exactly once;
  - incomplete payment (`unpaid`) → no credits;
  - $0 payment intent without amount metadata → clean skip, no credits;
  - $0 payment intent with metadata → credits the requested $5 (the $0
    receipt case) exactly once;
  - foreign payment intent → skipped, no credits.
- `api/pkg/stripe/stripe_topups_test.go` — ProcessWebhook returns 500 on a
  failed wallet mutation (so Stripe retries) and 200 for foreign payment
  intents.
