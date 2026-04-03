# Design: Long Session Performance in SpecTask Chat

## Component Audit

### EmbeddedSessionView (`frontend/src/components/session/EmbeddedSessionView.tsx`, 512 lines)
Used by: `SpecTaskDetailContent`, `TeamDesktopPage`, `ExternalAgentDesktopViewer`

**Good:**
- Clean sticky-scroll via `isAtBottomRef` + `isProgrammaticScrollRef` guard
- iOS Safari workarounds (passive wheel/touchmove listeners + RAF guard)
- WebSocket-aware: suppresses 3s polling when WS is connected
- `useLayoutEffect` for pre-paint scroll after DOM mutations

**Bad:**
- Renders **all** `session.interactions` in one `map()` — no pagination
- Fetches full session (including all interactions) via `useGetSession`

### Session.tsx (`frontend/src/pages/Session.tsx`, 1727 lines)
Used by: `PreviewPanel` (Optimus/Project Manager "New Chat", app preview) via `<Session previewMode />`.

**Good:**
- Has a block-based concept: `INTERACTIONS_PER_BLOCK = 20`, only shows the last 20 by default
- `MemoizedInteraction` with thorough equality check prevents unnecessary re-renders

**Bad:**
- Still fetches ALL interactions via `useGetSession` — block system is purely a render limit, not a data limit
- scroll-to-bottom uses `setTimeout(200ms)` + a debounce gate (`lastScrollTimeRef`) that fights with streaming
- Reads session ID from URL params (`router.params.session_id`), not a prop — tightly coupled to routing
- 1727 lines of complexity; difficult to embed as a controlled component

## Decision: Extend EmbeddedSessionView (Don't Switch to Session.tsx)

Switching SpecTask to `Session.tsx` would inherit its scroll bugs and routing coupling. The block-based render-only optimisation in Session.tsx doesn't solve the root cause (all data still fetched upfront).

The right fix is to add **data-level pagination** to `EmbeddedSessionView`, which already has the better scroll implementation.

## API

The backend already exposes a paginated interactions endpoint:

```
GET /api/v1/sessions/{id}/interactions?page=0&per_page=20
```

(see `session_interaction_handlers.go:listInteractions` and `store_interactions.go` with `Offset/Limit`)

The `useGetSession` response embeds all interactions inline. We will continue using it for session metadata and the live (last) interaction but switch the historical interaction list to the paginated endpoint.

## Architecture

### Data Fetching

1. On mount, call `GET /api/v1/sessions/{id}/interactions?per_page=20` (last page = highest page index) — but the store sorts interactions and doesn't expose a "last page" shortcut directly. Simplest approach: fetch `per_page=20` with no page param (or `page=0` which returns most recent if ordered desc) — **or** continue using `useGetSession` for the initial render but slice `session.interactions.slice(-20)`.

   **Preferred approach:** Keep `useGetSession` for session metadata + the live streaming interaction. Take `session.interactions.slice(-PAGE_SIZE)` as the initial visible list. When "Load older" is clicked, call `listInteractions` with pagination to fill in older interactions prepended to the list.

   Reason: avoids a second API call on mount, keeps WebSocket live-update path unchanged.

2. Store `olderInteractions: Interaction[]` in component state (prepended on each load-more). Derive the displayed list as `[...olderInteractions, ...session.interactions.slice(-PAGE_SIZE)]`.

3. Track `oldestLoadedIndex` (the slice offset into the full interactions array). When `oldestLoadedIndex > 0`, show a "Load older messages" button.

### Scroll Behavior

Preserve the existing `isAtBottomRef` / `isProgrammaticScrollRef` sticky-scroll logic exactly. When "Load older" loads new messages prepended at the top:
- Save `scrollHeight` before state update
- After state update, restore `scrollTop = newScrollHeight - savedScrollHeight` so the user's viewport doesn't jump

### Component Interface

`EmbeddedSessionView` interface unchanged — it still takes `sessionId: string`. Pagination is internal.

```ts
const PAGE_SIZE = 20  // interactions shown initially / per load-more
```

### Session.tsx (Optimus Chat) — Separate Follow-up

The scroll bug in `Session.tsx` (used by Optimus / Project Manager chat) is a separate issue caused by `setTimeout(200ms)` + `lastScrollTimeRef` debounce conflicting during streaming. That should be fixed independently by replacing the debounce with the same `isProgrammaticScrollRef` guard pattern from `EmbeddedSessionView`. This is out of scope for this task but noted here for a follow-up.

## Files to Change

| File | Change |
|------|--------|
| `frontend/src/components/session/EmbeddedSessionView.tsx` | Add pagination: `PAGE_SIZE` constant, `olderInteractions` state, "Load older" button, scroll-position-preserving prepend |
| `frontend/src/services/sessionService.ts` | Add `useListInteractions(sessionId, page, perPage)` hook wrapping the existing API endpoint |
