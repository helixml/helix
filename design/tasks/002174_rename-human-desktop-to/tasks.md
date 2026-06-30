# Implementation Tasks: Rename "Human Desktop" to "Project Desktop"

- [~] `router.tsx`: change desktop route `meta.title` to "Project Desktop"
- [~] `TeamDesktopPage.tsx`: change breadcrumb text to "Project Desktop"
- [~] `SpecTasksPage.tsx`: update snackbar messages (started/resumed/failed) to "Project Desktop"
- [~] `SpecTasksPage.tsx`: update button labels to "Open/Resume/View Project Desktop"
- [~] `TabsView.tsx`: update default tab titles, ListItem primary, and `desktopTitle` fallback to "Project Desktop"
- [~] `SpecTaskKanbanBoard.tsx`: update tooltip copy to reference "Project Desktop"
- [~] `HelixOrgWorkerDetail.tsx`: update user-facing copy referring to "Project Desktop session"
- [~] `workerChatSession.ts`: update error message to "failed to open Project Desktop session"
- [~] (Optional) Update frontend inline comments mentioning "Human Desktop" for coherence
- [ ] Run `cd frontend && yarn build` to confirm no breakage
- [ ] Re-grep `frontend/src` for "Human Desktop" — confirm no user-facing strings remain
- [ ] Visual check in inner Helix: breadcrumb, spec-task buttons, and kanban tooltip read "Project Desktop"
