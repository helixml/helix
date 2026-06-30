# Rename "Human Desktop" to "Project Desktop"

## Summary
Renames the user-facing label for the per-project exploratory desktop/chat
session from "Human Desktop" to "Project Desktop". The new name better describes
what it is (one desktop per project) and reads more naturally alongside the
per-task "Agent Desktop". This is a label-only change — no functional
identifiers, routes, API fields, or behaviour change.

## Changes
- `router.tsx`: desktop route `meta.title` → "Project Desktop".
- `pages/TeamDesktopPage.tsx`: breadcrumb label → "Project Desktop".
- `pages/SpecTasksPage.tsx`: snackbar messages and "Open/Resume/View" button
  labels → "Project Desktop".
- `components/tasks/TabsView.tsx`: default tab titles, list item, and
  `desktopTitle` fallback → "Project Desktop".
- `components/tasks/SpecTaskKanbanBoard.tsx`: tooltip copy → "Project Desktop".
- `pages/HelixOrgWorkerDetail.tsx`: worker session copy → "Project Desktop".
- `services/workerChatSession.ts`: error message → "failed to open Project
  Desktop session".
- Comment-only consistency updates in `RobustPromptInput.tsx` and two test files.

## Notes
- `api/.../api.ts` generated swagger comment left untouched (regenerates via
  `./stack update_openapi`).
- Verified: `yarn tsc` passes; `vite build` transforms all modules cleanly.

## Screenshots
Verified live in the inner Helix — the project board toolbar shows "Open Project Desktop":

![Open Project Desktop button](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002174_rename-human-desktop-to/screenshots/02-open-project-desktop-button.png)
