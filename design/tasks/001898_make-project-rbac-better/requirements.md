# Requirements: Make Project RBAC Better

## User Stories

### 1. Auto-add user to organisation when adding to project

**As** a project owner adding a user to my project,
**I want** the system to automatically add that user to the organisation if they're not already a member,
**So that** I don't have to manually navigate to the org settings first.

**Acceptance Criteria:**

- When creating a project access grant for a user who is not an org member, the backend automatically creates an org membership (role: `member`) before creating the grant.
- The API response includes metadata indicating that the user was auto-added to the organisation (e.g. `added_to_organization: true`).
- The frontend shows a notification/snackbar after the grant is created, informing the user: "{User} was also added to the organisation."
- The "Add User" dropdown must show **all org members**, not just those without existing grants — and additionally allow searching for users outside the org (by email) so the auto-add flow can be triggered.
- If the user is already an org member, behaviour is unchanged — no notification, no duplicate membership attempt.
- The acting user must have permission to add members to the org (org owner) for the auto-add to work. If they don't, the backend should return an error explaining that the user must first be added to the org by an org owner.

### 2. Exclude logged-in user from "Add User" dropdown

**As** a project owner managing access,
**I want** to not see myself in the list of users I can add to the project,
**So that** the list only shows people I can actually add.

**Acceptance Criteria:**

- The logged-in user (identified by `currentUser.id`) is filtered out of the user dropdown in the "Add User" dialog.
- This filtering happens in the frontend only — no backend changes needed.
- The project owner row (already shown separately at the top of the access list) remains visible.
