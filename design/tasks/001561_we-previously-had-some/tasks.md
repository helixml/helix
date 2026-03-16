# Implementation Tasks

## Setup
- [ ] Verify `GET /api/v1/sessions/{id}/toc` is in generated `api.ts` client (run `./stack update_openapi` if missing)
- [ ] Check whether `TypesSessionSummary` includes `metadata.config.title_history`; if not, confirm plan to fetch full session on sidebar hover

## Chapter Derivation Utility
- [ ] Create `src/utils/sessionChapters.ts`: function `deriveChapters(titleHistory: TypesTitleHistoryEntry[]): Chapter[]` that reverses title_history and maps to `{title, startTurn, startInteractionId}[]`

## Chapter Dividers in Chat
- [ ] Add stable `id` attributes to interaction container elements (e.g., `id={`interaction-${interaction.id}`}`)
- [ ] Insert `<ChapterDivider title={...} chapterNum={...} />` before the first interaction of each chapter in the interactions list
- [ ] Style `ChapterDivider`: horizontal rule with centered chapter title pill (MUI `Divider` + `Chip`)

## Conversation Outline Panel
- [ ] Add `useSessionTOC(sessionId)` React Query hook in `sessionService.ts`
- [ ] Create `SessionOutlinePanel.tsx`: collapsible right drawer showing chapters with expandable turn summaries
- [ ] Clicking a chapter/turn calls `document.getElementById('interaction-...').scrollIntoView({behavior: 'smooth'})`
- [ ] Add outline toggle `IconButton` to `SessionToolbar.tsx` (only visible when session has 2+ chapters)

## Current Chapter Indicator in Toolbar
- [ ] Use `IntersectionObserver` on interaction containers to track which interaction is in view
- [ ] Map visible interaction to chapter via `interaction_id → chapter` lookup
- [ ] Display current chapter title as small `Typography` in `SessionToolbar.tsx`

## Session Sidebar Topic Chips
- [ ] On session list item hover in `SessionsSidebar.tsx`, show `Collapse` section with `title_history` chips (oldest→newest, max 4)
- [ ] If `TypesSessionSummary` lacks `title_history`: fetch full session lazily on first hover (debounced, cached in local state)
- [ ] Hide chips section entirely when session has no `title_history` entries

## Polish
- [ ] Don't show any topic UI (dividers, outline button, chips) for sessions with only 1 topic (no title changes)
- [ ] Verify all components handle empty/loading/error states gracefully
- [ ] Run `cd frontend && yarn build` and confirm no TypeScript errors
