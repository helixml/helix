# Implementation Tasks

- [ ] Extract or reuse `handleEnsureDesktopRunning` logic: create a shared function/hook that checks `sandboxState` and calls either `v1SessionsResumeCreate` (existing session) or the `start-planning` API (no session), mirroring the existing handler in `SpecTaskDetailContent.tsx:687`
- [ ] In `DesignReviewContent.tsx`, add an `onStartDesktop` callback prop (or accept `sandboxState` + `sessionId` props) so the component can trigger desktop startup on comment submission
- [ ] In the comment submission handler inside `DesignReviewContent.tsx`, call `onStartDesktop()` in parallel with `createComment` when the desktop is not running
- [ ] In `SpecTaskDetailContent.tsx`, pass the `onStartDesktop` callback and current `sandboxState` down to `DesignReviewContent`
- [ ] In `SpecTaskReviewPage.tsx` (standalone review page), add the same desktop-start logic using the spec task's `planning_session_id` and session state
- [ ] Add user-facing feedback (toast or status message) when the desktop is being started as a result of comment submission (e.g. "Starting desktop so the agent can respond...")
- [ ] Handle desktop start errors gracefully: show a non-blocking error notification but do not prevent the comment from being submitted
- [ ] Verify that submitting a comment while the desktop is already running continues to work without any change in behavior
