# Implementation Tasks

## Code Changes

- [~] In `SpecTaskKanbanBoard.tsx`, compute `labelStorageKey` before the `labelFilter` state: `const labelStorageKey = projectId ? \`helix-label-filter-${projectId}\` : null;`
- [~] Replace the simple `useState<string[]>([])` for `labelFilter` (~line 640) with a lazy initializer that reads from `localStorage.getItem(labelStorageKey)`
- [~] Add a `useEffect` that syncs `labelFilter` to localStorage (set when non-empty, remove when empty), with `[labelFilter, labelStorageKey]` as dependencies
- [~] In `BacklogTableView.tsx`, remove the `labelStorageKey` const, the `labelFilter` state, and its `useEffect` (lines ~73–97)
- [~] In `BacklogTableView.tsx`, remove the `BacklogFilterBar` label filter props (`labelFilter`, `onLabelFilterChange`, `availableLabels`) and any related UI — or remove `BacklogFilterBar` entirely if label filtering was its only purpose
- [~] Build the frontend (`cd frontend && yarn build`) and confirm no TypeScript errors

## Manual QA

- [ ] Open the browser and navigate to `http://localhost:8080`
- [ ] Register an account (go to `/login`, click "Register here", use `test@helix.local` / `testpass123`)
- [ ] Complete onboarding: create an org when prompted
- [ ] Create a project
- [ ] Create several spec tasks, adding different labels to each (e.g. "frontend", "backend", "bug")
- [ ] Use the label filter dropdown in the kanban board toolbar to filter by one or more labels
- [ ] Navigate away from the project page (e.g. go to another project or home)
- [ ] Navigate back to the same project — confirm the label filter is still applied
- [ ] Open Chrome DevTools → Application → Local Storage → `http://localhost:8080` and confirm the key `helix-label-filter-<projectId>` exists with the correct value
- [ ] Clear the label filter — confirm the key is removed from Local Storage
