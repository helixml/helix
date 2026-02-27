# Implementation Tasks

- [ ] In `SpecTaskDetailContent.tsx`, remove the `isTaskCompleted` branch from desktop view conditional (big screen, ~L1886) - just render `ExternalAgentDesktopViewer` like non-completed tasks
- [ ] In `SpecTaskDetailContent.tsx`, remove the `isTaskCompleted` branch from desktop view conditional (mobile, ~L2352) - same change
- [ ] Test: open a finished task with running container → should see desktop
- [ ] Test: open a finished task with stopped container → should see paused state with play button to restart
- [ ] Test: click play on stopped finished task → container starts, desktop appears
- [ ] Test: archived tasks still show archived message regardless of container state