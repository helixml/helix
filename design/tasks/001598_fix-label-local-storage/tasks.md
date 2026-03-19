# Implementation Tasks

## Code Changes

- [x] In `SpecTaskKanbanBoard.tsx`, compute `labelStorageKey` before the `labelFilter` state: `const labelStorageKey = projectId ? \`helix-label-filter-${projectId}\` : null;`
- [x] Replace the simple `useState<string[]>([])` for `labelFilter` (~line 640) with a lazy initializer that reads from `localStorage.getItem(labelStorageKey)`
- [x] Add a `useEffect` that syncs `labelFilter` to localStorage (set when non-empty, remove when empty), with `[labelFilter, labelStorageKey]` as dependencies
- [x] In `BacklogTableView.tsx`, remove the `labelStorageKey` const, the `labelFilter` state, and its `useEffect` (lines ~73–97)
- [x] In `BacklogTableView.tsx`, remove the `BacklogFilterBar` label filter props (`labelFilter`, `onLabelFilterChange`, `availableLabels`) and any related UI — or remove `BacklogFilterBar` entirely if label filtering was its only purpose
- [x] Build the frontend (`cd frontend && yarn build`) and confirm no TypeScript errors

## Manual QA

- [x] Open the browser and navigate to `http://localhost:8080`
- [x] Register an account (go to `/login`, click "Register here", use `test@helix.local` / `testpass123`)
- [x] Complete onboarding: create an org when prompted
- [x] Create a project
- [x] Create several spec tasks, adding different labels to each (e.g. "frontend", "backend", "bug")
- [x] Use the label filter dropdown in the kanban board toolbar to filter by one or more labels
- [x] Navigate away from the project page (e.g. go to another project or home)
- [x] Navigate back to the same project — confirm the label filter is still applied
- [x] Open Chrome DevTools → Application → Local Storage → `http://localhost:8080` and confirm the key `helix-label-filter-<projectId>` exists with the correct value
- [x] Clear the label filter — confirm the key is removed from Local Storage
