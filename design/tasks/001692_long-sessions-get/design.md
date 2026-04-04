# Design: Long Session Performance in SpecTask Chat

## Component Audit

### EmbeddedSessionView (`frontend/src/components/session/EmbeddedSessionView.tsx`, 512 lines)
Used by: `SpecTaskDetailContent`, `TeamDesktopPage`, `ExternalAgentDesktopViewer`

**Good:**
- iOS Safari workarounds (passive wheel/touchmove listeners + RAF guard)
- WebSocket-aware: suppresses 3s polling when WS is connected
- `useLayoutEffect` for pre-paint scroll after DOM mutations

**Bad:**
- Renders **all** `session.interactions` in one `map()` — no pagination
- Fetches full session (including all interactions) via `useGetSession`
- Buggy scroll-to-bottom: doesn't reliably keep the user scrolled to the bottom when the agent responds

### Session.tsx (`frontend/src/pages/Session.tsx`, 1727 lines)
Used by: `PreviewPanel` (Optimus/Project Manager "New Chat", app preview) via `<Session previewMode />`.

**Good:**
- Has a block-based concept: `INTERACTIONS_PER_BLOCK = 20`, only shows the last 20 by default
- `MemoizedInteraction` with thorough equality check prevents unnecessary re-renders
- Scroll-to-bottom works reliably

**Bad:**
- Still fetches ALL interactions via `useGetSession` — block system is purely a render limit, not a data limit

## Decision: Switch SpecTask to Session.tsx

`Session.tsx` already works as an embedded component via `PreviewPanel` (used in app preview and Optimus/Project Manager chat on the kanban board). It has:
- Working scroll-to-bottom behavior
- Block-based virtual rendering (`INTERACTIONS_PER_BLOCK = 20`) that only renders recent interactions
- Auto-loading older interactions when scrolling up

The only change needed is **data-level pagination** — currently `useGetSession` fetches all interactions upfront. Switch to the paginated API so long sessions don't load hundreds of interactions on mount.

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

Leverage `Session.tsx`'s existing scroll behavior. When older interactions load on scroll-up:
- Save `scrollHeight` before state update
- After state update, restore `scrollTop = newScrollHeight - savedScrollHeight` so the user's viewport doesn't jump

### Component Interface

`EmbeddedSessionView` interface unchanged — it still takes `sessionId: string`. Pagination is internal.

```ts
const PAGE_SIZE = 20  // interactions shown initially / per load-more
```


## Files to Change

| File | Change |
|------|--------|
| `frontend/src/pages/Session.tsx` | Add data-level pagination using the existing paginated API (fetch last 20 interactions initially, load more on scroll-up) |
| `frontend/src/components/tasks/SpecTaskDetailContent.tsx` | Replace `EmbeddedSessionView` with `Session` component (via `PreviewPanel` or directly) |
| `frontend/src/services/sessionService.ts` | Add `useListInteractions(sessionId, page, perPage)` hook wrapping the existing API endpoint |
