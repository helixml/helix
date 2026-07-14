# WIP-queue block reason + gate formula fix

## Problem

A spec task could sit in `queued_spec_generation` indefinitely with no sandbox,
session, or agent — the orchestrator was deliberately holding it behind a WIP
limit, but the only record of *why* was a server log line. To the user the task
detail page just looked stuck.

Two problems, both fixed here:

1. **Invisible block reason** — nothing in the UI explained a queued task.
2. **Wrong WIP gate formula** — `planningCount + reviewCount >= reviewLimit`
   compared the sum of two columns against the review limit alone. With defaults
   `planningLimit=3, reviewLimit=2` the planning limit was dead (the review
   reservation always tripped first) and a full Review column permanently
   starved planning with zero user feedback.

## Implementation

### Backend
- Added transient `SpecTask.QueueReason` (`gorm:"-"`, `json:"queue_reason"`),
  recomputed each read — same pattern as `SandboxStatusMessage`. Never persisted.
- Extracted a single pure source of truth in `spec_task_orchestrator.go`:
  - `PlanningQueueReason(project, projectTasks, task) string` — dependency-not-ready,
    planning-full, or review-full → human-readable reason, else "".
  - `ImplementationQueueReason(projectTasks, task)` — dependency-only (the
    implementation-start transition has no WIP gate of its own).
  - `dependencyQueueReason` resolves the blocking task's name from `projectTasks`.
- `handleQueuedSpecGeneration` now calls `PlanningQueueReason` (behaviour
  identical: non-empty → stay queued + log).
- **Problem 2 fix**: review capacity is gated on `reviewCount >= reviewLimit`
  (the Review column's own limit), NOT the summed formula. Applied in both
  `handleQueuedSpecGeneration` and the backlog→queued gate in `handleBacklog` so
  the two sites agree and `planningLimit` regains effect.
- `server.populateQueueReasons` fills `QueueReason` for queued tasks in both
  `listTasks` (kanban) and `getTask` (detail page — it doesn't run the list
  enrichment, so it's called explicitly there). It loads the project task list
  `WithDependsOn: true` once, both as the WIP-count basis and to supply each
  task's `DependsOn`.

### Frontend
- Regenerated OpenAPI client (`./stack update_openapi`) → `TypesSpecTask.queue_reason`.
- `SpecTaskDetailContent.tsx`: info `<Alert>` ("Waiting to start" + reason) for
  queued tasks with a reason.
- `TaskCard.tsx`: queued card shows the reason inline (ellipsised) with a tooltip
  and a schedule icon instead of the misleading "Starting desktop…" spinner.
  Added `queue_reason` to the hand-maintained `SpecTaskWithExtras` interface.

## Tests
- `TestPlanningQueueReason` (table-driven): not-blocked, planning-full,
  review-full, dependency-blocked, and the Problem-2 regression (2 planning under
  limit 3 + empty review must NOT block — the old summed formula blocked here).
- Updated two existing orchestrator tests that encoded the old summed-reservation
  behaviour: `TestHandleBacklog_ReservesReviewCapacity` →
  `SkipsWhenReviewColumnFull` + new `ProgressesWithPlanningInFlightAndReviewEmpty`;
  `TestHandleQueuedSpecGeneration_ReservesFutureReviewSlot` →
  `RespectsPlanningLimit`.

## End-to-end verification (inner Helix, localhost:8080)
Registered `test@helix.ml`, created org `testorg` + project `testproj`. Set Review
WIP limit=1 via project metadata, seeded 2 `spec_review` tasks + 1
`queued_spec_generation` task.

- `GET /api/v1/spec-tasks?project_id=…` and `GET /api/v1/spec-tasks/{id}` both
  returned: `"Waiting for review capacity — 2 specs are awaiting review (limit 1).
  Approve or clear reviews in the Review column to start planning."`
- Detail page rendered the "Waiting to start" banner; kanban card rendered the
  reason inline with the Review column marked "FULL".
- Archived the 2 review tasks → `queue_reason` cleared to empty on the next read,
  and the orchestrator picked up the queued task within ~10s and attempted to
  start planning (gate no longer blocks). The task then failed with "user_id is
  required" because the minimal hand-seeded row lacked a full owner/app linkage —
  an artifact of the seed, not the feature.

Screenshots: `design/tasks/002256_complete-the-task-in-the/screenshots/` in the
helix-specs branch.
