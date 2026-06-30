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

## Implementation Notes

- Implemented as a straight string replace `Human Desktop` → `Project Desktop`
  across the affected frontend files. All occurrences in `frontend/src` were
  identical, so a single `sed` per file covered both JSX/string literals and
  inline comments — leaving the codebase fully consistent (zero "Human Desktop"
  remaining in `frontend/src`).
- Files touched (10): `router.tsx`, `pages/TeamDesktopPage.tsx`,
  `pages/SpecTasksPage.tsx`, `components/tasks/TabsView.tsx`,
  `components/tasks/SpecTaskKanbanBoard.tsx`, `pages/HelixOrgWorkerDetail.tsx`,
  `services/workerChatSession.ts`, plus comment-only updates in
  `components/common/RobustPromptInput.tsx`, `RobustPromptInput.test.tsx`,
  `pages/HelixOrgWorkerDetail.test.tsx`.
- `frontend/src/api/api.ts` has a generated swagger comment ("Human desktop",
  lowercase d) — left untouched; it regenerates from backend via
  `./stack update_openapi`. Not user-facing.
- No functional identifiers changed (route names, component names, `desktopTitle`/
  `exploratorySessionId` fields stay) — confirmed none contained "human".
- Verification: `yarn tsc` exits 0; `vite build` transformed all 21,657 modules
  successfully (the only failure was an EACCES writing to the root-owned `dist/`
  bind mount — a sandbox FS artifact, not a code error). Live UI check was not
  possible: the inner Helix stack (:8080) would not boot in this session.

## Follow-up fix: instant Project Desktop navigation (no-feedback bug)

**Symptom:** clicking "Open/Resume Project Desktop" gave no immediate UI feedback
until the desktop had fully started — the user sat on the board watching only a
"Starting…" button label.

**Root cause:** `startExploratorySession` (`api/pkg/server/project_handlers.go`)
called `externalAgentExecutor.StartDesktop()` **synchronously in the HTTP request
path**. `StartDesktop` does the whole sandbox container launch (sandbox selection
+ hydra RevDial container creation), which takes several seconds, so the POST
blocked the whole time. This is the outlier — spec tasks already run `StartDesktop`
in a goroutine off the request path (see `SpecTaskOrchestrator` → `wg.Add(1); go
StartSpecGeneration`).

**Fix:**
- Backend: provision the desktop in a detached goroutine
  (`detachContext(r.Context(), 10m)` — the existing helper that preserves the
  authed user but drops request cancellation) and return the session row
  immediately. Applied to both the new-session and existing-session-restart
  branches. Endpoint latency went from multi-second to **~9ms**.
- Frontend (`SpecTasksPage.tsx`): `handleResumeExploratorySession` now navigates
  to the desktop page first (it already has the session id) and fires the resume
  mutation in the background, so the page transition is instant. The desktop page
  (`TeamDesktopPage` → `ExternalAgentDesktopViewer`) already renders a
  connecting/reconnecting state while the container boots.

**Tradeoff:** errors that `StartDesktop` used to surface synchronously (e.g.
"desktop limit reached") now manifest as the desktop never connecting rather than
an immediate error snackbar — the same model spec tasks already use. The session
endpoint is polled every ~5.3s so status still converges.

**Verified:** `go build ./pkg/server/` and `yarn tsc` pass; POST returns in 9ms
(was blocking); api logs show "Starting dev container via Hydra" firing *after*
the response returned; in-browser the URL flips to `/desktop/<id>` instantly on
click. Desktop *streaming* itself was NOT verifiable in this env — see below.

## Note: desktop "Reconnecting…" loop is an environment/DNS issue (not this change)

While testing, the desktop stream sat in "Reconnecting (attempt N)…". Root cause
is **DNS inside the nested dev sandbox**, unrelated to any code here: the
`desktop-bridge` in the `ubuntu-external-*` container cannot resolve the `api`
host — `lookup api on 10.214.0.1:53: i/o timeout / connection refused` — so its
RevDial control connection back to the API never establishes, and the API logs
`Failed to connect to sandbox via RevDial for stream WebSocket: "no connection"`.
The container is up and GNOME ScreenCast is healthy; only the API hostname
resolution from within the DinD network is failing. The UI reconnect loop is the
bridge correctly retrying. This is a dev-environment infra problem, not a Helix
code bug, and not fixable in the frontend.
