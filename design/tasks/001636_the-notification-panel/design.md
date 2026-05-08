# Design: Notification Panel Count Fix

## Root Cause

`useAttentionEvents.ts` (lines 133–135) computes counts from raw API data:

```typescript
totalCount: query.data?.length || 0,
unreadCount: (query.data ?? []).filter(e => !e.acknowledged_at).length,
hasNew: (query.data ?? []).some(e => !e.acknowledged_at),
```

But `GlobalNotifications.tsx` (line 394) de-duplicates before rendering:

```typescript
const groups = deduplicateGroupsByTask(groupEvents(events))
```

The badge (lines 420–436) uses `unreadCount`/`totalCount` from the hook — which are raw counts —
while the user only sees the de-duplicated `groups` list.

## Fix: Compute Counts from De-duplicated Groups in GlobalNotifications.tsx

Move the count derivation to **after** de-duplication in `GlobalNotifications.tsx`. No changes
needed to `useAttentionEvents.ts` (its `events` array is still used for `newEvents` and sound
triggers elsewhere).

### Key files

| File | Change |
|------|--------|
| `frontend/src/components/system/GlobalNotifications.tsx` | Compute `deduplicatedUnreadCount`, `deduplicatedTotalCount`, `deduplicatedHasNew` from `groups` |

### Logic

```typescript
const groups = deduplicateGroupsByTask(groupEvents(events))

function isGroupUnread(group: EventGroup): boolean {
  if (group.kind === 'single') return !group.event.acknowledged_at
  return !group.primary.acknowledged_at || !group.secondary.acknowledged_at
}

const deduplicatedTotalCount = groups.length
const deduplicatedUnreadCount = groups.filter(isGroupUnread).length
const deduplicatedHasNew = groups.some(isGroupUnread)
```

Then use these in the `<Badge badgeContent={...}>`:

```tsx
<Badge
  badgeContent={deduplicatedHasNew ? deduplicatedUnreadCount : deduplicatedTotalCount}
  color={deduplicatedHasNew ? 'error' : 'default'}
  ...
>
```

### Acknowledge grouped notifications (already correct)

The existing grouped handler (lines 579–581) already acknowledges both events:
- `acknowledge(group.secondary.id)` — explicit call
- `handleNavigate(ev)` (where `ev` = `group.primary`) → `acknowledge(group.primary.id)`

No change needed.

## Implementation Notes

- Added `isGroupUnread(group)` helper at module scope (above `deduplicateGroupsByTask`) so it
  can be reused if needed — pure function, no closure dependencies.
- Computed `deduplicatedTotalCount`, `deduplicatedUnreadCount`, `deduplicatedHasNew` directly
  after the existing `groups` array is built — no extra passes through the data.
- Extended scope beyond just the badge: the header pill (lines 579–594), the "Dismiss all" button
  visibility (line 635), and the "All clear" empty-state check (line 690) all previously used
  the raw `totalCount` from the hook. Updated all of them to use `deduplicatedTotalCount` for
  consistency with what the user sees.
- Removed `totalCount`, `unreadCount`, `hasNew` from the `useAttentionEvents` destructuring
  since no consumer in `GlobalNotifications.tsx` uses them anymore. The hook still exposes them
  for any other callers.
- Verified `yarn build` succeeds with no type errors.
- Acknowledge behavior for grouped notifications was already correct (no change needed):
  `onNavigate={(ev) => { acknowledge(group.secondary.id); handleNavigate(ev) }}` —
  `handleNavigate` then calls `acknowledge(ev.id)` where `ev` is `group.primary`.

## Patterns Found in Codebase

- `groupEvents` and `deduplicateGroupsByTask` are pure functions defined at the top of
  `GlobalNotifications.tsx` — easy to call multiple times without side effects.
- The `useAttentionEvents` hook returns `events` (raw array) alongside the count fields.
  Counts can be overridden locally in the component without touching the hook.
- `isAcknowledged` check in `AttentionEventItem.tsx` (line ~200) already requires BOTH events
  in a group to be acknowledged to show as read — consistent with the proposed group unread logic.
