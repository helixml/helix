# Requirements: Smooth Sandbox Restart With Progress and Clean Reconnect

## Background

Clicking **Restart** on a spec task (or any agent view that embeds the sandbox
restart control) currently makes the UI look broken:

1. The desktop area flips to "Desktop paused — This task's sandbox is stopped"
   with a **Restart desktop** / **Start desktop** button and no spinner.
2. The toolbar's Restart/Stop buttons disappear and a **Start** button appears.
3. This state persists for the whole container rebuild (30–90s, sometimes
   longer), so the user assumes the restart failed and clicks Start — issuing a
   second lifecycle command against a session that is already mid-restart.

Root cause (verified by reading the code, see `design.md`):

- `restartCrashedAgentThread` → `restartSessionContainer`
  (`api/pkg/server/session_handlers.go:2868`) calls `StopDesktop` then
  `resumeSessionInternal`. `HydraExecutor.StopDesktop`
  (`api/pkg/external-agent/hydra_executor.go:882`) deletes the in-memory
  `h.sessions` entry immediately and writes `external_agent_status = ""` at the
  end, but **never clears `ContainerName`**.
- `getSession` (`api/pkg/server/session_handlers.go:80-92`) overrides the stored
  status with a live executor probe **whenever `ContainerName != ""`**. During a
  restart the container name is stale and the executor no longer tracks the
  session, so the handler returns `"stopped"` — clobbering both the empty status
  and the `"starting"` that `StartDesktop` writes at
  `hydra_executor.go:500`.
- `deriveSandboxState` (`frontend/src/components/external-agent/sandboxState.ts`)
  maps `"stopped"` → `"absent"` → `isPaused`, which renders the paused
  placeholder overlay (`ExternalAgentDesktopViewer.tsx:677`) and flips the
  toolbar to `showStart` (`SpecTaskDetailContent.tsx:2667`).
- `isRestarting` in `SpecTaskDetailContent` is local state wired only to the
  toolbar's `restartBusy`; nothing tells the desktop surface a restart is in
  flight.

The same override already has the correct carve-out in the task-list handler
(`spec_driven_task_handlers.go:534-546`: "Skip 'starting'"), so the two copies of
this logic have diverged.

## User Stories

### US-1: Restart shows progress, not a stopped sandbox
**As a** user of a spec task with a running sandbox
**I want** the desktop area to show a spinner and "Restarting desktop…" from the
moment I confirm Restart until the new container is streaming
**So that** I can tell the restart is working and don't think it failed.

Acceptance criteria:
- [ ] Confirming Restart switches the desktop surface to a progress state
      **within one render** (no wait for the next 3s poll).
- [ ] The progress state shows a spinner plus the backend status message
      (falling back to "Restarting desktop…").
- [ ] At no point during a successful restart does any surface show "Desktop
      paused", "Sandbox stopped", **Start desktop**, or **Restart desktop** as a
      call to action.
- [ ] The sandbox status indicator reads `starting` (not `stopped`) for the
      whole restart window.

### US-2: No second lifecycle command can be issued mid-restart
**As a** user
**I want** the Start/Stop/Restart controls to reflect that a restart is in
flight
**So that** I cannot send a conflicting command that confuses the backend.

Acceptance criteria:
- [ ] While a restart is in flight the toolbar shows no **Start** button.
- [ ] The Restart control is disabled (busy) until the sandbox is running again.
- [ ] Re-clicking Restart while one is in flight is a no-op (existing
      `isRestarting` guard stays, and is now driven by the real lifecycle state,
      not just the HTTP request duration).

### US-3: Clean reconnect to the new container
**As a** user
**I want** the streaming desktop to attach to the new container as soon as it is
up
**So that** I don't have to reload the page or click "Reconnect".

Acceptance criteria:
- [ ] When the sandbox returns to `running`, `DesktopStreamViewer` resets its
      reconnect retry budget and reconnects (same mechanism as the existing
      `wakeSignal` wake path), so an exhausted retry counter from the dead
      container cannot leave the viewer stuck on an error.
- [ ] Video frames from the new container render without a manual reconnect or
      page reload.
- [ ] Fullscreen is not exited by the restart (the stream stays mounted; only
      overlays change), matching today's behaviour for reconnects.

### US-4: A failed restart is still visible
**As a** user
**I want** a restart that fails to end in an actionable state
**So that** I am not left staring at a spinner forever.

Acceptance criteria:
- [ ] If the restart API call fails, the optimistic progress state is reverted
      and the error snackbar is shown.
- [ ] If the backend errors after teardown, the session's lifecycle status is
      cleared so the UI returns to the paused placeholder with a Start action
      and the startup error detail.
- [ ] If the sandbox stays in the restarting/starting state longer than the
      existing 2-minute watchdog, the surface says the desktop may have failed
      to start and offers a recovery action (same treatment as the existing
      `startingTooLong` path).
- [ ] An API process restart in the middle of a sandbox restart does not leave a
      session permanently pinned in the restarting state
      (`ClearStaleStartingSessions` covers it).

### US-5: Consistent across surfaces
**As a** user of any view with the sandbox restart button
**I want** the same behaviour on the spec task detail page, the mobile layout,
the embedded task view and the org agent workspace
**So that** the fix isn't a one-page patch.

Acceptance criteria:
- [ ] The fix lives in shared code (`deriveSandboxState`,
      `ExternalAgentDesktopViewer`, the backend status helper), not in
      `SpecTaskDetailContent` alone.
- [ ] Kanban cards / task list (`sandbox_state` from `listTasks`) show
      `starting`, not `absent`, during a restart.
- [ ] The org agent workspace (`OrgAgentSessionWorkspace`) does not show a
      "Sandbox not running" placeholder or a Start button while its bot's
      restart is in flight.

## Non-Goals

- Changing what a restart does to the Zed thread (`threadIsWedged` logic stays
  exactly as designed in `design/2026-07-21-restart-discards-thread-on-nonclean-turn.md`).
- Making the restart itself faster.
- Changing the org-bot restart's session-reset semantics (it deletes and
  re-provisions the session; only its *reported* status is in scope).

## Open Questions

1. **New status value vs. reusing `"starting"`.** The design adds a distinct
   `external_agent_status = "restarting"` so `StopDesktop` can tell "a restart
   owns this teardown" from "a user stopped a booting desktop" (the existing
   Stop-while-starting button). Reusing `"starting"` would need no new value but
   would make that distinction impossible. Assumption: add `"restarting"`.
   Confirm?
2. **Restart watchdog duration.** Reusing the existing 2-minute
   `startingTooLong` threshold. A container rebuild after a restart can be
   slower than a cold start (ZFS clone, image pull). Should the restart
   watchdog be longer (e.g. 4 minutes)?
3. **Backdrop during restart.** Assumption: keep the last frame of the dead
   container visible behind the dimmed "Restarting…" overlay (today's
   reconnecting behaviour), rather than falling back to the paused screenshot.
   Confirm this is the preferred look.
4. **Org-bot restart.** `restartBotAgent` deletes the session and activates a
   fresh one, so its progress state comes from the bot DTO's `sandbox_status`
   (`pending` → `starting`), not from the session row. We plan to verify it
   reports `pending` across the whole re-provision and fix it if not — is
   bringing the org-bot path to parity in scope for this task, or should it be
   split out?
5. **Testing depth.** Plan is to verify end-to-end in the inner Helix
   (`localhost:8080`) with a real spec-task sandbox, per `helix/CLAUDE.md`. Is a
   screenshot sequence of the restart window an acceptable artefact for review?
