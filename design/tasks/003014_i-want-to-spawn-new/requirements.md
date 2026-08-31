# Requirements: Give Spawned Spec Tasks Their Agent's Tool Access

## Background

A helix-org Agent (Bot) spawns spec tasks into its Helix project via the
`create_spectask` MCP tool. Product rule (confirmed, final rounds): **a task
operates with its Agent's tools.** Mechanically (per review):

> "The MCP server is just another mcp server to add. We don't even need to
> inspect the tools. Just pass the URL to the agent's mcp endpoint."

So there is **no client-side grant machinery at all**: no stamps, no sentinel
entries in task rows, no "armed/unarmed" terms, no prompt plumbing. The task's
sandbox already mounts one Helix MCP channel (`/api/v1/mcp/helix-tasks`,
session-scoped key); the rule becomes: mount that channel whenever the task
has *anything it is entitled to*, and the org MCP server itself decides —
per request, from stored server-side state — which tools that caller gets:

1. its own spec-task tools from existing `agent_tools` allowlists (today's
   behavior, unchanged), **plus**
2. if the task's project is the runtime home of an org Agent: that Agent's
   live MCP tool set (which includes `list_secrets`/`get_secret` for the
   Agent's credentials).

No bound Agent ⇒ nothing extra appears; an org task with no other tools simply
doesn't mount the channel. Non-org tasks are byte-identical to today. The
"which Agent" link is derived where it already lives (Agent runtime state names
its project; fail-closed at zero/ambiguous) — a server-side authority, not a
client-visible concept.

Identity and revocation model (unchanged since the design converged):

- The task calls as itself (`ActorSpecTask`; audit actor = task id) and tools
  whose semantics are "as the caller Agent" resolve their subject to the bound
  Agent — strictly more attributable than the Agent calling its own backend.
- Everything is live by construction: `attach_tool`/`detach_tool` on the
  Agent, ungranting a secret, and firing the Agent all propagate to tasks at
  the task's next session/tool-surface refresh (desktop restart/resume),
  because nothing is copied onto the task. Firing the Agent makes the surface
  shrink to the task's own tools and in-flight calls fail with clear errors,
  never half-effects.

**Rejected alternatives, recorded deliberately:**

- *Snapshotting tool names or `get_secret` values onto the task, env-var
  injection, copying bindings* — drift and stale authority; rejected early.
- *Hand the task the Agent's own key/URL on `mcp/helix-org`* — that backend
  authorizes on the key session's `OrgWorkerID`; task sessions must never have
  one. Full impersonation with no task attribution — rejected.
- *A wildcard/`"*"` stamp on task `agent_tools`* (rounds 5–6) — removed in
  round 7 per the "just add the MCP" ruling: the capability is visible in the
  runtime tool list and the audit trail instead of on the task row. (If
  row-visible capability is ever wanted again, that stamp design exists in
  git history.)
- *A new provenance column; per-task grant args; a project toggle; per-binding
  inheritable flags; a credentials UI; a creator-name column* — all rejected in
  earlier rounds; see Resolved questions.

## User Stories

1. As an org Agent, every spec task in my project can do what I can do through
   the Helix org tools, including using my granted credentials, with no setup
   per task.
2. As an org owner, the live set derives only from the Agent that truly **owns**
   the task's project (exactly one home; managed/allowlisted projects never
   count; no id may be passed in tool args), and nothing is copied onto tasks
   — so tools and secrets can never reach or remain in a project whose Agent
   doesn't hold them.
3. As a task agent, tools that speak of the caller ("send as me", "my
   reporting line", "my secrets") act for my project's Agent; anything that
   fundamentally cannot serve a delegated caller says so loudly with zero side
   effects, never silently half-succeeds.
4. As a task agent, I see my capabilities where MCP tools live — the tool list
   my harness shows — and credentials resolve fresh at every `get_secret`.
5. As the project's human owner: every task remains a first-class standalone
   task regardless of project binding; a task inside an Agent-owned project —
   spawned or UI-created — inherits the Agent's tool surface automatically and
   loses it the moment the Agent is gone (this automatic inheritance is the
   intended contract, and the task stays fully usable either way).
6. As an auditor: every org-tool call carries actor = task id (the org
   `ActorSpecTask`), `create_spectask` already records the creating Bot as
   actor, and worker-secret reads are audited — together fully answering
   "which task did what, using whose tools/credentials" without new state.

## Acceptance Criteria

- [ ] **Nothing persistent changes**: no schema change, no `create_spectask`
      argument, and — beyond round 6 — no write of any tool grant onto the
      task row at creation.
- [ ] The task's Helix org tool surface = (existing `agent_tools`-allowlisted
      org tools, unchanged rules) ∪ (live tool set of the project-owning Agent
      when one exists), at tools/list, the channel's mount decision, the
      cache-bust `rev`, and the REST tools view; derived identically
      server-side everywhere.
- [ ] The `helix-tasks` channel mounts exactly when that union is non-empty
      (the natural consequence of computing the surface at the existing
      zed-config choke point); unbound org tasks with no other tools keep
      today's zero-footprint behavior; non-org tasks and tasks with no org
      entitlement are byte-identical to today.
- [ ] Derivation fails closed: zero owners → task-own tools only (clean error
      if such a task calls org tools on a stale cached list); more than one
      owner → treated as zero, with a WARN log.
- [ ] Live propagation: `attach_tool`/`detach_tool`, secret rotation/ungrant,
      Agent firing or project-rebind take effect at the next
      tool-surface/secret refresh; no stale copies (rev flips on next config
      fetch = desktop restart/resume; values per `get_secret` call).
- [ ] Delegation identity: one caller→subject resolution rule used by every
      org tool reachable to tasks (`SubjectForCaller`: Bot callers =
      themselves; project-principal callers = bound Agent; unbound = clean
      error). Each such tool is classified and tested as on-behalf-of /
      task-scoped / unsupported-delegation (loud error, zero writes), with
      representative coverage: `chat` posts as the Agent, `managers`/`reports`
      read the Agent's line, `get_secret`/`list_secrets` read the Agent's
      bindings, spec-task CRUD keys to the task, at least one unsupported-deleg
      tool errors loudly. No tool ever offers more than the bound Agent's
      current grants.
- [ ] Every org call by a task is org-audited with actor = task id; worker-
      secret reads log subject = bound Agent+secret, success and failure.
- [ ] Prompt behavior is unchanged: existing delegation section untouched, no
      credentials section or task-list plumbing added — tool discovery is by
      the tool list itself.
- [ ] Guardrail: spec-task sessions never carry `Metadata.OrgWorkerID`;
      task keys still 403 on `/mcp/helix-org`; all task org access rides the
      task-scoped channel.
- [ ] e2e: Agent-owned project → both a Bot-spawned and a UI-created task show
      the union surface; a task in an Agent-less org project and any non-org
      task show zero change; firing the Agent leaves tasks usable with the
      surface shrunk and loud errors; rev/refresh semantics as specified; full
      planning→implementation→PR lifecycle verified with and without binding.

## Open Questions

1. **Spawned-vs-UI invisibility (accepted consequence):** with no stamp, the
   task row no longer shows org capability at all — it's visible only in the
   runtime tool list and audit trail, and UI-created tasks in owned projects
   are silently entitled (round-3 semantics). This is exactly the
   "just pass the MCP" simplicity you asked for; if you ever want row-visible
   capability back, say so and the round-6/7 wildcard-stamp design returns.

## Resolved questions

- **Terminology/machinery (round 7)** — no "armed" concept; server-side
  derivation decides the surface; URL mount happens iff entitled. Supersedes
  the round-5/6 stamp.
- No new DB fields; no project toggle; whole-set live inheritance (no flags,
  no UI); no env vars (`get_secret` per call); no creator-name column (offer
  stands); all-agent-tools scope over secrets-only (round 6).
