# Surface WIP-queue block reason and fix the WIP gate formula

## Summary
A spec task could sit in `queued_spec_generation` indefinitely with no sandbox,
session, or agent — the orchestrator was deliberately holding it behind a WIP
limit, but the reason was only in a server log the user never sees, so the task
looked dead. This PR (1) surfaces a live, human-readable block reason in the UI,
and (2) fixes the WIP gate formula so the planning and review limits are each
meaningful.

## Problem 1 — invisible block reason
- New transient `SpecTask.QueueReason` (`gorm:"-"`, recomputed each read, never
  persisted — same pattern as `SandboxStatusMessage`).
- Single pure source of truth `PlanningQueueReason(project, projectTasks, task)`
  (+ `ImplementationQueueReason`) shared by the orchestrator gate and the read
  handlers, covering dependency-not-ready, planning-full and review-full.
- Populated for queued tasks in both `listTasks` (kanban) and `getTask` (detail
  page) via `populateQueueReasons`.
- Frontend: info banner on the task detail page and an inline reason + tooltip on
  the queued kanban card (replacing the misleading "Starting desktop…" spinner).

## Problem 2 — wrong gate formula
- `planningCount + reviewCount >= reviewLimit` compared the sum of two columns
  against the review limit alone, making `planningLimit` dead and letting a full
  Review column permanently starve planning.
- Now gated independently: planning on `planningCount >= planningLimit`, review on
  `reviewCount >= reviewLimit`. Fixed in both `handleQueuedSpecGeneration` and the
  backlog→queued gate in `handleBacklog`.

## Changes
- `api/pkg/types/simple_spec_task.go`: add `QueueReason` transient field.
- `api/pkg/services/spec_task_orchestrator.go`: extract `PlanningQueueReason` /
  `ImplementationQueueReason` / `dependencyQueueReason`; use in the gate; fix the
  formula in both gate sites.
- `api/pkg/server/spec_driven_task_handlers.go`: `populateQueueReasons` in
  `listTasks` and `getTask`.
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx`: queued info banner.
- `frontend/src/components/tasks/TaskCard.tsx`: queued card reason + tooltip;
  add `queue_reason` to `SpecTaskWithExtras`.
- Regenerated OpenAPI client (`queue_reason`).
- Tests: `TestPlanningQueueReason` (table-driven) + reworked the two orchestrator
  tests that encoded the old summed-reservation behaviour.
- `design/2026-07-14-wip-queue-reason.md`.

## Testing
- `go build ./...` and `go test ./pkg/services/ -run 'TestSpecTaskOrchestratorTestSuite|TestPlanningQueueReason'` pass.
- Frontend `tsc --noEmit` clean; full `vite build` green (built to temp outDir —
  repo `dist/` is a root-owned prod bind-mount).
- End-to-end in the inner Helix: seeded a Review-full project; confirmed the
  reason appears on the detail banner and kanban card via both API endpoints, then
  drained Review and confirmed the reason clears and the task is picked up within
  ~10s. See the design note for the exact commands/observations.

## Screenshots
![Detail page banner](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002256_complete-the-task-in-the/screenshots/01-detail-banner.png)
![Kanban card + Review FULL](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002256_complete-the-task-in-the/screenshots/02-kanban-card.png)
