# Implementation Tasks

## Frontend Changes

- [x] Add `visibleSteps` memo that filters `STEPS` array based on `serverConfig?.billing_enabled`
- [x] Create step type enum or constant identifiers (e.g., `STEP_ORG`, `STEP_SUBSCRIPTION`, `STEP_PROVIDER`) to replace raw indexes
- [x] Update `renderStepContent` switch to use step identifiers instead of numeric indexes
- [x] Update step iteration in JSX to use `visibleSteps` instead of `STEPS`
- [x] Update `activeStep` initial value logic to account for filtered steps
- [x] Update `markComplete` to work with filtered step indexes

## Testing (via Chrome MCP)

- [ ] With `STRIPE_BILLING_ENABLED=false` (default): Navigate to onboarding, verify 5 steps shown, no subscription step visible
- [ ] With `STRIPE_BILLING_ENABLED=true`: Navigate to onboarding, verify 6 steps shown with subscription step at position 2
- [ ] Complete full onboarding flow with billing disabled - verify all step transitions work
- [ ] Complete full onboarding flow with billing enabled - verify subscription step blocks until payment
- [ ] Refresh browser mid-onboarding, verify correct step state preserved in both modes