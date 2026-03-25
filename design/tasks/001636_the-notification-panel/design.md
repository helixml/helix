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

## Patterns Found in Codebase

- `groupEvents` and `deduplicateGroupsByTask` are pure functions defined at the top of
  `GlobalNotifications.tsx` — easy to call multiple times without side effects.
- The `useAttentionEvents` hook returns `events` (raw array) alongside the count fields.
  Counts can be overridden locally in the component without touching the hook.
- `isAcknowledged` check in `AttentionEventItem.tsx` (line ~200) already requires BOTH events
  in a group to be acknowledged to show as read — consistent with the proposed group unread logic.
