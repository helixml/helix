# Design: Smooth Sandbox Restart With Progress and Clean Reconnect

## Summary

Restart is a **teardown followed by a boot**, but nothing persists "a boot is in
flight" across the teardown, and the session read path actively downgrades an
in-flight boot to `stopped`. Three small changes fix it:

1. **Persist the intent.** `restartSessionContainer` writes
   `external_agent_status = "restarting"` *before* `StopDesktop`, and `StopDesktop`
   does not clear that marker.
2. **Stop lying about it.** The live executor probe in `getSession` must never
   downgrade an in-flight lifecycle status (`starting` / `restarting`) to
   `stopped`. The task-list handler already does this correctly — extract one
   shared helper so the two copies cannot diverge again.
3. **Render it, then reconnect.** The frontend maps `restarting` to its existing
   `starting` state (spinner), writes it optimistically on click so there is no
   3s poll gap, and bumps the stream viewer's `wakeSignal` on the transition back
   into `running` so the transport reconnects to the new container with a fresh
   retry budget.

No new component, service or endpoint. The restart endpoint, thread-reset
semantics and container lifecycle are unchanged.

## Current flow (and where it breaks)

```
user clicks Restart
  └─ POST /api/v1/sessions/{id}/restart-agent      (blocking, 30-90s+)
       ├─ threadIsWedged()                          unchanged
       ├─ StopDesktop()
       │    ├─ delete h.sessions[sessionID]         ← executor probe now fails
       │    ├─ DeleteDevContainer                     …but ContainerName stays set
       │    └─ ExternalAgentStatus = ""             ← DB says "nothing happening"
       └─ resumeSessionInternal() → StartDesktop()
            └─ setExternalAgentStatus("starting")   ← written, then clobbered

meanwhile, every 3s:
  GET /api/v1/sessions/{id}
    └─ ContainerName != "" && executor.GetSession() fails
         → response.external_agent_status = "stopped"   ✗ THE BUG

frontend: deriveSandboxState("stopped") → "absent" → isPaused
  → ExternalAgentDesktopViewer.tsx:677 paused placeholder + "Restart desktop"
  → SpecTaskDetailContent.tsx:2667 showStart=true, showRestart=false
```

Key source anchors:

| Concern | File:line |
|---|---|
| Restart orchestration | `api/pkg/server/session_handlers.go:2868` |
| Live-probe override (buggy) | `api/pkg/server/session_handlers.go:80-92` |
| Live-probe override (correct) | `api/pkg/server/spec_driven_task_handlers.go:534-546` |
| Teardown clears status, keeps container name | `api/pkg/external-agent/hydra_executor.go:1004-1016` |
| Boot marks "starting" | `api/pkg/external-agent/hydra_executor.go:500` |
| Targeted JSONB status writes | `api/pkg/store/store_sessions.go:384-435` |
| State derivation | `frontend/src/components/external-agent/sandboxState.ts:29` |
| Paused overlay + Start button | `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx:613-709` |
| Wake signal / retry reset | `ExternalAgentDesktopViewer.tsx:140-149`, `DesktopStreamViewer.tsx:1569-1588` |
| Toolbar Start/Stop/Restart wiring | `frontend/src/components/tasks/SpecTaskDetailContent.tsx:2657-2688, 2790-2818` |
| Optimistic starting write | `frontend/src/utils/optimisticSessionStarting.ts` |

## Target flow

```
user confirms Restart
  ├─ optimistic cache write: external_agent_status="restarting",
  │    status_message="Restarting desktop…"        → spinner on next render
  └─ POST /restart-agent
       ├─ MarkSessionRestarting()                  persisted BEFORE teardown
       ├─ StopDesktop()  — preserves "restarting"
       ├─ resumeSessionInternal() → StartDesktop() → "starting" → "running"
       └─ on any error: ClearSessionRestartingStatus() → paused + error detail

GET /sessions/{id}: live probe SKIPPED while status ∈ {starting, restarting}
frontend: "restarting" → sandboxState "starting" → isStarting
  → reconnecting overlay with spinner + status message (stream stays mounted)
  → on starting→running edge: wakeSignal++ → resetRetryState + reconnect(500)
```

## Key decisions

### D1: A distinct `"restarting"` status, not a reuse of `"starting"`

`StopDesktop` must clear the status when a user stops a *booting* desktop (the
existing "Stop" button inside the starting spinner,
`ExternalAgentDesktopViewer.tsx:528`), but must **not** clear it when the stop is
the first half of a restart. A distinct value makes that condition exact
("preserve only `restarting`") instead of a heuristic. It also keeps restart
distinguishable in logs and in the status message.

The frontend does **not** grow a fourth `SandboxState`: `deriveSandboxState` maps
`restarting` → `starting`, so every existing consumer (viewer, toolbar, Kanban
card, `isSandboxOffline`, CLI) gets correct behaviour with no per-call-site
change. The user-visible difference is carried by `status_message`.

Rejected alternative: clear `ContainerName` in `StopDesktop` so the probe branch
is skipped. It would fix the read path but breaks the paused-screenshot and
reconnect/discovery paths that key on the container name, and it still leaves a
window where the status is `""` and the UI reads `absent`.

### D2: One shared live-status helper

Add `liveExternalAgentStatus(session *types.Session) string` in the server
package, implementing the single rule:

```
if status is "starting" or "restarting" → trust the DB (a boot is in flight)
if ContainerName == ""                  → trust the DB (pre-boot window)
if executor == nil                      → "stopped"
if executor.GetSession(id) errors       → "stopped"
otherwise                               → "running"
```

`getSession` and `listTasks` both call it. This is a de-duplication, not a new
abstraction: the two inline copies already exist and have already drifted. The
`listTasks` copy also treats `""` as probe-able (a stopped-but-unlabelled
session); keep that behaviour in the helper so the list keeps downgrading truly
dead containers.

Note the helper feeds `getSession`'s ETag (`session_handlers.go:103`), so the
status change naturally busts the cached response.

### D3: Optimistic write on click, reconciled by polling

`useSandboxState` polls every 3s; a restart that returns fast (or an overlay that
renders before the first poll) would otherwise flash the stopped state. Reuse the
existing pattern from `optimisticSessionStarting.ts` — the file's own comments
document why we write to both `full`/`skip` query-key variants and why we do
**not** `invalidateQueries` (a refetch races the backend write and flickers the
spinner off; see `002047_yet-again-sending-a`).

Generalise that helper rather than copy it:
`optimisticallyMarkSessionStarting(qc, id, { status, message, force })`, with the
restart call passing `status: 'restarting'` and `force: true` (the existing
no-op-if-already-running guard is wrong for restart, which is clicked precisely
*because* the session is running). Keep the default behaviour identical for the
existing send/start callers.

On restart failure, `invalidateQueries` to drop the optimistic value.

### D4: Drive the controls off lifecycle state, not the HTTP request

`isRestarting` is cleared in a `finally` when the POST returns, which is both too
early (the container may still be booting) and too late (the request blocks for
the whole teardown). After D1–D3 the derived state is authoritative:

- `effectiveIsDesktopPaused` is false while restarting → no **Start** button.
- `isDesktopRunning` is false while restarting → Restart/Stop hidden.
- Keep `isRestarting` only as the in-flight guard for the click handler and as
  `restartBusy`, OR'd with `isDesktopStarting` so the button stays busy for the
  whole boot.

### D5: Reconnect on re-entry to running, not only on wake-from-paused

`ExternalAgentDesktopViewer`'s `sawPausedRef` only fires `wakeSignal` on a
`paused → reachable` edge. After this fix a restart goes
`running → starting → running` and never touches `absent`, so the viewer would
keep its (possibly exhausted) retry budget pointed at the dead container.

Change the edge to: bump `wakeSignal` whenever the sandbox **re-enters
`running` after having left it** (track `wasRunningRef` / `leftRunningRef`).
That strictly covers the old paused→running case as well, so it replaces rather
than adds to the existing logic. First mount (`loading → running`) must still not
fire — guard on having observed a non-running state after a running one.

### D6: Overlay copy and watchdog

- `showReconnectingOverlay` already keeps the stream mounted; its non-paused
  branch hardcodes "Reconnecting…". Show `statusMessage` when present so the
  user sees "Restarting desktop…" and then the backend's boot progress messages
  (golden-cache unpack progress etc., `hydra_executor.go:512-530`).
- The `startingTooLong` watchdog (`ExternalAgentDesktopViewer.tsx:164-167`,
  ~2 min) currently only affects the pre-first-run starting screen. Apply it to
  the reconnecting overlay too, so a wedged restart offers a recovery action
  instead of an endless spinner.

### D7: Crash safety

`ClearStaleStartingSessions` (`store_sessions.go:374`) clears sessions stuck in
`starting` at API boot. It must also clear `restarting`, otherwise an API restart
mid-teardown pins a session in a permanent spinner. Same for
`ClearSessionStartingStatus`'s `WHERE` clause where it is used to abandon a boot.

Add `MarkSessionRestarting` / and reuse the targeted JSONB-merge style of
`MarkSessionStartingIfIdle` — full-row `GORM Save` races the streaming path's
writes (documented at `auto_wake_stuck_interactions.go:75-86`).

## Testing strategy

Per `helix/CLAUDE.md`, unit tests are not sufficient evidence here — this is a
lifecycle change, so it must be exercised against a live container.

1. **Go unit tests** (`api/pkg/server`, gomock + testify suite):
   `liveExternalAgentStatus` table test covering
   `{running, starting, restarting, "", stopped} × {container/no container} ×
   {executor tracks/doesn't}`; `restartSessionContainer` marks then clears on
   error.
2. **Frontend unit tests** (vitest, alongside `sandboxState.ts` and
   `SandboxStatusIndicator.test.tsx`): `deriveSandboxState('restarting')` →
   `starting`; the optimistic helper's `force` path; the `wakeSignal` edge.
3. **End-to-end in the inner Helix** (`localhost:8080`, register
   `test@helix.ml` / `helixtest`): create a spec task, wait for a live desktop,
   click Restart, and capture screenshots at ~2s, ~15s and after reconnect.
   Required evidence: no "Desktop paused"/Start button at any sample, spinner
   throughout, video frames from the new container without a manual reconnect.
   Confirm the new container id differs (`docker ps | grep ubuntu-external-<sid>`).
4. **Failure path**: restart a session whose project was removed (or stub a
   StartDesktop error) and confirm the UI lands on the paused placeholder with
   the error detail, not a stuck spinner.

## Learnings for future agents

- **Session lifecycle status is computed, not just stored.** `GET /sessions/{id}`
  overwrites `external_agent_status` from a live executor probe before
  responding. Anything that writes the status to drive UI must survive that
  probe, or it will never reach the browser. The same override is duplicated in
  `listTasks`, and the two copies had already diverged — the list one was right.
- **`StopDesktop` leaves `ContainerName` set** and only clears the status at the
  very end, so "has a container name" is not the same as "has a container".
- **Never `invalidateQueries` right after an optimistic session-status write** —
  the refetch races the backend goroutine and flickers the spinner off. This is
  documented in `frontend/src/utils/optimisticSessionStarting.ts` and traced in
  spec `002047_yet-again-sending-a`.
- **`hasEverBeenRunning` changes which empty state you get.** A first boot shows
  the full-screen starting spinner; a restart of a previously-running desktop
  shows the *overlay* branch over the mounted stream. Fixes to the boot spinner
  do not automatically fix the restart case.
- Related prior art: `design/2026-07-20-restart-clears-zed-thread-context-loss.md`,
  `design/2026-07-21-restart-discards-thread-on-nonclean-turn.md`,
  `design/2026-08-16-api-restart-live-turn-reconnect.md`.
