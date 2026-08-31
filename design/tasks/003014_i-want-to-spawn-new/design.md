# Design: Give Spawned Spec Tasks Their Agent's Tool Access

## Product rule (confirmed by review, final)

An armed task sees **its Agent's live tool set** — the whole org MCP surface
the bound Agent holds (which normally includes `list_secrets`/`get_secret`,
`chat`, events, etc.) on top of the task's own spec-task tools. Supersedes the
earlier secrets-pair scoping. Blast radius = the Agent's own surface by
construction (task lives in the Agent's project, acts with the Agent's grants);
the delta vs the Agent's desktop is strictly positive: calls are attributed to
the specific task (`ActorSpecTask`), never hidden inside the Bot.

## Current state (verified in code, `api/pkg/`)

- **Surface mechanics are already catalogue/string driven.**
  `server/mcp_backend_spectask.go` serves `/api/v1/mcp/helix-tasks` for task
  keys (`specTaskCaller{ID: taskID}`, `ActorSpecTask`, `ProjectPrincipal` on
  ctx) with tools = `(project.AgentTools ∪ task.AgentTools) ∩
  mcptools.SpecTaskAgentTools`; `types.EffectiveAgentTools` unions;
  `sanitizeAgentTools` (`agent_tools_handlers.go:34`) filters names through
  `IsSpecTaskAgentTool`. `specTaskAgentTools(ctx, session)`
  (`agent_tools_handlers.go:54`, session→task→project, nil for non-org) feeds
  BOTH `GenerateZedMCPConfig` zed-config sites (`zed_config_handlers.go:193/538`
  → `ContextServers["helix-tasks"]` mounted when `len(specTaskTools) > 0`, URL
  cache-busted via `AgentToolsRev`) and the REST view (`:71`). Planning prompt
  section: `services.BuildAgentToolsSection(project.AgentTools,
  task.AgentTools)` (`spec_driven_task_service.go:409`,
  `agent_instruction_service.go:646`).
- **Agent tool state is live and queryable**: `Bot.Tools` is the Bot's live MCP
  surface (reconciler prunes unknown names); org bots list via
  `orgStore.Nodes.List(orgID)`; bot home project via `LoadState(b).ProjectID`
  (`infrastructure/runtime/helix`, state.go); `Bot.ProjectIDs` is a managed
  allowlist — never ownership.
- **Every org MCP invocation is audited** with caller + args
  (`org/interfaces/server/mcp.go:168+`), so the whole "who did what as which
  task" trail exists for free once tasks call the same in-process registry.
- Spec-task callers ALREADY execute org tools through this registry today
  (the spec-task set) — the machinery for a project-principal `tool.Caller`
  exists; what changes is which set is served and how "acts-as-caller" tools
  interpret a delegated caller.

## Design

### 1. Arm marker: `"*"` in the existing `agent_tools` (no schema)

`create_spectask` → `helix.SpecTasks.Create` (the one port funnel for every org
spawn path, Bot or task principal) appends `"*"` to the new task's
`AgentTools` if absent. `"*"` is not a registry name, so existing
sanitize/intersect logic naturally drops it from *tool resolution*; the two
surface choke points special-case it in the raw list before sanitizing.
Admin can add/remove `"*"` via `agent_tools` REST update (same field the UI
already edits). This is the round-5 "intentional, recorded at creation,
visible on the task" requirement, generalized: the row literally says the task
runs with all of its agent's tools.

### 2. Two org helpers (new, `infrastructure/runtime/helix`)

```go
// single Agent whose LoadState home project == projectID; typed ErrNoBoundAgent
// at 0 or ambiguous (>1, WARN). Ownership only — never Bot.ProjectIDs.
func BoundAgentForProject(ctx, st *store.Store, orgID, projectID string) (orgchart.NodeID, error)
// live tool names of an Agent node (Nodes.Get → Node.Tools), nil-safe.
func AgentToolNames(ctx, st *store.Store, orgID string, nodeID orgchart.NodeID) ([]tool.Name, error)
```

Wired as small closures where needed (composition root `server/helix_org.go`
pattern) so `mcptools` stays store-free.

### 3. Surface = own tools ∪ armed(live agent tools)

At the choke points (keep ONE formula, implement identically):

```
own   = EffectiveAgentTools(project.AgentTools, task.AgentTools \ {"*"}) ∩ SpecTaskAgentTools
armed = task.AgentTools contains "*": AgentToolNames(BoundAgentForProject(project)) else ∅
surface = union(own, armed)   // deduped, order-stable for rev stability
```

- `server/agent_tools_handlers.go::specTaskAgentTools` — add the armed branch
  (org handles reachable from the server via the org-handler composition). This
  single change flows to mount + `AgentToolsRev` (so an Agent tool edit flips
  the task's rev at the task's next zed-config fetch = next start/resume, the
  normal context-server refresh point) and to the REST view; sanitization of
  `own` is unchanged, `armed` names are already reconciled-live registry names.
- `server/mcp_backend_spectask.go::eligibleSpecTaskTools` — same formula so
  served tools/list == rev input == view. Served via the existing
  `ServeMCPForCaller(w, r, caller, tools)` — registry owns handlers; every
  `agent_tools` name is a registered tool by construction (reconciler pruned).
- Unarmed ⇒ formula = today exactly (no passive surfacing; the Agent-owned
  project without a stamp sees nothing).

### 4. Delegation identity: on-behalf-of bound Agent (the risky part — audit per tool)

Backend stashes the resolved `NodeID` on ctx
(`runtime.WithBoundWorker` + `BoundWorkerFromContext`, sibling of
`ProjectPrincipal` in `infrastructure/runtime/principal.go`). Add one helper
used by tools: `tool.SubjectForCaller(ctx, caller) NodeID` = bound worker for
ProjectPrincipal callers, else `caller.ID()` (Bots unchanged). Tools resolve
their subject through it for anything keyed on caller identity — streams
`chat` posts as the Agent, `managers`/`reports` walk the Agent's line,
`ask_human`/`dm` originate from the Agent, secret reads resolve the Agent's
bindings (same helper — replaces the round-4 secrets-only branch).
Mandate: **every** tool reachable via `Bot.Tools` is classified + tested as:
(1) on-behalf-of (subject → bound Agent), (2) task-scoped (the spec-task set —
keys to `caller.ID()` = task id), or (3) unsupported-for-delegated-callers —
must return a clean error with zero side effects (e.g. anything requiring the
caller to be a live desktop node if no sensible bound-Subject behavior exists;
`bot_log` of "me" = the bound Agent's log). Fail closed, never half-work:
implementation PR must enumerate the catalogue and mark each tool (table in the
PR description), with at least the representative tests below.

Audit stays honest regardless: org audit actor = task id (`specTaskCaller`),
`Bot.Tools`-scoped set = live Agent grants, worker-secret recorder still logs
task-actor reads. Cross-org is enforced upstream (project's org).

### 5. Prompt (services, no org-store access)

`BuildAgentToolsSection`: strip `"*"` and non-delegation names from the
delegation enumeration (as designed in round 4); append a distinct section when
raw `task.AgentTools` contains `"*"`: "This task runs with the tools granted to
its agent through the Helix MCP channel (see the tool list); credentials:
`list_secrets` → names, `get_secret` → value." Both existing services call
sites pick it up via the shared builder; tools/list is the live enumeration.

### 6. Edge semantics

- Bound Agent fired/unhomed: armed branch → ∅ + task keeps its own set; org
  calls via stale cached list return clean unbound errors; task fully usable.
- Agent gains/loses tools: new names ride live; rev flips ⇒ agent restarts the
  context server on next desktop start/resume; mid-session Zed keeps its cached
  list until then (documented tool-caching semantics, same as all MCP).
- Sub-task spawn by a task principal: child is `"*"`-armed by the same funnel;
  child surface = child project's owner (never the parent's Agent set when
  projects differ — credentials/tools don't smuggle sideways).
- Manual `"*"` admin edit = same semantics; removal disarms on next refresh.

### 7. Guardrail

Never `Metadata.OrgWorkerID` on spec-task sessions (tests both backends).

## Tests (minimum)

- `helix`: stamp "*" on every port Create (Bot + principal), dedupe against
  existing entries, REST writers unaffected; `BoundAgentForProject` 0/1/2/
  allowlist-only/human; `AgentToolNames` nil-safe.
- server: surface formula — unarmed ≡ today; armed+bound = own ∪ live, nothing
  else; armed+unbound = own only; rev flips on simulated Agent tool-set change;
  REST view ≡ tools/list ≡ rev input; `"*"` never passes sanitization.
- mcptools: `SubjectForCaller` (Bot → itself; principal+bound → Agent;
  principal unbound → error); `get_secret`/`list_secrets` on-behalf read +
  audit actor = task; representatives of each identity class (e.g. `chat`
  as Agent, `managers` Agent line, one unsupported-class tool errors loudly
  with zero DB writes).
- services: section gates & enumerations (armed no-delegation → identity hint
  only, never delegation text; "*" excluded).
- integration (inner Helix, in tasks.md).

## Out of scope

No schema/contract changes; no creator-name column (offer stands); no live
push to running sessions (rev-at-refresh only); no UI; no values in rows/env;
no caching of bound-agent lookups until measured.
