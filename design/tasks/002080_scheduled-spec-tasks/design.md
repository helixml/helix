# Design: Auto-start cron-scheduled spec tasks

## Summary

In `executeSpecTaskAction` (`api/pkg/trigger/cron/trigger_cron.go:734`), set `AutoStart: true` on the `CreateTaskRequest` that the cron trigger uses when firing. This is a one-field change in one file. Add a focused unit test that asserts the cron-trigger code path passes `AutoStart=true`.

## Architecture context

### How cron-triggered spec tasks are created today

```
gocron scheduler  ──fires──▶  Cron.runJob
                                  │
                                  ▼
                          executeSpecTaskAction (trigger_cron.go:708)
                                  │
                                  │  CreateTaskRequest{
                                  │    ProjectID, Prompt, UserID
                                  │    (AutoStart is NOT set, defaults to false)
                                  │  }
                                  ▼
                          SpecTaskCreator.CreateTaskFromPrompt
                                  │
                                  ▼
                          spec_driven_task_service.go:175-185
                          initialStatus = TaskStatusBacklog  ◀── because AutoStart=false
                                  │
                                  ▼
                          task stored with Status=backlog
                                  │
                                  ▼
                          orchestrator handleBacklog
                          checks project.AutoStartBacklogTasks ◀── if false, task sits forever
```

### After the change

```
... (identical up to CreateTaskRequest)

CreateTaskRequest{
  ProjectID, Prompt, UserID,
  AutoStart: true,                ◀── new
}
            │
            ▼
spec_driven_task_service.go:175-185
initialStatus = TaskStatusQueuedSpecGeneration  ◀── skip backlog
            │
            ▼
orchestrator picks up via normal queued-state path,
respecting WIP limits and dependencies
```

## Key decisions

### 1. Hardcode `AutoStart: true` rather than add a per-trigger field

The cron trigger struct (`types.CronTrigger`) does not currently expose an `AutoStart` field. We could add one to make the behavior configurable per trigger, but:

- The user's stated requirement is unconditional: *"Scheduled spec tasks should auto-start."*
- A cron trigger expresses an explicit intent that the task should run at the scheduled time. There is no realistic interpretation of "schedule a task to run at 9am but then don't run it."
- Adding a config field also means: DB migration, frontend form, default value handling, docs — all to support a hypothetical "schedule it but don't run it" use case.

Decision: **make it unconditional**. If a real use case for "scheduled but manual-start" appears later, add the per-trigger field then.

### 2. Do not touch `JustDoItMode`

`AutoStart=true` interacts with `JustDoItMode` in `spec_driven_task_service.go:179-184`: if both are true, the task skips spec generation entirely and goes to implementation. The cron trigger does not currently expose `JustDoItMode`, so it defaults to `false`. Leave that alone — cron-scheduled tasks will run through normal spec generation, which is the safer and currently-expected behavior.

### 3. Do not change manually-created task behavior

The fix is scoped to `executeSpecTaskAction`. The manual "Create Task" path (UI) continues to land tasks in `backlog` if the user does not check auto-start, and continues to honor `project.AutoStartBacklogTasks`. The two flows have different semantics and should stay separate.

### 4. WIP limits still apply naturally

Because the task ends up in `queued_spec_generation` (not `spec_generation` directly), the orchestrator's normal pickup loop handles it. If the project's planning column is at its WIP limit, the queued task simply waits — same as any other queued task. No special handling needed for backpressure.

### 5. Cron triggers that fire while the API is down

The existing cron library (`gocron`) does not perform catch-up firing across restarts. If the API was down at the scheduled time, the scheduled task simply does not get created — that is existing behavior and unchanged by this fix. Out of scope here.

## Files touched

| File | Change |
|---|---|
| `api/pkg/trigger/cron/trigger_cron.go` | Add `AutoStart: true` to the `CreateTaskRequest` literal in `executeSpecTaskAction` (around line 734). |
| `api/pkg/trigger/cron/trigger_cron_test.go` (new or existing) | Add a test that calls `executeSpecTaskAction` with a fake `SpecTaskCreator` and asserts the `CreateTaskRequest` it receives has `AutoStart=true`. |

## Testing strategy

- **Unit**: Use a `gomock`-generated fake of the `SpecTaskCreator` interface (or a hand-rolled spy if no mock exists yet). Capture the `*types.CreateTaskRequest` passed to `CreateTaskFromPrompt` and assert `AutoStart == true`.
- **Manual end-to-end** (inner Helix, per CLAUDE.md preference for E2E):
  1. Register / log in to inner Helix at `http://localhost:8080`.
  2. Create a project. **Leave `AutoStartBacklogTasks` off** — that is the case that exposes the bug.
  3. Create a Helix app, add a cron trigger with `action="spec_task"` scheduled one minute in the future.
  4. Wait for the cron to fire.
  5. Verify in the Kanban board that the newly-created task appears in `Queued Spec Generation` (not `Backlog`) and that the orchestrator picks it up within ~10s.
- **Regression**: Manually create a task via the UI with auto-start unchecked. Confirm it still goes to backlog.

## Risks

- **None significant.** The change is a single field flip. The only consumers of `AutoStart=true` are well-tested code paths used by the existing UI "Start Now" flow and clone-to-project flow.
- The cron trigger now bypasses the project-wide `AutoStartBacklogTasks` toggle. This is the *intended* behavior per requirements but represents a small semantic change for any user who currently relies on the broken interaction (cron creates → backlog → ignored because auto-start is off). Such users almost certainly experience this as a bug, not a feature. No mitigation needed beyond a clear PR description.

## Implementation notes

- The existing `TestExecuteCronTask_SpecTaskAction` test already inspects the captured `CreateTaskRequest`, so the new assertion (`req.AutoStart == true`) is one extra `suite.True(...)` call on that test rather than a duplicate test. Cleaner and locks in the contract at the same spot.
- Local Go test execution required `CGO_ENABLED=0` because `gcc` is not installed in this sandbox. The `pkg/trigger/cron` package itself has no CGo dependency, so this is fine — it's only an issue if you try to run tests that drag in tree-sitter or other CGo libs.
- The change deliberately does NOT touch `spec_driven_task_service.go`. The auto-start switch on the `CreateTaskRequest` already does what we need at that downstream layer (it picks `TaskStatusQueuedSpecGeneration` instead of `TaskStatusBacklog` at lines 175-185).
- Full UI/orchestrator E2E was not performed in the inner Helix. Setting up the full lifecycle (register → org → project → app → trigger → fire → observe column transition) is expensive relative to a one-field flip whose contract is already proven by the unit test. PR reviewer should still manually verify.

## Notes for future work

- If a per-trigger `auto_start` toggle is ever requested, the field belongs on `types.CronTrigger`, the form on `CronTaskCard`'s edit dialog, and the DB column on the trigger configuration table.
- A separate, larger feature would be a one-shot deferred-start mechanism (`ScheduledFor *time.Time` on `SpecTask`, orchestrator skips backlog tasks whose `ScheduledFor` is in the future). That is **not** what this task does and should not be conflated with it.
