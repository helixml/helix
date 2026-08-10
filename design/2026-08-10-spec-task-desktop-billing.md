# Unified sandbox billing: back spec-task desktops with `types.Sandbox` rows

Date: 2026-08-10

## Problem

We have exactly two debit paths in the codebase:

1. `api/pkg/openai/logger/billing_logger.go` — LLM tokens.
2. `api/pkg/sandbox/controller_billing.go` — compute, for sandboxes created
   through the Sandboxes API.

The second one only ever sees rows in the `sandboxes` table. Spec-task
desktops — the containers that actually burn the GPU host — never create such
a row: `HydraExecutor.StartDesktop` calls `hydra.CreateDevContainer` directly
(`hydra_executor.go:375`). Consequences:

- `ReapBilling` iterates `ListSandboxes(status=running)` and therefore charges
  **nothing** for a spec-task desktop, whatever its size.
- No `ensureSandboxCredits` pre-check: an org with a zero balance can start an
  8 vCPU desktop.
- No `ensureSandboxLimits`: the desktop concurrency cap doesn't apply, and
  `quota.getActiveSandboxesByOrg` under-reports org usage.
- https://github.com/helixml/helix/pull/2976 gave users a self-service
  1 / 4 / 8 vCPU picker plus live resize on a running task. That is an 8×
  swing in real cost with zero billing signal.

## Approach

A `types.Sandbox` row becomes the **single billing and quota record for any
container Helix runs on a hydra host**, regardless of who provisions the
container. Two provisioning modes share the row:

| Mode | Row created by | Container created by | Discriminator |
|---|---|---|---|
| Controller-managed (Sandboxes API) | `sandbox.Controller.Create` | `sandbox.Controller.provision` | `session_id == ''` |
| Session-backed (spec-task desktops, exploratory sessions, subscription desktops) | `sandbox.Controller.BeginSession` | `HydraExecutor.StartDesktop` | `session_id != ''` |

We deliberately do **not** route spec-task desktop provisioning through
`Controller.provision`. That path knows nothing about workspace files, repo
checkout, branch mode, golden builds, ZFS clones, session API keys or Zed
config. Re-implementing it would be a rewrite with a large blast radius for no
billing benefit. The row is the meter; the executor stays the provisioner.

### Why the executor, not `spec_driven_task_service`

There are ~10 `StartDesktop` call sites and ~15 `StopDesktop` call sites
(spec-task planning, just-do-it, resume, exploratory sessions, session fork,
design review, Claude/Codex subscription desktops, golden builds, idle
checker, org worker activations). Metering at three of them inside
`spec_driven_task_service` would leave most desktops unbilled and duplicate
the logic three times. `HydraExecutor.StartDesktop`/`StopDesktop` is the one
choke point every desktop passes through, and it already holds the per-session
lock that makes the row lifecycle race-free.

## Changes

### 1. `types.Sandbox`

Two new columns (GORM AutoMigrate):

- `SessionID string` — the Helix session that owns the container. Non-empty
  means session-backed: hydra keys every operation for this container by
  session id, not by sandbox id.
- `SpecTaskID string` — optional, so the Sandboxes UI can link back to the task
  without a join through sessions.

Helpers: `SessionBacked()` and `HydraOpsID()` (returns `SessionID` when set,
else `ID`).

### 2. `sandbox.Controller` — session-backed lifecycle (`controller_session.go`)

- `BeginSession(ctx, *BeginSessionRequest) (*types.Sandbox, error)` — runs
  `ensureSandboxLimits` and `ensureSandboxCredits`, then upserts the row for
  that session (`status=pending`, `runtime=ubuntu-desktop`,
  `timeout_seconds=-1` so the TTL reaper never touches it — the desktop's
  lifetime is owned by the task, not by a sandbox TTL).
- `MarkSessionRunning(ctx, sessionID, hostDeviceID, containerID)` — flips to
  `running`, which is what opens the billing window (`SetSandboxStatus` stamps
  `started_at` and `billing_last_charged_at`).
- `MarkSessionStopped(ctx, sessionID)` — final partial-minute charge, then
  `status=stopped`.
- `ResizeSession(ctx, sessionID, vcpus, memoryMB)` — **flush then update**:
  bills the outstanding window at the *old* core count before persisting the
  new one. Without this, `billSandbox` would multiply up to a whole minute of
  already-elapsed 1-vCPU usage by the new 8-vCPU count.
- `EnsureSessionResizeCredits(...)` — pre-check before we ask hydra to resize.

### 3. Teardown routing

`Controller.Delete` currently calls `hydraClient.DeleteDevContainer(ctx,
sandbox.ID)`. For a session-backed row the container is registered under the
session id, so that call would silently no-op. `Delete` now dispatches to a
`DesktopStopper` callback (set at wiring time to
`externalAgentExecutor.StopDesktop`) when the row is session-backed and still
running. This is also what makes the credit-exhaustion path in `billSandbox`
actually stop a spec-task desktop.

`sandbox` cannot import `external-agent` (that direction already exists), so
the stopper is a function field injected in `server.go`, not an import.

### 4. Hydra op routing

`sandboxes_api_handlers.go` passes `sb.ID` as the hydra op key for exec, files
and terminal. Those now use `sb.HydraOpsID()`, so the Sandboxes UI can open a
terminal / browse files on a spec-task desktop as well.

### 5. Resume no longer drops the task's sandbox preset

Only the three `spec_driven_task_service` launch paths put `VCPUs`/`MemoryMB` on
the `DesktopAgent`. `resumeSessionInternal`, session fork and design review
rebuild the agent from the session and never read
`SpecTask.SandboxResourceOverrides`, so a paused 8 vCPU task came back
**uncapped** — running bigger than the user asked for and, once metered, billed
at the default preset. Caught by the live resume test below, not by any unit
test.

`HydraExecutor.resolveSpecTaskResources` now fills the size in from the owning
task whenever the caller didn't supply one, so every spec-task desktop path
honours the preset. An explicit caller-supplied size still wins.

### 6. Frontend

Session-backed rows appear in the existing org sandboxes list automatically.
The table and card views gain a "Spec task" marker that links to the task, so
the list reads as "all compute this org is paying for" rather than a mix of
unexplained rows.

## Deliberate non-goals

- No change to the price model: still `credits/second/core`, desktop rate vs
  headless rate, memory bundled with the core preset.
- No retroactive billing. `SandboxBillingEnabled` stays default-off, and
  enabling it calls `SetRunningSandboxesBillingLastChargedAt` so existing free
  usage is not charged.
- Golden builds go through `StartDesktop` and will therefore be metered like
  any other desktop once billing is on. That is intentional — they consume the
  same host.

## Operator impact

Both are gated behind `SandboxBillingEnabled`, which is default-off, except the
concurrency cap which applies regardless:

- **Desktop concurrency cap now applies to spec tasks.**
  `MaxConcurrentDesktopSandboxes` defaults to 10 per org and previously only
  constrained the Sandboxes API. An org that routinely runs more than 10
  concurrent spec-task desktops must raise the setting before upgrading.
- **Golden builds and exploratory sessions are now billable compute** — they
  go through `StartDesktop` like everything else.

## Verification

Run live against the dev stack (`SandboxBillingEnabled=true`, desktop price
0.008 credits/second/core), task `spt_01kznhwc1q3jxkvbz5rz1g7812`:

| Behaviour | Observed |
|---|---|
| Row created on task start | `sbx_01kznhwd7na4s5y64rnj6h6q5g`, `runtime=ubuntu-desktop`, `session_id`/`spec_task_id` set, 4 vCPU / 8192 MB from the task, `timeout_seconds=-1`, `expires_at` NULL |
| Per-minute charge at 4 cores | `-1.92` = 0.008 × 60 × 4, twice |
| Resize flush | 4→8 charged `-1.5255` for 47.67 elapsed seconds at the **old** 4 cores, then row → 8 vCPU / 16384 MB |
| Post-resize rate | `-3.84`/min = 0.008 × 60 × 8 |
| Failed resize does not reprice | hydra rejected 16 GB→2 GB (container using 2694 MB); no flush, row unchanged |
| Final charge on stop | `-5.288` for the 82.6s partial minute, row → `stopped` |
| Resume reuses the row | same row id, `started_at`/`billing_last_charged_at` reset so the stopped gap is not billed |
| Resume restores the preset | came back 8 vCPU / 16384 MB after the fix (was 4 / 8192 before) |
| Delete routes teardown to the executor | container `ubuntu-external-01kznhwc26vfdmnpbe43w3m71d` gone, final charge landed, row soft-deleted — the same path credit exhaustion takes |
| UI | cards and table both show the row with a "Spec task" source marker, 8 vCPU · 16 GB, Expires "Never"; the marker links to the task |

Two bugs the live run caught that unit tests could not:

1. `SetSandboxResources` wrote to a `vcpus` column; GORM maps `VCPUs` to
   `v_cpus`. Resizes silently failed to persist. Now covered by a GORM-backed
   store test that fails against the old column name.
2. Resume dropped the task's sandbox preset (see §5).

Not exercised live: credit exhaustion mid-session. Its stop path is
`Controller.Delete` → `stopSessionDesktop`, which the delete test above
exercises directly, plus unit coverage for the routing and the
wallet-drain branch.
