# Requirements: Grant Worker Secrets to Spawned Spec Tasks

## Background

A helix-org Worker (Bot) spawns spec tasks into its Helix project via the
`create_spectask` MCP tool. Worker credentials are granted as secret *bindings*
(org + worker + name → a Helix secret or connected-account value), readable by
that Worker through `list_secrets` (metadata only) and `get_secret` (value at
call time). A task spawned by the Worker has no way to reach those bindings, so
it cannot do credentialed work (push with the Worker's git token, call a deploy
API…). Different Workers hold different secrets, so task access must
always be worker-specific, never global.

Two independent facts exist today in code: the task's coding agent has a scoped
MCP channel into the org tool registry (`SpecTaskMCPBackend`,
`/api/v1/mcp/helix-tasks`, session-scoped key, task-as-actor identity), and the
org runtime knows which Agent owns which project (each Bot's runtime state
names its project). The design splits into the two layers review asked for:

- **Grant — intentional, recorded at creation.** `create_spectask` grants the
  pair `list_secrets` + `get_secret` to the task by **adding them to the task's
  existing `agent_tools` grant list**. This is deliberate, visible state on the
  task itself: the API response, the REST agent-tools view, and `GET task` all
  show the task was armed with credential tools at spawn time. The exact
  spawning Agent on the actor recorded in the org audit log (every
  `create_spectask` is audited with the Bot as actor), so creation provenance
  is already an explicit, queried fact — no schema change, no hidden "project
  detection" deciding who gets tools.
- **Authority — fail-closed at value-read time.** When a task calls the tools,
  *whose* credential set applies is derived: the Agent whose runtime home is
  the task's project, exactly one, or no access. Derivation here is deliberate
  and unhidden (it is an error-message'd lookup), and it is the only place the
  passive-mapping mechanism touches behavior.

**Rejected alternatives, recorded deliberately:**

- *Silently surfacing the tools for every task in an Agent-owned project* —
  rejected on review (round 5): hidden functionality; a task must *carry* its
  capability as an intentional grant, not be detected.
- *A new provenance column on the task* — rejected on review (round 3): the
  creator is already recorded (org audit) and the capability can live in the
  existing grant list; no schema change.
- *Attaching the Worker's own `helix-org` MCP key/URL to the task* — full
  impersonation (that backend authorizes on the key's session's `OrgWorkerID`);
  entire bot surface, audit-as-Bot, no fine revocation.
- *`secret_grants` argument; boot-env injection; copying bindings per-task* —
  ceremony / stale values / drift; all rejected in earlier rounds.

## User Stories

1. As a Worker, when I spawn a task, I want it to arrive already armed with
   `list_secrets`/`get_secret` — recorded on the task at creation, visible in
   the task data — so the intent "this task runs with my credentials" is
   explicit rather than inferred.
2. As an org owner, I want a credential read to only ever return bindings of
   the Agent that actually owns the task's project (exactly one home; a task
   can never supply a worker id), so no Agent's secrets can reach a project it
   doesn't own.
3. As a task agent, I want secrets resolved at use time (never copied into my
   prompt, task row, or container env), so rotation/revocation lands on my next
   `get_secret` call.
4. As a task agent, I want `list_secrets` first (names + usage metadata, never
   values or backend details), including an empty list when my project's Agent
   holds nothing yet.
5. As a task agent, I want tasks I spawn to inherit the same explicit grant,
   with credentials still scoped to each task's own project, so credential
   sets never smuggle sideways across projects.
6. As an auditor, I want every credential read logged with task id as actor
   and the source Agent + secret as subject, and `create_spectask` itself
   already logged with the spawning Bot as actor — together giving the full
   "who armed this task" trail with no new state.
7. As the project's human owner, I want every spec task — armed or not — to be
   a first-class, independently usable task; an armed task that loses its
   Agent must fail loudly on credential calls, not degrade silently.

## Acceptance Criteria

- [ ] Schema and create contract unchanged: no new DB column, no new
      `create_spectask` argument. Spawning via `create_spectask` (Worker or
      task-principal caller) records `list_secrets` and `get_secret` in the
      created task's `agent_tools` list (deduped); REST/UI-created tasks carry
      no such entry.
- [ ] The grant is **visible**: `GET /spec-tasks/{id}`, the REST agent-tools
      view, and the task's tool list in the MCP session all show the pair for
      armed tasks; un-armed tasks (including ones inside an Agent-owned
      project) are byte-identical to today — no tool appears without the
      intentional grant.
- [ ] Surface requires no special-case logic: the pair joins the spec-task tool
      catalogue, and existing grant-list mechanics (catalogue sanitation,
      rev cache-bust, `helix-tasks` mount when armed, REST view) carry it; an
      armed-but-otherwise-plain task gets the channel mounted by the stamp
      alone.
- [ ] At call time, `list_secrets`/`get_secret` from a task resolve exactly the
      bindings of the single Agent whose runtime home project **is** the task's
      project (ownership only — never a managed/allowlisted project); zero
      owners or ambiguous ones yield a clean, actionable error and no values.
      Tool arguments never accept worker or task ids.
- [ ] Values resolve per call: rotation/new grants land on the next call;
      revoking/ungranting, firing the Agent, or un-homing the project turns
      subsequent calls into clean errors — the stamp (grant) remains visible,
      the task itself stays fully usable, and re-binding a project restores
      access with no task edit.
- [ ] Admins can intentionally revoke or (re-)arm via the existing
      `agent_tools` update path; manual addition of the pair to a REST-created
      task is valid and yields the same project-owner-scoped access.
- [ ] Prompt: an armed task sees a dedicated "credentials granted by the
      spawning agent — use `list_secrets`/`get_secret`" section; the existing
      "Delegating to other spec tasks" section enumerates delegation/lifecycle
      tools only (armed tasks are never told they can spawn sub-tasks from the
      secret pair). Un-armed prompt unchanged.
- [ ] Reads are audited as today's workers-secret trail: task id as actor,
      resolved Agent + secret as subject, success and failure records included.
- [ ] Guardrail: a spec-task session never receives `Metadata.OrgWorkerID`
      (that flag authorizes the full bot backend; its absence keeps the task on
      the task-scoped channel).
- [ ] No secret values in the task row, prompts, logs, or container env;
      lifecycle (planning→implementation→PR, approvals, labels, UI, start/stop)
      is completely independent of being armed, proven e2e both ways.

## Open Questions

1. **Creator name on the task row:** the capability (armed) lives on the task;
   the exact creator identity lives in the org audit log (Bot actor on
   `create_spectask`). If the product wants the creator's *name* displayed on
   the task itself (UI/row), that is exactly one new column — deliberately not
   included per the "no new fields" ruling; say the word and it lands.

## Resolved questions

- **Intentional vs passive (round 5)** — grant is now an explicit record made
  by `create_spectask` on the existing `agent_tools` list; derivation remains
  only at reads, as the scoped security authority with loud errors.
- **No new DB fields (round 3)**, **no project toggle (round 2)**, **whole-set
  inheritance / no inheritable flag (round 2)**, **no UI changes**, **no env
  vars — `get_secret` + shell per call**, all confirmed.
