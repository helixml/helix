# Implementation Tasks

- [ ] In `SpecTaskDetailContent.tsx`, update desktop view conditional (big screen, ~L1886) to show `ExternalAgentDesktopViewer` when `isDesktopRunning || isDesktopStarting`, even if task is completed
- [ ] In `SpecTaskDetailContent.tsx`, update desktop view conditional (mobile, ~L2352) with same logic
- [ ] Test: open a finished task with running container → should see desktop
- [ ] Test: open a finished task with stopped container → should see "Task finished" message
- [ ] Test: archived tasks still show archived message regardless of container state