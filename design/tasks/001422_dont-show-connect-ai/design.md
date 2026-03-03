# Design: Hide "Connect AI Provider" Step When Provider Exists

## Overview

Modify the Onboarding component to dynamically filter out the "Connect an AI provider" step when the system already has usable providers with enabled chat models.

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

Create a `visibleSteps` array filtered based on whether usable providers exist. Use a mapping to translate between "original step index" (used in content rendering) and "visible step index" (used in UI).

This approach:
- Minimizes changes to existing step content logic
- Keeps the `renderStepContent` switch cases unchanged
- Only requires changes to step iteration and index tracking

## Key Changes

### 1. Determine if usable providers exist

The current `hasAnyEnabledModels` logic needs refinement to exclude Helix models when no runners are connected:

```typescript
const { data: dashboardData } = useGetDashboardData();
const hasConnectedRunners = (dashboardData?.runners?.length ?? 0) > 0;

const hasUsableProviders = useMemo(() => {
  if (!providers) return false;
  return providers.some((p) =>
    (p.available_models || []).some((m) => {
      if (!m.enabled || m.type !== "chat") return false;
      // Helix models require a connected runner to be usable
      if (m.owned_by === "helix") return hasConnectedRunners;
      return true;
    })
  );
}, [providers, hasConnectedRunners]);
```

### 2. Create filtered steps array

```typescript
const visibleSteps = useMemo(() => {
  if (hasUsableProviders) {
    // Skip step 2 (provider step)
    return STEPS.filter((_, index) => index !== 2);
  }
  return STEPS;
}, [hasUsableProviders]);
```

### 3. Map visible index to original index

```typescript
const getOriginalStepIndex = (visibleIndex: number): number => {
  if (!hasUsableProviders) return visibleIndex;
  // If providers exist, step 2+ needs +1 offset (skipping original index 2)
  return visibleIndex >= 2 ? visibleIndex + 1 : visibleIndex;
};
```

### 4. Update step rendering loop

Change from iterating `STEPS` to iterating `visibleSteps`, using `getOriginalStepIndex` when calling `renderStepContent`, `markComplete`, etc.

### 5. Update initial completed steps

When `hasUsableProviders` is true on load:
- Don't add step 2 to completedSteps (it's hidden, not completed)
- Don't try to skip from step 2 to step 3

### 6. Update completion check

Change completion check from `completedSteps.size === STEPS.length` to check against required visible steps.

## Files Changed

| File | Change |
|------|--------|
| `frontend/src/pages/Onboarding.tsx` | Add dashboard data hook, refine provider check, filter steps, add index mapping, update completion logic |

## Testing

1. Mock providers with Helix models only, no runners → verify step 2 is shown
2. Mock providers with Helix models only, with runners → verify step 2 is hidden
3. Mock providers with external providers (OpenAI/Anthropic) → verify step 2 is hidden
4. Mock empty providers → verify all 5 steps shown
5. Verify step completion works correctly in all scenarios
6. Verify "Go to your workspace" button appears after completing all visible steps