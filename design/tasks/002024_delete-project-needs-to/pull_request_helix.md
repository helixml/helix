# fix(frontend): make project delete feel synchronous and refresh the list

## Summary

Deleting a project felt asynchronous: the confirmation dialog closed and the
user was navigated back to the projects list where the just-deleted project
was still visible. Clicking it and re-trying the delete then returned 404
because the backend had in fact already deleted it.

Root cause was **frontend-only**, not backend. The backend handler at
`api/pkg/server/project_handlers.go:753-833` is fully synchronous. The bug
was that `useDeleteProject.onSuccess` called
`queryClient.invalidateQueries({ queryKey: projectsListQueryKey() })`, which
produces `['projects', undefined]`. The live list pages
(`Projects.tsx`, `Home.tsx`, `Jobs.tsx`, `GitRepoDetail.tsx`,
`ProjectManagerSkill.tsx`) all query with the real org id
(`['projects', 'org_xxx']`), so the cache was never invalidated and the list
rendered stale data.

A secondary issue was that the MUI `<Dialog>` could be dismissed by clicking
the backdrop, pressing Escape, or hitting Cancel while the mutation was still
in flight — letting the user leave the dialog in a partial state.

## Changes

- `frontend/src/services/projectService.ts` — `useDeleteProject.onSuccess`
  is now `async` and `await`s invalidation of both `['projects']` (short
  prefix matches every per-org variant) and `pinnedProjectsQueryKey()`
  (so the sidebar's pinned-project list also refreshes). Awaiting in
  `onSuccess` means `mutateAsync` only resolves after the cache is settled.
- `frontend/src/pages/ProjectSettings.tsx` — the delete confirmation
  `<Dialog>` `onClose` callback is a no-op while
  `deleteProjectMutation.isPending` is true, and the **Cancel** button is
  `disabled` for the same condition. The existing **Delete** button already
  showed `<CircularProgress />` and `"Deleting…"`; no change needed there.

No backend changes. No new dependencies. No type changes — `tsc -b
tsconfig.json` is clean.

## Test plan

- [x] `npx tsc -b tsconfig.json` clean (full `yarn build` is blocked only by
  the `:ro` bind mount on `frontend/dist`).
- [x] Manual end-to-end in inner Helix (`http://localhost:8080`):
  register → create org `testorg` → create project `testproj-delete` →
  Project Settings → Danger Zone → type the name → **DELETE PROJECT**.
  Spinner stays on, Cancel is disabled, Escape doesn't dismiss the dialog,
  and the projects list shows "No projects yet" after the dialog closes.

## Screenshots

![Danger Zone tab](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002024_delete-project-needs-to/screenshots/01-danger-zone-tab.png)
![Confirmation dialog](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002024_delete-project-needs-to/screenshots/02-confirm-dialog.png)
![Deleting state — Cancel disabled, spinner running](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002024_delete-project-needs-to/screenshots/03-deleting-spinner.png)
![Projects list after delete — no stale entry](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002024_delete-project-needs-to/screenshots/04-after-delete-list-empty.png)
