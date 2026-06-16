# Requirements: Reliable Backend Restart of Agent Session Desktop Container

## Problem

"Restart agent session" does not actually restart the agent. A worker whose
desktop container / Zed process is stuck cannot be recovered by any of the
restart buttons in the UI, because each button does something different and
only one of them actually tears down and recreates the container.

Three restart surfaces exist today, with three divergent behaviours:

1. **Worker detail page** — "Restart agent session" button calls
   `POST /api/v1/orgs/{org}/workers/{id}/activate`. This runs a normal
   activation, which finds the worker's persisted session and calls
   `SendMessage` (`POST /sessions/{id}/messages`). The desktop container /
   Zed process is **never recreated** — a stuck worker stays stuck.
2. **In-chat prompt input** — restart button calls
   `POST /api/v1/sessions/{id}/restart-agent`. This *does* tear down and
   recreate the container (`StopDesktop` → resume → reset crashed prompts).
3. **Spec-task detail page** — restart button orchestrates the restart **in
   the frontend**: `stop-external-agent` → `setTimeout(1000ms)` → `resume`.
   This puts restart logic in the wrong layer and relies on a forbidden
   `setTimeout` race.

The fix: one canonical backend restart operation that reliably recreates the
desktop container for a session, used by every restart surface. No restart
orchestration in the frontend. Cover it with tests so it cannot regress.

## User Stories

### US-1: Recover a stuck worker from the worker page
As an operator, when I click "Restart agent session" on a worker detail page,
the worker's desktop container / Zed process is destroyed and recreated, so a
stuck worker is recovered.

### US-2: Recover a stuck session from the in-chat button
As a user, when I click restart in the chat prompt input, the session's
container is destroyed and recreated (current behaviour preserved), with
conversation context and queued/crashed prompts handled correctly.

### US-3: Recover a stuck session from the spec-task page
As a user, when I click restart on the spec-task detail page, the container is
destroyed and recreated by the **backend** in a single call — no
frontend-driven stop/sleep/resume sequence.

### US-4: Consistent, tested behaviour
As a maintainer, all restart surfaces converge on one backend code path that
is covered by tests, so "restart" reliably means "recreate the container"
everywhere and cannot silently diverge again.

## Acceptance Criteria

- [ ] A single backend restart operation tears down the existing desktop
      container (`StopDesktop`, best-effort) and recreates it (via the resume
      path / `StartDesktop`), preserving `ZedThreadID` so conversation context
      is restored.
- [ ] Crashed prompts for the session are reset and re-dispatched after the
      container is recreated (existing `restart-agent` behaviour, retained).
- [ ] The worker detail page "Restart agent session" button results in the
      container being recreated — **not** a `SendMessage` to the existing
      session.
- [ ] The spec-task detail page restart button calls one backend endpoint; the
      frontend no longer performs stop → `setTimeout` → resume.
- [ ] The in-chat restart button continues to recreate the container via the
      same shared backend path.
- [ ] If a worker has no live session yet, "restart" falls back to starting a
      fresh session (does not error).
- [ ] Restart is authorized: the caller must have update access to the session
      / worker; otherwise 403.
- [ ] Tests (gomock + suite pattern) assert the shared restart path calls
      `StopDesktop` then `StartDesktop` (recreate), preserves the thread id,
      resets crashed prompts, and that every restart entrypoint funnels into
      that path.

## Out of Scope

- Changing how a healthy/normal activation works (a normal activation of a
  running worker should still `SendMessage`, not recreate the container).
- Changes to streaming, ACP protocol, or Zed-internal thread handling.
- New UI design — only re-wiring existing buttons to the unified endpoint(s).
