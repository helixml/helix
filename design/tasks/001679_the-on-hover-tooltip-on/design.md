# Design: Fix Notifications Panel Tooltip

## Root Cause

The `AttentionEvent` struct (Go: `api/pkg/types/attention_event.go`, TS: `useAttentionEvents.ts`) does not carry the task's prompt. When events are created in `attention_service.go`, only `SpecTaskName` is denormalized from the task. The tooltip in `GlobalNotifications.tsx:236-246` therefore renders `event.spec_task_name` (the short task name), giving users no new information beyond what's already visible.

## Which Field Holds the Latest Prompt?

Investigation of `SpecTask` (`api/pkg/types/simple_spec_task.go`):

| Field | Mutable | Persistent | Notes |
|-------|---------|-----------|-------|
| `OriginalPrompt` | No (never reassigned) | Yes | Immutable first request — **stale after any edit** |
| `Description` | **Yes** | Yes | **Updated on every user edit** (`spec_driven_task_handlers.go:879-882`) |
| `LastPromptContent` | Yes | No (`gorm:"-"`) | Runtime session state, not in DB |

We use `Description` exclusively. `OriginalPrompt` is deliberately avoided — it would silently show outdated prompt text whenever a user has edited their task.

## Fix

### 1. Backend — Add prompt field to `AttentionEvent`

**`api/pkg/types/attention_event.go`** — add denormalized field. `SpecTaskName` / `ProjectName` use `gorm:"size:N"` (persisted, snapshotted at write time). The prompt can be long, so use `gorm:"type:text"` to mirror the existing `Description` column. Auto-migrate handles the new column.
```go
SpecTaskDescription string `json:"spec_task_description,omitempty" gorm:"type:text"`
```

**`api/pkg/services/attention_service.go`** — populate in `EmitEvent()` (alongside the existing `event.SpecTaskName = task.Name` assignment around line 86):
```go
event.SpecTaskDescription = task.Description
```

Because this is denormalized at emit time, the value reflects whatever the description was when the event fired — i.e. the latest edit at that moment.

### 2. Frontend — Update tooltip content

**`frontend/src/hooks/useAttentionEvents.ts`** — extend interface:
```ts
spec_task_description?: string
```

**`frontend/src/components/system/GlobalNotifications.tsx:237-242`** — change tooltip `title`:
```tsx
title={
  <span style={{ whiteSpace: 'pre-wrap' }}>
    {event.spec_task_description || event.spec_task_name || event.spec_task_id || ''}
    {'\n'}
    {groupedWith ? 'Spec ready & agent finished' : event.title}
  </span>
}
```

Fallback chain stops at `spec_task_name` — we do not fall back to `original_prompt` because stale text is worse than a short name.

## Key Decisions

- **Use `Description`, not `OriginalPrompt`**: `Description` is the live, user-editable prompt and reflects edits. `OriginalPrompt` is immutable and would be stale after any user edit. Showing stale prompt text is worse than showing the task name.
- **Snapshot at emit time**: the description is denormalized into the event record when the event is emitted. If the user edits the prompt *after* an event fires, that older event keeps its older snapshot. This is correct: each notification refers to a moment in time. The next notification will pick up the new text.
- **Persist as `gorm:"type:text"`**: existing denormalized fields (`SpecTaskName`, `ProjectName`) are *persisted at write time* with `gorm:"size:N"`, not computed at read time. The prompt can be long, so use `text` like the existing `Description` field. GORM auto-migrate adds the column on next API start.
- **Field naming `spec_task_description`**: follows the existing `spec_task_name` / `spec_task_id` pattern. Avoids colliding with `AttentionEvent.Description` (the event's own description field).

## Files Changed

| File | Change |
|------|--------|
| `api/pkg/types/attention_event.go` | Add `SpecTaskDescription` field |
| `api/pkg/services/attention_service.go` | Populate it from `task.Description` in `EmitEvent()` |
| `frontend/src/hooks/useAttentionEvents.ts` | Add `spec_task_description?: string` to interface |
| `frontend/src/components/system/GlobalNotifications.tsx` | Update tooltip `title` to render `spec_task_description` first |
