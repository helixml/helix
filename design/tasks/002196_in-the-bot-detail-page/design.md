# Design: Fix Restart Agent Session to Fully Reset Desktop and Context

## Current flow (broken)

```
HelixOrgBotDetail.tsx  "Restart agent session"
  → useRestartBotAgent  (helixOrgService.ts)
  → POST /api/v1/orgs/{org}/bots/{id}/restart-agent
  → restartBotAgent          (api/pkg/org/interfaces/server/api/bots.go:466)
      resolves sessionID via BotRuntime.State
  → SessionRestarter.RestartSession(sessionID)
  → inProcHelixClient.RestartSession   (api/pkg/server/helix_org_inproc.go:543)
  → POST /sessions/{id}/restart-agent → restartCrashedAgentThread
  → restartSessionContainer  (api/pkg/server/session_handlers.go:2451)
        StopDesktop (best-effort, errors swallowed)
        resumeSessionInternal  ← PRESERVES ZedThreadID + workspace volume
        ResetCrashedPromptsForSession
        kick queue
```

`restartSessionContainer` is intentionally context-preserving (crash recovery).
The bot-page button reuses it, so it never wipes the thread or forces a truly
fresh desktop — that is the defect.

## The correct primitive already exists

`ClearSession` (`api/pkg/server/session_clear.go`) is the "fresh context"
operation:

- `ClearSessionInteractions` — removes all interactions from the DB (source of
  truth for both runtimes).
- For Zed/ACP sessions (`AgentType == "zed_external"`) the `zedACPBackend.Clear`
  sets `session.Metadata.ZedThreadID = ""`, so the next message opens a clean
  thread.

The spawner already combines "clear context, then restart" on every
re-activation (`spawner.go:~486`), gated by the per-bot `PreserveContext` flag.
The bot-page restart should reuse the same building blocks.

## Proposed change

Introduce a **fresh-start** path for the bot-page "Restart agent session"
button, distinct from crash recovery. Order of operations:

1. **Clear context first** — call the existing `ClearSession` primitive on the
   bot's session: wipes interactions and resets `ZedThreadID = ""`.
2. **Recreate the desktop** — run the container tear-down + recreate
   (`StopDesktop` → `resumeSessionInternal`). Because `ZedThreadID` is now empty,
   the new container comes up on a brand-new thread — i.e. genuinely fresh.
3. **Surface failures** — for this user-initiated action, `StopDesktop` failure
   (and any step failure) must propagate to an error response, not be swallowed.

### Where to put the "fresh" behavior

**Decision (recommended): add a `fresh bool` parameter to the restart primitive**
rather than duplicating it.

- Extend `restartSessionContainer` (and the `SessionRestarter.RestartSession`
  port) so callers choose `fresh` vs `preserve`.
- When `fresh == true`: call `ClearSession` before the container recreate, and
  make tear-down failures fatal.
- When `fresh == false` (existing in-chat / spec-task callers): unchanged
  crash-recovery behavior — `ZedThreadID` preserved, `StopDesktop` best-effort.
- `restartBotAgent` passes `fresh = true`.

This keeps one implementation while making the divergence explicit, satisfying
AC-6. (Alternative — a separate handler — was rejected to avoid two nearly
identical restart code paths drifting apart, the exact problem the shared
primitive was created to prevent.)

### "Fresh desktop" scope — workspace volume

The user says "completely fresh desktop and context." There are two levels:

- **Level A (recommended default):** fresh conversation/thread + fresh container
  process, workspace volume (git checkout, files, agent's persisted session)
  preserved. Matches how the spawner's fresh-window re-activation already works;
  low risk; solves the reported symptom (tools re-init, thread cleared, new
  desktop process).
- **Level B (optional):** also discard/re-clone the workspace ZFS volume so the
  desktop filesystem is pristine. Heavier; only needed if stale on-disk agent
  session state is the problem. Recommend deferring unless testing shows Level A
  leaves stale state behind.

Go with **Level A**; note Level B as a follow-up toggle if required.

### Frontend

- Add a confirmation dialog (reuse the existing `DeleteConfirmWindow` pattern
  already in `HelixOrgBotDetail.tsx`) warning that current context will be
  permanently discarded.
- Keep the existing snackbar success/error handling; ensure a failed backend
  response shows the error (already wired via `err?.response?.data?.error`).

## Key files

| Concern | File |
|---|---|
| Button + handler + confirm dialog | `frontend/src/pages/HelixOrgBotDetail.tsx` |
| Mutation hook | `frontend/src/services/helixOrgService.ts` |
| Bot restart handler | `api/pkg/org/interfaces/server/api/bots.go` (`restartBotAgent`) |
| SessionRestarter port | `api/pkg/org/interfaces/server/api/api.go` |
| In-proc restart client | `api/pkg/server/helix_org_inproc.go` (`RestartSession`) |
| Restart primitive | `api/pkg/server/session_handlers.go` (`restartSessionContainer`) |
| Clear primitive (reuse) | `api/pkg/server/session_clear.go` (`ClearSession`) |
| Desktop lifecycle | `api/pkg/external-agent/hydra_executor.go` (`StopDesktop`/`StartDesktop`) |
| Existing restart test | `api/pkg/server/restart_session_container_test.go` |

## Risks / Notes

- Changing the shared port signature (`RestartSession`) touches all callers —
  update the in-chat and spec-task surfaces to pass `fresh = false`.
- Making `StopDesktop` fatal only for the fresh path avoids regressing crash
  recovery (where the container is often already gone).
- Validate the bot's session is a `zed_external` agent before clearing; the
  fresh path is only meaningful for those.
