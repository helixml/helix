# Design: Split Screen View — Buttons Change Wrong Panel

## Architecture Overview

The split screen (workspace) view is built with a tree-based panel layout in `TabsView.tsx`. Each leaf panel can display a `SpecTaskDetailContent` instance for a different task. The bug is that `SpecTaskDetailContent` uses two pieces of **global shared state** to manage what should be **per-instance local state**:

1. **`router.mergeParams({ view })`** — writes the current view mode (desktop/chat/changes/details) into the URL query params. Every mounted instance reads from and writes to the same `router.params.view`.
2. **`streaming.setCurrentSessionId()`** — sets a single global "current session" for WebSocket subscriptions. Multiple panels fight over this, with the last one to mount/update winning.

## Key Decision: Make View State Purely Local

**Chosen approach**: Remove the router param sync from `SpecTaskDetailContent` when it's used inside `TabsView` (embedded mode). Keep it for the standalone `SpecTaskDetailPage` route.

**Why not just remove `router.mergeParams` entirely?**
The standalone `SpecTaskDetailPage` (route: `/projects/:id/tasks/:taskId`) legitimately uses the `?view=desktop` URL param so users can bookmark or share a direct link to a specific view. We only need to suppress the router sync when the component is embedded in a multi-panel workspace.

**Why not use per-panel URL params (e.g. `?view_panel1=desktop&view_panel2=chat`)?**
Over-engineered. Panel IDs are ephemeral (generated UUIDs), so encoding them in the URL provides no real benefit. The view state within a workspace panel is transient — users don't need to bookmark "panel 2 is showing chat."

## Changes

### 1. Add `embedded` prop to `SpecTaskDetailContent`

Add an optional `embedded?: boolean` prop. When `true`:
- **Skip `router.mergeParams` calls** — the component manages `currentView` via `useState` only, without writing to or reading from URL params.
- **Skip the `useEffect` that syncs `currentView` from `router.params.view`** — prevents other instances from overriding this panel's view.
- **Skip `router.mergeParams` in the auto-switch `useEffect`** (the one that switches to desktop when a session starts, or to details when it stops) — still call `setCurrentView` locally.

The `getInitialView` function should return `"desktop"` (or `"details"` if no session) when `embedded` is true, ignoring the URL param entirely.

### 2. Pass `embedded={true}` from `TabsView`

In `TabsView.tsx` where `SpecTaskDetailContent` is rendered inside `TaskPanel`, add `embedded={true}`:

```
<SpecTaskDetailContent
  key={`${panel.id}-${activeTab.id}`}
  taskId={activeTab.id}
  onOpenReview={...}
  onTaskArchived={onTaskArchived}
  embedded={true}
/>
```

The standalone `SpecTaskDetailPage` does NOT pass `embedded`, so it defaults to `false` and keeps existing URL-synced behavior.

### 3. Fix streaming session competition

The `streaming.setCurrentSessionId()` call is global — only one session ID can be active at a time. When multiple panels each call `setCurrentSessionId` with their own session, the last one wins and the others lose their WebSocket subscription.

**Approach**: When `embedded` is true, skip the `streaming.setCurrentSessionId()` call. The `EmbeddedSessionView` (chat component) already manages its own session data fetching via React Query. The streaming context is primarily used for the single-session pages (standalone detail page, Session page). In the workspace, each panel's chat already fetches interactions independently.

If we discover that embedded panels do need real-time streaming updates (not just polling), we can later refactor `StreamingContext` to support multiple concurrent session subscriptions. But polling with `refetchInterval: 2300` already provides near-real-time updates for the workspace use case.

### 4. No changes needed to `SpecTaskActionButtons`

The action buttons (`Start Planning`, `Review Spec`, `Reject`, `Open PR`) delegate to callback props (`onStartPlanning`, `onReviewSpec`, `onReject`). These callbacks are defined in `SpecTaskDetailContent` and operate on the specific `taskId` prop — they don't use router params. The `handleStartPlanning` function calls `handleViewChange("desktop")` which will now correctly only update local state when `embedded` is true. No changes needed to `SpecTaskActionButtons.tsx`.

### 5. No changes needed to `SpecTaskDetailPage`

The standalone page doesn't pass `embedded`, so all existing router-synced behavior is preserved. URLs like `/projects/:id/tasks/:taskId?view=desktop` continue to work.

## Codebase Patterns Discovered

- **react-router5**: This project uses `react-router5` with a custom `useRouter` hook wrapping `RouterContext`. The `mergeParams` function calls `router.navigate(route.name, { ...route.params, ...newParams }, { replace: true })` — this is a full route navigation that triggers re-renders in all components using `useRoute()`.
- **Streaming context**: `StreamingContextProvider` in `contexts/streaming.tsx` maintains a single `currentSessionId` state. The `clearSessionData` function (exposed as `setCurrentSessionId`) clears previous session data before setting a new one. This is fundamentally single-session by design.
- **TabsView panel tree**: Panels use a tree structure (`PanelNode` with `type: "leaf" | "split"`). Each leaf has tabs, and the active tab renders either `SpecTaskDetailContent`, `DesignReviewContent`, or `NewSpecTaskForm`. The `key` prop uses `${panel.id}-${activeTab.id}` which ensures proper React instance isolation — the problem isn't React mounting, it's the shared global state.
- **`onOpenReview` callback**: When present (workspace mode), `handleReviewSpec` calls `onOpenReview` which opens a new tab in the same panel group. When absent (standalone mode), it navigates to a new route. This is the existing pattern for workspace-aware behavior.

## Risk Assessment

- **Low risk**: The change is additive (new prop, default preserves existing behavior). Only the workspace code path changes.
- **Testing**: Build frontend (`cd frontend && yarn build`), manually test: open two tasks in split screen, toggle views independently, start/stop sessions, click action buttons. Verify standalone detail page still syncs with URL.