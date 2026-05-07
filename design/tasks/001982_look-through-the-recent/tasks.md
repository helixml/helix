# Implementation Tasks

- [ ] Reproduce the regression on Helix-in-Helix: pause a spec-task desktop, send a chat message, observe how long until the **Starting Desktop...** spinner appears (capture timing in design notes for before/after comparison) — deferred until inner-Helix stack finishes booting
- [x] Add an optional `onWillSend` callback prop to `RobustPromptInput` and invoke it inside `handleSend` immediately after `saveToHistory`, before `syncEntryImmediately`
- [x] Extract shared helper `optimisticallyMarkSessionStarting` in `frontend/src/utils/optimisticSessionStarting.ts` (writes both `'full'` / `'skip'` query slots, no-ops when status is already `"running"` / `"starting"`, also fires a prefix `invalidateQueries` to nudge the next poll)
- [x] In `SpecTaskDetailContent.tsx`, define a `handleWillSend` callback that calls the helper, and pass `onWillSend={handleWillSend}` to both `RobustPromptInput` mounts (around lines 1938 and 2742)
- [x] In `ExternalAgentDesktopViewer.tsx`, define `handleWillSend` and pass `onWillSend={handleWillSend}` on the prompt input
- [~] In `frontend/src/contexts/streaming.tsx`, fix the `session_update` handler so `getQueryData` and `setQueryData` use the correct keys (`["session", id, "full"]` and `["session", id, "skip"]`) instead of the bare `["session", id]` key — write to both variants, prefer `'full'` when reading
- [ ] Verify the optimistic flip is reverted to the authoritative value within one polling cycle (≤3 s) by polling response — add a brief comment in code explaining the lifecycle so future readers don't strip it
- [ ] Manual end-to-end: pause desktop → send chat → spinner ≤ 500 ms → backend boot completes → stream live
- [ ] Manual end-to-end: live desktop → send chat → no flicker, no false spinner
- [ ] `cd frontend && yarn build` clean
- [ ] Open a Helix PR; include before/after screen recordings in the PR description
