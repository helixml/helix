# Design: Fix Notifications Panel Tooltip

## Root Cause

The `AttentionEvent` struct (Go: `api/pkg/types/attention_event.go`, TS: `useAttentionEvents.ts`) does not carry the task's original prompt. When events are created in `attention_service.go`, only `SpecTaskName` is denormalized from the task — `OriginalPrompt` is not.

The tooltip in `GlobalNotifications.tsx:236-246` renders `event.spec_task_name` (which is the short task name, not the prompt), giving users no new information beyond what's already visible.

## Fix

### 1. Backend — Add `original_prompt` to `AttentionEvent`

**`api/pkg/types/attention_event.go`** — add field:
```go
OriginalPrompt string `json:"original_prompt" gorm:"-"`
```
Use `gorm:"-"` (no DB column needed) since it's denormalized at query time, same pattern as `SpecTaskName` and `ProjectName`.

**`api/pkg/services/attention_service.go`** — populate in `EmitEvent()` (around line 86 where `SpecTaskName` is set):
```go
event.OriginalPrompt = task.OriginalPrompt
```

### 2. Frontend — Update tooltip content

**`frontend/src/hooks/useAttentionEvents.ts`** — add field to interface:
```ts
original_prompt?: string
```

**`frontend/src/components/system/GlobalNotifications.tsx:237-242`** — change tooltip `title`:
```tsx
title={
  <span style={{ whiteSpace: 'pre-wrap' }}>
    {event.original_prompt || event.spec_task_name || event.spec_task_id || ''}
    {'\n'}
    {groupedWith ? 'Spec ready & agent finished' : event.title}
  </span>
}
```

## Key Decisions

- **`gorm:"-"` for new field**: Consistent with existing denormalized fields (`SpecTaskName`, `ProjectName`). No migration needed.
- **Frontend fallback chain**: `original_prompt → spec_task_name → spec_task_id` preserves existing behavior when prompt is absent.
- **Keep event title in tooltip**: Useful secondary context (what happened, e.g. "Spec ready").

## Files Changed

| File | Change |
|------|--------|
| `api/pkg/types/attention_event.go` | Add `OriginalPrompt` field |
| `api/pkg/services/attention_service.go` | Populate `OriginalPrompt` from `task.OriginalPrompt` in `EmitEvent()` |
| `frontend/src/hooks/useAttentionEvents.ts` | Add `original_prompt?: string` to interface |
| `frontend/src/components/system/GlobalNotifications.tsx` | Update tooltip `title` to prefer `original_prompt` |
