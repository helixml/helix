# Implementation Tasks

- [x] In `SpecTaskDetailContent.tsx`, identify the task status values that represent "queued for planning but no session yet" (`queued_spec_generation`, `spec_generation`)
- [x] Derive `effectiveIsDesktopPaused`: false when `isQueuedForPlanning` (suppresses paused state during queued phase)
- [x] Replace `isDesktopPaused` with `effectiveIsDesktopPaused` in the two toolbar "Start desktop" buttons (desktop + mobile)
- [x] Pass `initialSandboxState="starting"` to both `ExternalAgentDesktopViewer` instances when `isQueuedForPlanning`, so the viewer itself shows "Starting Desktop..." instead of "Desktop Paused"
- [x] Build frontend to verify no TypeScript errors (`yarn build` passes)
- [ ] Verify no stopped/absent desktop flash occurs immediately after clicking "Start Planning"
- [ ] Verify the normal "Starting Desktop" → "Running" → "Stopped" flow still works correctly for subsequent state transitions
