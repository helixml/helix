# Design: Onboarding Subscription Step for Default Deploys

## Overview

Make the onboarding subscription step conditional on `billing_enabled` server config. When billing is disabled (default), skip the subscription step entirely.

## Current Behavior

In `frontend/src/pages/Onboarding.tsx`:
- `STEPS` array is static with 6 steps (indexes 0-5)
- Step 2 is always "Activate subscription"
- Users cannot proceed past step 2 without an active subscription
- `serverConfig?.billing_enabled` is already fetched but only used for wallet queries, not step visibility

## Solution

### Approach: Dynamic Steps Array

Filter the `STEPS` array based on `billing_enabled` config. This is simpler than trying to skip steps or reorder them.

```typescript
// Derive visible steps from config
const visibleSteps = useMemo(() => {
  if (!serverConfig?.billing_enabled) {
    // Remove subscription step (index 2) when billing disabled
    return STEPS.filter((_, index) => index !== 2);
  }
  return STEPS;
}, [serverConfig?.billing_enabled]);
```

### Key Changes

1. **`STEPS` filtering**: Use `useMemo` to create `visibleSteps` that excludes the subscription step when billing is disabled

2. **Step index mapping**: Update all step references to use the filtered array:
   - `activeStep` state still works (it's a number into `visibleSteps`)
   - `completedSteps` set still works (same indexing)
   - `renderStepContent(step)` needs to map filtered index → original step type

3. **Content rendering**: Change `renderStepContent` to use step identifier (e.g., step title or enum) rather than raw index, since indexes shift when steps are filtered

### Alternative Considered: Skip-and-Continue

Could auto-mark step 2 complete when billing disabled. Rejected because:
- Leaves confusing "Activate subscription ✓" in UI
- More complex state management
- Step numbering would still show gaps

## File Changes

| File | Change |
|------|--------|
| `frontend/src/pages/Onboarding.tsx` | Filter STEPS based on `billing_enabled`, adjust step rendering |

## Testing

1. **Billing disabled (default)**: Onboarding should show 5 steps, no subscription step
2. **Billing enabled**: Onboarding should show 6 steps with subscription required
3. **Step completion**: All step transitions work correctly in both modes

## Risks

- **Low**: Change is isolated to frontend, doesn't affect billing logic
- **Low**: Existing `billing_enabled` config is already well-tested in other components