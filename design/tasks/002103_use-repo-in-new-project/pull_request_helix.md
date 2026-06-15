# fix(frontend): preselect repo in "Use Repo in New Project" modal

## Summary

Clicking **"Use in New Project" / "Use in Another Project"** on a git repository's detail page opened the Create Project dialog with **no repo selected and no way to pick one** — leaving the user stuck. Creating a project from the Projects page worked fine.

Root cause: `GitRepoDetail.tsx` loaded the dialog's repository list with `useGitRepositories({ ownerId })`, passing the **org id as `owner_id`**. The backend filters `owner_id` and `organization_id` as separate columns, and org repos are keyed by `organization_id` — so the query returned an empty list. With no repos in the list, the preselected repo wasn't an available option (the disabled `Select` rendered blank) and the locked mode tiles offered no escape.

The Projects page worked because it correctly queries by `organizationId` when in an org.

## Changes

- **`frontend/src/pages/GitRepoDetail.tsx`**: load the repo list org-aware — `{ organizationId: currentOrg.id }` in an org, else `{ ownerId: account.user?.id }` (mirrors `Projects.tsx`). Removed the now-unused `ownerId` local.
- **`frontend/src/components/project/CreateProjectDialog.tsx`**: stop disabling the repository dropdown and locking the repo-mode tiles when a repo is preselected. The repo is still preselected as the default (via the existing effect), but the user can now change the repository or switch repo mode — preventing a stuck state even in edge cases.

## Testing

- `yarn tsc -b` passes (exit 0). (`yarn build`/vite only fails writing to the root-owned `dist` bind mount in the dev env — not a code error.)
- Verified end-to-end in the inner Helix (org context): the repo is now preselected, the dropdown is interactive, and the mode tiles switch correctly.

## Screenshots

![Repo preselected in modal](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002103_use-repo-in-new-project/screenshots/01-modal-repo-preselected.png)
![Dropdown is interactive](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002103_use-repo-in-new-project/screenshots/02-dropdown-interactive.png)
![Mode tiles unlocked](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002103_use-repo-in-new-project/screenshots/03-tiles-unlocked.png)
