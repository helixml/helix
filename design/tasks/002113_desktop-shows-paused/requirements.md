# Requirements: Fix Desktop Showing "Paused" While Container Is Running

## Background

When a session is started through `StartExternalAgentSession`
(`api/pkg/server/session_handlers.go`), the desktop viewer renders "paused"
even though the container is up and running. This affects every session
launched via this path — helix-org Workers and cron-triggered sessions.

### Root cause (confirmed)

1. `HydraExecutor.StartDesktop` (`api/pkg/external-agent/hydra_executor.go:502-525`)
   re-fetches the session row from the store and persists the container
   metadata: `container_name`, `external_agent_status="running"`,
   `container_id`, `dev_container_id`, and `sandbox_id`.
2. After `StartDesktop` returns, `StartExternalAgentSession`
   (`session_handlers.go:2548-2554`) re-saves its **stale in-memory** `session`
   struct via `UpdateSession(*session)`. That struct never received the
   container metadata, so the write wipes `container_name` and
   `external_agent_status` back to empty. It only re-applies `DevContainerID`
   and `SandboxID` — both of which `StartDesktop` already persisted.
3. The frontend `useSandboxState` hook
   (`frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx:47-77`)
   treats an empty `container_name` (and empty `external_agent_status`) as
   `absent` → `isPaused = true` → shows "paused".

## User Stories

**As a helix-org operator**, when a Worker or cron trigger starts a session,
I want the desktop viewer to show the running container instead of "paused", so
that I can see and use the live desktop without confusion.

**As a developer**, I want `StartExternalAgentSession` to stop clobbering
metadata that `StartDesktop` already persisted, so the session row is the single
source of truth.

## Acceptance Criteria

- [ ] After `StartExternalAgentSession` returns, the session row retains
      `container_name` and `external_agent_status="running"` set by `StartDesktop`.
- [ ] The desktop viewer shows the running desktop (not "paused") for sessions
      started via this path (helix-org Workers, cron triggers).
- [ ] `DevContainerID` and `SandboxID` remain correctly persisted on the row.
- [ ] No regression for the already-running early-return path in `StartDesktop`
      (where the response carries empty `DevContainerID`/`SandboxID`).
- [ ] A regression test covers the read-modify-write clobber.

## Out of Scope

- The frontend `absent → paused` mapping is correct and unchanged.
- Other call sites that start desktops (spec-task launch, exploratory resume)
  are not modified unless they share the identical clobber pattern.
