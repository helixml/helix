# Approving a design review does not wake a stopped planning desktop

Investigation and fix, 2026-09-22. Reported by Luke; live repro on the meta instance.

## Symptom

Clicking **Approve** on a spec-task design review refused with:

> **Desktop paused**
> This task's sandbox is stopped. Start the desktop to interact with it.
>
> `prepare implementation branch: git checkout/push feature branch: failed to connect
> to desktop ses_01m2j8z52dpg755pk2v8zv88r6 via RevDial: no connection`

The task parked in `implementation_queued`, the review row stayed `in_review`, and the
agent was never woken or messaged. Approve was a dead end that told the user to go start
the desktop by hand — which, done manually, then required clicking Approve again.

Live repro: task `spt_01m2j8z3srtk2e5qxke7mfkp4x`, review `53497ebd-…`, session
`ses_01m2j8z52dpg755pk2v8zv88r6` with `config.external_agent_status = "stopped"`.

## Root cause

`SpecDrivenTaskService.ApproveSpecs` assumed the planning desktop was still running.
Nothing in the path started it.

1. `submitDesignReview` → `ApproveSpecs`.
2. `ApproveSpecs` atomically claims the handoff via `TransitionSpecTaskStatus`; status
   becomes `implementation_queued`. **This part succeeded.**
3. `syncGitIdentityToUser` failed (logged non-fatal).
4. `ensureFeatureBranchInContainer` failed and **returned**, aborting the approval.
5. The code that actually messages the agent — `BuildApprovalInstruction` +
   `TransitionToImplementation` — sits *after* step 4 and was never reached.

Both failing calls go through `ExecInDesktop`, which needs a live RevDial connection to
the desktop container. `StartDesktop` appeared nowhere in `ApproveSpecs`; its only call
sites in that file are planning-session *creation* paths.

## Why it regressed now

`ensureFeatureBranchInContainer` has been there since 2026-05-20 (`8ef0b9d8a`), so the
missing wake is not new code. What changed is **when approval happens**.

PR #3222 (`feat/spec-task-planning-agent`, merged 2026-09-15) turned approval into an
asynchronous human review that can happen days later. The old inline flow ran while the
planning agent was still live, so the container was almost always up.
`HELIX_DESKTOP_IDLE_TIMEOUT` defaults to **1h**, so the planning desktop is reliably gone
by approval time.

The reported review was created 2026-09-15 10:21 and approved 2026-09-22 13:38 — seven
days. The precondition ("desktop is up at approval time") did not change; the probability
of it holding did.

**Generalisation worth remembering: any operation that follows an asynchronous human
review must assume the sandbox is stopped. That is now the common case, not the edge.**

## Second bug: hot retry loop

`SpecTaskOrchestrator.handleImplementationQueued` re-drove the marker by calling
`ApproveSpecs` again on every orchestration tick (~10s), failing identically each time.
Measured live: **167 retries between 13:39:03 and 14:06:43** — 27 minutes and still
going when captured. Each iteration re-ran `ApproveSpecs` from the top, including
`SyncBaseBranch`: **a fetch from the external GitHub remote every 10 seconds.** The
user-visible error was produced once; the loop then ran silently.

## The fix

### 1. A desktop-readiness gate in `ApproveSpecs`

New `EnsureDesktopReady` callback on `SpecDrivenTaskService` (same pattern as the
existing `ExecInDesktop` / `TransitionToImplementation` callbacks, wired in `server.go`),
implemented by `ensureDesktopReadyForSession` in `api/pkg/server`:

1. External-agent WebSocket already connected → ready. **The warm path is byte-for-byte
   the previous behaviour.**
2. Not connected → `MarkSessionStartingIfIdle` (so the UI spinner engages immediately),
   then `startDevContainerForSession`.
3. `wait > 0` → `waitForExternalAgentReady`; `wait == 0` → return not-ready after kicking.
4. A refused start (quota, subscription, boot failure) returns a real error — the only
   case that becomes a user-visible hard failure.

`ApproveSpecs` calls this immediately before `syncGitIdentityToUser` and only proceeds to
the two exec calls when it reports ready.

`startDevContainerForSession` is the *existing* canonical wake path, shared with
`resumeSession`, `startDevContainerForSpecTask` and the auto-wake watchdog. Per repo
rules: one mechanism, no new fallback path.

**Readiness is gated on the external-agent WebSocket, not the desktop bridge.** A stopped
desktop is destroyed, not suspended — `StopDesktop` removes the container and its zvol,
so a restart re-runs `helix-workspace-setup.sh` and re-clones the repos. The bridge (what
`StartDesktop` itself waits for) comes up long before `/home/retro/work/<repo>` exists,
which is the directory `ensureFeatureBranchInContainer` cds into. `start-zed-core.sh`
waits for `~/.helix-setup-complete` before launching Zed, and Zed dials the WS afterwards
— so "WS connected" is the existing proxy for "workspace is ready".

### 2. Split entry points

One shared body, two named entry points:

```go
func (s *SpecDrivenTaskService) ApproveSpecs(ctx, task) error               // handler: wait = 0
func (s *SpecDrivenTaskService) DriveImplementationHandoff(ctx, task) error // orchestrator: waits
```

The handler path never blocks on a boot: `submitDesignReview` runs under
`detachContext(r.Context(), 30*time.Second)` and a cold desktop boot is 90–150s. When the
desktop is cold it claims `implementation_queued`, kicks the start, and returns the new
sentinel `ErrDesktopStarting`, which every caller maps to success. The orchestrator then
finishes the handoff with `DriveImplementationHandoff`.

Re-driving an already-claimed handoff (`status == implementation_queued`) **skips
`SyncBaseBranch`** — the base branch was synced and persisted (`base_branch`,
`branch_name`) when the handoff was claimed. This removes the per-10s GitHub fetch.

### 3. Backoff

Per-task attempt state in the orchestrator (mutex-guarded map; in-memory by design — an
API restart resetting backoff is harmless and lets a given-up task be retried once the
operator has fixed whatever broke):

- at most one attempt in flight per task, on a detached goroutine so `processTask` stays
  non-blocking (it carries an explicit "should be fast and non blocking" contract);
- base 10s, ×2 per failure, ceiling 5 min;
- "desktop still booting" does not count as a failed attempt inside a ~5-min cold-start
  grace, mirroring `coldStartGracePeriod()` in `auto_wake_stuck_interactions.go`;
- after a ~20-min budget of real failures the actual reason is persisted onto the task
  once and retrying stops.

### 4. UX

`spec_tasks.metadata.error` is a **user-visible** field: it renders as the detail line
under "Desktop paused" on the task detail page (`persistSpecApprovalError` writes it;
`SpecTaskDetailContent.tsx` reads it into `desktopStartupMessage` →
`ExternalAgentDesktopViewer` `startupErrorMessage` → `TaskSessionPlaceholder` `detail`).
Not writing transient infrastructure states there is most of the UX fix.

Additionally:
- the frontend optimistically marks the planning session `starting` on approve success,
  so the spinner engages without waiting for the next 3s poll;
- `implementation_queued` now suppresses the paused placeholder the same way
  `queued_spec_generation` already did, so the user sees a starting state rather than
  "Desktop paused — start the desktop to interact with it".

The "Desktop paused" copy itself is unchanged — it is correct for a genuinely idle task.
The fix is that approval no longer routes through it.

### 5. Review-row ordering

`submitDesignReview` used to persist the review only at the very end, after
`ApproveSpecs`, so an error left the review at `in_review` while the task had advanced to
`implementation_queued`. The review now records the human's decision (`approved`,
`approved_at`, `overall_comment`) **before** the handoff is driven. The decision is a fact
that already happened; a handoff failure belongs on the task, not on the review.

## Sibling-path audit

- `approveSpecs` handler (`/spec-tasks/{id}/approve-specs`) — same `ApproveSpecs` call,
  same `persistSpecApprovalError` + 500. Given the same `ErrDesktopStarting` handling.
- `approveImplementation` and the `request_changes` branch of `submitDesignReview` — their
  only agent contact is `enqueueSpecTaskAgentMessage` → prompt queue →
  `sendCommandToExternalAgent`, which already kicks `autoStartDevContainerForSession` and
  replays on reconnect. **No fix needed.**
- `syncGitIdentityAsync` already retries in the background and is non-fatal. Unchanged.
- `handleSpecApproved` (orchestrator), the auto-approve goroutine in
  `approveImplementation`, and the helix-org `specTaskWorkflow` adapter all treat
  `ErrDesktopStarting` as success.

## Rejected alternatives

- **Keep the desktop alive through review** — fights the idle reaper, costs GPU hours for
  reviews that take days, and doesn't fix approval after an API restart.
- **Make `ensureFeatureBranchInContainer` non-fatal** — the branch would be wrong and the
  agent would commit to `main`; the pre-receive hook then rejects the push. That is the
  exact failure the function was added (`8ef0b9d8a`) to prevent.
- **Do the branch prep server-side in the bare repo** — doesn't fix the container's
  working tree, which is what the agent commits from.
- **A new wake helper local to `ApproveSpecs`** — `startDevContainerForSession` already is
  the one mechanism.

## Notes for future work

- RevDial `desktop-<sessionID>` readiness ≠ workspace readiness. A restarted desktop is a
  *fresh* container; repos are re-cloned. Gate container-filesystem work on the
  external-agent WebSocket, not on the bridge.
- `implementation_queued` is a durable, idempotent, re-drivable marker. Anything expensive
  or external (like `SyncBaseBranch`) must be done once at claim time, not on every
  re-drive.
- Don't put transient infrastructure states in `spec_tasks.metadata.error`.
