# Onboarding card-backed trial research

## Decision

Use Stripe Checkout in `subscription` mode for a **72-hour, card-required
trial that automatically converts to the existing paid subscription**.

Stripe supports both options:

| Duration | Stripe configuration | Assessment |
| --- | --- | --- |
| 48 hours | `subscription_data.trial_period_days=2` | Supported, but it sits on Checkout's minimum boundary for an exact `trial_end` timestamp and gives users little time to evaluate Helix. |
| 72 hours | `subscription_data.trial_period_days=3` | Recommended. It is still a short trial, avoids the 48-hour boundary, and gives users 50% more evaluation time. |

This is a product recommendation, not a Stripe limitation or a claim that 72
hours will convert better. Measure checkout completion, trial activation,
first meaningful use, cancellation, first-payment success, and refund/dispute
rates. Revisit the duration after there is enough traffic to compare cohorts.

## Stripe support and conversion behavior

Checkout accepts either an integer `trial_period_days` (minimum one day) or an
exact `trial_end` Unix timestamp. For Checkout, an exact `trial_end` must be at
least 48 hours in the future. Using `trial_period_days=3` is the simplest way
to express the recommended duration and avoids clock-skew/boundary failures.

Checkout Sessions in subscription mode collect a payment method by default;
`payment_method_collection=always` makes the card-backed intent explicit. At
checkout completion Stripe creates a subscription in `trialing` state and a
zero-value invoice. At `trial_end`, Stripe creates the first paid invoice and
attempts collection automatically. The subscription leaves `trialing`; its
subsequent invoice and subscription states reflect whether payment succeeds
or follows the account's configured retry and final-action settings.

Automatic conversion therefore requires no Helix timer or scheduled job.
Stripe remains the source of truth and Helix continues syncing subscription
state from `customer.subscription.created`, `.updated`, and `.deleted`
webhooks. The existing `invoice.paid` handler continues applying the paid
subscription credit when the first charge succeeds.

## Smallest implementation in Helix

The existing onboarding path already calls `GetCheckoutSessionURL` and the
Checkout Session already uses subscription mode. The implementation should be
limited to that shared session builder:

```go
checkoutParams := &stripe.CheckoutSessionParams{
	// Existing fields omitted.
	PaymentMethodCollection: stripe.String("always"),
	SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
		TrialPeriodDays: stripe.Int64(3),
		Metadata: map[string]string{
			"user_id": params.UserID,
			"org_id":  params.OrgID,
		},
	},
}
```

Before implementation, scope the three-day trial to onboarding requests so
the existing account and organization billing entry points do not silently
become trial offers. A single boolean or trial-days field on the existing
`SubscriptionSessionParams` is sufficient; no new Stripe flow or abstraction
is needed.

Update onboarding copy to disclose the price, 72-hour duration, automatic
renewal, and how to cancel before the first charge. Keep the post-checkout
success handling unchanged: webhook state, rather than the redirect query,
confirms access.

## Operational requirements

1. Enable Stripe's trial-ending emails and set the cancellation-policy URL in
   **Dashboard → Billing → Subscriptions and emails → Manage free trial
   messaging**. Stripe sends the reminder immediately for trials shorter than
   seven days.
2. Confirm the first post-trial statement descriptor remains recognizable;
   Stripe can append trial-ending text, subject to the 22-character descriptor
   limit.
3. Test with a Stripe Test Clock: successful first charge, card decline and
   retry/final action, cancellation during trial, webhook delay/replay, and an
   abandoned Checkout Session.
4. Verify applicable consumer-law and card-network disclosures with counsel in
   every market where the trial is offered. Stripe's tooling helps with card
   network messaging but does not transfer compliance responsibility.

## Sources

- [Stripe Checkout Session API](https://docs.stripe.com/api/checkout/sessions/create)
  — trial parameters, the 48-hour `trial_end` minimum, and payment-method
  collection behavior.
- [Configure free trials with Checkout](https://docs.stripe.com/payments/checkout/free-trials)
  — card collection defaults and trial setup.
- [Use trial periods on subscriptions](https://docs.stripe.com/billing/subscriptions/trials)
  — automatic conversion, trial webhooks, reminders, and card-network
  compliance requirements.
- [Test a Billing integration](https://docs.stripe.com/billing/testing)
  — Test Clocks and trial-to-paid testing.
