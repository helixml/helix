# Design: Navigation History in Notification Panel

## Overview

Enhance the existing notification sliding panel (`GlobalNotifications.tsx`) with two changes:
1. Deduplicate alerts by spec task
2. Add a "Recently visited" section below the alerts, populated from client-side navigation history

The standalone Kanban board history button idea from the original design is **dropped** — this integrates cleanly into the existing notification UX.

## Existing System

- **Component:** `frontend/src/components/system/GlobalNotifications.tsx`
- **Panel:** 360px right-side sliding panel, triggered by bell icon in the AppBar
- **Data:** `TypesAttentionEvent` objects polled every 10s from `/api/v1/attention-events?active=true`
- **Fields relevant to dedup:** `spec_task_id`, `event_type`, `created_at`, `idempotency_key`
- **Existing grouping:** Already groups `specs_pushed` + `agent_interaction_completed` within a 60s window (lines 59-97). This is similar logic — extend or refactor it.

## Change 1: Deduplicate Alerts by Spec Task

**Where:** Inside `GlobalNotifications.tsx`, the existing event list rendering logic.

**How:** After fetching events, group by `spec_task_id`. Within each group, keep only the event with the latest `created_at`. If `spec_task_id` is null, fall back to grouping by `idempotency_key` or treat each as its own group.

This is a view-layer transform only — no backend change, no mutation of data. The raw events remain unchanged; we just pick one representative per task to display.

**Decision:** Replace/extend the existing 60s-window grouping with a simpler "one per task" rule. The 60s window was already trying to solve this — the new rule is cleaner.

## Change 2: "Recently Visited" Section

### Navigation History Hook

New hook: `frontend/src/hooks/useNavigationHistory.ts`

- Subscribes to router5 route changes via `router.router.subscribe()`
- On each navigation, records `{ url, routeName, params, title, timestamp }` to `localStorage` key `helix_nav_history`
- Deduplicates by `url` (remove older entry, insert new at front)
- Caps at 30 entries total (before filtering for display)
- Derives `title` from route name + params (e.g., `"Task: {spec_task_name}"`, `"Review: {task_id}"`, `"Board: {project_name}"`)
- Returns the history array

### Data Shape

```ts
interface NavHistoryEntry {
  url: string;           // dedup key, e.g. "/orgs/x/projects/y/specs/z"
  routeName: string;     // router5 route name, for clean SPA navigation
  params: Record<string, string>;
  title: string;         // human-readable
  timestamp: number;     // Date.now()
}
```

### Panel Integration

In `GlobalNotifications.tsx`, below the existing alerts list:

1. Call `useNavigationHistory()` to get the history array
2. Get the set of `url`s currently covered by active alerts (derive from alert navigation targets)
3. Filter history: exclude pages already in alerts, take top 10
4. If any remain, render a `"Recently visited"` section heading followed by clickable rows
5. Each row: small page-type icon + truncated title; click calls `router.navigate(routeName, params)`
6. Section is hidden entirely when the filtered list is empty

### Visual Layout (within panel)

```
┌─────────────────────────────┐
│ 🔔 Needs Attention    [X]  │
├─────────────────────────────┤
│ ● Task A - spec ready       │
│ ● Task B - agent finished   │
│ ● Task C - PR ready         │
├─────────────────────────────┤
│ Recently visited            │  ← new section, only if non-empty
│   Task D detail             │
│   Design review: auth       │
│   Task E detail             │
└─────────────────────────────┘
```

## File Changes

| File | Change |
|------|--------|
| `frontend/src/hooks/useNavigationHistory.ts` | Create — history tracking hook |
| `frontend/src/components/system/GlobalNotifications.tsx` | Modify — dedup alerts by task; add "Recently visited" section |

No backend changes. No new API endpoints.

## Patterns Used in This Codebase

- Notification panel uses Lucide icons (not MUI icons) — use Lucide's `Clock` or `History` for the section icon
- Router navigation uses `useRouter()` hook → `router.navigate(routeName, params)`
- Event grouping logic already exists in `GlobalNotifications.tsx` lines 59-97 — extend this rather than duplicating
- Styling in this file uses inline `style={{}}` objects and Tailwind-like class names (not MUI `sx`) — match that convention
