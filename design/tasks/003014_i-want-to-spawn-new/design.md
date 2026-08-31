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
  `BaseReadTools` — every Bot gets them and resolves values **at use time**
  against `inv.Caller.ID()` as the Worker id. The design comment in
  `mcptools/defaults.go` is explicit: there is *no boot-time env-var fallback*
  for Bot credentials; `get_secret` is the generic credential primitive.
- **Spec-task → org MCP channel.** `server/mcp_backend_spectask.go` serves the
  org MCP registry to a task's coding agent at `/api/v1/mcp/helix-tasks` using
  the session-scoped API key. Caller = `specTaskCaller{ID: task.ID}` with
  `AuditActorType() == ActorSpecTask`; `runtime.ProjectPrincipal` rides ctx. The
  visible tools are `(project.AgentTools ∪ task.AgentTools) ∩
  mcptools.SpecTaskAgentTools` (`defaults.go`) — today only the 14 spec-task
  CRUD tools. The sandbox binds this server via
  `ContextServers["helix-tasks"]` in `external-agent/zed_config.go` (~L375-386),
  cache-busted by `AgentToolsRev(specTaskTools)`; the same effective-tool helper
  feeds `services.BuildAgentToolsSection` (planning prompt) and
  `server/agent_tools_handlers.go`.
- **Spawn path.** `mcptools.CreateSpecTask` → `spectasks.Service` →
  `infrastructure/runtime/helix.SpecTasks.Create` writes a `types.SpecTask` (in
  `types/simple_spec_task.go`) attributed to the project and the Worker's hiring
  user. **The spawning Worker's id is not persisted anywhere on the task** — that
  is the missing provenance.

## Decision: audited delegation, not injection

Chosen: **keep `get_secret`/`list_secrets` as the retrieval primitive and give
the spawner's bindings a delegation pointer.** The task row stores who spawned
it and which named bindings it may borrow; the two tools gain a
spec-task-caller branch that resolves through that provenance.

Alternatives considered:

- **Env-var injection at sandbox boot** (resolve values, put on container env):
  rejected — values go stale on rotation/revocation, land in the container
  environment and process list, need plumbing through session creation, and
  contradicts the existing "no boot env fallback" `get_secret` philosophy.
- **Copy bindings to a task-id pseudo-workspace at create time:** rejected —
  `workersecrets.Service.Put` requires a real org-chart node; snapshots drift
  from the Worker (no revocation); orphaned rows per task; no audit story.
- **Pass values as `create_spectask` args:** rejected — values transit the model
  context and DB row of both agents. Explicitly prohibited.

Delegation keeps everything the org philosophy asks for: small MCP surface (zero
new tools), data over code (two columns + one wiring closure), social
enforcement at the Worker, hard enforcement at the secret boundary, and
per-call resolution so rotation/revocation is instant.

## Design

### 1. Provenance on the task (`types/simple_spec_task.go`)

```go
SecretsWorkerID string   `json:"secrets_worker_id,omitempty" gorm:"size:255;index"`
SecretGrants    []string `json:"secret_grants,omitempty" gorm:"type:jsonb;serializer:json"`
```

GORM AutoMigrate handles the columns (repo convention). Fields are only ever
written by the org spawn path; REST/UI handlers must not accept them
(not-allowed-list in create/update request mapping).

### 2. Create-side plumbing

- `runtime.CreateSpecTaskInput` gains `SecretGrants []string`;
  `mcptools.createSpecTaskArgs` gains `secret_grants` (description updated:
  "names from your `list_secrets`; the task inherits YOUR grants, subset").
  Wire through `spectasks.Service.Create` (application layer forwards verbatim;
  it owns no policy).
- `helix.SpecTasks.Create` stores provenance and validates: each requested name
  must exist on the caller's bindings. Read the existence check through
  `orgStore` — it already owns org bindings (`store.WorkerSecretBindings`
  repo; `loadState`-equivalent access exists in the same package). For a
  ProjectPrincipal caller (task spawning sub-task): inherit
  `SecretsWorkerID` from the parent task and reject any name outside the
  parent's grants (subset rule). Parent task id is available via
  `ProjectPrincipal`/session lookup — if resolving it proves awkward in the
  Create path, the backend can stamp the sub-task after creation instead; keep
  it out of the happy-path signature if that's simpler.
- Sub-task grant subset check must happen after resolving the parent task;
  non-subset → error listing the permitted names.

### 3. Tool resolution for a spec-task caller

`mcptools.Deps` gains one seam (wired in the composition root,
`server/helix_org.go`, over the Helix store):

```go
// resolves a task's delegated secret source: worker whose bindings back the
// task + the granted names. error when the task has no grants.
SpecTaskSecretSource func(ctx context.Context, orgID, taskID string) (workerID orgchart.NodeID, names []string, err error)
```

In `ListSecrets.Invoke` and `GetSecret.Invoke`: when
`runtime.ProjectPrincipalFromContext(ctx)` is present (spec-task callers always
have one; Bots never do), resolve `(workerID, names)` via the seam; 404-style
error when the task has no grants (the tools are also hidden from tools/list,
see §4); for `get_secret`, reject names not in `names` before touching
`deps.WorkerSecrets`; then call the existing service with the **source worker
ID** while the audit `Recorder` continues to attribute the call to
`inv.Caller.ID()` (the task). Bots keep the current behaviour (caller ==
worker). `GetSecret`'s existing caller-identity check stays.

Security invariants: the task id used for the lookup is the authenticated
`inv.Caller.ID()` — never an argument; cross-org already blocked by the
backend's org check; workers still can't read other workers' bindings
unchanged.

### 4. Effective tool surface = union, not just allowlist intersection

`mcp_backend_spectask.go`: after the existing
`(project ∪ task) ∩ SpecTaskAgentTools` computation, append
`mcptools.ListSecretsName, mcptools.GetSecretName` iff
`task.SecretGrants` is non-empty — independent of the project allowlist, because
the grant itself, not the project, is the opt-in. Add the two names to the
`SpecTaskAgentTools` catalogue so a hand-crafted AgentTools entry can't grant
secrets to a task **without** grants (the intersection then keeps stale entries
harmless).

The rev computation and prompt surface must agree, otherwise Zed's cached
tools/list and the planning prompt lie about the task's surface: pass the same
grant-aware list into `zed_config.go` (`specTaskTools` / `AgentToolsRev`),
`services.BuildAgentToolsSection`, and `server/agent_tools_handlers.go`.
Add a tiny shared helper (e.g. `types.EffectiveSpecTaskAgentTools(projectTools,
taskTools []string, hasSecretGrants bool)`) used by all three call sites —
mirrors the existing `EffectiveAgentTools` pattern in `types/agent_tools.go`.

### 5. Prompt text (data over code)

`BuildAgentToolsSection` (or a sibling in the same builder): when grants exist,
add "This task inherited N credential grant(s) from the agent that spawned it.
Call `list_secrets` for names/usage, `get_secret` immediately before an
authenticated operation." Names/usage only — values never.

### 6. Tests

Follow existing patterns:
- `application/spectasks` + `infrastructure/runtime/helix/spectasks_test.go` —
  create stores provenance; unknown name rejected; sub-task subset rule.
- `mcptools/get_secret.go` + `list_secrets.go` — add spec-task-caller unit tests
  over a fake `SpecTaskSecretSource` (see `worker_secrets_test.go` and
  `spec_tasks_test.go` fakes).
- `server/mcp_backend_spectask_*` — grants flip tools/list membership; rev
  changes with grants.
- In-proc server: `helix_org_inproc_test.go` shows the full wiring path for a
  real create → task-agent MCP call if feasible.

### 7. Out of scope / deliberately not done

- No values in `create_spectask` args, task rows, logs, or container env.
- No per-binding "inheritable" flag or UI changes in v1.
- No revocation propagation to already-running shell commands (inherent to
  per-call resolution; same semantics Workers already have).
