# Design: Sort Branch Dropdowns on New SpecTask Form

## Decision: Sort in the Frontend

Sort branches client-side in the `NewSpecTaskForm` component, right where `branchesData` is consumed.

**Why frontend, not backend?**
- The backend endpoint (`listGitRepositoryBranches`) is a general-purpose API used by multiple consumers. Adding sort there could affect other callers that may rely on the current order.
- Sorting a list of branch name strings is trivial client-side — no performance concern.
- The frontend already filters branches in one dropdown (removes default branch). Adding a `.sort()` in the same pipeline is the simplest fix.

## Implementation

In `NewSpecTaskForm.tsx`, sort `branchesData` alphabetically (case-insensitive) before rendering in both dropdown locations:

**Location 1 — "Base branch" dropdown (~line 735):**
```tsx
{branchesData
  ?.slice()
  .sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }))
  .map((branch: string) => (
    <MenuItem ...>
```

**Location 2 — "Select existing branch" dropdown (~line 797):**
```tsx
{branchesData
  ?.filter((branch: string) => branch !== defaultBranchName)
  .sort((a, b) => a.localeCompare(b, undefined, { sensitivity: 'base' }))
  .map((branch: string) => (
    <MenuItem ...>
```

## Codebase Notes

- **Frontend component:** `frontend/src/components/tasks/NewSpecTaskForm.tsx`
- **Branch data fetched via:** `useQuery` with `listGitRepositoryBranches` API call (lines 133-144)
- **Two branch dropdowns exist:** "Base branch" (line ~735) and "Select existing branch" (line ~797)
- **A separate `BranchSelect.tsx` component** exists at `frontend/src/components/git/BranchSelect.tsx` — it has the same unsorted issue but is out of scope for this task (only fixing the NewSpecTaskForm)
- **Backend handler:** `api/pkg/server/git_repository_handlers.go` — no changes needed
