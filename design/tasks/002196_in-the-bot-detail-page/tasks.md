# Implementation Tasks: Fix Restart Agent Session to Fully Reset Desktop and Context

## Backend

- [ ] Add a `fresh bool` parameter to `restartSessionContainer` in `api/pkg/server/session_handlers.go`.
- [ ] When `fresh == true`: call `ClearSession(ctx, sessionID)` (wipes interactions, resets `ZedThreadID = ""`) before recreating the container.
- [ ] When `fresh == true`: make `StopDesktop` failure fatal (return an error) instead of swallowing it.
- [ ] Keep `fresh == false` behavior identical to today (crash recovery: preserve `ZedThreadID`, best-effort `StopDesktop`).
- [ ] Update `SessionRestarter.RestartSession` port signature (`api/pkg/org/interfaces/server/api/api.go`) to carry the fresh/preserve intent.
- [ ] Update `inProcHelixClient.RestartSession` (`api/pkg/server/helix_org_inproc.go`) to pass the intent through.
- [ ] Update `restartBotAgent` (`api/pkg/org/interfaces/server/api/bots.go`) to request a **fresh** restart.
- [ ] Update the in-chat `restartCrashedAgentThread` and spec-task callers to request **preserve** (unchanged behavior).
- [ ] Confirm the empty-session fallback still calls `Activate` for first-time start.

## Frontend

- [ ] In `handleRestartSession` (`frontend/src/pages/HelixOrgBotDetail.tsx`), after `restartAgent.mutateAsync` succeeds, refresh the chat window so it shows the new empty session/thread and fresh desktop: re-resolve via `fetchExistingWorkerSession` and re-set `chatSessionId` (remount `EmbeddedSessionView` or use `sessionViewRef` if the id is unchanged).
- [ ] Default the session panel to the Chat tab (`sessionTab='chat'`) after restart so the fresh transcript is visible.
- [ ] Update the success snackbar wording to "fresh session started"; keep error handling so a failed backend response surfaces an error snackbar (no false success). No confirmation dialog.

## Tests

- [ ] Extend `api/pkg/server/restart_session_container_test.go`: fresh path clears interactions, resets `ZedThreadID`, and errors on `StopDesktop` failure.
- [ ] Test that the preserve path (in-chat) still keeps `ZedThreadID` and remains best-effort on tear-down.
- [ ] Test `restartBotAgent` invokes the fresh path for a live session and falls back to `Activate` when there is none.

## Verification

- [ ] Manual: add a tool to a bot, click "Restart agent session", confirm the desktop restarts and the chat window immediately shows a new, empty session/thread with no prior context.
- [ ] Manual: confirm a tear-down failure shows an error (not "restart queued").
