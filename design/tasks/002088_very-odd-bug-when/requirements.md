# Requirements: Populate ProjectID and RepositoryIDs when Helix-OR worker starts its first session

## Background

When a Helix-OR (org-graph) worker is hired for the first time, the desktop
container it spins up never finishes initializing. The container is created,
but `helix-workspace-setup.sh` aborts because `HELIX_REPOSITORIES` is empty.
This leaves the marker file `/home/retro/.helix-setup-failed` in place, and
`start-zed-helix.sh` loops `Still waiting for setup...` forever.

### Root cause

`StartExternalAgentSession` in `api/pkg/server/session_handlers.go` constructs
a `DesktopAgent` at ~line 2474 with only:

```go
zedAgent := &types.DesktopAgent{
    OrganizationID: session.OrganizationID,
    SessionID:      session.ID,
    UserID:         userID,
    Input:          "Initialize Zed development environment",
    ProjectPath:    "workspace",
}
```

It does **not** populate `ProjectID`, `RepositoryIDs`, or `PrimaryRepositoryID`.

The hydra executor (`api/pkg/external-agent/hydra_executor.go:316-330`) only
emits `HELIX_REPOSITORIES` when `agent.RepositoryIDs` is non-empty, so the
desktop container starts with an empty env var and the setup script bails.

Two other call sites already do the correct lookup and can serve as the
template:
- `api/pkg/server/spec_task_design_review_handlers.go:967-983` (auto-wake / resume)
- `api/pkg/server/session_handlers.go:1991-2007` (exploratory resume)
- `api/pkg/services/spec_driven_task_service.go:602-635` (spec-task launch)

### Why auto-wake doesn't recover

`auto_wake_stuck_interactions.go` re-invokes `startDevContainerForSession`,
which *would* populate `RepositoryIDs`. But `hydra_executor.go:149` short-
circuits with `Dev container already running, returning existing session` —
so env vars on the running container are never updated. Only killing the
container forces a recreation with the corrected env.

## User Stories

### Story 1: Worker session starts on first try
**As** a user who has just hired a Helix-OR worker for a project with
attached git repositories,
**I want** the worker's first session to start a fully initialized desktop
container,
**so that** Zed comes up against the project's repos without me having to
manually kill the container or reload the page.

#### Acceptance Criteria
- WHEN a worker is hired against a project that has one or more git
  repositories attached,
  THEN the desktop container started for the worker's first session must
  have `HELIX_REPOSITORIES` set with all attached repos in
  `id:name:type` form.
- WHEN the worker's first session reaches `running` status,
  THEN `/home/retro/.helix-setup-failed` must not exist inside the container
  and `start-zed-helix.sh` must launch Zed instead of looping on the
  setup-waiting check.
- WHEN the project has a `default_repo_id` set,
  THEN that repo ID must be passed as `PrimaryRepositoryID` (used to set
  `HELIX_PRIMARY_REPO_NAME` for the Zed workspace).
- WHEN the project has no repositories attached,
  THEN the session must still be created successfully (no regression for
  projects without repos), matching today's behaviour.

### Story 2: Single helper covers all desktop-launch sites
**As** a maintainer of the session-handler code,
**I want** the repo-loading logic to live in one place,
**so that** future call sites can't silently drift back into the same bug.

#### Acceptance Criteria
- WHEN a future contributor adds a third desktop-launch entry point,
  THEN they can call a single helper to attach `ProjectID`,
  `RepositoryIDs`, and `PrimaryRepositoryID` to a `DesktopAgent`.
- WHEN the helper is invoked,
  THEN its behaviour must match what the three existing call sites do
  today, so swapping them over does not change observable behaviour
  (other than fixing the bug at the worker-hire site).
