# Fix(frontend): offer 72-hour trial CTA only to trial-eligible users

## Summary
The onboarding subscription step always showed "Start 72-hour free trial" — even for users who are already onboarded, for whom the backend grants no trial (`onboardingTrialPeriodDays` in `api/pkg/server/wallet_handlers.go` returns 3 trial days only when `onboarding_completed=false` and the wallet has no subscription). Those users reached Stripe Checkout with no trial and were charged $499 immediately, while the UI promised a 72-hour free trial.

The frontend now mirrors the backend eligibility exactly: `trialEligible = !account.user.onboarding_completed && !wallet.stripe_subscription_id`.

- Trial-eligible (genuinely new) users keep the existing trial CTA: step title "Start your free trial", 72-hour copy, button "Start 72-hour free trial" — unchanged.
- Already-onboarded (ineligible) users now see the real thing: step title "Subscribe to Helix Business", copy "Subscribe to Helix Business for $499/month, charged today.", button "Subscribe". The confirming-state labels ("Confirming your subscription...") are also corrected so no trial wording leaks into ineligible flows.

No backend or Stripe changes needed — the backend was already correct; this is a display-only fix.

## Testing
- `yarn test src/pages/Onboarding.test.tsx` (vitest): 25/25 pass, including two new tests — an `onboarding_completed: true` user sees the "Subscribe" button and no trial wording (no "free trial" button, no "card will not be charged for 72 hours" copy), and an `onboarding_completed: false` user still gets the trial CTA.
- `yarn tsc`: clean. `yarn build` (vite): succeeds.
- NOT tested against a live Stripe checkout: the inner dev stack was down and has no Stripe keys, so the subscription step cannot render there. Backend checkout behavior is unchanged by this diff, so the $0-today trial checkout for new users and immediate $499 checkout for existing users are as before.
