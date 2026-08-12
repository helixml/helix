# Implementation Tasks: Escape Cancels the Current Turn in the Spec Task Chat

- [x] Add an `Escape` branch to `handleKeyDown` in `frontend/src/components/common/RobustPromptInput.tsx`, after the `composerTrigger` block: when `isAgentBusy && onCancel && !isCancelling`, `preventDefault()` and call `onCancel()`; otherwise return and let the event bubble
- [x] Guard the window-level Escape handler in `frontend/src/pages/SpecTaskDetailPage.tsx` with `if (e.defaultPrevented) return` so a cancel does not also close the New Task panel (mirror `HelixOrgSideDrawer.tsx`)
- [x] Update the Stop button tooltip to `Stop generation (Esc)`, leaving the `aria-label` unchanged
- [x] Nudge the queue after a user-initiated cancel: in `cancelSessionTurn` (`api/pkg/server/session_handlers.go`), call `s.nudgeSessionQueue(sessionID)` when `cancelActiveTurn` returns a non-`noop` status
- [x] Add frontend tests to `RobustPromptInput.test.tsx`: Escape cancels when busy; no-op when idle; no-op while `isCancelling`; Escape closes the completion popup first when it is open
- [~] Add a backend test: cancelling a session with a pending queue-mode prompt dispatches the oldest prompt; a `noop` cancel dispatches nothing
- [ ] Run `cd frontend && yarn build` and the vitest suite; run the Go build/tests for the touched packages
- [ ] End-to-end in the inner Helix at `http://localhost:8080` with a live Zed spec task: Escape stops a running turn; Escape with two queued messages starts the first one; Escape when idle still closes the New Task panel; sending a new message after the cancel works
- [ ] Commit with a conventional-commit message and open the PR; check CI (Drone/`gh pr checks`) and fix any failures
