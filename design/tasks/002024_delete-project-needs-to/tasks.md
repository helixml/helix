# Implementation Tasks: Synchronous Project Delete with Visible Spinner

- [x] In `frontend/src/services/projectService.ts:103-105`, update
      `useDeleteProject` `onSuccess` to `async` and `await Promise.all([…])`
      invalidating `['projects']` and `['pinned-projects']`.
- [x] In `frontend/src/pages/ProjectSettings.tsx:2022-2025`, gate the dialog's
      `onClose` callback so it is a no-op when
      `deleteProjectMutation.isPending` is true.
- [x] In `frontend/src/pages/ProjectSettings.tsx:2061-2068`, add
      `disabled={deleteProjectMutation.isPending}` to the **Cancel** button.
- [x] Run `cd frontend && yarn build` and fix any type errors. (Used
      `npx tsc -b tsconfig.json`; the full `yarn build` writes into the
      read-only `frontend/dist:/www:ro` bind mount and fails on permissions,
      but the TypeScript compile is clean and Vite already transformed all
      21104 modules. Dev mode is active — `.env` has no `FRONTEND_URL=`, so
      Vite HMR in `helix-frontend-1` picks up the changes live.)
- [x] Manually verify the scenario in `design.md`'s "Manual Test Plan" against
      the inner Helix at `http://localhost:8080`. Verified end-to-end:
      registered `test@helix.ml`, created `testorg` and `testproj-delete`,
      opened Project Settings → Danger Zone, typed the name, clicked
      **DELETE PROJECT**. Observed: spinner + "DELETING…", **CANCEL** went
      to `disabled` state (a11y snapshot confirms `disabled` attribute),
      pressing Escape did not dismiss the dialog while pending. Once the
      mutation resolved the dialog closed and the projects list showed
      "No projects yet" — the cache invalidation fix is working
      (previously the stale entry was still rendered). Screenshots in
      `screenshots/`.
- [-] ~~Manually verify the error path by temporarily stopping the API
      container.~~ **Skipped:** stopping the shared API container would
      disrupt other in-flight spec-task agents in this dev environment. The
      change is frontend-only and the `catch` branch in
      `handleDeleteProject` is unchanged (still calls
      `snackbar.error("Failed to delete project")` and leaves the dialog
      open). The new `onClose` gating only blocks dismissal while `isPending`
      is true; on `mutateAsync` rejection `isPending` returns to `false`, so
      the user regains full control of the dialog.
- [x] Push the feature branch
      (`origin/feature/002024-synchronous-project`, commit `32faa59c9`).
