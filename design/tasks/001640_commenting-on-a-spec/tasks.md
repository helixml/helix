# Implementation Tasks

- [x] Extract `useSandboxState` from `ExternalAgentDesktopViewer.tsx` into a shared hook (or `sessionService.ts`) so it can be used in multiple components without prop-drilling
- [x] Create a shared `useEnsureDesktopRunning(sessionId)` hook that checks sandbox state and calls `v1SessionsResumeCreate` when the desktop is absent; reuse the existing resume logic from `SpecTaskDetailContent.tsx:~687`
- [x] In `DesignReviewContent.tsx`, call `ensureDesktopRunning()` in parallel with `createComment` on comment submission when the desktop is not running
- [x] In `SpecTaskDetailContent.tsx`, wire the session id and sandbox state so the above hook has what it needs
- [x] In `SpecTaskReviewPage.tsx` (standalone review page), apply the same pattern using the spec task's `planning_session_id`
- [x] In the session chat input component, call `ensureDesktopRunning()` in parallel with message submission when the desktop is not running
- [x] Add user-facing feedback (toast or status indicator) when desktop auto-start is triggered (e.g. "Starting desktop so the agent can respond...")
- [x] Handle desktop start errors gracefully: show a non-blocking error notification but do not prevent the message from being submitted
- [x] Verify that sending messages while the desktop is already running continues to work without any change in behavior
