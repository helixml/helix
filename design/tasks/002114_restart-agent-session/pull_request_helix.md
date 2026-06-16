# fix: make "Restart agent session" actually recreate the desktop container

## Summary

"Restart agent session" didn't restart the agent. The three restart surfaces
each did something different, and only one actually recreated the desktop
container / Zed process — so a stuck worker couldn't be recovered:

- **Worker detail page** button → `POST /orgs/{org}/workers/{id}/activate` →
  normal activation → `SendMessage` to the existing session. **Never recreated
  the container.**
- **In-chat** button → `POST /sessions/{id}/restart-agent` → recreated the
  container (correct).
- **Spec-task page** button → orchestrated stop + `setTimeout(1000)` + resume
  **in the frontend** (forbidden `setTimeout`, logic in the wrong layer).

This change makes "restart" mean one thing — recreate the container — across
every surface, with all the logic in the backend and covered by tests.

## Changes

### Backend
- Extract `restartSessionContainer(ctx, user, session)` — the single canonical
  restart primitive (StopDesktop → recreate via `resumeSessionInternal`,
  preserving `ZedThreadID` → reset crashed prompts → kick the queue).
  `restartCrashedAgentThread` (`/sessions/{id}/restart-agent`) is now a thin
  auth wrapper over it.
- Add `POST /api/v1/orgs/{org}/workers/{id}/restart-agent`: resolves the
  worker's current session and recreates its container via a new
  `SessionRestarter` port (implemented over the in-proc helix client, so it
  shares the exact same primitive as the in-chat button). Falls back to a fresh
  activation when the worker has no live session yet.
- Wire `SessionRestarter` into the helix-org API deps.

### Frontend
- Worker detail "Restart agent session" button now calls the new worker restart
  endpoint (`useRestartWorkerAgent`) instead of `activate`.
- Spec-task page restart now makes a single `v1SessionsRestartAgentCreate` call;
  removed the frontend stop/`setTimeout`/resume sequence.
- Regenerated the OpenAPI client + swagger.

### Tests (TDD)
- `restart_session_container_test.go`: asserts StopDesktop **then** StartDesktop
  (recreate, via `gomock.InOrder`), crashed-prompt reset + count, queue kick,
  and the 403 / 400 guards.
- `restart_worker_test.go`: worker restart with a live session hits the
  `SessionRestarter` port (not `DispatchManual`); with no session falls back to
  activate; 404 on unknown worker.

## Notes
- Normal activation of a healthy worker still `SendMessage`s (unchanged) —
  "restart" is a distinct, explicit operator action with its own endpoint.
- `ZedThreadID` is preserved (same session row reused), so conversation context
  is restored after restart.
