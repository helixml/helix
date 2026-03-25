# Design: Notifications Panel Polish

## Affected File

Single file: `frontend/src/components/system/GlobalNotifications.tsx`

All changes are self-contained here. No backend changes required.

---

## 1. Recently Visited — Deduplication

**Current behaviour:** `recentPages` filters out entries whose `routeName` isn't a task/review route, then filters out active alert task IDs, then slices 10. URL-level dedup happens in `navHistory.ts` at write time, but the same `taskId` can appear via two different URLs (task-detail + task-review).

**Fix:** After the existing filter, additionally deduplicate by `params.taskId` before slicing:

```ts
const recentPages = navHistory
  .filter(entry => {
    if (entry.routeName !== 'org_project-task-detail' && entry.routeName !== 'org_project-task-review') return false
    return !alertTaskIds.has(entry.params.taskId)
  })
  // Deduplicate by taskId — keep first (most-recent) occurrence
  .filter((entry, _, arr) =>
    arr.findIndex(e => e.params.taskId === entry.params.taskId) === arr.indexOf(entry)
  )
  .slice(0, 10)
```

History is sorted newest-first from `loadNavHistory` (insert-at-front), so the first hit per taskId is the most recent visit.

---

## 2. Recently Visited — Time Ago

**Current behaviour:** `RecentPageItem` shows only `entry.title` with no timestamp.

**Fix:** The existing `timeAgo(dateStr)` function takes an ISO date string, but `NavHistoryEntry.timestamp` is a Unix-ms number. Add a small helper or adapt inline:

```ts
function timeAgoMs(ms: number): string {
  return timeAgo(new Date(ms).toISOString())
}
```

In `RecentPageItem`, add the time label between the title and the clock icon:

```tsx
<Typography sx={{ color: 'rgba(255,255,255,0.3)', fontSize: '0.65rem', whiteSpace: 'nowrap', flexShrink: 0 }}>
  {timeAgoMs(entry.timestamp)}
</Typography>
```

Layout: `[🕒 icon] [title — flex:1] [time label]` — matches the event item layout.

---

## 3. Notification Items — Swap Title & Subtitle

**Current layout:**
- Primary: `event.title` (e.g. "Spec ready") — bold, single line, ellipsis
- Secondary: `event.spec_task_name` · `event.project_name` — dim, single line, ellipsis

**New layout:**
- Primary: `event.spec_task_name || event.spec_task_id` — bold, max 2 lines (`WebkitLineClamp: 2`, `whiteSpace: 'normal'`, `display: '-webkit-box'`, `WebkitBoxOrient: 'vertical'`)
- Secondary: `event.title` · `event.project_name || event.project_id` — dim, single line, ellipsis (same as before)

For grouped events, primary = `event.spec_task_name || event.spec_task_id` (task name), secondary = `"Spec ready & agent finished"` · project name.

---

## 4. Icons — Replace Emojis with Lucide Icons

**Current:** `eventEmoji()` returns an emoji string rendered inside a `<Box sx={{ fontSize: '0.9rem' }}>`.

**New approach:** Replace `eventEmoji()` with `eventIcon()` that returns a lucide `React.ReactElement`:

```ts
import { Sparkles, Hand, AlertCircle, GitMerge, Bell } from 'lucide-react'

function eventIcon(eventType: AttentionEventType, color: string): React.ReactElement {
  const props = { size: 14, color }
  switch (eventType) {
    case 'specs_pushed': return <Sparkles {...props} />
    case 'agent_interaction_completed': return <Hand {...props} />
    case 'spec_failed':
    case 'implementation_failed': return <AlertCircle {...props} />
    case 'pr_ready': return <GitMerge {...props} />
    default: return <Bell {...props} />
  }
}
```

Icon is colored with the existing `accentColor` from `eventAccentColor()`, so it inherits the color-coding system.

Grouped events: use `<Sparkles size={14} color={eventAccentColor('specs_pushed')} />`.

The `<Box sx={{ fontSize: '0.9rem', flexShrink: 0 }}>` wrapper stays (just renders the icon instead of an emoji string). May need `display: 'flex'` on the wrapper for proper vertical alignment.

---

## Icon Choice Rationale

- `Hand` for `agent_interaction_completed`: friendlier than a stop sign (🛑) — wave-like, not aggressive. Available in lucide ≥0.200.
- `Sparkles` for `specs_pushed`: matches "spec ready" excitement better than 📋. Available in lucide ≥0.200.
- `AlertCircle` for failures: cleaner than ❌, consistent with how errors appear elsewhere in the codebase.
- `GitMerge` for `pr_ready`: clear PR association, replaces 🔀 emoji.

All icons come from `lucide-react` (already installed, already imported in this file for `Bell`, `X`, `BellRing`, `BellOff`). No new dependencies required.

---

## Pattern Notes (for future agents)

- `GlobalNotifications.tsx` is the single source of truth for the notifications panel UI.
- `navHistory.ts` handles localStorage with URL-level dedup at write time; display-level dedup by taskId is done at render time in `GlobalNotifications.tsx`.
- `timeAgo()` is a local helper in this file — no shared utility yet. If a shared date util is needed elsewhere, extract it later.
- Icon library preference: lucide-react (already used for panel chrome icons). MUI icons are also available but lucide is more consistent here.
- The `eventAccentColor()` function maps event types to color hex strings — reuse it for icon coloring.
