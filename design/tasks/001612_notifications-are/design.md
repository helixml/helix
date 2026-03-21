# Design: Explicit Read State + Notification Grouping

## 1. Remove Auto-Acknowledge on Panel Open

### Current behavior

`handleDrawerOpen()` in `GlobalNotifications.tsx` (line 240) loops over all events and calls `acknowledge(event.id)` for each unacknowledged one. This sets `acknowledged_at` in the DB, which dims the notification row (opacity 0.65, font-weight 400).

### Fix

Remove the loop entirely. `handleDrawerOpen` becomes:

```tsx
const handleDrawerOpen = useCallback(() => {
  setDrawerOpen(true)
  onOpenChange?.(true)
}, [onOpenChange])
```

### Acknowledge on explicit click

`handleNavigate` already handles click-to-navigate. Add an `acknowledge()` call there, before navigating:

```tsx
const handleNavigate = useCallback(async (event: AttentionEvent) => {
  acknowledge(event.id)  // mark as read on explicit click
  // ... existing navigation logic unchanged
}, [acknowledge, account, api])
```

For grouped notifications (see below), acknowledge both underlying events.

---

## 2. Notification Grouping

### What to group

A `specs_pushed` event and an `agent_interaction_completed` event for the **same `spec_task_id`** whose `created_at` timestamps are within **60 seconds** of each other should be displayed as a single item.

Rationale: these two events are emitted by the backend at almost the same time when an agent finishes a spec. Showing them separately is redundant noise.

### Where to implement

Pure frontend — no backend changes needed. Implement a grouping step in `GlobalNotifications.tsx` before rendering the event list.

### Grouping algorithm

```tsx
type EventGroup =
  | { kind: 'single'; event: AttentionEvent }
  | { kind: 'grouped'; primary: AttentionEvent; secondary: AttentionEvent }

function groupEvents(events: AttentionEvent[]): EventGroup[] {
  const WINDOW_MS = 60_000
  const used = new Set<string>()
  const groups: EventGroup[] = []

  for (const event of events) {
    if (used.has(event.id)) continue

    if (event.event_type === 'specs_pushed' || event.event_type === 'agent_interaction_completed') {
      const partnerType = event.event_type === 'specs_pushed'
        ? 'agent_interaction_completed'
        : 'specs_pushed'

      const partner = events.find(
        (e) =>
          !used.has(e.id) &&
          e.id !== event.id &&
          e.spec_task_id === event.spec_task_id &&
          e.event_type === partnerType &&
          Math.abs(new Date(e.created_at).getTime() - new Date(event.created_at).getTime()) <= WINDOW_MS,
      )

      if (partner) {
        used.add(event.id)
        used.add(partner.id)
        // Primary is always specs_pushed (determines navigation behavior)
        const primary = event.event_type === 'specs_pushed' ? event : partner
        const secondary = event.event_type === 'specs_pushed' ? partner : event
        groups.push({ kind: 'grouped', primary, secondary })
        continue
      }
    }

    used.add(event.id)
    groups.push({ kind: 'single', event })
  }

  return groups
}
```

### Grouped item rendering

The grouped `AttentionEventItem` shows:
- A combined emoji/label: `📋 Spec ready & agent finished`
- Title from the `specs_pushed` event (primary)
- Subtitle: task name · project name (same as today)
- Time from the primary event
- X button dismisses both events
- Clicking navigates as `specs_pushed` (to the spec review page)
- Read state: both events are acknowledged on click

The `isAcknowledged` visual state for a grouped item: acknowledged if **both** underlying events have `acknowledged_at` set.

### `AttentionEventItem` component change

Add an optional `groupedWith?: AttentionEvent` prop. When present:
- Override the emoji/label display
- The `onDismiss` callback dismisses both IDs
- The `onNavigate` callback acknowledges both IDs before navigating

---

## Files to Change

| File | Change |
|------|--------|
| `frontend/src/components/system/GlobalNotifications.tsx` | Remove auto-acknowledge loop from `handleDrawerOpen`; add `acknowledge()` call in `handleNavigate`; add `groupEvents()` function; update `AttentionEventItem` to accept `groupedWith` prop and handle grouped dismiss/acknowledge |

No backend changes required. The `acknowledged_at` field, the `acknowledge` mutation, and all API endpoints remain as-is.

## Pattern Notes

- This project uses React Query for all API calls; mutations are in `useAttentionEvents`
- Dependency arrays must only include primitives that change — after removing `events`/`acknowledge` from `handleDrawerOpen`, update deps accordingly
- `groupEvents()` is a pure function — easy to unit test independently
- Events API returns newest-first; the grouping preserves that order
