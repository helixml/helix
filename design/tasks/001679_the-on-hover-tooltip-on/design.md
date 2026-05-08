# Design: Fix Notifications Panel Tooltip

## Root Cause

The `AttentionEvent` struct (Go: `api/pkg/types/attention_event.go`, TS: `useAttentionEvents.ts`) does not carry the task's prompt. When events are created in `attention_service.go`, only `SpecTaskName` is denormalized from the task — neither `Description` nor `OriginalPrompt` is.

The tooltip in `GlobalNotifications.tsx:236-246` therefore renders `event.spec_task_name` (the short task name), giving users no new information beyond what's already visible.

## Which Field Holds the Current Prompt?

Investigation of `SpecTask` (`api/pkg/types/simple_spec_task.go`):

| Field | Mutable | Persistent | Notes |
|-------|---------|-----------|-------|
| `OriginalPrompt` | No (never reassigned) | Yes | Immutable first request |
| `Description` | **Yes** | Yes | **Updated on every user edit** (`spec_driven_task_handlers.go:879-882`) |
| `LastPromptContent` | Yes | No (`gorm:"-"`) | Runtime session state, not in DB |

The frontend's existing pattern (`SpecTaskDetailContent.tsx:470`, `BacklogTableView.tsx:140`) already uses the chain `description || original_prompt`. We mirror that.

## Fix

### 1. Backend — Add prompt fields to `AttentionEvent`

**`api/pkg/types/attention_event.go`** — add denormalized fields (no migration; same pattern as `SpecTaskName`/`ProjectName`):
```go
SpecTaskDescription   string `json:"spec_task_description" gorm:"-"`
SpecTaskOriginalPrompt string `json:"spec_task_original_prompt" gorm:"-"`
```

**`api/pkg/services/attention_service.go`** — populate in `EmitEvent()` (alongside the existing `event.SpecTaskName = task.Name` assignment around line 86):
```go
event.SpecTaskDescription = task.Description
event.SpecTaskOriginalPrompt = task.OriginalPrompt
```

### 2. Frontend — Update tooltip content

**`frontend/src/hooks/useAttentionEvents.ts`** — extend interface:
```ts
spec_task_description?: string
spec_task_original_prompt?: string
```

**`frontend/src/components/system/GlobalNotifications.tsx:237-242`** — change tooltip `title` to use the fallback chain:
```tsx
title={
  <span style={{ whiteSpace: 'pre-wrap' }}>
    {event.spec_task_description
      || event.spec_task_original_prompt
      || event.spec_task_name
      || event.spec_task_id
      || ''}
    {'\n'}
    {groupedWith ? 'Spec ready & agent finished' : event.title}
  </span>
}
```

## Key Decisions

- **Use `Description` as primary, `OriginalPrompt` as fallback**: matches the frontend's existing convention (`SpecTaskDetailContent.tsx:470`). `Description` is the live, user-editable prompt; `OriginalPrompt` is a stable backup.
- **Carry both fields, not just one**: the frontend already does this fallback elsewhere; carrying both keeps the AttentionEvent self-sufficient and consistent.
- **`gorm:"-"` for new fields**: consistent with existing denormalized fields (`SpecTaskName`, `ProjectName`). No schema migration required.
- **Field naming `spec_task_description` / `spec_task_original_prompt`**: follows the existing `spec_task_name` / `spec_task_id` naming pattern on `AttentionEvent`. Avoids colliding with `AttentionEvent.Description` (the event's own description field).

## Files Changed

| File | Change |
|------|--------|
| `api/pkg/types/attention_event.go` | Add `SpecTaskDescription`, `SpecTaskOriginalPrompt` fields |
| `api/pkg/services/attention_service.go` | Populate both from `task` in `EmitEvent()` |
| `frontend/src/hooks/useAttentionEvents.ts` | Add corresponding fields to interface |
| `frontend/src/components/system/GlobalNotifications.tsx` | Update tooltip `title` with fallback chain |
