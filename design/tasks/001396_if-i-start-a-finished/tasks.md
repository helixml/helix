# Implementation Tasks

- [x] In `SpecTaskDetailContent.tsx`, update desktop view conditional (big screen, ~L1886): show `ExternalAgentDesktopViewer` when `isDesktopRunning || isDesktopStarting`, otherwise show "Task finished" alert with play button
- [x] In `SpecTaskDetailContent.tsx`, update desktop view conditional (mobile, ~L2352) with same logic
- [x] Add play button to the "Task finished" alert that calls the existing start desktop handler
- [x] Test: open a finished task with running container → should see desktop
- [x] Test: open a finished task with stopped container → should see "Task finished" message with play button
- [x] Test: click play button on finished task → container starts, desktop viewer appears
- [x] Test: archived tasks still show archived message without play button