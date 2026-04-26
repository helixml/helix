# Design

## Summary

Gate the desktop-stop call in `handleDone()` on the task's `keep_alive` flag. The task still transitions to `TaskStatusDone` on PR merge (the merge is a real event we want recorded), but if the user has explicitly turned the lock green, we skip the `StopDesktop` call. We also wire the lock toggle so that turning Keep Alive **off** on a Done task with a running desktop triggers the deferred shutdown.

## Current State

Three orchestrator code paths transition a task to `TaskStatusDone` after a merge:

| Path | File | Lines |
|------|------|-------|
| All tracked PRs merged | `api/pkg/services/spec_task_orchestrator.go` | 778–800 |
| Branch merged to main (PR fallback) | `api/pkg/services/spec_task_orchestrator.go` | 848–857 |
| External PR detected as merged | `api/pkg/services/spec_task_orchestrator.go` | 1063–1086 |
| Branch merged, no PR found | `api/pkg/services/spec_task_orchestrator.go` | 1116–1129 |

All four ultimately cause the orchestrator's status-change handling to invoke `handleDone()`, which calls `containerExecutor.StopDesktop(ctx, task.PlanningSessionID)`.

The `keep_alive` flag exists at `api/pkg/types/simple_spec_task.go:199` and is exposed in the UI via the lock icon at `frontend/src/components/tasks/SpecTaskDetailContent.tsx:2215-2241`. The flag is updated through `spec_driven_task_handlers.go:967`. Today nothing in `handleDone()` reads it.

## Key Decision: Where To Gate

**Chosen: gate inside `handleDone()`.** One change, covers all four merge-detection paths, lowest risk of regression. Status, `MergedAt`, `CompletedAt`, and downstream listeners (golden Docker cache build trigger, attention events, PR-tracking UI) all keep working unchanged.

**Rejected: gate at each of the four merge-detection sites.** Would require either (a) skipping the status transition entirely — which breaks the PR-tracking UI and prevents the golden-build trigger from firing, or (b) duplicating the same `if task.KeepAlive` check in four places. Both are worse.

**Rejected: introduce a new task status like `TaskStatusDoneKeepAlive`.** Adds schema/UI surface area for a transient distinction. The existing flag is enough.

## Changes

### 1. Gate the desktop stop in `handleDone()`

`api/pkg/services/spec_task_orchestrator.go:1212`:

```go
func (o *SpecTaskOrchestrator) handleDone(ctx context.Context, task *types.SpecTask) error {
    if task.KeepAlive {
        log.Info().
            Str("task_id", task.ID).
            Msg("Task done but keep_alive is set — leaving desktop running")
        return nil
    }

    err := o.containerExecutor.StopDesktop(ctx, task.PlanningSessionID)
    if err != nil {
        return fmt.Errorf("failed to stop desktop: %w", err)
    }

    log.Info().
        Str("task_id", task.ID).
        Msg("Task in done status - stopping desktop")

    return nil
}
```

### 2. Stop the desktop when Keep Alive is turned off on an already-Done task

`api/pkg/server/spec_driven_task_handlers.go:967` (the keep-alive update handler). After the `KeepAlive` field is updated in the store, if the new value is `false` **and** the task is in `TaskStatusDone` **and** the desktop is still running, call `containerExecutor.StopDesktop()`. This honors acceptance criterion #5 — the user has an explicit way to release the desktop after merge.

Implementation note: the handler currently doesn't have direct access to the orchestrator's container executor. Two options:
- (Preferred) Inject `containerExecutor` into the handler the same way it's wired into the orchestrator and call it directly.
- (Alternative) Re-invoke `orchestrator.handleDone(ctx, task)` after the update — but this would also need to pass through the new gate, so it'd require a "force" parameter. More plumbing for no benefit. Use the direct call.

### 3. UI: keep the lock icon visible after merge

`frontend/src/components/tasks/SpecTaskDetailContent.tsx:2216` already gates the lock-icon visibility on `isDesktopRunning`, not on task status — so when `keep_alive=true` and the desktop survives the merge, the icon remains visible. **No frontend code change required.** Verify this in the browser during implementation testing.

## Data Model

No schema changes. `KeepAlive bool` already exists.

## Edge Cases

- **Race: PR merged + user toggling Keep Alive off at the same instant.** The orchestrator runs `handleDone` while the handler runs `StopDesktop`. Both call into `containerExecutor.StopDesktop` for the same `PlanningSessionID`. `StopDesktop` must be idempotent — verify, and if not, accept that one call returns "already stopped" as a benign error.
- **Multiple PRs across multiple repos.** The "all PRs merged" path (line 778) only fires once all are merged, so the gate fires once. Fine.
- **Task with no `PlanningSessionID`.** `StopDesktop` handles this today (returns nil or a benign error). The new gate doesn't change that.
- **Keep Alive turned off while task is still running (not yet merged).** Existing behavior preserved — the idle-shutdown logic re-engages, no special handling needed.

## Testing

- **Unit:** add a test in `api/pkg/services/` covering `handleDone` for both `KeepAlive=true` (no `StopDesktop` call) and `KeepAlive=false` (current behavior). Use `gomock` per the repo convention.
- **Manual / browser:** in the inner Helix at `localhost:8080`, create a task, push a branch, open and merge a PR on a real or mocked external repo. With Keep Alive ON: verify the desktop stays alive (lock icon still visible, can still interact with Zed). With Keep Alive OFF: verify desktop stops as before. Then toggle Keep Alive OFF on a Done task and verify the desktop stops.
- **Logs to check:** `"Task done but keep_alive is set — leaving desktop running"` should appear in API logs when the gate fires.

## Notes for Future Implementers

- The four merge-detection sites in `spec_task_orchestrator.go` were a recurring source of confusion when investigating this — they look duplicated but each handles a slightly different scenario (PR poll, branch fallback, external PR detection, no-PR fallback). The gate-in-`handleDone` approach is robust because it sits downstream of all four.
- `task.KeepAlive` is the canonical name (snake_cased to `keep_alive` in JSON). Don't confuse with any session-level keep-alive concept.
- The existing tooltip wording ("Keep Alive ON — won't auto-sleep") is technically narrow after this change — it now also prevents merge-triggered shutdown. Consider broadening to "Keep Alive ON — desktop won't be auto-stopped" if the user wants. Out of scope unless asked.
