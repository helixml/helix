# Add a 72-hour card-backed onboarding trial

## Summary

Add a Stripe Checkout trial to the onboarding subscription flow. New onboarding subscriptions collect a payment method, remain free for 72 hours, and automatically continue at $499 per month after the trial unless canceled.

Keep existing non-onboarding subscription checkout behavior unchanged, update onboarding copy to clearly disclose the trial and automatic conversion, and document the Stripe integration decision and operational requirements.

Validate that the Stripe organization lookup key resolves to an active USD $499 monthly Price before creating Checkout or admin trial subscriptions, preventing stale Stripe configuration from silently presenting a different renewal price.

Grant onboarding trial users $1 in AI credits once per user. Use the Stripe subscription ID to make webhook retries idempotent, revoke any unused trial grant when the trial is canceled, and preserve separately purchased credits.

## Testing

- `go test ./api/pkg/server -run 'TestOnboardingTrialPeriodDays$' -count=1` — passed
- `go test ./api/pkg/stripe -count=1` — passed
- `go test ./api/pkg/store -run '^$' -count=1` — compiled successfully
- `go test ./api/pkg/server -run 'TestOnboardingTrialPeriodDays|TestAdmin.*Trial' -count=1` — passed
- `yarn --cwd frontend test Onboarding.test.tsx` — 21 tests passed
- `yarn --cwd frontend tsc` — passed
- `git diff --check` — passed
