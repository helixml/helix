# Implementation Tasks: Surface WIP-Queue Block Reason and Fix the WIP Gate Formula

## Backend
- [x] Add transient `QueueReason string \`json:"queue_reason,omitempty" gorm:"-"\`` to `SpecTask` in `api/pkg/types/simple_spec_task.go` (next to `SandboxStatusMessage`)
- [x] Add pure `PlanningQueueReason(project, projectTasks, task) string` in `spec_task_orchestrator.go` covering dependency-not-ready, planning-full, review-full (corrected formula) → returns "" when startable
- [x] Resolve the blocking dependency's name (from `projectTasks`, fall back to ID) for the dependency reason string
- [x] Add sibling `ImplementationQueueReason(...)` (dependency-only) for `queued_implementation`
- [x] Refactor `handleQueuedSpecGeneration` to use `PlanningQueueReason` (identical behaviour; keep the existing structured log fields when leaving queued)
- [x] Fix Problem 2 formula: gate review capacity on `reviewCount >= reviewLimit` (not `planningCount+reviewCount >= reviewLimit`) in both `handleQueuedSpecGeneration` and the backlog→queued gate in `handleBacklog`
- [x] Populate `QueueReason` for queued-state tasks in `listTasks` (add `populateQueueReasons` helper; load project + tasks-with-deps once)
- [x] Populate `QueueReason` for queued-state tasks in `getTask` (reuse `populateQueueReasons` with a single-task slice)

## Frontend
- [ ] Regenerate OpenAPI types (`./stack update_openapi`) so `TypesSpecTask` includes `queue_reason`
- [ ] Detail banner: render `<Alert severity="info">{task.queue_reason}</Alert>` in `SpecTaskDetailContent.tsx` when status is queued and `queue_reason` is set
- [ ] Kanban card: show `queue_reason` compactly (tooltip on queued indicator, or short inline caption) in `TaskCard.tsx`

## Tests
- [ ] Table-driven Go unit tests for `planningQueueReason` (planning-full, review-full, dependency-blocked, not-blocked) — extend `spec_task_orchestrator_test.go`
- [ ] Test that the corrected formula allows planning up to `planningLimit` with an empty Review column
- [ ] `go build ./...` passes
- [ ] `cd frontend && yarn build` passes

## End-to-End Verification (inner Helix, localhost:8080)
- [ ] Register/onboard, create a project; lower Review WIP limit; pile specs into Review; create a task and Start it
- [ ] Confirm task sits in `queued_spec_generation` and BOTH detail page banner and kanban card show the specific reason
- [ ] Drain the Review column; confirm reason clears and the task starts within ~10s
- [ ] Record exact commands + observations in `design/YYYY-MM-DD-*.md`

## Deliverable
- [ ] Write short design note under `design/YYYY-MM-DD-wip-queue-reason.md` in the helix repo
- [ ] Open one PR against `main` (https://github.com/helixml/helix) covering Problem 1 + Problem 2 with tests; verify CI green
