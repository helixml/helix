# Implementation Tasks

## Backend

- [ ] Add `AttentionEventFilters` struct to `api/pkg/types/attention_event.go` with a `CreatedBy string` field
- [ ] Update `ListAttentionEvents` in `api/pkg/store/store_attention_events.go` to accept `AttentionEventFilters`; when `CreatedBy` is non-empty, JOIN `spec_tasks` and filter by `spec_tasks.created_by = createdBy`
- [ ] Update the `Store` interface to match the new `ListAttentionEvents` signature
- [ ] Update `listAttentionEvents` handler in `api/pkg/server/attention_event_handlers.go` to read `?filter=mine` query param and pass `AttentionEventFilters{CreatedBy: user.ID}` when set
- [ ] Update `dismissAllAttentionEvents` handler — no filter needed there, dismiss-all always acts on the user's full set

## Frontend

- [ ] Update `useAttentionEvents` hook to accept a `filterMine: boolean` param; append `&filter=mine` to the fetch URL when true; include `filterMine` in the React Query key
- [ ] Add `'mine' | 'all'` state to `GlobalNotifications` component, initialised from `localStorage` key `attention-filter-mode` (default `'all'`)
- [ ] Render a small pill/toggle ("Mine | All") in the panel header between the title and the dismiss-all button
- [ ] Pass the active filter mode into `useAttentionEvents` so data and badge count both reflect it
- [ ] Persist toggle changes to `localStorage`
