# Implementation Tasks

- [ ] In `SpecTaskKanbanBoard.tsx`, compute `labelStorageKey` before the `labelFilter` state: `const labelStorageKey = projectId ? \`helix-label-filter-${projectId}\` : null;`
- [ ] Replace the simple `useState<string[]>([])` for `labelFilter` (line ~640) with a lazy initializer that reads from `localStorage.getItem(labelStorageKey)`
- [ ] Add a `useEffect` that syncs `labelFilter` changes to localStorage (set when non-empty, remove when empty), with `[labelFilter, labelStorageKey]` as dependencies
- [ ] Build the frontend (`cd frontend && yarn build`) and verify no TypeScript errors
- [ ] Test in browser: select a label filter on a project page, navigate away, return — confirm the filter is restored and the key `helix-label-filter-<projectId>` appears in Chrome DevTools → Application → Local Storage
