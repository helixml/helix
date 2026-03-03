# Implementation Tasks

- [ ] Add `visibleSteps` memo that filters out step 2 when `hasAnyEnabledModels` is true
- [ ] Add `getOriginalStepIndex` helper function to map visible index → original step index
- [ ] Update the step rendering loop to iterate `visibleSteps` instead of `STEPS`
- [ ] Update `renderStepContent`, `isStepCompleted`, `isStepActive`, `isStepLocked` calls to use `getOriginalStepIndex`
- [ ] Update `markComplete` calls to use original step index
- [ ] Remove the `useEffect` that auto-completes step 2 (no longer needed when step is hidden)
- [ ] Update completion check to compare against `visibleSteps.length` instead of `STEPS.length`
- [ ] Add/update tests: mock providers with enabled models → verify step 2 is hidden
- [ ] Add/update tests: mock empty providers → verify all 5 steps shown
- [ ] Manual test: deploy and verify onboarding flow works in both scenarios