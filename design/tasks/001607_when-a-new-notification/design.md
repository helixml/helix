# Design: Notification Bar Count Badge Red on New Notifications

## Root Cause (Two Issues)

### Issue 1 — `hasNew` is transient (primary bug)

`useAttentionEvents.ts` lines 52-62 compute `newEvents` (and therefore `hasNew`) inline on every render:

```typescript
const newEvents: AttentionEvent[] = []
if (query.data) {
  for (const event of query.data) {
    if (!event.acknowledged_at && !prevEventIdsRef.current.has(event.id)) {
      newEvents.push(event)
    }
  }
  // ← runs every render, not just on data change
  prevEventIdsRef.current = new Set(query.data.map((e) => e.id))
}
```

`prevEventIdsRef.current` is overwritten on **every render**, so an event is only considered "new" during the single render cycle where it first appears in `query.data`. On the next render — triggered by anything (parent re-render, mutation callback, React Query background refetch completing) — the event's ID is already in the ref and `hasNew` returns false immediately.

This explains the intermittent behavior: the badge *does* sometimes flash red for a few milliseconds (the one render cycle), and users who happen to glance at that moment see it. But it doesn't stay red.

### Issue 2 — Count pill in panel header doesn't use `hasNew` at all (secondary bug)

`GlobalNotifications.tsx` lines ~377-392 render the count pill with hard-coded muted colors that never change:

```tsx
<Box sx={{
  color: 'rgba(255,255,255,0.5)',
  backgroundColor: 'rgba(255,255,255,0.06)',
}}>
  {totalCount}
</Box>
```

Even if Issue 1 were fixed, the count pill in the header would still not turn red because it ignores `hasNew`.

## Fix

### Fix 1 — Base `hasNew` on stable server-side state

Instead of tracking "appeared since last render" (transient), derive `hasNew` from `acknowledged_at` (stable, server-persisted). Any event with `acknowledged_at === null` is unacknowledged because the drawer has not been opened since it arrived. Once the user opens the drawer, all events are acknowledged server-side and `acknowledged_at` gets a timestamp on the next poll.

In `useAttentionEvents.ts`, replace:
```typescript
hasNew: newEvents.length > 0,
```
with:
```typescript
hasNew: (query.data ?? []).some(e => !e.acknowledged_at),
```

The `newEvents` array and browser notification firing logic can still use the existing "not in prev ref" detection — that only needs to fire once per new event. Only `hasNew` (the badge color signal) needs to switch to the stable derivation.

### Fix 2 — Wire `hasNew` into the count pill

In `GlobalNotifications.tsx`, update the count pill `<Box>` in the header (around line 378) to use conditional colors:

```tsx
<Box sx={{
  color: hasNew ? '#fff' : 'rgba(255,255,255,0.5)',
  backgroundColor: hasNew ? '#ef4444' : 'rgba(255,255,255,0.06)',
  ...
}}>
  {totalCount}
</Box>
```

`#ef4444` matches the red already used in `eventAccentColor()` for failure events.

## Pattern Notes

- `acknowledged_at` is set server-side when the user opens the drawer (`handleDrawerOpen` calls `acknowledge()` for every visible event, which PUTs to the API).
- After opening the drawer, the next poll (10s interval, or immediately via `invalidate()` after the mutation) returns events with `acknowledged_at` set — `hasNew` drops to false naturally.
- The two files to change: `frontend/src/hooks/useAttentionEvents.ts` and `frontend/src/components/system/GlobalNotifications.tsx`.
