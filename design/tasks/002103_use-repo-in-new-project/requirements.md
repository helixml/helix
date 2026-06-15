# Requirements: Fix Repo Not Preselected in "Use Repo in New Project" Modal

## Background

On a git repository's detail page there is a **"Use in New Project"** / **"Use in Another Project"** button. Clicking it opens the *Create Project* dialog (`CreateProjectDialog`) with the current repo intended to be preselected.

In practice, when the user is working inside an **organisation**, the dialog opens with:
- the repo **not** selected (the dropdown shows nothing / "No repositories available"), and
- **no way to select any other repo** (the dropdown is disabled and the repo-mode tiles are locked).

So the user is stuck and cannot create the project.

Creating a project from scratch on the **Projects page** works fine — the same repo is found and selectable there. The difference is in how each page loads the list of repositories that populates the dialog.

## Root Cause

`CreateProjectDialog` only displays repos that exist in the `repositories` array it is handed. It also disables the dropdown and locks the mode tiles whenever a `preselectedRepoId` is supplied.

- **Projects page (works):** loads repos with `organizationId` when in an org, else `ownerId`.
  (`frontend/src/pages/Projects.tsx:149-153`)
- **Git repo detail page (broken):** always loads repos with `ownerId`, and sets `ownerId = currentOrg?.id || account.user?.id`.
  (`frontend/src/pages/GitRepoDetail.tsx:137,157`)

When in an org, the org id is sent as the **`owner_id`** query param. The backend store filters `git_repositories.owner_id = ?` (`api/pkg/store/git_repository.go:75-76`), but org-owned repos have `owner_id` = the *creating user's* id and `organization_id` = the org id. The filter therefore returns an **empty list**.

Result: the dialog's repository list is empty, the preselected repo isn't among the options (so the disabled Select renders blank), and because it's disabled the user cannot pick anything else.

## User Stories

### Story 1: Launch a new project from a repo page (org context)
**As** a user viewing a git repository inside an organisation
**When** I click "Use in New Project" / "Use in Another Project"
**Then** the Create Project dialog opens with that repository already selected, so I can name the project and continue.

**Acceptance Criteria:**
- [ ] The repository list passed to the dialog contains the current repo when in an org context.
- [ ] The current repo is preselected (shown as the chosen repository) in the dialog.
- [ ] The flow also works in a personal (non-org) workspace.

### Story 2: Choose a different repository if desired
**As** a user who opened the Create Project dialog from a repo page
**When** the dialog is open with the repo preselected
**Then** I can still change the selection to another available repository (or another repo mode), rather than being locked in.

**Acceptance Criteria:**
- [ ] The repository dropdown is interactive (not disabled) when a repo is preselected; the preselected repo is the default value.
- [ ] The user can select any other available repository from the dropdown.
- [ ] No regression to the working Projects-page flow.
