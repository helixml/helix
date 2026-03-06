# Implementation Tasks

## Frontend Changes

- [ ] Add `visibleSteps` memo that filters `STEPS` array based on `serverConfig?.billing_enabled`
- [ ] Create step type enum or constant identifiers (e.g., `STEP_ORG`, `STEP_SUBSCRIPTION`, `STEP_PROVIDER`) to replace raw indexes
- [ ] Update `renderStepContent` switch to use step identifiers instead of numeric indexes
- [ ] Update step iteration in JSX to use `visibleSteps` instead of `STEPS`
- [ ] Update `activeStep` initial value logic to account for filtered steps
- [ ] Update `markComplete` to work with filtered step indexes

## Testing

- [ ] Test onboarding with `STRIPE_BILLING_ENABLED=false` (default) - should show 5 steps, no subscription
- [ ] Test onboarding with `STRIPE_BILLING_ENABLED=true` - should show 6 steps with subscription required
- [ ] Test step completion flow works correctly in both modes
- [ ] Test browser refresh during onboarding preserves correct step state