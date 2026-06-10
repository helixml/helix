# Design: Populate ProjectID and RepositoryIDs when Helix-OR worker starts its first session

## Summary

Fix the missing project/repo wiring in `StartExternalAgentSession` by
attaching `ProjectID`, `RepositoryIDs`, and `PrimaryRepositoryID` to the
`DesktopAgent` before calling `externalAgentExecutor.StartDesktop`. Extract
the duplicated lookup into a single helper so all three call sites use the
same code path.

## Files Involved

| File | Role |
|------|------|
| `api/pkg/server/session_handlers.go` | **Bug site** at ~line 2474 (StartExternalAgentSession). Also already does the same lookup at ~line 1991 (exploratory resume). |
| `api/pkg/server/spec_task_design_review_handlers.go` | Same lookup at ~line 967 (startDevContainerForSession). |
| `api/pkg/services/spec_driven_task_service.go` | Same agent-population logic at ~line 522-535 (spec-task launch). Different shape — caller already loaded `projectRepos` at line 506 because `SyncBaseBranchForTask` needs them. Reuses the **pure** helper but not the fetching wrapper. |
| `api/pkg/types/external_agent.go` (or wherever `DesktopAgent` is defined) | Receives the new `(*DesktopAgent).SetRepoContext` method — pure, no I/O, callable from any package. |
| `api/pkg/external-agent/hydra_executor.go` | Consumer at ~line 316-330. No change. |
| `api/pkg/types/external_agent.go` (or wherever `DesktopAgent` lives) | No change — the helper only writes to existing fields. |

## Helper Design

Two layers — a **pure** method on `*types.DesktopAgent` (callable from any
package, no I/O) and a thin **server-side wrapper** that does the store
lookups. This lets all three call sites converge on the same agent-population
logic without forcing the spec-task service to do redundant DB fetches.

### Layer 1 — pure method on `*types.DesktopAgent`

Lives next to the `DesktopAgent` type definition (likely
`api/pkg/types/external_agent.go`; confirm during implementation):

```go
// SetRepoContext populates RepositoryIDs and PrimaryRepositoryID from the
// given project repos. defaultRepoID is the project's preferred repo
// (typically project.DefaultRepoID); when empty the first repo wins.
//
// No-op when repos is empty — leaves both fields untouched so callers can
// pre-set or skip safely.
func (a *DesktopAgent) SetRepoContext(repos []*GitRepository, defaultRepoID string) {
    if len(repos) == 0 {
        return
    }
    a.RepositoryIDs = make([]string, 0, len(repos))
    for _, repo := range repos {
        if repo.ID != "" {
            a.RepositoryIDs = append(a.RepositoryIDs, repo.ID)
        }
    }
    if defaultRepoID != "" {
        a.PrimaryRepositoryID = defaultRepoID
    } else if len(a.RepositoryIDs) > 0 {
        a.PrimaryRepositoryID = a.RepositoryIDs[0]
    }
}
```

### Layer 2 — server-side wrapper

Lives in `session_handlers.go` (or a sibling `agent_project_context.go`):

```go
// attachProjectContext loads the project's repos and sets ProjectID,
// RepositoryIDs, and PrimaryRepositoryID on agent. Safe to call when
// projectID is empty (no-op).
func (s *HelixAPIServer) attachProjectContext(ctx context.Context, agent *types.DesktopAgent, projectID string) error {
    if projectID == "" {
        return nil
    }
    agent.ProjectID = projectID

    repos, err := s.Store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{ProjectID: projectID})
    if err != nil {
        return fmt.Errorf("list git repositories for project %s: %w", projectID, err)
    }
    if len(repos) == 0 {
        return nil
    }

    var defaultRepoID string
    if project, err := s.Store.GetProject(ctx, projectID); err == nil && project != nil {
        defaultRepoID = project.DefaultRepoID
    }
    agent.SetRepoContext(repos, defaultRepoID)
    return nil
}
```

## Call-Site Changes

### `StartExternalAgentSession` (the bug)

At `session_handlers.go:2474`, change:

```go
zedAgent := &types.DesktopAgent{
    OrganizationID: session.OrganizationID,
    SessionID:      session.ID,
    UserID:         userID,
    Input:          "Initialize Zed development environment",
    ProjectPath:    "workspace",
}
```

to:

```go
zedAgent := &types.DesktopAgent{
    OrganizationID: session.OrganizationID,
    SessionID:      session.ID,
    UserID:         userID,
    Input:          "Initialize Zed development environment",
    ProjectPath:    "workspace",
}
if err := s.attachProjectContext(ctx, zedAgent, session.ProjectID); err != nil {
    return nil, fmt.Errorf("attach project context: %w", err)
}
```

### `spec_task_design_review_handlers.go:967-983`

Replace the inline `ListGitRepositories` + `GetProject` block with a single
`s.attachProjectContext(ctx, agent, agent.ProjectID)` call. Preserve the
pre-existing `agent.ProjectID` derivation just above it (lines 945-956);
the helper only writes `agent.ProjectID` when it's empty input — that's
fine because we already set it via the helper's arg.

### `session_handlers.go:1991-2007` (exploratory resume)

Same replacement. Note the existing block also fetches `project` separately
for an error-return on missing project; the helper logs+continues instead.
Keep the existing `GetProject` error-check (which returns 500 if the project
disappeared) — it's a separate concern from repo loading. After it, call the
helper to populate repos.

### `spec_driven_task_service.go:522-535`

The caller already has `projectRepos` and `project` in hand (fetched at
line 506 because `SyncBaseBranchForTask` needs them). Replace the local
`repositoryIDs` slice build (522-528) and `primaryRepoID` derivation
(530-535) with the pure helper, and remove `RepositoryIDs` /
`PrimaryRepositoryID` from the `DesktopAgent` literal at line 602:

```go
zedAgent := &types.DesktopAgent{
    OrganizationID: orgID,
    SessionID:      session.ID,
    UserID:         task.CreatedBy,
    Input:          "Initialize Zed development environment for spec generation",
    ProjectID:      task.ProjectID,
    ProjectPath:    "workspace",
    SpecTaskID:     task.ID,
    // RepositoryIDs / PrimaryRepositoryID set by SetRepoContext below.
    DisplayWidth:        displayWidth,
    DisplayHeight:       displayHeight,
    // ... rest unchanged ...
}
zedAgent.SetRepoContext(projectRepos, project.DefaultRepoID)
```

The pre-existing `repositoryIDs` / `primaryRepoID` local variables become
dead and should be removed. `SyncBaseBranchForTask` continues to receive
`projectRepos` directly — no change there.

## Why Not Fix It in Hydra

Could `hydra_executor.go` look up repos from `session.ProjectID` itself when
`agent.RepositoryIDs` is empty? No:

1. Hydra is the consumer, not the source of truth — it doesn't know whether
   "empty repos" means "project really has none" or "caller forgot".
2. Two of the three call sites already do the lookup correctly; pushing it
   into hydra would duplicate behaviour and create a fallback path that
   masks future bugs at new call sites.
3. The existing short-circuit at `hydra_executor.go:149` ("Dev container
   already running, returning existing session") is the right behaviour —
   we don't want hydra silently rebuilding env vars on a running container.
   The fix belongs at the caller.

## Alternatives Considered

- **Inline the fix at the bug site only, don't extract a helper.**
  Rejected: the user request explicitly calls out the duplication, and the
  same bug class is one copy-paste away every time someone adds a new
  desktop-launch entry point.
- **Single helper that always re-fetches from the store.**
  Rejected: the spec-task service path already loads `projectRepos` for
  `SyncBaseBranchForTask`. Forcing it through a fetching helper would
  double the DB calls or require threading the cached repos through as an
  optional parameter — ugly either way. Splitting into a pure
  agent-population method + a server-side fetching wrapper is cleaner.
- **Make `attachProjectContext` a free function instead of a method.**
  Rejected: it needs `s.Store`, and the existing pattern in this package
  uses methods on `*HelixAPIServer`. (The pure `SetRepoContext` *is* a
  free-of-server method, which is why it sits on `*DesktopAgent` itself.)
- **Have the helper fail loudly when the project has no repos.**
  Rejected: projects without repos are legal today (e.g., exploratory
  chat-only sessions). Changing that is a separate decision.

## Operational Workaround (Not Part of This Task)

For sessions already stuck in this state (container running with empty
`HELIX_REPOSITORIES`):

```bash
docker compose -f docker-compose.dev.yaml exec -T sandbox-nvidia \
    docker rm -f ubuntu-external-<sessionID-without-ses_-prefix>
```

Then reload the desktop page. The next auto-wake invocation will call
`startDevContainerForSession`, which already loads repos correctly, and the
recreated container will have the right env. **This workaround is not part
of the code change** — it's only relevant for unblocking the user-reported
session before the fix ships.

## Risks

- **Helper called from a context where `s.Store` is nil.** All current
  call sites already use `s.Store`, so this is no new risk.
- **`ListGitRepositories` returns repos with empty `ID`.** Existing code
  filters these out (`if repo.ID != ""`); the helper preserves that.
- **`session.ProjectID` is empty for legitimate sessions.** Helper is a
  no-op in that case — same as today.

## Notes for Future Implementers

- The desktop image's `helix-workspace-setup.sh` is what reads
  `HELIX_REPOSITORIES`. Don't be tempted to add a fallback in that script —
  the contract is "if the env var is missing, the caller messed up", and
  failing loudly with the marker file is the correct behaviour.
- The auto-wake loop (`auto_wake_stuck_interactions.go`) is a safety net,
  not a fix. Don't rely on it to mask broken first-start paths.
- When adding a new desktop-launch site, search for `DesktopAgent{` and
  decide which helper applies:
  - Server-package site with only a `projectID` in hand → call
    `s.attachProjectContext(ctx, agent, projectID)`.
  - Site that already loaded `[]*types.GitRepository` for another reason →
    call `agent.SetRepoContext(repos, project.DefaultRepoID)` directly and
    set `agent.ProjectID` yourself.
  In both cases: build the struct, call the helper, hand off to the executor.
