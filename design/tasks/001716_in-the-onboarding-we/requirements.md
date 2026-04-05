# Requirements

## User Story

As a new user on a Helix deployment that already has AI providers configured (global/system providers), I should not see the "Connect an AI provider" onboarding step at all, so I can get started faster without being asked to do something that's already done.

## Acceptance Criteria

1. When any global/system AI provider with enabled chat models exists on the deployment, the "Connect an AI provider" step is completely removed from the onboarding wizard (not shown in the stepper, not navigated to).
2. When no global providers exist, the provider step still appears as normal (users can connect their own or skip).
3. The provider step removal should happen immediately on page load — no flash of the step appearing then disappearing.
4. Existing behavior for the subscription step filter (`billing_enabled`) is not affected.
5. The auto-complete logic for the provider step (lines 400-412 in `Onboarding.tsx`) should still work as a fallback for cases where org-level providers exist but no global providers exist.
