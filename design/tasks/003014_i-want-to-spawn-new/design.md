# Design: Grant Worker Secrets to Spawned Spec Tasks

## Current state (verified in code, `api/pkg/`)

- **Worker secrets.** Bindings keyed `(OrganizationID, WorkerID, Name)` —
  `domain/workersecret/binding.go`; sources `helix_secret` / `connected_account`.
  `workersecrets.Service` gives `Descriptors` (metadata, never values) and `Get`
  (fresh resolve + `Recorder` audit). Grants at
  `/api/v1/orgs/{org}/agents/{id}/secrets`.
- **Bot tools.** `list_secrets`/`get_secret` are in `mcptools.BaseReadTools` —
  every Bot has them regardless of bindings; values resolve at call time. There
  is deliberately *no boot env-var fallback* (comment in `mcptools/defaults.go`).
- **Bot↔project mapping already exists.** Each hired Bot's runtime state
  carries its project (`LoadState` → `WorkerState.ProjectID`,
  `infrastructure/runtime/helix/state.go`); bot nodes list per org via
  `orgStore.Nodes.List(orgID)`. Own-project is 1:1 by construction; the node's
  `ProjectIDs` list is a *managed* allowlist, distinct from ownership.
- **Spec-task → org MCP channel.** `server/mcp_backend_spectask.go` serves the
  tool registry at `/api/v1/mcp/helix-tasks` (session-scoped key → session →
  `Metadata.SpecTaskID` → task → project; caller `specTaskCaller{ID: task.ID}`,
  `ActorSpecTask`; `runtime.ProjectPrincipal` rides ctx). Visible tools =
  `(project.AgentTools ∪ task.AgentTools) ∩ mcptools.SpecTaskAgentTools`.
- **One choke point already computes the task's tool surface for the sandbox.**
  `HelixAPIServer.specTaskAgentTools(ctx, session)`
  (`server/agent_tools_handlers.go:54`) does session → task → project, returns
  nil for non-org projects, and feeds BOTH `GenerateZedMCPConfig` call sites
  (`zed_config_handlers.go:193/538`, which mount `ContextServers["helix-tasks"]`
  with `rev=AgentToolsRev(...)` when non-empty) plus the REST view. The planning
  prompt gets its section from `types.EffectiveAgentTools(project.AgentTools,
  task.AgentTools)` in `services.BuildAgentToolsSection` — called from
  SpecDrivenTaskService with `project`/`task` in hand
  (`spec_driven_task_service.go:409`, `agent_instruction_service.go:646`).
- **Task creation** (`infrastructure/runtime/helix/spectasks.go`) needs nothing
  new — see Rejected alternatives.

## Decision: derive the bound Agent from existing state; the pair is "just an addition"

`SecretsWorkerID` columns and create-path provenance plumbing are **rejected**
(review feedback): store nothing; derive `project → bound Agent` on each request
from the bot runtime-state mapping. A task sees a project's Agent bindings
because its project is that Agent's; when the Agent is gone, the bond is gone.
The origin event (who spawned, via which API) is deliberately not recorded —
creation path must not matter (a UI task in a bot project gets the same pair).

Rejected alternatives (beyond the column): hand the task the Worker's own
`helix-org` key/URL = full impersonation (that backend authorizes on the key's
session's `OrgWorkerID`; `mcp_backend_helix_org.go:50`) with the entire bot
surface; per-task `secret_grants` args = ceremony; boot env vars / copying
bindings = stale, drift, values on disk. See requirements.md.

## Design

### 1. Derivation helper (new, ~40 lines, no schema)

In `infrastructure/runtime/helix` (owns `LoadState` + `store.Store`):

```go
// BoundAgentForProject returns the single org Agent that owns projectID as its
// own runtime project. ErrNoBoundAgent at 0 or >1 matches (ambiguous = fail
// closed). Ownership means LoadState(b).ProjectID == projectID only — never a
// managed entry in b.ProjectIDs (else a manager Bot's org-wide credentials leak
// into every project it supervises).
func BoundAgentForProject(ctx context.Context, st *store.Store, orgID, projectID string) (orgchart.NodeID, error)
```

Implementation: `Nodes.List(orgID)`, filter to Bot-kind, `LoadState` each
(per-bot KV get — orgs are small; no cache in v1, add one only if measurement
shows it hot), count matches. Deterministic error, never a panic, on ambiguous
(>1) with a WARN log.

### 2. Resolution for the tools (no new `mcptools.Deps` seam)

`mcp_backend_spectask.go` already loads project + task per request. After the
existing checks, call `BoundAgentForProject`; on success stash the id on the
request ctx with a tiny sibling of `WithProjectPrincipal`,
e.g. `runtime.WithBoundWorker(ctx, nodeID)` /
`runtime.BoundWorkerFromContext(ctx)` in `infrastructure/runtime/principal.go`.

`get_secret.go` / `list_secrets.go` `Invoke`: when `ProjectPrincipalFromContext`
is present → require `BoundWorkerFromContext`; resolve the value/descriptors
through the existing `workersecrets.Service` with **that** worker id; absent →
clean "this project is not bound to a helix-org agent" error. Audit
`Recorder` keeps attributing to `inv.Caller.ID()` (the task). Bot callers
unchanged (no principal). Failures of `workersecrets.Service` (fired worker →
its `nodes.Get` misses) already surface as ordinary errors — task unaffected.

### 3. Surface gating at the existing choke points (additive only)

- `specTaskAgentTools` (server): after project/nil logic, when
  `BoundAgentForProject` resolves, append `list_secrets`/`get_secret` after
  `EffectiveAgentTools`, deduped — this single change flows to both zed-config
  call sites (mount + correct rev flip when a binding is granted/removed;
  in-flight desktops refresh at next config fetch — same lifecycle as every
  context server), which also keeps the REST agent-tools view honest. It needs
  the org store: reachable in the server package where the org MCP handlers are
  assembled (`helix_org.go` wiring); pass/store the `*store.Store` accordingly.
- `mcp_backend_spectask.go` `eligibleSpecTaskTools`: same helper-derived
  addition so tools/list equals the rev input.
- Provenance-less/unbound tasks: pair never appears; the helper only ever adds.
- Do NOT compute the pair from `task.AgentTools`/project allowlist names —
  stale allowlist entries must not surface the pair for an unbound project.

### 4. Prompt hint — own gate, no false delegation (server→services wiring)

`SpecDrivenTaskService` (call sites at `spec_driven_task_service.go:409`,
`agent_instruction_service.go:646`) gains one injected func, wired at the server
composition root over the same `BoundAgentForProject`:
`BoundAgentForProjectFn(ctx, orgID, projectID) bool` (error → false). When true
(both call sites have `task.OrganizationID` + `task.ProjectID` already), append
a dedicated fragment: "This project is operated by a helix-org agent; this
task may use its granted credentials. Call `list_secrets` for names/usage,
`get_secret` immediately before an authenticated operation." The existing
"Delegating to other spec tasks" section is untouched and must keep rendering
from CRUD tools alone — a bound task with no delegation tools sees the credential
hint and nothing about delegation. No values.

### 5. Prohibitions (guardrail)

Never write `Metadata.OrgWorkerID` onto a spec-task planning session: that is
the authorization flag of the `helix-org` backend; a spec task must reach secrets
only through the `helix-tasks` backend + `BoundWorkerFromContext` path. A
session with both flags indicates an impersonation bug. Add a narrow assertion/
test if convenient.

### 6. Tests

- `helix` package: `BoundAgentForProject` — 1 bound → hit; 0 → typed error;
  2 → fail-closed error (+ log); managed-allowlist-only match does NOT count;
  human nodes ignored.
- `mcptools`: get_secret/list_secrets with principal+bound-worker ctx → source
  worker resolution + task-actor audit; principal without bound worker → clean
  error; bot path untouched.
- `server`: `specTaskAgentTools` adds pair iff bound (and mounts helix-tasks
  for a bound task with empty tools list); `mcp_backend_spectask` tools/list =
  rev input = prior surface ∪ pair, nothing removed; unbound project + stale
  allowed names → pair absent.
- Services: prompt shows hint with bound fn true, not false; delegation section
  unchanged across tool combos.
- Integration/e2e (inner Helix): bot-owned project → spawned AND UI-created
  tasks see the pair and can read; a second (manager) bot managing that project
  without owning leaks nothing; unbound org project's task: no pair, helix-tasks
  unmounted when it had no other tools; fire the bot → pair gone after restart,
  task usable; full UI lifecycle on an unbound task with zero changes.

### 7. Out of scope

- No schema, no contract, no task-creation changes; no origin/creation-path
  tracking.
- No cache invalidation machinery (per-request derivation; config refresh covers
  desktops).
- No org tool beyond the pair; no values in rows/logs/prompts/env; no
  inheritance/relation flags; no UI.
