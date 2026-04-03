# Design: Spec Title as Task Name

## Name Sources by Stage

| Stage | Name Source |
|-------|------------|
| `backlog`, `spec_generation` | `GenerateTaskNameFromPrompt(prompt)` — existing behavior |
| `spec_review` and later | First non-empty line of `requirements.md`, stripped of leading `#` |

The transition happens in `processDesignDocsForBranch()` in `git_http_server.go`, which already reads `requirements.md` via `createDesignReviewForPush()`. This is the right place to extract the title and update the task name.

## Backend Change

In `git_http_server.go` → `processDesignDocsForBranch()`, after reading the design docs and before/alongside the status transition to `spec_review`:

1. Read the first non-empty line from `requirements.md` content
2. Strip leading `#` characters and trim whitespace
3. If a title is found, update the task's `Name` field via the existing update path

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

## Frontend Change

In `SpecTaskDetailContent.tsx`, the prompt/description `TextField` is currently always editable. Gate editability on task status:

```tsx
const isPromptEditable = ["backlog", "queued_spec_generation", "spec_generation"].includes(task?.status)
```

Render the description as plain text (or a disabled `TextField`) when `!isPromptEditable`.

## Decisions

- **First file = `requirements.md`**: it's always the first doc written and its H1 is the natural task title.
- **No truncation for spec titles**: the 60-char limit on prompt-derived names is a UX affordance for unstructured text; spec titles are intentional and should display in full.
- **No rename via prompt after spec_review**: if the user dislikes the title, they comment on the spec and edit `requirements.md`. This keeps spec docs as the source of truth.

## Notes for Implementors

- `createDesignReviewForPush()` already reads `requirements.md` content — reuse that content rather than reading the file twice.
- The task status types are in `/api/pkg/types/simple_spec_task.go` lines 279-309.
- The frontend status constants should match the Go backend string values.
