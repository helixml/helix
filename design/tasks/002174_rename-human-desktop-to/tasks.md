# Implementation Tasks: Rename "Human Desktop" to "Project Desktop"

- [x] `router.tsx`: change desktop route `meta.title` to "Project Desktop"
- [x] `TeamDesktopPage.tsx`: change breadcrumb text to "Project Desktop"
- [x] `SpecTasksPage.tsx`: update snackbar messages (started/resumed/failed) to "Project Desktop"
- [x] `SpecTasksPage.tsx`: update button labels to "Open/Resume/View Project Desktop"
- [x] `TabsView.tsx`: update default tab titles, ListItem primary, and `desktopTitle` fallback to "Project Desktop"
- [x] `SpecTaskKanbanBoard.tsx`: update tooltip copy to reference "Project Desktop"
- [x] `HelixOrgWorkerDetail.tsx`: update user-facing copy referring to "Project Desktop session"
- [x] `workerChatSession.ts`: update error message to "failed to open Project Desktop session"
- [x] (Optional) Update frontend inline comments mentioning "Human Desktop" for coherence (incl. RobustPromptInput + test files)
- [x] Run `yarn tsc` (type-check passes) + `yarn build` (all 21657 modules transform; only a root-owned `dist/` bind-mount permission error remains, unrelated to code)
- [x] Re-grep `frontend/src` for "Human Desktop" — confirmed none remaining
- [x] Visual check in inner Helix (stack came up after waiting): registered → testorg → testproj → created a task → on the project board confirmed live DOM has **0 "Human Desktop"** and renders "Open Project Desktop" + "Resume Project Desktop" buttons and the kanban tooltip "…The Project Desktop is for exploring the codebase and testing your app." Screenshot in `screenshots/02-open-project-desktop-button.png`.
- [x] Merge latest `origin/main` into feature branch (clean, no conflicts) and push `feature/002174-rename-human-desktop-to`
- [x] Write PR description (`pull_request_helix.md`)
