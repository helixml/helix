# Design: Grant Worker Secrets to Spawned Spec Tasks

## Current state (verified in code, `api/pkg/`)

- **Worker secrets.** Bindings live in the org store keyed by
  `(OrganizationID, WorkerID orgchart.NodeID, Name)` — `domain/workersecret/binding.go`.
  Sources are `helix_secret` (SecretID) or `connected_account` (AccountID +
  ExportKey). The `workersecrets.Service`
  (`application/workersecrets/workersecrets.go`) exposes `Descriptors` (names +
  metadata, never values) and `Get` (fresh resolve + `Recorder` audit). REST
  grants at `/api/v1/orgs/{org}/agents/{id}/secrets`.
- **Bot tools.** `list_secrets` / `get_secret`
  (`interfaces/mcptools/list_secrets.go`, `get_secret.go`) are in
  `BaseReadTools` — every Bot gets them **regardless of current bindings**, and
  resolves values at use time against `inv.Caller.ID()` as the Worker id. The
  comment in `mcptools/defaults.go` is explicit: there is *no boot-time
  env-var fallback*; `get_secret` is the generic credential primitive.
- **Spec-task → org MCP channel.** `server/mcp_backend_spectask.go` serves the
  org MCP registry to a task's coding agent at `/api/v1/mcp/helix-tasks` using
  the session-scoped API key. Caller = `specTaskCaller{ID: task.ID}` with
  `AuditActorType() == ActorSpecTask`; `runtime.ProjectPrincipal` rides ctx.
  Visible tools = `(project.AgentTools ∪ task.AgentTools) ∩
  mcptools.SpecTaskAgentTools` (`defaults.go`) — today only the 14 spec-task
  CRUD tools. The sandbox binds this server via `ContextServers["helix-tasks"]`
  in `external-agent/zed_config.go` (~L375-386), cache-busted by
  `AgentToolsRev(specTaskTools)`; the same effective-tool computation feeds
  `services.BuildAgentToolsSection` (planning prompt) and
  `server/agent_tools_handlers.go`.
- **The worker-facing MCP backend cannot be reused verbatim.**
  `server/mcp_backend_helix_org.go:50` 403s unless the key's session carries
  `Metadata.OrgWorkerID`. A spec task's planning session has `SpecTaskID`, not
  `OrgWorkerID`. This is why "attach the helix-org MCP URL to the task" is not
  a zero-change option (see Rejected alternatives).
- **Spawn path.** `mcptools.CreateSpecTask` → `spectasks.Service` →
  `infrastructure/runtime/helix.SpecTasks.Create` writes a `types.SpecTask` (in
  `types/simple_spec_task.go`) attributed to the project and the Worker's hiring
  user. **The spawning Worker's id is not persisted anywhere on the task** —
  that is the missing provenance.

## Decision: provenance + full inheritance over the existing task channel

The spawning Worker's id is stored on the task. A task that has provenance gets
`list_secrets`/`get_secret` on its existing `helix-tasks` MCP surface — the
same in-process tool registry the Worker uses, same as the reviewer's instinct
of "same MCP tools", but scoped to the secret pair instead of the whole bot
surface. The tools gain a spec-task-caller branch that resolves **the whole of
the spawner's current bindings** through the stored provenance, at call time.
No `create_spectask` contract change, no per-task grant list.

### Rejected alternatives

- **Hand the task the Worker's own desktop API key / helix-org URL.** The
  backend can only derive Worker identity from the key's session
  (`OrgWorkerID`); a task session isn't one. Passing the Worker's key into the
  container is full impersonation — every tool (`chat`, `create_bot`, org
  mutations, sandboxes), audit that logs as the Worker, and no revocation finer
  than "kill the Worker". Explicitly worse than a provenance column.
- **Resolve task→worker impersonation inside the shared `helix-org` backend.**
  Needs the same provenance this design already stores; placing it below the
  abstraction (a non-Bot caller impersonating a Bot on the full registry)
  removes the project-principal scoping and multiplies the exposed surface for
  a secrets-only need.
- **Per-task `secret_grants` argument** — rejected on review: ceremony without
  material gain (the Worker is the already-trusted grant holder and can paste
  values into a description today). Whole-set inheritance instead.
- **Env-var injection at boot** and **copying bindings per task** — rejected
  first round: stale-on-rotation, values on disk/container env, drift and
  orphaned rows, and they contradict the "no boot env fallback, get_secret is
  the primitive" philosophy.

## Design

### 1. Provenance on the task (`types/simple_spec_task.go`)

```go
SecretsWorkerID string `json:"secrets_worker_id,omitempty" gorm:"size:255;index"`
```

GORM AutoMigrate handles the column (repo convention). Written only by the org
spawn path; REST/UI spec-task create/update handlers must neither accept nor
persist it (check their request→struct mapping and clear it on task clone).
There is no grant list: "has provenance" is the whole opt-in.

### 2. Create-side plumbing (no contract change)

`infrastructure/runtime/helix.SpecTasks.Create` sets
`task.SecretsWorkerID = workerID` when invoked by a Worker session. The id is
already available at that call site (`inv.Caller` flows through
`spectasks.Service` → port arg `workerID orgchart.NodeID`).

Sub-task spawns: a `ProjectPrincipal` caller's `inv.Caller.ID()` **is the parent
task id** (`specTaskCaller` is built from the authenticated session), so in
`helix.SpecTasks.Create`'s principal branch, load the task row by that id and
inherit its `SecretsWorkerID`. (If a principal's id ever fails the
GetSpecTask lookup, fail closed: no provenance, no secrets — never fall back to
the legacy `LoadState`.) A sub-task of a task is therefore scoped to the root
spawner; tasks cannot name any other worker or task id because the id comes
from the caller, not the args.

### 3. Tool resolution for a spec-task caller

`mcptools.Deps` gains one seam (wired in the composition root,
`server/helix_org.go`, over the Helix store):

```go
// resolves the Worker whose bindings a task may read; error when the task
// has no provenance. Task id comes only from the authenticated caller.
SpecTaskSecretsWorker func(ctx context.Context, orgID, taskID string) (orgchart.NodeID, error)
```

In `ListSecrets.Invoke` and `GetSecret.Invoke`: when
`runtime.ProjectPrincipalFromContext(ctx)` is present (spec-task callers always
have one; Bots never do), resolve the source Worker via the seam and call the
existing `workersecrets.Service` with **that** worker id, while the audit
`Recorder` continues to attribute to `inv.Caller.ID()` (the task id — already
the actor the org audit log records via `ActorSpecTask`). A task whose Worker
holds no bindings gets an empty `Descriptors` list and a not-found
`get_secret`, exactly like an empty-handed Bot. Bots keep the current
behaviour.

Security invariants: org comes from the backend's project/org check; task id
from the authenticated caller; no new id-shaped tool arguments anywhere;
cross-tenant already blocked upstream.

### 4. Effective tool surface = grant-aware union

`mcp_backend_spectask.go`: after the existing `(project ∪ task) ∩
SpecTaskAgentTools` computation, append `mcptools.ListSecretsName,
mcptools.GetSecretName` iff `task.SecretsWorkerID != ""` — independent of the
project allowlist, because provenance, not the allowlist, is the gate. Add the
two names to the `SpecTaskAgentTools` catalogue (the intersection then makes
stale AgentTools entries harmless for provenance-less tasks).

The rev computation and prompt surface must agree or Zed's cached tools/list
and the planning prompt lie about the task: pass a shared
`types.EffectiveSpecTaskAgentTools(projectTools, taskTools []string,
hasSecretProvenance bool)` helper (new, mirrors `EffectiveAgentTools` in
`types/agent_tools.go`) at all three call sites: `external-agent/zed_config.go`
(`specTaskTools`/`AgentToolsRev`), `services/spec_task_prompts.go`
(`BuildAgentToolsSection`), `server/agent_tools_handlers.go`.

### 5. Prompt text (data over code)

In the existing agent-section builder: when the task has provenance, add one
line — "This task inherited credentials from the agent that spawned it. Call
`list_secrets` for names/usage, `get_secret` immediately before an
authenticated operation." No enumeration (the prompt builder has no org-store
access; `list_secrets` is the enumeration).

### 6. Tests

Follow existing patterns:
- `infrastructure/runtime/helix/spectasks_test.go` — Worker spawn stores
  provenance; principal spawn inherits parent's; principal id that doesn't
  resolve a task fails closed without provenance.
- `mcptools` — spec-task-caller branch tests in get_secret/list_secrets over a
  fake seam (patterns: `worker_secrets_test.go`, `spec_tasks_test.go`); audit
  attribution stays on the task id.
- `server/mcp_backend_spectask*` — provenance flips tools/list both ways;
  provenance-less task sees neither tool even if the catalog lists them.
- types: `EffectiveSpecTaskAgentTools` union/intersect truth table.

### 7. Out of scope / deliberately not done

- No exposure of any non-secret org tool to tasks (full registry stays
  bot-only).
- No values in task rows, logs, prompts, or container env.
- No "inheritable per binding" flag, no UI, no post-hoc grant editing.
- No revocation propagation to already-running shell commands (inherent to
  per-call resolution; same semantics Workers already have).
