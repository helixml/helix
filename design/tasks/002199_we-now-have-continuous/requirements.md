# Requirements: Reconcile Stale Dev Container State After Sandbox Restart

## Background

Continuous delivery (see `.drone.yml`, `deploy-prod` pipeline → `scripts/deploy-prod.sh`)
rolls the SaaS by stopping and starting the stack. When the sandbox/hydra host is
restarted this way, the dev containers it hosted are gone, but the control-plane's
stored/cached state still reports them as **running**.

As a result, the spec-task details page and the Kanban view keep showing dev
containers as "running". When a user clicks one to view it (the desktop/screenshot
viewer), the request fails — the container no longer exists — and the viewer shows
repeated 503 errors ("Sandbox not connected" → "Session ended - desktop is no
longer available").

## Root cause (established during investigation)

The Kanban card's `sandbox_state` is derived in `listTasks`
(`api/pkg/server/spec_driven_task_handlers.go:309-351`). Its "live check" calls
`externalAgentExecutor.GetSession(...)`, which only reads the API's **in-memory**
`h.sessions` map (`api/pkg/external-agent/hydra_executor.go:731-744`). After a
*sandbox* restart where the *API* stayed up, that map still holds a stale
"running" entry, so the check passes and the card stays "running".

The only reconciler that flips a session `running → stopped` is
`OnSandboxDisconnected` (`hydra_executor.go:1629`), which requires the sandbox's
disconnect grace period to expire. On a fast CD redeploy the sandbox reconnects
quickly, so this path is unreliable. `DiscoverContainersFromSandbox`
(`hydra_executor.go:1415`) runs on reconnect but is **additive only** — it marks
discovered containers "running" and never marks now-missing sessions "stopped".

## User Stories

### US-1: Kanban reflects reality after a redeploy
As a user viewing the spec-task Kanban after a CD redeploy, I want cards whose dev
containers no longer exist to show as **stopped/absent**, so I don't try to open a
dead container.

**Acceptance Criteria**
- Given a sandbox restart that destroys its dev containers, when the sandbox
  reconnects (or within the normal Kanban poll interval afterwards), then each
  affected task's `sandbox_state` becomes `absent` (not `running`).
- Given a container that genuinely survived/was re-adopted after restart, its task
  continues to show `running`. (No false downgrades.)

### US-2: No misleading "view container" affordance for dead containers
As a user, I don't want to be offered a live desktop/screenshot preview for a
container that is gone.
**Acceptance Criteria**
- When `sandbox_state` is `absent`, the card does not render the live screenshot
  preview / "open desktop" affordance as if it were running.

### US-3: Correct active-sandbox counts
As an operator, I want the `active_sandboxes` counter to match the real number of
running containers after a restart.
**Acceptance Criteria**
- After reconciliation, the per-sandbox container count equals hydra's ground-truth
  count (already handled by `SetSandboxContainerCount`; verify it still holds).

## Out of Scope
- Automatically restarting/recreating dev containers after a redeploy.
- Changing the CD pipeline itself.
- Persisting/restoring container runtime across restarts.
