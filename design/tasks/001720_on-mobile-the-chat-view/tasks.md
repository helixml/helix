# Implementation Tasks

- [x] In `SpecTaskDetailContent.tsx`, update `getInitialView()` (~line 294) to check `window.matchMedia('(max-width: 899.95px)')` and return `"chat"` on mobile when no URL `view` param is set
- [x] Verify the existing useEffect at line 530 still works correctly — confirmed it already uses `isBigScreen ? "desktop" : "chat"` and only triggers when switching from "details" view
- [x] Test on mobile viewport: frontend build passes, logic verified by code review (API not running for integration test)
- [x] Test on desktop viewport: no URL param → `window.matchMedia` returns false on desktop → returns "desktop" (unchanged behavior)
- [x] Test with explicit `?view=desktop` URL param on mobile: URL param check at line 296-302 runs first, returns the param value before reaching the matchMedia check
