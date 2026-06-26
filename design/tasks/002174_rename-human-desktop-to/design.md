# Design: Rename "Human Desktop" to "Project Desktop"

## Overview

A find-and-replace of the user-facing string **"Human Desktop"** → **"Project
Desktop"** across the frontend. There is no architecture to change — the desktop
session feature already exists and works; only its display label changes.

## Key Decision: labels only, not identifiers

The codebase already separates the *concept* (a per-project exploratory session)
from its *label*. The concept is implemented via stable identifiers that do NOT
contain "human":

- Route name: `org_project-team-desktop` / `project-team-desktop`
- Page component: `TeamDesktopPage` (`frontend/src/pages/TeamDesktopPage.tsx`)
- API/types fields: `exploratorySessionId`, `desktopTitle`

Because the identifiers are already neutral, the rename touches **only string
literals shown to users**. We deliberately leave identifiers untouched to avoid
breaking deep links, saved routes, and API compatibility.

## Affected Files (frontend, user-facing strings)

Found via `grep -rn "Human Desktop" frontend/src`:

| File | What to change |
|------|----------------|
| `frontend/src/router.tsx` | `title: 'Human Desktop'` → `'Project Desktop'` (route meta) |
| `frontend/src/pages/TeamDesktopPage.tsx` | Breadcrumb `<Typography>Human Desktop</Typography>` |
| `frontend/src/pages/SpecTasksPage.tsx` | Snackbars ("started"/"resumed"/"Failed to start"/"Failed to resume"/"Failed to resume Human Desktop") and button labels ("Open/Resume/View Human Desktop") |
| `frontend/src/components/tasks/TabsView.tsx` | Default tab/title fallbacks (`tab.desktopTitle \|\| "Human Desktop"`), ListItem `primary="Human Desktop"`, and `desktopTitle: title \|\| "Human Desktop"` default |
| `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` | Tooltip copy describing the desktop |
| `frontend/src/pages/HelixOrgWorkerDetail.tsx` | User-facing copy referring to the "Human Desktop session" |
| `frontend/src/services/workerChatSession.ts` | Error message `'failed to open Human Desktop session'` |

Note: `frontend/src/api/api.ts` contains a generated comment mentioning "Human
desktop" — it is auto-generated from backend swagger and should not be hand-edited
(regenerated via `./stack update_openapi` if the backend comment is updated).

## Optional: backend comments / tests

These contain "Human Desktop" only in comments / test names (no functional code):
`api/pkg/types/types.go`, `api/pkg/server/session_handlers.go`,
`api/pkg/server/*_test.go`, `api/pkg/org/...`, `design/2026-06-09-...md`.

Recommendation: update inline comments for coherence; leave Go test function
names (e.g. `TestHumanSurfaceNoOp`) as-is unless the rename is trivial, since the
underlying concept ("a human is present, no auto-restart") is still accurate.

## Implementation Approach

1. Replace user-facing literals file-by-file (review each match — some are
   fallback defaults, some are JSX text, some are toast strings).
2. Keep capitalisation/case consistent with surrounding labels ("Project Desktop").
3. Run `cd frontend && yarn build` to confirm no breakage.
4. Re-grep `frontend/src` to confirm no user-facing "Human Desktop" remains.

## Testing

- `yarn build` passes.
- Manual/visual check (inner Helix): the desktop breadcrumb, the spec-tasks page
  buttons, and the kanban tooltip all read "Project Desktop".
