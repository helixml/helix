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
- [x] Regenerate OpenAPI types (`./stack update_openapi`) so `TypesSpecTask` includes `queue_reason`
- [x] Detail banner: render `<Alert severity="info">{task.queue_reason}</Alert>` in `SpecTaskDetailContent.tsx` when status is queued and `queue_reason` is set
- [x] Kanban card: show `queue_reason` compactly (tooltip + ellipsised caption) in `TaskCard.tsx` (also added `queue_reason` to the hand-maintained `SpecTaskWithExtras` interface)

## Tests
- [x] Table-driven Go unit tests for `PlanningQueueReason` (planning-full, review-full, dependency-blocked, not-blocked) — extend `spec_task_orchestrator_test.go`
- [x] Test that the corrected formula allows planning up to `planningLimit` with an empty Review column (regression case in table test + handler tests)
- [x] `go build ./...` passes
- [x] `cd frontend && yarn build` passes (tsc --noEmit clean; full vite build succeeds — note: repo `dist/` is a root-owned prod bind-mount, so built to temp outDir)

## End-to-End Verification (inner Helix, localhost:8080)
- [x] Register/onboard (test@helix.ml), create project `testproj`; set Review WIP limit=1 (Board Settings JSON); seed 2 `spec_review` + 1 `queued_spec_generation` task
- [x] Confirmed `queue_reason` returned by BOTH `GET /spec-tasks` (list) and `GET /spec-tasks/{id}` (detail): "Waiting for review capacity — 2 specs are awaiting review (limit 1)..."
- [x] Confirmed detail page banner ("Waiting to start" + reason) and kanban card (inline reason + Review column "FULL") — screenshots 01/02
- [x] Drained Review column → `queue_reason` cleared to empty on next read; orchestrator picked up the task within ~10s and attempted planning (gate no longer blocks). The task's subsequent failure was "user_id is required" from the minimal hand-seeded row (not the feature).
- [x] Recorded observations in `design/2026-07-14-wip-queue-reason.md` (helix repo)

## Deliverable
- [x] Write short design note `design/2026-07-14-wip-queue-reason.md` in the helix repo
- [x] Push feature branch `feature/002256-surface-wip-queue-block` covering Problem 1 + Problem 2 with tests (platform opens the PR against main)
