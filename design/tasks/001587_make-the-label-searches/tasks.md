# Implementation Tasks

- [x] Add `projectId?: string` prop to `BacklogTableView` component interface
- [x] Replace `useState<string[]>([])` for `labelFilter` with a lazy initializer that reads from `localStorage.getItem(\`helix-label-filter-${projectId}\`)` when `projectId` is defined
- [x] Add `useEffect` to sync `labelFilter` to localStorage on change (remove key when array is empty, set JSON otherwise)
- [x] Pass `projectId` from `SpecTasksPage` to `BacklogTableView`
- [x] Check `SpecTaskKanbanBoard.tsx` — if it also renders `BacklogTableView`, pass `projectId` there too
