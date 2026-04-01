# Design: Red Dot Notification Indicator on Kanban Cards

## Architecture

### Data Flow

The existing `useAttentionEvents` hook polls `/api/v1/attention-events?active=true` every 10 seconds and returns all unread events. Each `AttentionEvent` has a `spec_task_id` field linking it to a specific task.

**New flow:**
1. `useAttentionEvents` already fetches all active events.
2. Pass the full events list (or a derived map of `spec_task_id → event[]`) down to `SpecTaskKanbanBoard` → `TaskCard`.
3. Each `TaskCard` receives the unread events for its task.
4. `TaskCard` uses an `IntersectionObserver` to detect when it enters the viewport.
5. On first intersection, call `acknowledge(eventId)` for each unread event on the task.
6. The React Query cache invalidation causes the red dot to disappear (event no longer returned as active).

### Component Changes

**`useAttentionEvents.ts`** — no changes needed. Already exposes the raw events list.

**`SpecTaskKanbanBoard.tsx`**:
- Import `useAttentionEvents` and compute a map: `taskAttentionEvents: Record<string, AttentionEvent[]>` — keyed by `spec_task_id`, values are unread events.
- Pass `attentionEvents` (the array for this task) as a prop to `TaskCard`.

**`TaskCard.tsx`**:
- Accept new prop: `attentionEvents: AttentionEvent[]` (defaults to `[]`).
- Derive `hasUnreadNotification = attentionEvents.length > 0`.
- Add `useEffect` with `IntersectionObserver` on the card's root element ref.
  - When `isIntersecting` becomes true and `hasUnreadNotification` is true, call `acknowledge(eventId)` for each event.
  - Use `{ threshold: 0.5 }` so the card is meaningfully visible before auto-ack.
- Render a small red dot badge (absolute-positioned, top-right of the card) when `hasUnreadNotification`.

### Red Dot Visual

Position: absolute, top-right corner of the card container (`top: -6px, right: -6px`).

```tsx
{hasUnreadNotification && (
  <Box sx={{
    position: 'absolute',
    top: 8,
    right: 8,
    width: 10,
    height: 10,
    borderRadius: '50%',
    backgroundColor: 'error.main',  // MUI red
    zIndex: 1,
    border: '2px solid',
    borderColor: 'background.paper',  // white ring to pop against card
  }} />
)}
```

The card's root `Box` needs `position: 'relative'` (likely already set).

### Acknowledge on View

```tsx
const cardRef = useRef<HTMLDivElement>(null)

useEffect(() => {
  if (!hasUnreadNotification || !cardRef.current) return
  const observer = new IntersectionObserver(
    ([entry]) => {
      if (entry.isIntersecting) {
        attentionEvents.forEach(event => acknowledge(event.id))
        observer.disconnect()
      }
    },
    { threshold: 0.5 }
  )
  observer.observe(cardRef.current)
  return () => observer.disconnect()
}, [hasUnreadNotification, attentionEvents])
```

The `acknowledge` function comes from `useAttentionEvents` (already exported). It calls `PUT /api/v1/attention-events/{id}` and invalidates the React Query cache, causing the dot to disappear.

### Key Decisions

- **IntersectionObserver over click**: Matches the requirement of "viewing is sufficient." Threshold of 0.5 prevents false positives from barely-visible cards at scroll edges.
- **API acknowledgment (not local state)**: Must use the same `acknowledged_at` mechanism as the notification panel so both indicators stay in sync.
- **No change to existing orange dot**: The orange dot (`needsAttention` from `useAgentActivityCheck`) is a separate local-state concern for agent activity; this red dot is the notification-system concern. They coexist independently.
- **Filter to `agent_interaction_completed` events only**: Other event types (e.g., `specs_pushed`, `pr_ready`) are not specifically about agent completion and don't belong on cards in this context.

### Codebase Notes

- `useAttentionEvents` is at `frontend/src/hooks/useAttentionEvents.ts` — already exports `acknowledge(id)` and the raw events list.
- `TaskCard.tsx` at `frontend/src/components/tasks/TaskCard.tsx` — card root element already uses a ref-able `Box`; `position: relative` may need to be confirmed.
- `SpecTaskKanbanBoard.tsx` at `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` — renders `TaskCard` instances and is the right place to inject the events map.
