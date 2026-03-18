# Implementation Tasks

- [ ] Add `projectId?: string` prop to `BacklogTableView` component interface
- [ ] Replace `useState<string[]>([])` for `labelFilter` with a lazy initializer that reads from `localStorage.getItem(\`helix-label-filter-${projectId}\`)` when `projectId` is defined
- [ ] Add `useEffect` to sync `labelFilter` to localStorage on change (remove key when array is empty, set JSON otherwise)
- [ ] Pass `projectId` from `SpecTasksPage` to `BacklogTableView`
- [ ] Check `SpecTaskKanbanBoard.tsx` — if it also renders `BacklogTableView`, pass `projectId` there too
