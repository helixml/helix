# Design: Generate Snappy LLM Titles for Spec Tasks

## Current state (what's already in place)

- **Schema:** `SpecTask` already has the columns we need —
  `name`, `short_title` (size:100), `user_short_title` (size:100). See
  `api/pkg/types/simple_spec_task.go:114-122`.
- **Display chain:** the frontend already uses
  `user_short_title || short_title || name` in `TabsView.tsx` (lines 381,
  471, 808, 956, 1394, 1853, 1999, 2071). But other components — `TaskCard`,
  `TasksTable`, `CronTaskCard`, `EmptyTasksState`, kanban tooltips and the
  spec-review opener in `SpecTaskDetailContent` — read `task.name` directly
  and are missing the chain.
- **Initial name:** `GenerateTaskNameFromPrompt()` in
  `spec_driven_task_service.go:1691` turns the prompt into a single-line
  60-char string. This is the source of the "first line of prompt" complaint.
- **Late update:** when the planning agent pushes `requirements.md`,
  `git_http_server.go:1660-1671` parses the H1 via
  `SpecTitleFromRequirements` and overwrites `task.Name`. `task.ShortTitle`
  is never written in any code path.
- **LLM title pattern already exists:** `SummaryService` in
  `api/pkg/server/summary_service.go` already generates one-line summaries
  and session titles using the kodit enrichment model with rate limiting,
  timeouts, and graceful fallback. **We reuse it.**

## Approach (one paragraph)

Add a thin async title-generation step that runs after `CreateSpecTask` and
fills `task.ShortTitle` with an LLM-generated 60-char title from
`OriginalPrompt`. Reuse `SummaryService`'s plumbing (kodit enrichment
provider, rate limiter, cleanup helpers) rather than building a parallel
LLM-call stack. Update the handful of frontend components that still read
`task.name` directly so they use the same
`user_short_title || short_title || name` chain `TabsView` already uses.
Extend the existing `git_http_server.go` H1-parse so it also writes
`ShortTitle` (so the agent's chosen title wins once planning completes).
The schema is unchanged.

## Backend changes

### 1. New method on `SummaryService`

Add `GenerateSpecTaskTitleAsync(ctx, taskID, ownerID, prompt string)` to
`api/pkg/server/summary_service.go`:

```go
func (s *SummaryService) GenerateSpecTaskTitleAsync(
    ctx context.Context, taskID, ownerID, prompt string,
) {
    // Rate-limit check — reuse s.mu / s.pendingCount / s.maxConcurrent.
    // Fire goroutine with 30s timeout context.
    // 1. GetEffectiveSystemSettings; if KoditEnrichmentProvider/Model empty → return.
    // 2. providerManager.GetClient(...)
    // 3. CreateChatCompletion: system prompt below, user content = prompt,
    //    MaxTokens: 50, Temperature: 0.3.
    // 4. Trim quotes; truncate at word boundary ≤ 60 chars (mirror updateSessionTitle).
    // 5. store.UpdateSpecTaskShortTitle(ctx, taskID, title).
}
```

System prompt:

```
Generate a snappy task title (max 60 characters) for a software
engineering task. The title appears on a Kanban card and tab strip, so
it must be scannable at a glance.

Guidelines:
- Start with an imperative verb (Add, Fix, Refactor, Generate, Wire up, ...).
- Mention the concrete subject (component, file, behaviour).
- No quotes, no trailing punctuation, no "Task:" prefix.
- Do NOT echo the whole prompt — distil it.
```

Why these params: identical to `summary_service.go:140-166`, which has been
in production for session summaries. No new tuning needed.

### 2. New store method

Add to `api/pkg/store/store_spec_tasks.go`:

```go
func (s *PostgresStore) UpdateSpecTaskShortTitle(ctx context.Context, id, title string) error
```

Implementation: a `Model(&types.SpecTask{}).Where("id = ?", id).Update("short_title", title)`
gorm call. Add to the `Store` interface alongside the existing `UpdateSpecTask`.
This avoids a read-modify-write race with whatever is mutating the task row
concurrently (planning starts can fire within seconds of creation).

### 3. Fire it from `CreateSpecTask`

In `api/pkg/services/spec_driven_task_service.go` `CreateTask` flow, after
`s.store.CreateSpecTask(...)` succeeds (around line ~250 — verify when
implementing), kick the async title generation:

```go
if s.summaryService != nil {
    s.summaryService.GenerateSpecTaskTitleAsync(
        ctx, task.ID, task.UserID, task.OriginalPrompt,
    )
}
```

`SpecDrivenTaskService` doesn't currently hold a `SummaryService` reference.
Wire it in via the constructor (the API server already constructs both —
see how `SummaryService` is created in `api/pkg/server/server.go`; pass it
through `NewSpecDrivenTaskService`). Leave the field nilable so unit tests
that don't need title generation can still construct the service.

### 4. Also update `ShortTitle` from the H1

In `git_http_server.go:1660-1671`, alongside the existing `task.Name`
update, set `task.ShortTitle = specTitle` too. The H1 is already constrained
to ≤ 100 chars by the agent's prompt template (`spec_task_prompts.go:60-71`).

Rationale: once the agent has produced a curated title in `requirements.md`,
it's strictly better than the LLM-from-prompt draft. Letting it overwrite
keeps the data source consistent.

### 5. (No) migration

`short_title` and `user_short_title` columns already exist on the
`spec_tasks` table via the existing GORM AutoMigrate (`gorm:"size:100"` on
the struct fields). No new migration is needed.

## Frontend changes

Introduce a tiny helper and use it everywhere — no inline `||` chains
duplicated across files. Add to `frontend/src/components/tasks/taskTitle.ts`:

```ts
import { TypesSpecTask } from '../../api/api';
export const specTaskTitle = (t: Pick<TypesSpecTask,
  'user_short_title' | 'short_title' | 'name'>) =>
  t.user_short_title || t.short_title || t.name || 'Untitled task';
```

Then replace direct `task.name` reads in display contexts with
`specTaskTitle(task)`:

- `TaskCard.tsx:838` — the card title.
- `TasksTable.tsx:131` — the name cell.
- `CronTaskCard.tsx:99` — cron-task card title.
- `EmptyTasksState.tsx:168` — empty-state list item.
- `SpecTaskDetailContent.tsx:910` — `task.name || "Spec Review"` becomes
  `specTaskTitle(task) || "Spec Review"`.
- `TabsView.tsx` lines 381-384, 471-475, 808, 956, 1394-1397, 1853-1856,
  1999-2002, 2071 — collapse to a single `specTaskTitle()` call (cleanup,
  not behaviour change).

`TabsView.tsx:1396` (the "task.name in tooltip" path) is the one place that
should keep showing the **long** name as the tooltip subtitle — that's the
intent: short title on the tab, long name on hover. Don't change tooltip
content.

Editing flow (`handleTabRename`, `spec_driven_task_handlers.go:1077`) is
unchanged — it writes to `user_short_title` and the chain already handles
that.

## Concurrency & ordering

Two writers can update `short_title` for the same task:

1. The async LLM call right after creation (fast — ~1-3s).
2. The git-push handler when the agent pushes `requirements.md` (slow —
   minutes after planning starts).

If (2) finishes before (1), the LLM draft would overwrite the agent's H1
title — incorrect. Mitigations:

- The agent-driven update path (`git_http_server.go`) compares the parsed
  title to `task.Name` and only updates if different. Add the same guard for
  `ShortTitle`: only overwrite if the H1 differs from the current
  `ShortTitle`.
- The LLM-async path should only set `ShortTitle` when it's still empty —
  use a conditional update: `UPDATE spec_tasks SET short_title = $1 WHERE
  id = $2 AND (short_title = '' OR short_title IS NULL)`. This makes the
  late-arriving LLM call a no-op once the agent has already written a
  title. Implement this in `UpdateSpecTaskShortTitle`.

`user_short_title` is never touched by either path, so user overrides are
safe by construction.

## Test plan

- `summary_service_test.go`: add a test that
  `GenerateSpecTaskTitleAsync` calls the LLM and stores the truncated
  result. Mock the provider client (existing tests already do this for
  session-title tests — copy the pattern).
- `summary_service_test.go`: add a no-config test (empty
  `KoditEnrichmentProvider`) and confirm `store.UpdateSpecTaskShortTitle` is
  NOT called.
- `spec_driven_task_service_test.go`: existing `CreateTask` tests should not
  break — `summaryService` is nilable.
- `git_http_server_test.go` (or wherever the H1 parsing is tested today):
  assert `ShortTitle` is updated alongside `Name`.
- Manual E2E in inner Helix (see CLAUDE.md "Browser Testing Setup"):
  1. Register & onboard if needed.
  2. Create a task with a long prompt like the user's example.
  3. Confirm the Kanban card shows a snappy title within ~5 s of creation.
  4. Run planning; confirm the H1 from `requirements.md` replaces the LLM
     title.
  5. Double-click the tab to rename; confirm the user override sticks.

## Out of scope

- No new system-settings keys, no per-org model overrides.
- No backfill of `short_title` for existing tasks. The display chain
  gracefully falls back to `name`, so old tasks look exactly as they do
  today.
- No UI to manually trigger regeneration. (The user can already rename
  with `user_short_title`.)
- No streaming/inline UI updates beyond what the existing TanStack Query
  refetch loop already does — the Kanban polls `listSpecTasks` regularly
  and will pick up the new `short_title` automatically.
