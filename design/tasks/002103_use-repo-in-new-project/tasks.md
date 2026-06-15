# Implementation Tasks: Fix Repo Not Preselected in "Use Repo in New Project" Modal

- [x] In `frontend/src/pages/GitRepoDetail.tsx`, load the repo list passed to `CreateProjectDialog` org-aware: use `{ organizationId: currentOrg.id }` when in an org, else `{ ownerId: account.user?.id }` (mirror `Projects.tsx:149-153`).
- [x] Verify the existing `ownerId` variable is not broken: it became unused after the change (handlers use `currentOrg?.id` directly), so removed the dead declaration. `ownerSlug` is untouched.
- [~] In `frontend/src/components/project/CreateProjectDialog.tsx`, remove the `!!preselectedRepoId` condition from the repository Select's `disabled` prop (`:585`) so the dropdown stays interactive with the repo as default.
- [~] In `frontend/src/components/project/CreateProjectDialog.tsx`, unlock the repo-mode tiles when a repo is preselected (`:537,542-543,547,557`) so the user can switch repo/mode.
- [ ] Run `cd frontend && yarn build` to confirm the build passes.
- [ ] Test end-to-end in the inner Helix: from a repo page in an org, click "Use in New Project" → repo is preselected, dropdown is interactive and lists other repos, project creation succeeds.
- [ ] Regression test: create a project from the Projects page (still works) and, if feasible, repeat the repo-page flow in a personal (no-org) workspace.
