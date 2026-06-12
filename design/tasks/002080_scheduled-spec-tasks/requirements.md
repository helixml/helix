# Requirements: Auto-start cron-scheduled spec tasks

## Background

When a user attaches a cron trigger to a Helix app with `action="spec_task"`, the trigger fires on schedule and calls `SpecTaskCreator.CreateTaskFromPrompt` to create a new spec task. Today this request is built **without** setting `AutoStart=true`, so the new task lands in `backlog` status.

Whether the task ever starts then depends on the project's `AutoStartBacklogTasks` setting. If that setting is off (it commonly is), the scheduled task sits in backlog forever and never runs — defeating the entire point of scheduling it. The user explicitly told the system "run this at 9am every weekday"; we should not also require them to flip a separate project-wide auto-start toggle.

Location of the bug: `api/pkg/trigger/cron/trigger_cron.go:734-738` — `CreateTaskRequest` omits `AutoStart`.

## User stories

**As a developer who has set up a cron trigger on a Helix app to create a spec task daily,**
I want the scheduled task to start running on its own when the cron fires,
so that the schedule actually executes the work without requiring me to manually click "Start Planning" each time, and without requiring the project's auto-start setting to be enabled.

**As a developer who has set up a cron trigger but my project's `AutoStartBacklogTasks` is off (because I want manual control over ad-hoc backlog work),**
I still want my cron-scheduled tasks to auto-start,
because scheduling them was the explicit signal that they should run automatically. Manual backlog and scheduled backlog are different intents and should behave differently.

## Acceptance criteria

1. When a `cron_trigger` with `action="spec_task"` fires, the resulting spec task is created with `AutoStart=true`.
2. As a result, the task skips `backlog` status and is created directly in `queued_spec_generation` (or `queued_implementation` if the trigger ever supports JustDoItMode — currently it does not).
3. Behavior is independent of `project.AutoStartBacklogTasks`. A cron-scheduled task auto-starts even when the project's auto-start toggle is off.
4. The orchestrator (`spec_task_orchestrator.go`) picks up the queued task on its next tick and progresses it through the normal lifecycle. WIP limits still apply (a queued task waits if the planning column is full — this is correct behavior, no change needed).
5. The existing trigger execution record continues to record success/failure of task creation as today; the change does not affect notification or callback behavior.
6. Manually-created tasks (via UI "Create Task" with the user not ticking auto-start) continue to land in `backlog` and continue to honor `AutoStartBacklogTasks` — i.e. the existing manual flow is not regressed.
7. Existing cron triggers do not need any migration or reconfiguration; the new behavior applies on the next fire.

## Out of scope

- Adding any new field to `CronTrigger` to make auto-start configurable per-trigger. The user's stated requirement is unconditional ("should auto-start"); a per-trigger override is YAGNI for now and easy to add later if asked.
- Adding a `ScheduledFor *time.Time` field to `SpecTask` for one-shot deferred scheduling. The current scheduling mechanism is cron-based (recurring), not one-shot defer-until-time. Deferred one-shots are a separate feature.
- Changing the `JustDoItMode` defaulting for cron-created tasks. Currently it is always `false` (spec generation runs); leave it that way.
- UI changes. There is no user-facing UI surface for this — it is a backend-behavior fix.
