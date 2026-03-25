# Add Mine/All toggle to "Needs Attention" alerts

## Summary

Adds a toggle to the "Needs Attention" panel so users can switch between seeing only alerts for their own tasks ("mine") and all alerts ("all").

"Mine" filters by task ownership: assignee first, falling back to creator when no assignee is set.

## Changes

- **`api/pkg/types/attention_event.go`** — new `AttentionEventFilters` struct with `MineOnly bool`
- **`api/pkg/store/store_attention_events.go`** — `ListAttentionEvents` accepts filters; when `MineOnly`, JOINs `spec_tasks` and filters by `assignee_id = user OR (assignee_id IS NULL AND created_by = user)`
- **`api/pkg/store/store.go`** — updated Store interface signature
- **`api/pkg/store/store_mocks.go`** — updated mock to match new signature
- **`api/pkg/server/attention_event_handlers.go`** — reads `?filter=mine` query param and passes filters to store
- **`frontend/src/hooks/useAttentionEvents.ts`** — accepts `filterMine: boolean`; appends `&filter=mine` to API call; includes it in React Query key so both modes cache independently
- **`frontend/src/components/system/GlobalNotifications.tsx`** — adds Mine/All pill toggle in the panel header; state persisted to `localStorage` under `attention-filter-mode`; badge count reflects the active filter
