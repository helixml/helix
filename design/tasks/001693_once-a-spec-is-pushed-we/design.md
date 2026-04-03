# Design: Spec Title as Task Name

## Name Sources by Stage

| Stage | Name Source |
|-------|------------|
| `backlog`, `spec_generation` | `GenerateTaskNameFromPrompt(prompt)` — existing behavior |
| `spec_review` and later | First non-empty line of `requirements.md`, stripped of leading `#` |

The transition happens in `processDesignDocsForBranch()` in `git_http_server.go`, which already reads `requirements.md` via `createDesignReviewForPush()`. This is the right place to extract the title and update the task name.

## Backend Change

In `git_http_server.go` → `processDesignDocsForBranch()`, after reading the design docs:

1. Read the first non-empty line from `requirements.md` content
2. Strip leading `#` characters and trim whitespace
3. If a title is found, update the task's `Name` field

This runs on **every push**, not just the initial `spec_review` transition. That way re-pushing an updated `requirements.md` always keeps the task name in sync.

Extract a helper:
```go
func specTitleFromRequirements(content string) string {
    for _, line := range strings.Split(content, "\n") {
        line = strings.TrimSpace(strings.TrimLeft(line, "#"))
        if line != "" {
            return line
        }
    }
    return ""
}
```

## Frontend Changes

### 1. Read-only prompt field

In `SpecTaskDetailContent.tsx`, gate the description/prompt `TextField` on task status:

```tsx
const isPromptEditable = ["backlog", "queued_spec_generation", "spec_generation"].includes(task?.status)
```

Render the description as plain text (or a disabled `TextField`) when `!isPromptEditable`.

### 2. Name display — kanban and breadcrumbs

The task name already flows through `task.name` to all the right places:

- **Kanban card**: `TaskCard.tsx` line 770 renders `task.name` — no change needed
- **Breadcrumb (detail page)**: `SpecTaskDetailPage.tsx` lines 127-130 uses `task?.name || "Task"` — no change needed
- **Breadcrumb (review page)**: `SpecTaskReviewPage.tsx` lines 92-96 uses `task?.name || "Task"` — no change needed

### 3. Hover tooltip — show original prompt

Currently the hover tooltip on the task name shows `task.description || task.name`. Once the name comes from the spec title, the useful thing to show on hover is the **original prompt**.

Update the tooltip value in these three places to use `task.original_prompt`:

| File | Lines | Current tooltip | Updated tooltip |
|------|-------|----------------|-----------------|
| `TaskCard.tsx` | 749-758 | `task.description \|\| task.name` | `task.original_prompt \|\| task.description \|\| task.name` |
| `SpecTaskDetailPage.tsx` | 127-130 | `task?.description \|\| task?.name` | `task?.original_prompt \|\| task?.description \|\| task?.name` |
| `SpecTaskReviewPage.tsx` | 92-96 | (no tooltip) | Add `tooltip: task?.original_prompt \|\| task?.description \|\| task?.name` |

The `SpecTaskReviewPage.tsx` breadcrumb entry for the task name currently has no tooltip at all — add one using the same pattern. The `Page.tsx` breadcrumb renderer already supports the optional `tooltip` field on `IPageBreadcrumb`.

## Decisions

- **Update on every push, not just first**: re-pushing a spec with a revised title is the intended workflow for renaming a task from `spec_review` onwards, so every push must sync the name.
- **First file = `requirements.md`**: always written first; its H1 is the natural task title.
- **No truncation for spec titles**: the 60-char limit on prompt-derived names is a UX affordance for unstructured text; spec titles are intentional.
- **Hover shows original prompt**: the name now comes from the spec, not the prompt — showing the original prompt on hover preserves discoverability without cluttering the UI.

## Notes for Implementors

- `createDesignReviewForPush()` already reads `requirements.md` content — reuse that content rather than reading the file twice.
- The task status types are in `/api/pkg/types/simple_spec_task.go` lines 279-309.
- `original_prompt` field exists on the `SpecTask` type — confirmed present in `SpecTaskKanbanBoard.tsx` line 203 and used as a fallback in `SpecTaskDetailContent.tsx`.
