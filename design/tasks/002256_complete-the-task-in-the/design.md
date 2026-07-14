# Design: Surface WIP-Queue Block Reason and Fix the WIP Gate Formula

## Overview

Add a transient `QueueReason` field to `SpecTask`, computed live at read time by
a single reusable pure function, populated in both list and single-task GET
handlers, and rendered as a banner (detail page) and inline/tooltip (kanban
card). At the same time, fix the WIP gate formula so planning and review limits
are each independently meaningful.

Follows the existing precedent of `SandboxStatusMessage`
(`types/simple_spec_task.go:177`, populated in `listTasks` at
`spec_driven_task_handlers.go:349`) — a `gorm:"-"` field recomputed each read.

## Current State (findings)

- The gate lives in `api/pkg/services/spec_task_orchestrator.go`:
  - `handleQueuedSpecGeneration` (~L533–605): queued → spec_generation transition.
    Gate at L576: `if planningCount >= planningLimit || planningCount+reviewCount >= reviewLimit`.
  - `handleBacklog` (~L396–430): backlog → queued transition. **Same buggy
    formula** (`planningCount+reviewCount >= reviewLimit`), introduced in the
    same commit `bef171d44` ("limits", authored 2026-07-14).
- Helpers already exist: `getProjectWIPLimits(project) (planning, review, impl int)`
  (defaults 3/2/5, overridable via `project.Metadata.BoardSettings.WIPLimits`) and
  `countTasksByStatus(tasks, statuses...)`.
- Dependency gate: `areBacklogDependenciesReady(task.DependsOn) (bool, blockingID)`
  at L472 — returns the blocking dependency's **ID** (not name).
- `getTask` (`spec_driven_task_handlers.go:144`) does **not** run the `listTasks`
  enrichment — must add reason population there explicitly.
- Frontend: the queued detail UI is rendered by **`SpecTaskDetailContent.tsx`**
  (shared by page + tabs view), where `isQueuedForPlanning` already exists
  (L529). `SpecTaskDetailPage.tsx` just wraps it — the banner belongs in
  `SpecTaskDetailContent.tsx`. Kanban status→column mapping is in
  `SpecTaskKanbanBoard.tsx` `mapStatusToPhase` (L121); card rendering is
  `TaskCard.tsx`.

## Backend Design

### 1. Transient field
`api/pkg/types/simple_spec_task.go`, next to `SandboxStatusMessage`:
```go
QueueReason string `json:"queue_reason,omitempty" gorm:"-"` // Why a queued task hasn't started; recomputed each read.
```

### 2. Pure reason function (single source of truth)
New function in `spec_task_orchestrator.go`:
```go
// planningQueueReason returns "" if the task could start planning now,
// else a human-readable reason. Pure: no store access, no side effects.
func planningQueueReason(project *types.Project, projectTasks []*types.SpecTask, task *types.SpecTask) string
```
Logic (mirrors the gate, using the corrected formula):
1. Dependency: `if ready, blockingID := areBacklogDependenciesReady(task.DependsOn); !ready` →
   resolve the blocking task's name from `projectTasks` (fall back to ID) →
   `Waiting for dependency "<name>" to finish.`
2. `planningLimit, reviewLimit, _ := getProjectWIPLimits(project)`
   - `planningCount := countTasksByStatus(projectTasks, TaskStatusSpecGeneration)`
   - `reviewCount := countTasksByStatus(projectTasks, TaskStatusSpecReview, TaskStatusSpecRevision, TaskStatusSpecApproved)`
3. If `planningCount >= planningLimit` →
   `Waiting for planning capacity — N tasks are already generating specs (limit M).`
4. If `reviewCount >= reviewLimit` →
   `Waiting for review capacity — N specs are awaiting review (limit M). Approve or clear reviews in the Review column to start planning.`
5. Else return `""`.

All numbers derived from the same counts the gate uses — nothing hardcoded.

A sibling `implementationQueueReason(...)` (or reuse via a small shared helper)
covers `queued_implementation`; its only block cause is dependency-not-ready
(no separate implementation WIP gate exists on that transition), so it returns
the dependency reason or "".

### 3. Use it in the orchestrator (identical behaviour)
`handleQueuedSpecGeneration` replaces its inline dependency check + gate with a
call to `planningQueueReason(project, projectTasks, latestTask)`. If non-empty:
log as today (keep the existing structured log fields for observability) and
`return nil` (leave queued). If empty: proceed to reserve the slot.

**Problem 2 fix**: because the reason function uses `reviewCount >= reviewLimit`
(not the sum), the gate is corrected by construction. Apply the same corrected
condition to the backlog→queued gate in `handleBacklog` so both sites agree and
`planningLimit` regains effect.

Behaviour note: the dependency check currently runs *before* taking the project
lock. Keep that ordering (cheap early-out) — the lock-held recompute for the WIP
counts stays authoritative for the reservation.

### 4. Populate on read
- `listTasks`: after loading `tasks`, load the project once and, for each task
  whose status is `queued_spec_generation` or `queued_implementation`, set
  `task.QueueReason = planningQueueReason(project, tasks, task)` (or the
  implementation sibling). Reuse the already-fetched `tasks` slice as
  `projectTasks` (all project tasks are in scope of the list query).
- `getTask`: `getTask` fetches a single task, so it must additionally load the
  project and the project's task list (`ListSpecTasks{ProjectID}`) to compute the
  reason, then set `task.QueueReason`. Only do this work when the task is in a
  queued state (avoid the extra query otherwise).

The reason functions are exported/available to the server package (same module).

## Frontend Design

### 1. Regenerate API types
Add the swagger field via the Go struct tag, then `./stack update_openapi` so
`TypesSpecTask` gains `queue_reason?: string`. Consume via the generated client
only (no raw fetch).

### 2. Detail banner — `SpecTaskDetailContent.tsx`
Near the queued rendering (where `isQueuedForPlanning` is used, ~L529), when
`task.status` is a queued state and `task.queue_reason` is non-empty, render:
```tsx
<Alert severity="info" sx={{ mb: 3 }}>{task.queue_reason}</Alert>
```
placed above the "Starting Desktop" area so the user sees the explanation
instead of a perpetual spinner. Keep existing behaviour when `queue_reason` is
empty.

### 3. Kanban card — `TaskCard.tsx`
For a queued card with `queue_reason`, show it compactly: a short inline caption
under the title, or wrap the queued status chip in a `<Tooltip title={queue_reason}>`.
Keep it compact (brief mentions "compact"). Prefer a tooltip on the queued
indicator to avoid growing card height.

## Key Decisions

- **One pure function, no duplicated counting** — orchestrator and handlers share
  `planningQueueReason`; the read path can never disagree with the gate.
- **Non-persisted, recompute each read** — matches `SandboxStatusMessage`; the
  reason must change as the queue drains (US-3). Do NOT add a DB column / migration.
- **Fix both gate sites** — `handleQueuedSpecGeneration` and `handleBacklog` share
  the bug; fixing only one leaves an inconsistent board. The reason function
  encodes the corrected formula once.
- **Banner in `SpecTaskDetailContent`, not `SpecTaskDetailPage`** — the page is a
  thin wrapper; the shared content component is where queued state actually
  renders and is reused by the tabs/workspace view too.
- **Drop the summed reservation** — `planningCount+reviewCount >= reviewLimit` was
  the bug; gate review capacity on `reviewCount >= reviewLimit` alone so each
  column's limit is independent and a full Review column can't silently starve
  planning (and now it's visible anyway via Problem 1).

## Implementation Notes (as-built)

- **`swag` not on PATH** — `./stack update_openapi` needs `swag`, installed to
  `$(go env GOPATH)/bin` but not on PATH. Prefix with
  `export PATH="$PATH:$(go env GOPATH)/bin"` before running.
- **`yarn build` and the root-owned `dist/`** — the repo `frontend/dist` is a
  root-owned prod bind-mount, so `vite build` fails at the copy-to-dist step
  (`EACCES mkdir dist/external-libs`) even though compilation succeeds. Validate
  with `npx tsc --noEmit` (clean) and `npx vite build --outDir /tmp/fe-dist` (green).
  Do NOT `rm -rf dist`. Dev UI runs via Vite HMR (port 8081), which doesn't need dist.
- **Air did not hot-reload on file save** in this sandbox (inotify across the bind
  mount didn't fire). `docker compose -f docker-compose.dev.yaml restart api`
  forces Air to rebuild from the mounted source — needed to pick up Go changes.
- **`SpecTaskWithExtras` is hand-maintained** (not generated) — new DTO fields
  consumed by `TaskCard.tsx` must be added to that interface too.
- **Kanban board spreads `...task`** from the list API into `BoardTask`, so
  `queue_reason` flows through automatically — no manual mapping needed.
- **Existing tests encoded the bug**: `TestHandleBacklog_ReservesReviewCapacity`
  and `TestHandleQueuedSpecGeneration_ReservesFutureReviewSlot` asserted the old
  summed-reservation behaviour; both were rewritten for the independent-limit
  semantics.

## Testing Strategy

- **Go unit tests** (extend `spec_task_orchestrator_test.go`): table-driven over
  `planningQueueReason` — planning-full, review-full, dependency-blocked,
  not-blocked; assert reason string and empty-when-clear. Verify the corrected
  formula lets planning run up to `planningLimit` with an empty Review column.
- **`go build ./...`** and **`cd frontend && yarn build`** before finishing.
- **End-to-end in the inner Helix (`localhost:8080`)** — mandatory:
  1. Register/onboard, create a project.
  2. Lower Review WIP limit via Board Settings, pile specs into Review, create a
     new task, click Start → confirm it sits in `queued_spec_generation` and both
     the detail page banner and the kanban card show the specific reason.
  3. Drain the Review column (approve/archive) → confirm the reason clears and the
     task starts within ~10s (tests the seam, not just the static message).
  Record exactly what was run and observed in the design note.
