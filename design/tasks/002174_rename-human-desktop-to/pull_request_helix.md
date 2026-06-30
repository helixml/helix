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

## Also: instant Project Desktop navigation
Fixes the lack of UI feedback when opening/resuming the Project Desktop. The
exploratory-session POST blocked on the full container launch before returning;
it now provisions the desktop in a background goroutine and returns the session
row immediately (matching the spec-task pattern). Endpoint latency dropped from
multi-second to ~9ms, and the UI jumps straight to the desktop page (which shows
its own connecting state). Resume navigates optimistically using the known
session id.

- `api/pkg/server/project_handlers.go`: async `StartDesktop` via `detachContext` for both new and restart paths.
- `frontend/src/pages/SpecTasksPage.tsx`: resume navigates first, resumes in background.

## Also: fix nested desktop streaming regression (Reconnecting loop)
`sandbox/05-start-dns-proxy.sh` hard-coded the bridge gateway `10.213.0.1`, only
valid at docker nesting depth 1. Since #2641 desktop containers get their DNS set
to the depth-aware gateway `10.(212+DEPTH).0.1`; in nested (helix-in-helix,
depth≥2) sandboxes the proxy fatally failed to bind `10.213.0.1`, so desktops
couldn't resolve `api`, RevDial never connected, and the stream looped on
"Reconnecting" (in-desktop `git clone` also failed). The script now derives the
gateway from `HELIX_DOCKER_DEPTH` identically to `04-start-dockerd.sh` and the Go
`sandboxDNSGateway`. Requires `./stack build-sandbox` to deploy.

![Desktop streaming after fix](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002174_rename-human-desktop-to/screenshots/04-desktop-stream-fixed.png)
