# Design: Hide "Connect AI Provider" Step When Provider Exists

## Overview

Modify the Onboarding component to dynamically filter out the "Connect an AI provider" step when the system already has providers with enabled chat models.

## Current Architecture

The onboarding flow uses a static `STEPS` array:

```typescript
const STEPS: StepConfig[] = [
  { /* 0: Sign in */ },
  { /* 1: Set up organization */ },
  { /* 2: Connect an AI provider */ },  // <-- Hide this when providers exist
  { /* 3: Create your first project */ },
  { /* 4: Create your first task */ },
];
```

Step indices are used throughout the component for:
- `activeStep` state (which step is currently active)
- `completedSteps` Set (which steps are done)
- `renderStepContent(index)` switch statement
- `markComplete(stepIndex)` function

## Design Decision

**Approach: Dynamic steps array with index mapping**

Create a `visibleSteps` array filtered based on `hasAnyEnabledModels`. Use a mapping to translate between "original step index" (used in content rendering) and "visible step index" (used in UI).

This approach:
- Minimizes changes to existing step content logic
- Keeps the `renderStepContent` switch cases unchanged
- Only requires changes to step iteration and index tracking

**Note on Helix models & runners**: The backend already handles this. In `provider_manager.go`, the Helix provider is excluded from the list when no runners are connected. The frontend does not need dashboard data or runner checks - if Helix models appear in the provider list, runners are available.

## Key Changes

### 1. Create filtered steps array

```typescript
const visibleSteps = useMemo(() => {
  if (hasAnyEnabledModels) {
    // Skip step 2 (provider step)
    return STEPS.filter((_, index) => index !== 2);
  }
  return STEPS;
}, [hasAnyEnabledModels]);
```

### 2. Map visible index to original index

```typescript
const getOriginalStepIndex = (visibleIndex: number): number => {
  if (!hasAnyEnabledModels) return visibleIndex;
  // If providers exist, step 2+ needs +1 offset (skipping original index 2)
  return visibleIndex >= 2 ? visibleIndex + 1 : visibleIndex;
};
```

### 3. Update step rendering loop

Change from iterating `STEPS` to iterating `visibleSteps`, using `getOriginalStepIndex` when calling `renderStepContent`, `markComplete`, etc.

### 4. Update initial completed steps

When `hasAnyEnabledModels` is true on load:
- Don't add step 2 to completedSteps (it's hidden, not completed)
- Don't try to skip from step 2 to step 3

### 5. Update completion check

Change completion check from `completedSteps.size === STEPS.length` to check against required visible steps.

## Files Changed

| File | Change |
|------|--------|
| `frontend/src/pages/Onboarding.tsx` | Filter steps, add index mapping, update completion logic |

## Testing

1. Mock `useListProviders` to return providers with enabled models → verify step 2 is hidden
2. Mock `useListProviders` to return empty → verify all 5 steps shown
3. Verify step completion works correctly in both scenarios
4. Verify "Go to your workspace" button appears after completing all visible steps