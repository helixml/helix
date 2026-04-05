# Implementation Tasks

- [x] In `SpecTaskDetailContent.tsx`, update `getInitialView()` (~line 294) to check `window.matchMedia('(max-width: 899.95px)')` and return `"chat"` on mobile when no URL `view` param is set
- [x] Verify the existing useEffect at line 530 still works correctly — confirmed it already uses `isBigScreen ? "desktop" : "chat"` and only triggers when switching from "details" view
- [~] Test on mobile viewport: opening a task with an active session should show chat tab by default
- [ ] Test on desktop viewport: opening a task should still show desktop view by default
- [ ] Test with explicit `?view=desktop` URL param on mobile: should respect the param and show desktop
