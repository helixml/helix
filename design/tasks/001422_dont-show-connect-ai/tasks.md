# Implementation Tasks

- [ ] Import `useGetDashboardData` from dashboard service in Onboarding.tsx
- [ ] Add `hasConnectedRunners` derived from `dashboardData?.runners?.length`
- [ ] Replace `hasAnyEnabledModels` with `hasUsableProviders` memo that excludes Helix models when no runners are connected
- [ ] Add `visibleSteps` memo that filters out step 2 when `hasUsableProviders` is true
- [ ] Add `getOriginalStepIndex` helper function to map visible index → original step index
- [ ] Update the step rendering loop to iterate `visibleSteps` instead of `STEPS`
- [ ] Update `renderStepContent`, `isStepCompleted`, `isStepActive`, `isStepLocked` calls to use `getOriginalStepIndex`
- [ ] Update `markComplete` calls to use original step index
- [ ] Remove the `useEffect` that auto-completes step 2 (no longer needed when step is hidden)
- [ ] Update completion check to compare against `visibleSteps.length` instead of `STEPS.length`
- [ ] Add test: mock Helix models only + no runners → verify step 2 is shown
- [ ] Add test: mock Helix models only + runners connected → verify step 2 is hidden
- [ ] Add test: mock external providers (OpenAI/Anthropic) → verify step 2 is hidden
- [ ] Add test: mock empty providers → verify all 5 steps shown
- [ ] Manual test: deploy and verify onboarding flow works in all scenarios