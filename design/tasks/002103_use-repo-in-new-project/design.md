# Design: Fix Repo Not Preselected in "Use Repo in New Project" Modal

## Summary

A frontend-only fix. The bug is entirely in how `GitRepoDetail.tsx` loads the repository list it passes to `CreateProjectDialog`, plus an overly aggressive "lock" in the dialog when a repo is preselected. No backend or API changes are required.

## Key Findings (for future agents)

- `CreateProjectDialog` renders only repos present in its `repositories` prop, filtered to `repo_type !== 'internal'` (`CreateProjectDialog.tsx:227`). If the preselected id isn't in that list, the Select renders blank.
- The backend `ListGitRepositories` store query treats `owner_id` and `organization_id` as **separate columns** (`api/pkg/store/git_repository.go:75-80`). Org repos have `owner_id` = a user id and `organization_id` = the org id. Passing the org id as `owner_id` matches nothing.
- The canonical, correct way to load the repo list (proven by the working Projects page) is:
  ```ts
  useGitRepositories(
    currentOrg?.id
      ? { organizationId: currentOrg.id, enabled: isLoggedIn }
      : { ownerId: account.user?.id, enabled: isLoggedIn }
  )
  ```
  (`frontend/src/pages/Projects.tsx:149-153`)

## Changes

### 1. Load the repo list with the correct filter (the actual fix)

In `frontend/src/pages/GitRepoDetail.tsx` (~line 137, 157), replace the always-`ownerId` query with the org-aware pattern used by `Projects.tsx`:

- When `currentOrg?.id` is set → query with `{ organizationId: currentOrg.id }`.
- Otherwise → query with `{ ownerId: account.user?.id }`.

This ensures `allRepositories` actually contains org-owned repos, so the preselected repo (and all other org repos) appear in the dialog.

> Note: the existing `ownerId` variable is also used elsewhere in this file (e.g. create/link handlers). Do **not** change that variable's meaning; introduce the corrected query options separately so the only behavioural change is which repos are listed for the dialog. Verify no other consumer of `allRepositories` regresses.

### 2. Make preselection a default, not a lock (UX hardening)

In `frontend/src/components/project/CreateProjectDialog.tsx`:

- **Dropdown** (`:585`): change `disabled={reposLoading || !!preselectedRepoId}` → `disabled={reposLoading}`. Keep the preselected repo as the default `value`, but let the user change it (satisfies Story 2 and prevents a stuck state even in edge cases).
- **Mode tiles** (`:537,542-543,547,557`): allow switching repo mode even when a repo is preselected. The preselected repo still sets the initial mode to `'select'` (`:267-271`); removing the lock just lets the user pick a different mode/repo.

This second change is defensive: if the list is ever empty for any reason, the user is no longer trapped.

## Decisions & Rationale

- **Fix the data source, don't widen the dialog's lock logic.** The minimal correct fix is making the repo list correct. The lock-removal is an additional safeguard directly requested by the user ("no way to select any other repos").
- **Mirror `Projects.tsx` exactly** rather than inventing a new loading scheme — it's the known-good path and keeps the two flows consistent.
- **No backend change.** The store filter is correct; the frontend was sending the wrong parameter.

## Testing

End-to-end in the inner Helix (`http://localhost:8080`):
1. Register / log in, create an org, create a repo inside it.
2. Open the repo's detail page → click "Use in New Project".
3. Confirm the repo is preselected and the dropdown lists other org repos and is interactive.
4. Confirm project creation succeeds with the preselected repo.
5. Regression: create a project from the Projects page — still works.
6. (If feasible) repeat in a personal/no-org workspace.

Also run `cd frontend && yarn build` before committing.
