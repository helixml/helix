# Design: Remove Auto-Acknowledgment on Panel Open

## Current Behavior

In `GlobalNotifications.tsx`, the `handleDrawerOpen()` callback (line 240) automatically calls `acknowledge(event.id)` for every unacknowledged event when the panel opens:

```tsx
const handleDrawerOpen = useCallback(() => {
  setDrawerOpen(true)
  onOpenChange?.(true)
  // Acknowledge all visible events when drawer opens
  for (const event of events) {
    if (!event.acknowledged_at) {
      acknowledge(event.id)
    }
  }
}, [events, acknowledge, onOpenChange])
```

The `acknowledge()` call hits `PUT /api/v1/attention-events/{id}` with `{ acknowledge: true }`, which sets `acknowledged_at` in the database. The red badge is driven by the count of events without `acknowledged_at`, so opening the panel clears the badge.

From the user's perspective this feels like dismissal — the visual urgency indicator is gone even though the notification is still in the list.

## Fix

**Remove the auto-acknowledge loop from `handleDrawerOpen`.**

```tsx
const handleDrawerOpen = useCallback(() => {
  setDrawerOpen(true)
  onOpenChange?.(true)
  // No auto-acknowledgment — user must explicitly dismiss
}, [onOpenChange])
```

The `acknowledged_at` field and the `acknowledge()` mutation can remain in the codebase (they may be useful for future features or analytics), but they should not be called automatically on panel open.

The badge and notification state will now only change via:
- Individual X button click → `dismiss(eventId)` sets `dismissed_at`
- "Dismiss all" button → `dismissAll()` bulk-sets `dismissed_at`

## What Stays the Same

- The X button and "Dismiss all" button behavior is unchanged
- The backend API and database schema are unchanged
- The `useAttentionEvents` hook internals are unchanged (no backend changes needed)
- Browser (OS) notification behavior is unchanged

## Files to Change

| File | Change |
|------|--------|
| `frontend/src/components/system/GlobalNotifications.tsx` | Remove the auto-acknowledge loop from `handleDrawerOpen` (lines 243-248) and remove `acknowledge` from the dependency array |

## Pattern Notes

- This project uses React Query for all API calls — `acknowledge` and `dismiss` are mutation callbacks from `useAttentionEvents`
- Dependency arrays must only include primitives that change — removing `acknowledge` and `events` from the `handleDrawerOpen` dependency array is correct after this change
- No backend changes needed; `acknowledged_at` column stays but just won't be set automatically
