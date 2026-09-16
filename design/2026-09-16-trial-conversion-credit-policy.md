# Trial conversion credit policy (72-hour onboarding trial)

Observed 2026-09-15: the 72-hour onboarding trial started with no credits and
the signup copy did not say whether or when credits would be granted. Chris
expected $100 in credits after the first successful bill. This note records
the policy and its source of truth before any code change.

## Established policy

1. **Trial start**: no credits. `STRIPE_INITIAL_BALANCE` defaults to 0
   ("new signups arrive with an empty wallet"; on-prem can override). The
   trial's $0 `subscription_create` invoice must never grant the monthly
   allotment.
2. **Trial → paid conversion**: the first successful bill (real charge at
   trial end, `invoice.paid` with `amount_due > 0`, subscription `active`)
   grants the Stripe product's `credits` metadata allotment. Thereafter every
   paid cycle invoice re-grants it.
3. **Admin-granted trials** (separate flow, no card): the admin's exact grant
   lands at trial start; auto-credit is suppressed while trialing; if the
   trial converts via the portal, credits resume normally.

## Source of truth (three legs agree)

- **Product configuration**: org subscription is $499/month
  (`STRIPE_ORG_PRICE_CENTS=49900`, validated against Stripe); the credit
  amount is the Stripe product's `credits` metadata — operator-configured in
  Stripe, deliberately never hardcoded in Helix. **Confirmed 2026-09-16 in
  the live Stripe dashboard: the product behind the $499/month
  `helix-org-subscription` price has no `credits` metadata**, so every
  subscription invoice — trial conversion included — grants $0 credits
  (webhook logs `product credits metadata missing, skipping subscription
  topup`). The $100 figure Chris expected is not encoded anywhere and was
  not assumed.
- **History**: until 2026-03-12 (`413b571c2`) the webhook credited the
  invoice's `AmountPaid` — the subscription fee literally became credits,
  which is what the signup benefit line "Your entire subscription fee
  becomes credits for running AI models" described. `91910128e` ("control
  topup") switched the grant to `product.metadata.credits` so operators
  control the per-cycle amount, but that metadata was never set on the live
  product, silently turning the promise off.
- **Stripe behavior**: card-backed checkout trial (`trial_period_days=3`,
  `payment_method_collection=always`) creates a `trialing` subscription plus
  a $0 invoice (auto-paid, no money moves); at trial end Stripe charges the
  first real invoice and flips the subscription to `active`.
  `design/2026-09-11-onboarding-card-backed-trial-research.md` line 38: skip
  the $0 invoice while trialing, "then apply the paid subscription credit
  when the first real charge succeeds".
- **Billing implementation**: `api/pkg/stripe/stripe_invoices.go`
  `handleInvoicePaymentPaidEvent`, pinned by tests in
  `api/pkg/stripe/stripe_invoices_test.go` (trial-create skips,
  admin-granted skips while trialing, converted-to-paid credits).

## Enforcement gap fixed

The trial-start skip relied on a live `subscription.Get`; a transient Stripe
API failure made the handler "proceed with default credit logic", which would
have granted the monthly allotment on the trial's $0 invoice. The invoice
event payload itself carries `amount_due`, so the handler now skips any
zero-amount invoice before any fetch: a $0 invoice is never the successful
bill that grants credits. Normal paid subscriptions (first invoice
`amount_due=49900`) are unaffected; the fetch-failure fall-through still
covers them. New test:
`Test_handleInvoicePaymentPaidEvent_ZeroValueInvoice_SkipsWithoutFetch`.

## Copy fix

`frontend/src/pages/Onboarding.tsx` now states the policy as it is enforced
today, without hardcoding the Stripe-metadata amount: the trial starts with
no Helix credits, and the benefit line promising that the subscription fee
becomes credits is replaced with the true statement (credits come from
top-ups). Both lines must be revisited the moment an operator sets `credits`
metadata on the live product — the webhook will start granting that amount
on the next paid invoice, and the copy should then promise it again.

## Open decision for the operator

Whether the org subscription should include monthly credits is a product
decision only the live Stripe config can answer today. If yes (e.g. $100/m
per Chris, or $499/m per the pre-2026-03 behavior), add `credits: <amount>`
to the product's metadata in the live dashboard — no code change needed. If
no, the current copy is already accurate.
