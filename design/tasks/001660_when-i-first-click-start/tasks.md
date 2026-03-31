# Implementation Tasks

- [ ] In `SpecTaskDetailContent.tsx`, identify the task status values that represent "queued for planning but no session yet" (`queued_spec_generation`, `spec_generation`)
- [ ] Derive `effectiveIsStarting`: true if `isDesktopStarting` OR (task is queued/planning AND `activeSessionId` is empty)
- [ ] Replace all uses of `isDesktopStarting` with `effectiveIsStarting` in `SpecTaskDetailContent.tsx` where it affects which desktop UI is rendered
- [ ] Verify no stopped/absent desktop flash occurs immediately after clicking "Start Planning"
- [ ] Verify the normal "Starting Desktop" → "Running" → "Stopped" flow still works correctly for subsequent state transitions
