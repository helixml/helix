# Requirements: Give Spawned Spec Tasks Their Agent's Tool Access

## Background

A helix-org Worker (Bot) spawns spec tasks into its Helix project via the
`create_spectask` MCP tool. Product rule confirmed on review: **a spawned task
should operate with the same tool surface as the Agent that spawned it** — the
worker's live MCP tool set, including `list_secrets`/`get_secret` — because
that is the intuitive contract ("it's implied tasks have the same level of
access as the agent") and it compounds: org tooling keeps working in tasks with
no per-feature re-granting. The task's coding agent already reaches the org
tool registry through its session-scoped MCP channel (`SpecTaskMCPBackend`,
`/api/v1/mcp/helix-tasks`); this feature widens the tool set that channel
serves to an armed task.

Two layers, both explicit (per earlier review rounds):

- **Armed = recorded at creation.** `create_spectask` stamps a wildcard entry
  (`"*"`) into the created task's existing `agent_tools` list: "this task runs
  with its Agent's tools." Visible on the task, intentional, no schema change,
  no hidden detection deciding eligibility. UI-created tasks are not armed;
  an admin can arm or disarm a task through the same `agent_tools` field.
- **Which Agent, which tools = live, at session time.** The bound Agent is the
  one whose runtime home project **is** the task's project; the tool set is
  that Agent's **current** tool list, resolved fresh whenever the task's MCP
  session (re)initializes. Granting the Agent a tool later (`attach_tool`)
  reaches its tasks; removing one or firing the Agent removes it from them on
  the next refresh. A task can never supply a worker id; the set cannot drift
  from the live org because nothing is snapshotted onto the task.

Attribution model: the task calls as itself (`ActorSpecTask`, audit actor =
task id) but tools whose semantics are "as the caller Agent" resolve their
subject to the bound Agent (on-behalf-of). Blast radius equals the Agent's
existing one — the task lives in the Agent's project and uses the Agent's
grants; what changes is that every org action is attributable to the specific
task, which the Agent's own desktop calls are not.

**Rejected alternatives, recorded deliberately:**

- *Secrets pair only* — superseded by product decision above.
- *Snapshot-copy the Agent's tool names onto the task* — drifts; revocations
  wouldn't propagate (rejected in earlier rounds for the same reason as
  copying bindings).
- *Hand the task the Worker's own `helix-org` key/URL* — impersonates the Bot
  itself (that backend authorizes on the key session's `OrgWorkerID`): no task
  attribution, no finer revocation, wrong audit trail.
- *Passive project-membership auto-surfacing, new provenance column, per-task
  grant lists, boot env vars, copying bindings* — rejected in earlier rounds;
  see Resolved questions.

## User Stories

1. As a Worker, every task I spawn can do everything I can do — reply on
   streams, read events, fetch my granted credentials, manage sub-tasks and
   sub-agents — with no per-work grant fiddling, because tasks are how I
   delegate.
2. As an org owner, I want the live set to come only from the Agent that
   actually **owns** the task's project (exactly one home; managed/allowlisted
   projects never count; no id may be supplied in arguments), so a task can
   never borrow tools or secrets from an Agent unrelated to its project.
3. As a task agent, I want org tools that act "as the caller" to behave on
   behalf of my project's Agent consistently, and every tool that fundamentally
   cannot serve a delegated caller to fail with a clear, actionable error —
   never a half-effect, never a silent success.
4. As an org owner, I want revocations to be real: remove a tool from the
   Agent (or fire it) and tasks lose it at their next session refresh; ungrant
   a secret and the next `get_secret` fails.
5. As an auditor, I want org audit to show the task id as actor on every
   org-tool call, with the bound Agent as the acting-for subject, plus the
   existing worker-secret read audit — so task actions are traceable to the
   task, unlike calling directly as the Agent.
6. As the project's human owner, I want armed/unarmed to be visible on the
   task and fully intentional, and task lifecycle (plan, implement, approve,
   PR, chat, restart) to remain independent of the org channel: a broken,
   unbound, or unarmed task is still a perfectly usable normal task.

## Acceptance Criteria

- [ ] Schema and `create_spectask` contract unchanged (no arguments, no new
      columns): any `create_spectask` call on the org MCP path (Bot or task
      principal) records a `"*"` wildcard entry in the created task's existing
      `agent_tools` list; REST/UI-created tasks carry none.
- [ ] Armed tasks' tool surface = their Helix spec-task tools (existing
      project∪task grants, unchanged semantics) **plus** the bound Agent's
      live tool set, at every surface: tools/list, the `helix-tasks` mount
      flag and cache-bust `rev`, the REST agent-tools view, and the planning
      prompt. Unarmed tasks are byte-identical to today, including inside an
      Agent-owned project.
- [ ] Tool set is live: `attach_tool`/`detach_tool` on the Agent flips the
      task's rev at the task's next config refresh (`rev` changes ⇒ agent
      restarts the cached context server); firing/unhoming the Agent leaves the
      armed task with only its own spec-task tools and clean errors on org
      calls, task fully usable throughout; re-binding restores.
- [ ] `get_secret`/`list_secrets` (when in the Agent's set) resolve the bound
      Agent's bindings with values at call time, per existing worker-secret
      semantics (no values in rows/env, audit actor = task).
- [ ] Delegation identity model: for an armed caller, tools that act "as / on
      behalf of the calling Agent" resolve their subject to the bound Agent
      (streams messages posted as the Agent, reporting-line reads of the
      Agent, `ask_human` from the Agent, etc.); the caller identity is never
      accepted from arguments. Tools whose semantics are natively the task's
      own (the spec-task set) stay keyed to the task.
- [ ] Every org tool reachable to an armed task is explicitly classified and
      tested as one of: acts-as/for-the-Agent (resolves to bound Agent),
      task-scoped (keys to task identity), or unsupported-for-delegation (loud
      clean error, zero side effects). The classification lives with the tool
      implementations (their principal handling), and the spec-task tool tests
      cover every category with at least one representative (e.g. `chat`,
      `managers`, `list_secrets`, `create_spectask` self, `delete_bot`/graph
      mutation path, `create_bot`).
- [ ] Org-graph mutations from an armed task are permitted exactly to the
      degree the Agent itself holds the tool (same catalog the Agent has —
      `Bot.Tools` is the live surface), with audit actor = task; no tool ever
      widens beyond the bound Agent's current grant state.
- [ ] The prompt gains an explicit section for armed tasks: "This task runs
      with the tools granted to its spawning agent via the Helix MCP channel;
      use `list_secrets`/`get_secret` for credentials" (no enumeration;
      tools/list is the enumeration), while the delegation section stays CRUD-
      tools-only (`"*"` / secret names never enumerated there).
- [ ] Admin intentionally arming/disarming via `agent_tools` REST update works
      and yields identical live semantics.
- [ ] Guardrail: spec-task sessions never carry `Metadata.OrgWorkerID`; the
      `helix-org` backend still 403s task keys; everything runs on
      `helix-tasks` as `ActorSpecTask`.
- [ ] Full lifecycle (planning→implementation→approval→PR, start/stop/restart,
      human UI) independent of armed/Agent state, verified e2e both with and
      without a bound Agent.

## Open Questions

1. **Creator name on the task row:** capability is stamped on the task, actor
   provenance is the org audit (`create_spectask` records the Bot actor); a
   *visible creator name* on the task row/UI would be one new column — held
   off pending the "no new fields" ruling; one word and it lands.

## Resolved questions

- **Pair vs all agent tools (this round)** — all of the Agent's live tools.
- **Intentional vs passive** — armed stamp (`"*"`) at creation; live sets may
  still be derived (they're the Agent's own live state, not detection).
- **No new DB fields; no project toggle; whole-set inheritance incl. secrets
  (no inheritable flags); no UI changes; `get_secret` + shell, no env vars.**
