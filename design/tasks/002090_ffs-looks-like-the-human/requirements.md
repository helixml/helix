# Requirements: Worker Activation Must Reuse the Project's Human Desktop Session

## Background

A "Human Desktop" is a Helix project's long-lived `session_role="exploratory"`
session — one per project — surfaced to the operator through the
"Open / Resume / View Human Desktop" buttons in the SpecTasks topbar
(`frontend/src/pages/SpecTasksPage.tsx:948-1010`).

The UI decides which button to show by GETting
`/api/v1/projects/{id}/exploratory-session` and reading
`session.config.external_agent_status`:

| `external_agent_status` | Button shown |
|---|---|
| (no session row) | Open Human Desktop |
| `stopped` | Resume Human Desktop |
| `running` / `starting` | View Human Desktop + Stop |

The server (`api/pkg/server/project_handlers.go:1253-1313`,
`getProjectExploratorySession`) computes that status by:

1. Loading the project's exploratory session row from Postgres:
   ```sql
   SELECT * FROM sessions
   WHERE config->>'project_id' = $1
     AND config->>'session_role' = 'exploratory'
   ORDER BY created DESC
   LIMIT 1;
   ```
2. Calling `externalAgentExecutor.GetSession(session.ID)` on the in-memory
   hydra session map. If found → `"running"`. If not found → `"stopped"`.

The hydra map is keyed by `agent.SessionID` (`hydra_executor.go:498`:
`h.sessions[agent.SessionID] = session`). So "is the project running?"
collapses to one question: *does the latest exploratory session row's ID
match the SessionID some recent `StartDesktop` registered?*

## What broke (the regression Phil hit)

Two code paths create sessions with `session_role="exploratory"` and the
same `project_id`:

1. **`startExploratorySession`** (`project_handlers.go:1327`) — fires
   when the user clicks "Open Human Desktop". Generates a fresh session
   ID, persists it, calls `StartDesktop`.
2. **`StartExternalAgentSession`** via the helix-org spawner
   (`session_handlers.go:2421` ← `helix_org_inproc.go:470`
   `inProcHelixClient.StartSession` ← `EnsureAndSend` ←
   `SpawnerConfig.ensureSession` at `spawner.go:430`) — fires when the
   helix-org runtime activates a Worker whose persisted
   `WorkerRuntimeState.SessionID` is empty.

Both paths mint a brand-new `system.GenerateSessionID()` and write a
fresh row, with **no check that an exploratory row already exists for
that `project_id`**. So a workflow like:

- (a) Hire a Worker → activation runs → exploratory session **A** created
  (state.SessionID=A persisted, desktop container A registered in
  `h.sessions`).
- (b) Operator clicks **Open Human Desktop** → `startExploratorySession`
  doesn't see state.SessionID (it doesn't know about it), looks up the
  project's exploratory row, finds A, restarts container A. Still A.
  Project shows **running**. ✅
- (c) Time passes, container A is reaped for idleness. Operator clicks
  **Resume Human Desktop** → frontend GETs the exploratory session
  (still A), POSTs `/sessions/A/resume` → container A restarts,
  registered in `h.sessions[A]`. Project shows **running**. ✅

…works *as long as nothing creates a second exploratory row in between*.
But the contract has no enforcement against that: a second
`StartExternalAgentSession` invocation (any path that calls it without
first checking the project's existing exploratory session) writes row
**B**, `ORDER BY created DESC LIMIT 1` now returns B, the operator's
resumed container at session A is no longer the one the project status
query checks, and the UI shows **Resume Human Desktop / stopped** even
though there's a live container behind the recent resume click.

A specific trigger that demonstrably hits this in production: a worker
auto-spawn or manual re-activation that runs **after** the operator has
already created a Human Desktop session via the UI but **before** the
spawner's `WorkerRuntimeState.SessionID` has been re-loaded. The spawner
takes the in-memory `state.SessionID==""` branch, calls `StartSession`,
gets a fresh session B, persists it. Now the project has rows A and B.

## User Stories

1. **As an operator**, when I click "Resume Human Desktop" and the
   resume succeeds, the project status pill must flip to **running**
   (and the button to **View Human Desktop**) on the very next status
   refetch — not stay stuck on **Resume Human Desktop**.

2. **As an operator**, when a worker is activated by the helix-org
   runtime (manual `/workers/{id}/activate`, cron trigger, dispatched
   event), the activation must reuse the project's existing exploratory
   session if one is already present. It must not silently mint a
   parallel session that orphans the operator-facing one.

3. **As a maintainer**, there must be a regression test that fails on
   the current code: build a project with an exploratory session already
   in the DB at row A, run the worker-activation `StartSession` path,
   and assert that `GetProjectExploratorySession` still returns A (not a
   freshly-created B).

## Acceptance Criteria

### A. Behavioural contract

- **One exploratory session per project, period.** After any combination
  of `startExploratorySession` and worker activation runs against the
  same project, `SELECT COUNT(*) FROM sessions WHERE config->>'project_id'=$1
  AND config->>'session_role'='exploratory'` returns at most 1.
- **Resume → running, observably.** Calling
  `POST /api/v1/sessions/{exploratorySessionID}/resume` followed by
  `GET /api/v1/projects/{projectID}/exploratory-session` returns a
  session whose `config.external_agent_status == "running"`. (Today this
  is the happy path; the test pins it as a regression gate.)
- **Activation reuses Resume's session id.** When `startExploratorySession`
  creates session A and then the worker activation pipeline runs for the
  worker that owns that project, the activation must call `StartDesktop`
  with `agent.SessionID = A` — *not* a new id. Equivalently: the
  spawner's `state.SessionID` is set to A and stays A across activations.

### B. Test that pins the contract (TDD-first)

A new Go test added before the fix lands, failing on `main` and passing
after the fix:

- File: `api/pkg/server/exploratory_session_activation_test.go` (or a
  parallel suite in `api/pkg/org/infrastructure/runtime/helix/`,
  whichever has the lighter wiring — see design.md).
- Setup: create a project P, call `startExploratorySession(P)` → returns
  session A; assert one row in the exploratory query.
- Action: invoke the worker-activation path that today calls
  `StartExternalAgentSession` (or, at the spawner-test level, an
  `EnsureAndSend` with `state.SessionID==""` for project P).
- Assertions:
  1. The exploratory-session count for P is still 1 (no parallel B).
  2. `GetProjectExploratorySession(P).ID == A`.
  3. The session id the activation registered against
     `externalAgentExecutor` (or its test fake) equals A.
  4. After a `resumeSession` against A, the project status read by
     `getProjectExploratorySession` is `"running"`.

The test must fail on `main` with a message that names the second row
(`expected 1 exploratory row for project P, got 2`) so future
re-introductions of the bug surface the same way.

### C. Manual end-to-end check

Operator-facing repro to run after the unit test goes green:

1. Open the inner Helix at `http://localhost:8080`, complete onboarding,
   create project `testproj`.
2. Click **Open Human Desktop**. Wait until the button reads **View
   Human Desktop**. Note the session id in the URL.
3. Hire any AI Worker against `testproj`. Wait for its activation to
   land (status moves through `provisioning → ready`).
4. Reload the SpecTasks page. The status must still read **View Human
   Desktop / running** — *not* **Resume Human Desktop / stopped**.
5. DB cross-check (one row, not two):
   ```bash
   docker exec helix-postgres-1 psql -U postgres -d postgres -c \
     "SELECT id, created FROM sessions
      WHERE config->>'project_id' = '<projectID>'
        AND config->>'session_role' = 'exploratory'
      ORDER BY created DESC;"
   ```
   Expect exactly one row.

## Out of Scope

- Changes to the frontend status pill / button transitions — the bug
  is server-side; the UI reflects whatever
  `getProjectExploratorySession` returns.
- Reworking the `session_role` taxonomy or splitting "human exploratory"
  from "worker exploratory" into distinct roles. The two are deliberately
  the same role today (`session_role="exploratory"` was chosen so worker
  activations are discoverable via `GetProjectExploratorySession`; see
  `helix_org_inproc.go:468-469`). The fix is "don't create a second
  one", not "give them different roles".
- Multi-user concurrency hardening beyond the single-project
  single-process case. If two API replicas race to mint exploratory rows
  for the same project, a CAS-style guard analogous to commit
  `2e2009d05` (`SetPlanningSessionIDIfEmpty`) may be needed later — flag
  as follow-up, do not block this fix on it.
- Resume-while-spec-task-active interactions. Spec tasks use their own
  `planning_session_id` / `agent_session_id` claim path (task 002080's
  `SetPlanningSessionIDIfEmpty`); that's a separate guard and is not
  re-implemented here.

## Related History

- Task **002089** (`b8cb8e230`): republish role/identity on every
  activation. Made activation more eager about touching the worker's
  per-Worker repo — does **not** touch session row creation, but is
  worth re-reading because it's the most recent change in the same
  spawner pipeline.
- Task **002080**-era commit `2e2009d05`: atomic claim of
  `planning_session_id` to prevent duplicate desktops for spec tasks.
  Pattern (CAS + delete-on-loser) is the prior art if a future hardening
  pass extends this fix to the multi-process case.
- Commit `1c8ad6fc7` (2026-04-24): set `Metadata.ProjectID` so desktop
  resume works. Reading that diff is the fastest way to understand why
  `config->>'project_id'` is the query key, not the top-level
  `project_id` column.
