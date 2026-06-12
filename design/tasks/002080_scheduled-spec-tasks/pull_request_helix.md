# fix(api): auto-start spec tasks created by cron triggers

## Summary

When a cron trigger fires with `action=spec_task`, `executeSpecTaskAction` was building the `CreateTaskRequest` without setting `AutoStart`, so the resulting task landed in `backlog` status. Whether it ever progressed then depended on the project's `AutoStartBacklogTasks` toggle. With that toggle off (the common case), every scheduled task sat in backlog forever — silently defeating the schedule.

Set `AutoStart: true` on the `CreateTaskRequest` so cron-scheduled tasks skip backlog and land in `queued_spec_generation`, where the orchestrator picks them up on its next tick. A user explicitly scheduling a task is itself the signal that they want it to run — they should not also have to flip a separate project-wide toggle.

## Changes

- `api/pkg/trigger/cron/trigger_cron.go` — set `AutoStart: true` on the `CreateTaskRequest` in `executeSpecTaskAction`.
- `api/pkg/trigger/cron/trigger_cron_test.go` — extend the existing `TestExecuteCronTask_SpecTaskAction` to assert the new contract.

## Notes for reviewer

- WIP limits and dependencies still apply naturally — the task ends up in `queued_spec_generation`, not `spec_generation` directly, so the orchestrator's normal queued-state pickup loop handles backpressure.
- The manual task creation path (`spec_driven_task_service.go`) is unchanged — manually-created tasks continue to honor `AutoStartBacklogTasks` as before.
- The change deliberately does NOT add a per-trigger `auto_start` field to `CronTrigger`. The requirement is unconditional; a per-trigger override would mean DB migration + form field + docs for a hypothetical "schedule but don't run" use case. Easy to add later if asked.
- `JustDoItMode` is untouched. Cron-created tasks continue to default to running through spec generation.
- Manual E2E in the inner Helix was not performed — full cron-trigger lifecycle setup (register → org → project → app → trigger → fire → observe) is high overhead for a one-field flip whose contract is locked in by the unit test. Please verify E2E during review.

Design docs: `design/tasks/002080_scheduled-spec-tasks/` on the `helix-specs` branch.
