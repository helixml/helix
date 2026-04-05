# Requirements

## User Story

As a new user on a Helix deployment that already has AI providers configured (global/system providers), I should not see the "Connect an AI provider" onboarding step at all, so I can get started faster without being asked to do something that's already done.

## Context: Existing Auto-Complete Logic

There is already auto-complete logic in `Onboarding.tsx` (lines 400-412) that marks the provider step as "completed" and skips past it when providers with enabled chat models exist. However, this is **not sufficient** for this task because:

1. **It only fires after org creation** — `useListProviders` requires `orgId`, so providers don't load until the user completes the organization step. The provider step remains visible in the stepper sidebar the whole time.
2. **The step still appears in the stepper** — auto-complete marks it done and jumps past it, but the user still sees it as a completed circle in the step list. We want to remove it entirely.
3. **There's a timing flash** — if the user reaches the provider step before the provider API responds, they briefly see the step content before being auto-jumped forward.

The fix is to **not show the step at all** when global providers exist, rather than showing it and auto-completing it.

## Acceptance Criteria

1. When any global/system AI provider with enabled chat models exists on the deployment, the "Connect an AI provider" step is completely removed from the onboarding wizard (not shown in the stepper, not navigated to).
2. When no global providers exist, the provider step still appears as normal (users can connect their own or skip).
3. The provider step removal should happen immediately on page load — no flash of the step appearing then disappearing.
4. Existing behavior for the subscription step filter (`billing_enabled`) is not affected.
5. The existing auto-complete logic (lines 400-412) can remain as a fallback for edge cases, or be removed if no longer needed — implementer's discretion.

## Testing Requirements

This change needs careful browser testing in the inner Helix at `http://localhost:8080`. Test scenarios:

1. **Global providers exist** — register a new user, verify onboarding skips straight from org step to project step, provider step not visible in stepper.
2. **No global providers** — register a new user, verify provider step appears as before, can be completed or skipped.
3. **Step numbering** — verify all remaining steps work correctly (no off-by-one errors in step indices) when provider step is hidden.
4. **Full flow** — complete the entire onboarding in both scenarios to verify nothing breaks downstream.
