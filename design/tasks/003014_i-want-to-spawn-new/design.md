# Design: Give Spawned Spec Tasks Their Agent's Tool Access

## Decision

The org MCP "endpoint for the task's Agent" already exists: the task's own
`/api/v1/mcp/helix-tasks` channel (session-scoped key → session → task →
project). Per final review direction, the sandbox config logic does nothing new
except mount that channel when there is an entitled tool set; the server decides
the set:

```
serve(task) = sanitize(EffectiveAgentTools(project∪task allowlists) ∩ SpecTaskAgentTools)
            ∪ live(Bot.Tools of BoundAgentForProject(task.project))          // ∅ if none
```

No stamps, no markers on task rows, no special tool names, no prompt plumbing,
no "armed" vocabulary. Client behavior = "add the MCP URL when it has
something to serve"; entitlement = server-side derivation from state the org
already maintains. Rounds 5–6's wildcard-stamp mechanism is explicitly
**reverted** (design preserved in git history; see requirements.md).

## Current state (verified in code, `api/pkg/`)

- `server/mcp_backend_spectask.go`: serves the registry to task keys
  (`specTaskCaller{ID: taskID}`, `AuditActorType()==ActorSpecTask`,
  `runtime.ProjectPrincipal{ProjectID, ActingUserID}` on ctx) with tools =
  `(project∪task)∩SpecTaskAgentTools` and forwards to
  `mcpServer.ServeMCPForCaller(w, r, caller, tools)` — the same in-process
  registry Bots use.
- `specTaskAgentTools(ctx, session)` (`server/agent_tools_handlers.go:54`):
  session→task→project, nil for non-org projects; feeds both
  `GenerateZedMCPConfig` sites (`zed_config_handlers.go:193/538`). Mount rule in
  `external-agent/zed_config.go`: `ContextServers["helix-tasks"]` exists iff
  `len(specTaskTools) > 0`, URL carries `AgentToolsRev(specTaskTools)`; Zed
  refreshes tools/list when the URL changes. REST view: same helper.
  `sanitizeAgentTools` (`:34`) keeps only `IsSpecTaskAgentTool` names.
- Prompt: `services.BuildAgentToolsSection` renders from allowlists with a
  `len==0` gate — **stays literally untouched this round** (no new section;
  capability discovery happens via the MCP tool list, which MCP clients render
  to the model).
- Org state for derivation: `orgStore.Nodes.List(orgID)` (reconciled bot nodes;
  `Node.Tools` = live, registry-pruned surface); `LoadState(bot).ProjectID` =
  home project (`infrastructure/runtime/helix/state.go`);
  `Node.ProjectIDs` = managed allowlist (never ownership).
- Bots' own access to the same registry at
  `/api/v1/mcp/helix-org` authorizes via `session.Metadata.OrgWorkerID`
  (`mcp_backend_helix_org.go:50`) — task sessions never have it; do not add it
  (guardrail).
- Secret tools: `mcptools.GetSecret`/`ListSecrets` resolve
  `workersecrets.Service` against `inv.Caller.ID()` as a worker id; the audit
  `Recorder` logs caller; org MCP audits every invocation with caller+args
  (`org/interfaces/server/mcp.go:168`).

## Design

### 1. Derivation helpers (`infrastructure/runtime/helix`, new, store-backed)

```go
func BoundAgentForProject(ctx, st, orgID, projectID) (orgchart.NodeID, error)
  // Nodes.List → Kind==Bot → LoadState home == projectID
  // exactly 1 → it; 0 → ErrNoBoundAgent; >1 → WARN + ErrNoBoundAgent (fail closed)
func AgentToolNames(ctx, st, orgID, nodeID) ([]tool.Name, error) // Node.Tools, nil-safe
```

### 2. Serve-set at the two choke points (formula above, one impl)

- `specTaskAgentTools(ctx, session)`: after today's body, resolve bound agent;
  when found append `AgentToolNames` (deduped, names after `sanitizeAgentTools`
  so non-catalog org tools survive — they're registry names, not hand-written).
  Consequences for free: mount decision (non-empty ⇒ URL present), correct
  `AgentToolsRev` ⇒ agent edits flip the task's rev at next desktop
  start/resume, REST view honest. Wire org-store access (composition root
  pattern in `server/helix_org.go` closures — server package already has it).
- `mcp_backend_spectask.go` `eligibleSpecTaskTools`: same helper-derived union
  so tools/list ≡ rev input ≡ REST view; also `BoundWorkerFromContext(ctx)`
  stash for §3 (single per-request resolution, cache the value).
- Unbound / un-entitled: `[]` ⇒ no mount ⇒ today's byte-identical behavior,
  including for UI-created tasks in Agent-less projects and all non-org tasks.
- Stale in-flight sessions after unbind: `ServeMCPForCaller` builds handlers
  from the request's tool list; also add the §3 unbound error so any org call
  (secret or otherwise) fails loudly — no half-auth.

### 3. Subject resolution + per-tool classification (the risk; do it in one PR)

> Superseded during implementation: the (a)/(b)/(c) classification stands, but
> (a) is applied by the single `mcptools.DelegatedCaller` ingress wrapper
> rather than per-tool `SubjectForCaller` conversions — see Implementation
> Notes (as built). The per-tool conversion form described below was built,
> shipped in the first pass, then replaced at review.

`runtime/principal.go`: `WithBoundWorker`/`BoundWorkerFromContext` (sibling of
`ProjectPrincipal`). New rule + helper
`tool.SubjectForCaller(ctx, caller) orgchart.NodeID`:

- Bot caller → `caller.ID()` (today, zero change).
- Project-principal caller → bound worker; absent → typed error
  "project is not bound to an org agent" surfaced as a tool error.

Every org tool reachable via some `Bot.Tools` must be classified and tested:
**(a) on-behalf-of** — any caller-identity read (`chat` sender, `managers`/
`reports` line, `dm`/`ask_human` origin, `get_secret`/`list_secrets` bindings,
`bot_log`, …) goes through `SubjectForCaller`; **(b) task-keyed** — spec-task
CRUD continues on `caller.ID()` = task id (correct: the task manages tasks);
**(c) unsupported-delegated** → loud error, zero writes, if any tool resists
sane bound-Subject semantics (per-tool judgment during impl; the PR carries the
full registry table (a)/(b)/(c)). Blast radius note: an armed task can do
exactly what its Agent could, with the Agent's grants, and org membership
checks for the *user* behind the task already upstream — same as Bot desktop;
audit is *stricter* (actor=task vs Bot).

`get_secret.go`/`list_secrets.go`: replace the current caller-ID-as-worker
resolution with `SubjectForCaller`; `Recorder` still records `inv.Caller.ID()`
(task) as actor plus subject worker; per-call value semantics unchanged (this
subsumes round-4's secrets-only branch — one generic mechanism).

### 4. Reverted / explicitly absent (do not re-add)

No `create_spectask` change (args, persistence, description), no `SpecTaskAgent
Tools` catalogue additions, no `"*"`, no `agent_tools` writes from code paths,
no `BuildAgentToolsSection` change, no prompt credential section, no services
diff. If future work wants "who spawned this" on the row itself, add the
single `created_by_agent` column then — not part of this spec (Open Questions).

### 5. Guardrail

Never set `Metadata.OrgWorkerID` on spec-task planning sessions; test: a task
session key 403s at `/mcp/helix-org`, 200 + tool list at `/mcp/helix-tasks`.

## Tests

- `helix`: `BoundAgentForProject` 0/1/2/allowlist-only/human-node;
  `AgentToolNames` nil-safe.
- server: `specTaskAgentTools` = own ∪ bound ∅ when unbound; org-unbound+
  own-only ≡ today (mount + rev identical); bound ⇒ mount+rev includes live
  names; rev flips on simulated Agent `Tools` change; REST view equality;
  unbound+own-only ≡ today.
- mcp_backend: tools/list ≡ formula; bound worker stashed on ctx; ServeMCP-
  ForCaller gets live names; unbound org call on stale list → clean error.
- mcptools: `SubjectForCaller` cases (bot / principal+bound / unbound);
  representatives per class: `chat` posts bound-Worker, `managers`/`reports`
  on behalf, `get_secret`/`list_secrets` read bound bindings with audit
  actor=task subject=agent; CRUD task-keyed; one (c)-class loudly failing.
- Prompt builder asserted untouched (golden). Guardrail session test.

## Out of scope / v1

No row-visible capability state; no push to running desktops
(refresh-at-restart/resume on rev); no bound-agent lookup cache (orgs small;
memoize per request only); no UI; no values in rows/env.

## Implementation Notes (as built)

- Delegation is resolved once at the trust boundary, not per tool: the MCP
  backend wraps a bonded task's caller in `mcptools.DelegatedCaller`
  (`api/pkg/org/interfaces/mcptools/delegated_caller.go`) before
  `ServeMCPForCaller`, so every class-(a) tool sees exactly one identity
  (`caller.ID()` = bound agent) with zero tool-side changes. An unbound
  task caller is never wrapped — it only ever receives the task-keyed
  CRUD catalogue. The bound subject is not stashed in ctx; tools need no
  knowledge of delegation at all.
- Audit identity deliberately diverges from acting identity at the same
  boundary: `DelegatedCaller` implements `orgaudit.SelfDescribingActorID`
  so the entry's `ActorType=spec_task` and `ActorID=spt…` even though
  `ID()` is the agent; project attribution comes from the project
  principal already on ctx. The spectasks service principal branch ignores
  worker identity, so the swap is inert there.
- The "same code path" rule is enforced structurally:
  `(*HelixAPIServer).specTaskToolSurface` in `mcp_backend_spectask.go` is
  the only composer. `specTaskAgentTools` (rev/REST view) and the MCP
  backend's serve decision both call it; the backend wraps the caller in
  `DelegatedCaller` iff it returns a bound agent.
- Class-(c) enforcement = served-list exclusion (`mcptools.
  SpecTaskBlockedTools`). Org MCP dispatch authorizes solely by tool
  presence in the per-caller served registry, so unserved names are
  uncallable by construction (zero DB writes), and tools/list can never
  drift from reality. Bot surfaces are untouched.
- Judgment calls recorded with the data: `create_trigger`, processors,
  sandbox admin, `sandbox_ssh_access` → (c); `server_ssh_access` → (a)
  (same capability class as `server_run_command`); `get_bot_project` →
  (b); membership-gated service reads (`list_projects`, sandboxes) stay
  (b) and fail closed for tasks until their services learn the principal.
- `bot_log` attaches the (agent's) transcript while service-side audit
  attribution stays the task via `SelfDescribingActorID` — audit vs
  subject rule.
- Live e2e record incl. env caveats is in tasks.md item 12; the
  reviewer-facing summary + tool table is in `pull_request_helix.md`.
