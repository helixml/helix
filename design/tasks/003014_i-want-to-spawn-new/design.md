# Design: Grant Worker Secrets to Spawned Spec Tasks

## Two layers, per review

- **Grant layer — explicit, at creation.** `create_spectask` puts the pair
  `list_secrets`/`get_secret` onto the created task's EXISTING `agent_tools`
  list (jsonb `[]string`, already round-tripped through API/UI/sanitizer). No
  schema, no tool argument: the platform records the grant. Visible in
  `GET task`, the REST agent-tools view, tools/list. Un-armed tasks — including
  in Agent-owned projects — see nothing: no passive detection of who gets
  tools.
- **Authority layer — derivation at read time.** *Whose* bindings a call
  returns = the single Agent whose runtime home project is the task's project
  (fail-closed). Hidden magic is limited here, and it ends in loud errors.

## Current state (verified in code, `api/pkg/`)

- `workersecret.Binding` (org, worker, name) + `workersecrets.Service`:
  `Descriptors` (metadata only) / `Get` (fresh resolve, `Recorder` audit).
- Bot baseline carries the pair via `mcptools.BaseReadTools`; per-call value
  resolution; no env fallback by design.
- `mcptools.SpecTaskAgentTools` = spec-task tool catalogue;
  `server/mcp_backend_spectask.go` serves it at `/api/v1/mcp/helix-tasks`
  (caller `specTaskCaller{ID: taskID}`, `ActorSpecTask`, `ProjectPrincipal`
  with ProjectID on ctx; visible = `(project.AgentTools ∪ task.AgentTools) ∩
  catalogue`). `sanitizeAgentTools` (`agent_tools_handlers.go:34`) filters by
  the SAME catalogue predicate, and `specTaskAgentTools(ctx, session)` feeds
  the zed-config `AgentToolsRev` mount check (`zed_config_handlers.go:193/538`)
  and the REST view. **Adding the pair to the catalogue makes all of these flow
  off the stored grant with zero new logic.**
- Org MCP server audits every invocation: `newMCPAuditEntry(caller, toolName,
  args)` + success/failure (`org/interfaces/server/mcp.go:168+`) — creator
  provenance for "who armed this task" already exists as an audit fact
  (Bot actor on `create_spectask`); `workersecrets.Service` recorder covers the
  reads.
- Bot→own-project: bot nodes `orgStore.Nodes.List(orgID)` +
  `LoadState(b).ProjectID`; `Bot.ProjectIDs` is a *managed* allowlist — never
  ownership.
- SpecTask creation for org: `infrastructure/runtime/helix.SpecTasks.Create`
  (all MCP `create_spectask` callers pass through it — Bot or task-principal).

## Rejected (accumulated; full prose in requirements.md)

- **Silently surfacing tools for any task in an Agent-owned project** —
  round-5 rejection: hidden functionality; grants must be carried state.
- **New provenance column** — round-3 rejection (creator already in audit;
  grant fits an existing column). If display of the creator name on the row is
  ever wanted, that is exactly one column — kept out until requested.
- **Worker `helix-org` key/URL handed to the task** — full impersonation
  (`mcp_backend_helix_org.go:50` authorizes on the key session's
  `OrgWorkerID`).
- **`secret_grants` arg / boot env / copied bindings** — earlier rounds.

## Design

### 1. Catalogue membership (`mcptools/defaults.go`)

Add `GetSecretName`, `ListSecretsName` to `SpecTaskAgentTools`. Every existing
mechanism keys off catalogue membership: sanitize keeps the names,
`EffectiveAgentTools`+backend intersect let them through ONLY where present in
project/task lists, `len(specTaskTools) > 0` mounts `helix-tasks` for an
armed task with nothing else, `AgentToolsRev` flips on task change. No
special-case unions anywhere.

### 2. Grant at creation (`infrastructure/runtime/helix/spectasks.go` `Create`)

Append `runtime.SecretToolNames = []string{string(mcptools.GetSecretName),
string(mcptools.ListSecretsName)}`-equivalent literals (use the tool constants
where imports allow) to the new task's `AgentTools`, deduped, for **every**
port-level Create (Bot-spawned and task-spawned sub-tasks — both intentionally
armed), before persisting. REST/UI creation never stamps. This is the whole
"intentional" mechanism: one append in one funnel + a description note on
`create_spectask` ("grants this task `list_secrets`/`get_secret`, scoped to
your credentials at time of use").

### 3. Authority at invocation (`mcptools/get_secret.go`, `list_secrets.go`)

When `ProjectPrincipalFromContext(ctx)` is set (always for task callers, never
Bots): require the project principal's `ProjectID`; resolve the owning Agent via
a new `BoundAgentForProject(ctx, orgStore, orgID, projectID) (NodeID, error)`
in `infrastructure/runtime/helix` (`Nodes.List`→Bot filter→`LoadState` match on
home project; typed errors: none / ambiguous-with-WARN, both fail closed;
ownership only, never `ProjectIDs`); pass that id + `inv.Caller.OrganizationID()`
into the existing `workersecrets.Service`; audit `Recorder` still logs
`inv.Caller.ID()` (task) as actor. Surface grant check for cheap defense:
confirm the tool called is in the task's `AgentTools` (stale cache defense;
tools/list already governs). Unbound/no-tool → distinct clean errors; never
fall back to Bot semantics. Bot callers: unchanged code path.
(Deps seam: one `func(ctx, orgID, projectID) (NodeID, error)` closure wired in
`server/helix_org.go` keeps the interface package store-free, per its existing
pattern.)

### 4. Prompt (`services/spec_task_prompts.go` + `BuildAgentToolsSection` callers)

`BuildAgentToolsSection(projectTools, taskTools)` already receives the lists
that now may contain the pair. Filter `list_secrets`/`get_secret` from the
DELEGATION enumeration (so an armed-without-delegation task is never told it
can spawn), and append the dedicated hint section when those names are
present: "Your agent armed this task with credential tools — `list_secrets`
for names/usage, `get_secret` immediately before an authenticated operation."
Both services call sites (409, 646) get it automatically; no org-store access
needed in services. `types.EffectiveAgentTools` unchanged.

### 5. Guardrails & invariants

- Never write `Metadata.OrgWorkerID` on spec-task sessions; a test asserts the
  helix-org backend still 403s task keys while helix-tasks serves armed tasks.
- Manual REST edits of `agent_tools` can intentionally remove/re-add the pair;
  resolution stays project-owner-scoped, so a manual add is equivalent to the
  Bot stamp (project's own Agent secrets only).
- Armed-but-broken (agent fired / project unbound / secret revoked) fails at
  call with clear errors; stamp remains visible — loud, not silent.
- Sub-tasks get armed by stamping too; a sub-task in a project owned by agent P
  resolves to P (never the parent-task spawner's set) — credentials don't drift
  across projects.

### 6. Tests

- `helix/spectasks_test.go`: stamp on Bot + principal creates (dedup, order),
  absent for other writers; description/contract unchanged.
- mcptools: principal+bound → source resolution + audit actor=task; principal
  unbound → error; tool-on-unarmed-task defended; Bot path unchanged.
- `BoundAgentForProject`: 1/0/2, allowlist-only, human-node cases.
- server: `specTaskAgentTools` + helix-tasks mount/rev for armed-only tasks;
  REST view shows pair iff armed; sanitize keeps names (catalogue edit).
- services: delegation enumeration excludes pair; hint presence ≡ armed;
  unchanged for un-armed.
- Guardrail test (OrgWorkerID prohibition).

### 7. Out of scope

No schema/contract changes; no creator-name column (until asked); no cache of
`BoundAgentForProject` (orgs small; revisit only if measured); no UI; no
non-pair org tool exposure; no values in rows/env/prompts.
