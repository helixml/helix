# Design

Two small, independent changes scoped to the spec-task kanban surface.

## Files touched
- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` — search state ↔ URL sync
- `frontend/src/components/tasks/TaskCard.tsx` — tooltip content (date line)

No backend, no API, no schema, no new dependencies.

## Change 1 — Search filter in URL

### Current state
`SpecTaskKanbanBoard.tsx:669` keeps search as local React state initialized from a `searchFilterProp`:

```tsx
const [searchFilter, setSearchFilter] = useState(searchFilterProp);
```

The parent (`SpecTasksPage.tsx`) does not pass `searchFilter` and never reads it back — the prop is effectively unused for URL purposes.

### New approach
Replace the `useState` with a router5-backed reader/writer, mirroring the existing `useUrlTab` pattern at `frontend/src/hooks/useUrlTab.ts`. We don't need a generic hook — a small inline reader + a debounced effect that calls `router.mergeParams({ search: value || undefined })` is sufficient and matches the codebase style.

Key decisions:
- **Param name:** `search` (short, matches placeholder text "Search tasks...", no collision with existing params: `tab`, `openTask`, `openDesktop`, `openReview`, `view`, `invite`, `new`, `id`).
- **Debounce ~250ms** before writing to the URL so each keystroke doesn't flood the router history. The visible input value updates instantly (controlled by local state); only the URL write is debounced.
- **Empty value clears the param** by passing `undefined` to `mergeParams` (router5 drops undefined params). Avoids leaving `?search=` in the URL.
- **Use `mergeParams`, not `window.history.replaceState`.** The comment at `useUrlTab.ts:14` documents a real bug — bypassing router5 corrupts the back-stack. This is the load-bearing constraint.
- **Initial value** comes from `router.params.search` (string) on mount; no need for a separate `useEffect` to sync inbound URL→state because router5 re-renders on URL change anyway, and the user is the only writer.

The `searchFilterProp` parameter on the component can be removed (callers don't pass it) — but to stay minimal, leave it as a fallback only if no URL param is present. Either is acceptable; prefer removing dead props if there are no external callers (a quick grep confirmed `SpecTasksPage.tsx:1138` does not pass it).

### Why not localStorage like label/assignee filters?
The user explicitly asked for *Back button* behaviour. localStorage survives navigation but doesn't restore on Back if other URL state changed (e.g. coming back from a task detail). URL params are the right primitive.

## Change 2 — Date in tooltip

### Current state
`TaskCard.tsx:766-789` renders:
```tsx
<Tooltip title={<span style={{ whiteSpace: "pre-wrap" }}>{task.description || task.name}</span>} ...>
```

### New approach
Tooltip body becomes two lines: a date line, then the existing content.

```tsx
<Tooltip
  title={
    <span style={{ whiteSpace: "pre-wrap" }}>
      {task.created_at ? `Created ${formatCreatedAt(task.created_at)}\n\n` : ""}
      {task.description || task.name}
    </span>
  }
  ...
>
```

Date formatting:
- Use native `Date` + `toLocaleString` — the file already establishes the pattern (`SpecTaskKanbanBoard.tsx:68` says "Removed date-fns dependency - using native JavaScript instead").
- Format: `new Date(task.created_at).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })` → e.g. `6 May 2026, 14:32` (locale-aware).
- Wrap in a `try/catch` or guard with `isNaN(d.getTime())` so a malformed `created_at` falls through to the existing `description || name` content.

Helper lives inline at the top of `TaskCard.tsx` (one small function, no new file).

## Risks / non-issues
- **URL noise on every keystroke** — mitigated by debounce; even without it, `mergeParams` uses replace-style routing (no new history entry per keystroke), so Back still works correctly. The debounce is a nice-to-have for cleanliness.
- **Other surfaces with their own search** — out of scope. `AgentKanbanBoard` has its own search input but the user request is specifically about spec tasks.
- **Tooltip content layout** — using `\n\n` inside `whiteSpace: "pre-wrap"` gives a visible blank line between date and prompt, matching the request "show the creation date *before* the prompt".

## Patterns / learnings worth recording
- **Router5 `mergeParams` is the project's blessed way to write URL state.** Never `window.history.replaceState` — there's a documented bug at `useUrlTab.ts:14` about it corrupting the back-stack.
- **No date-fns** in this codebase — use native `Date.toLocaleString` (`SpecTaskKanbanBoard.tsx:68`, `AgentKanbanBoard.tsx:73`).
- **Search/filter pattern** — the codebase uses `matchesAllTokens()` from `utils/searchUtils.ts` for tokenized AND-search; this change keeps that intact.
- **Existing URL params on the spec-tasks route:** `tab`, `openTask`, `openDesktop`, `openReview`, `view`, `invite`, `new`, `id` (see `SpecTasksPage.tsx:151-155`). Add `search` alongside without conflict.
