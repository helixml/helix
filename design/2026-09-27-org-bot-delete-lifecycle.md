# Org Bot delete lifecycle

What an Org Bot creates over its life, and what `DELETE /api/v1/orgs/{org}/bots/{id}`
(and the `delete_bot` MCP tool) does with each. Entry point:
`lifecycle.Service.delete` → `AgentDelivery.CleanupAgent` →
`inProcHelixClient.DeleteProject` → `DeleteLinkedAgent` → `Mirror.Stop` → reconcilers.

## Problems found (2026-09-27)

1. **Delete failed on fresh databases.** `DeleteLinkedAgent` deleted from
   `org_subscriptions`, a table dropped by migration 0005 and no longer
   auto-migrated since #3087. Tests created the table by hand, hiding it.
2. **Delete only stopped the newest desktop.** `deleteProject` stops the latest
   exploratory session and planning sessions. Stop is not destroy: hydra keeps
   `/data/workspaces/{sessions,spec-tasks}/<id>`, `/data/sessions/<ses>`, the
   zvol / file-copy Docker data, and — when it has lost the container from its
   in-memory map (any hydra restart) — even the container and `docker-data-<ses>`
   volume. Dev sandbox at the time: 13G session workspaces, 159G spec-task
   workspaces, ~40 orphaned `docker-data-ses_*` volumes.
3. **The running activation outlived the delete.** The delivery queue spawned
   on `context.Background()`, so a deleted bot's activation kept one of 8
   process-wide spawner slots for up to 24h and could re-provision a project,
   repo, and session after `DeleteProject`.
4. **The orphan reaper protected the wrong things.** Its live set required
   `model_name = 'external_agent'`, but bot and exploratory sessions carry real
   model names, so stopped bot desktops were reaped while still in use. Tasks of
   deleted projects stayed live forever (non-terminal or Keep Alive).
   `/data/sessions/<ses>` was never swept.

## Resource map

| Resource | Created by | On bot delete |
|---|---|---|
| `org_bots` row, reporting lines, attachments, worker-secret bindings, asset links | lifecycle Create / MCP tools | Deleted (row + FK cascades) |
| Transcript/team/DM triggers, Slack auto-route | reconcilers | Deleted by reconcilers (best-effort) |
| `org_bot_runtime_state` (project, app, repo, session, hiring user) | spawner / `WorkerProject.Ensure` | Deleted |
| Agent App + knowledge rows | lifecycle Create | Deleted |
| NATS consumer + queued activations | agent delivery | Deleted |
| Running activation | agent delivery | **Cancelled; delete waits for it (≤30s)** |
| Bot instances (`org_bot_instance` sessions) | instances API / MCP | **Destroyed** (sandbox + host data) and session rows deleted, before the project |
| Bot project | `WorkerProject.Ensure` | Soft-deleted (no restore path exists) |
| Every desktop of the project (bot session, inline forks, task sessions, soft-deleted sessions) | spawner / spec tasks | **Destroyed**: container, `docker-data-<ses>`, zvol, file-copy dir, workspace, `/data/sessions/<ses>`, paused screenshot, session API keys |
| Spec-task workspaces of the project's tasks | hydra executor | **Destroyed** |
| Sandboxes with the project's `project_id` | sandboxes API | **Deleted** |
| Golden Docker cache of the project | golden builds | **Deleted** on every online sandbox |
| Per-bot git repo `<bot>-<project>` | `ensureWorkerRepo` | Kept on purpose (d887ef669e) |
| Transcript events, activations, audit | spawner / mirror | Kept on purpose (audit trail) |
| Triggers, processors, assets the bot created, child bots | MCP tools | Kept (org-level, owned by the org) |
| Sandboxes the bot created outside its project | `create_sandbox` tool | Kept; expire by TTL |

Host teardown is best-effort (project desktops and, during bot delete, instance
sandboxes): an unreachable sandbox host must not make a bot undeletable.
Whatever it misses, the orphan reaper removes once its grace period
(`HELIX_ORPHAN_REAPER_GRACE_PERIOD`, default 30 days) passes, because sessions
and tasks of deleted projects are no longer live. The reaper covers zvols,
workspace dirs, `/data/sessions`, file-copy dirs, stopped `ses_` containers, and
unused `docker-data-ses_*` volumes. It does not cover running containers (hydra
keeps tracked ones live) or golden caches.

The delete runs detached from the caller's context (10-minute bound), so a
proxy timeout cannot leave a half-deleted bot whose retry skips the archived
project. RevDial requests to hydra now honour their context after the dial.

## Hydra: stop vs destroy

`DELETE /api/v1/dev-containers/{ses}` stops (warm-restart friendly).
`POST /api/v1/dev-containers/{ses}/destroy[?spec_task_id=spt_…]` destroys — a
separate route so an older hydra answers 404 instead of silently stopping:
finds containers by `helix.session_id` label or legacy name even when hydra
isn't tracking them, and removes every on-host resource. IDs are validated as
single path elements with the expected prefix. Executor entry point:
`Executor.DestroyDesktop(ctx, sessionID, specTaskID)`. It replaces the
workspace-only `DeleteWorkspace` / `DELETE …/workspace` route that bot
instance delete used: that path left `/data/sessions/<ses>`, inner Docker
data, and the paused screenshot its own stop wrote.

## Verification (dev stack, 2026-09-27)

- Hydra fixtures: a plain stop of an untracked, exited session left container,
  volume, and dirs; destroy removed all of them for tracked and untracked
  sessions, left a bystander intact, and rejected `spt_../../x`.
- Reaper fixtures (live set = every id on the host but the fixtures, grace 0,
  dry run first): reaped an exited `ses_` container, its volume, and a dangling
  volume; kept a running container and an `sbx_` container.
- End to end through the API: bot + instance created and activated; the bot's
  container crashed and hydra restarted (untracked); `DELETE /bots/{id}` → 204.
  Afterwards: bot row, runtime state, app, session API keys, sandbox rows,
  containers, volumes, workspace and `/data/sessions` dirs all gone; project
  archived and repo kept by design. Recreating the same bot id activated
  cleanly. Deleting a bot mid-activation cancelled it (`agent delivery:
  activation cancelled`, row closed) and cleaned up the same way.

## Not addressed

- Bot-project workspaces are removed immediately, including org members' spec
  tasks and sessions in that project (members have access). Previously they
  lingered until the reaper; there has never been a project restore path.
- The bot's main session row is kept (not soft-deleted) as history; its host
  data is gone and the reaper treats it as dead.
- A desktop start already in flight (`go StartDesktop`) when the bot is deleted
  can still bring a container up afterwards; nothing reaps running containers.
- Golden caches on sandboxes that are offline at delete time, or promoted by a
  golden build finishing after the delete, are not removed.
- Generic `deleteProject` (user-initiated) still only stops desktops; the
  reaper now reclaims its leftovers after the grace period.
- `deleteSession` does not stop the session's desktop.
- In-memory per-session maps in the API (`creationLocks`, `cancelTurnMutexes`,
  `promptDrainMutexes`, `contextMappings`) grow for the process lifetime.
- Recreating a bot with a deleted bot's ID resurfaces its old transcript events.
