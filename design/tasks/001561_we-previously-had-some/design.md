# Design: Conversation Topic Visualization

## What Already Exists (Do Not Rebuild)

- **`GET /api/v1/sessions/{id}/toc`** — returns `SessionTOCResponse` with `entries[]` (turn, id, summary) and a pre-formatted string. Already exposed in generated API client.
- **`config.title_history`** (`TypesTitleHistoryEntry[]`) — array of `{title, changed_at, turn, interaction_id}` available on any full session object. Ordered newest-first.
- **`interactions[].summary`** — one-line LLM-generated summary per turn.
- **`TabsView.tsx`** already fetches and renders `title_history` on hover in the SpecTask view — that pattern can be reused.

## Chapter Concept

A "chapter" = a contiguous span of turns under the same session title. Derived client-side from `title_history` (which already records `turn` and `interaction_id` when the title changed). Reverse the array (oldest-first) to get chapter boundaries:

```
Chapter 1: turns 1–N₁  (title = first session name)
Chapter 2: turns N₁+1–N₂  (title = second name from history)
...
```

No new backend work needed — this is a pure frontend derivation.

## UI Surfaces

### 1. Chapter Dividers in Chat (`Interaction.tsx` area)

In the interactions list (rendered in the main session view), insert a `<ChapterDivider>` component before the first interaction of each chapter. The divider shows the chapter title with a subtle horizontal rule and chapter number. Use `title_history` from the session object already loaded on the page.

**Implementation note**: The session object is available in the chat view context. `title_history` is part of `SessionMetadata.config`. Reverse and map it to chapter boundaries indexed by `interaction_id`.

### 2. Conversation Outline Panel (new component: `SessionOutlinePanel.tsx`)

A collapsible right-side drawer or left-sidebar section that:
- Fetches `/api/v1/sessions/{id}/toc` via React Query
- Groups TOC entries by chapter (same derivation as above)
- Renders a numbered chapter list; each chapter expands to show individual turn summaries
- Clicking a chapter/turn scrolls the main interaction list to that interaction (use `element.scrollIntoView()` with stable IDs on interaction containers)
- Toggle button added to `SessionToolbar.tsx`

### 3. Session Sidebar Topic Chips (`SessionsSidebar.tsx`)

On session list item hover (or expand toggle), show `title_history` chips. The session list currently fetches `TypesSessionSummary[]` — this type already includes `config` (metadata). Render up to 4 chips with the topic titles (oldest → newest), truncated with ellipsis if long. Use MUI `Chip` with small size.

**If `TypesSessionSummary` doesn't include `title_history`**: Either add it to the summary query, or fetch the full session on hover (lazy, debounced). Check `store_sessions.go` `ListSessions` query to see if metadata is included.

### 4. Current Chapter Indicator (`SessionToolbar.tsx`)

In the toolbar, show the current chapter title as a small `Typography` label. Update it based on scroll position: listen to scroll events on the interaction list container, determine which interaction is in view, map to chapter via the `interaction_id` → chapter lookup built from `title_history`.

## Data Flow

```
Session object (already loaded)
  └─ metadata.config.title_history  →  derive chapter boundaries (client-side)
                                    →  ChapterDivider components
                                    →  Current chapter indicator (scroll-linked)
                                    →  Session sidebar chips (on hover)

GET /sessions/{id}/toc  (React Query)
  └─ entries[]           →  SessionOutlinePanel (grouped by chapter)
```

## Key Files to Touch

| File | Change |
|------|--------|
| `frontend/src/components/session/Interaction.tsx` (or parent list) | Insert `ChapterDivider` between interactions |
| `frontend/src/components/session/SessionToolbar.tsx` | Add chapter label + outline toggle button |
| `frontend/src/components/session/SessionsSidebar.tsx` | Add topic chips on hover/expand |
| `frontend/src/components/session/SessionOutlinePanel.tsx` | New component — TOC drawer |
| `frontend/src/services/sessionService.ts` | Add `useSessionTOC` React Query hook |

## Patterns Found in Codebase

- **React Query pattern**: See `sessionService.ts` — use `useQuery` with `queryKey: ['session-toc', sessionId]`, extract `.data` from axios response.
- **Generated client**: Must use `api.getApiClient().v1SessionsTocDetail(sessionId)` — don't use raw fetch. Run `./stack update_openapi` if endpoint is missing from generated client.
- **MUI Chip + Collapse**: Already used in `SessionsSidebar.tsx` for execution ID grouping — reuse that `Collapse` pattern for topic chips.
- **Scroll-linked state**: Use `IntersectionObserver` on interaction containers for performant scroll tracking (not raw scroll events).
- **Routing**: Use `useRouter()` not `<Link>` — already established convention.

## What NOT to Do

- Do not add new backend endpoints — all needed data already exists.
- Do not add topic extraction LLM calls — summaries and title history are already generated asynchronously.
- Do not show the outline panel if `title_history` has < 2 entries (single-topic sessions don't benefit).
