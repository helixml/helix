# Implementation Tasks: Synchronous Project Delete with Visible Spinner

- [~] In `frontend/src/services/projectService.ts:103-105`, update
      `useDeleteProject` `onSuccess` to `async` and `await Promise.all([…])`
      invalidating `['projects']` and `['pinned-projects']`.
- [ ] In `frontend/src/pages/ProjectSettings.tsx:2022-2025`, gate the dialog's
      `onClose` callback so it is a no-op when
      `deleteProjectMutation.isPending` is true.
- [ ] In `frontend/src/pages/ProjectSettings.tsx:2061-2068`, add
      `disabled={deleteProjectMutation.isPending}` to the **Cancel** button.
- [ ] Run `cd frontend && yarn build` and fix any type errors.
- [ ] Manually verify the scenario in `design.md`'s "Manual Test Plan" against
      the inner Helix at `http://localhost:8080` (register `test@helix.ml` /
      `helixtest`, create org, create a project, delete it, confirm it is gone
      from the projects list, sidebar, and pinned list).
- [ ] Manually verify the error path by temporarily stopping the API container
      (`docker compose -f docker-compose.dev.yaml stop api`) and confirming
      the dialog stays open with a "Failed to delete project" toast.
- [ ] Commit using conventional format, e.g.
      `fix(frontend): make project delete synchronous and refresh list`,
      and push.
- [ ] Open PR against `helixml/helix` `main`; paste full URL into the spec
      task; watch CI (`gh pr checks <num>`) until green.
