# Implementation Tasks

- [ ] Reproduce the regression on Helix-in-Helix: pause a spec-task desktop, send a chat message, observe how long until the **Starting Desktop...** spinner appears (capture timing in design notes for before/after comparison) — deferred until inner-Helix stack finishes booting
- [x] Add an optional `onWillSend` callback prop to `RobustPromptInput` and invoke it inside `handleSend` immediately after `saveToHistory`, before `syncEntryImmediately`
- [x] Extract shared helper `optimisticallyMarkSessionStarting` in `frontend/src/utils/optimisticSessionStarting.ts` (writes both `'full'` / `'skip'` query slots, no-ops when status is already `"running"` / `"starting"`, also fires a prefix `invalidateQueries` to nudge the next poll)
- [x] In `SpecTaskDetailContent.tsx`, define a `handleWillSend` callback that calls the helper, and pass `onWillSend={handleWillSend}` to both `RobustPromptInput` mounts (around lines 1938 and 2742)
- [x] In `ExternalAgentDesktopViewer.tsx`, define `handleWillSend` and pass `onWillSend={handleWillSend}` on the prompt input
- [x] In `frontend/src/contexts/streaming.tsx`, fix the `session_update` handler so `getQueryData` and `setQueryData` use the correct keys (`["session", id, "full"]` and `["session", id, "skip"]`) instead of the bare `["session", id]` key — write to both variants, prefer `'full'` when reading
- [x] Helper has lifecycle comment explaining no-op when status already `running`/`starting` and that polling will reconcile within ~3 s
- [ ] Manual end-to-end: pause desktop → send chat → spinner ≤ 500 ms → backend boot completes → stream live — **BLOCKED**: inner-Helix stack failed to build (`/zed-build/app-icon.png: not found` in startup log), so no running stack to test against. Reviewer must verify on a working Helix instance before merge
- [ ] Manual end-to-end: live desktop → send chat → no flicker, no false spinner — **BLOCKED**: same as above
- [x] `cd frontend && yarn build` clean (verified after re-applying changes — build succeeded in 1m 7s with no errors)
- [x] Vitest unit tests for the helper — 10 cases covering paused→starting, no-op when running/starting, empty-slot guard, status_message default vs preserve, field preservation, and invalidate-queries call. All pass.
- [x] Push feature branch (Helix UI auto-creates the PR on click of "Open PR")
