# Requirements: Grant Worker Secrets to Spawned Spec Tasks

## Background

A helix-org Worker (Bot) can spawn spec tasks into its Helix project via the
`create_spectask` MCP tool. In helix-org, credentials are granted to a Worker as
secret *bindings* (org + worker + name → a Helix secret or connected-account
value); the Worker discovers them with `list_secrets` and reads values on demand
with `get_secret`. A spec task spawned by that Worker runs its coding agent in a
sandbox that has **no access to those bindings**, so the task cannot do
credentialed work (push with the Worker's git token, call a deploy API, post to
Slack). Different Workers hold different secrets, so whatever the task receives
must be derived per-agent, never global.

The helix-org runtime **already records which Agent owns which project**: every
hired Bot has runtime state pointing at its project, and every spec task has a
project. That mapping is sufficient to answer "whose secrets does a task use" —
the Agent bound to the task's project. The task's coding agent already has a
scoped MCP channel into the org tool registry (`SpecTaskMCPBackend`,
`/api/v1/mcp/helix-tasks`, authenticated by its session-scoped API key). This
feature is strictly **additional MCP tools** on that channel, granted by
derivation, not by new state.

**Rejected alternatives, recorded deliberately:**

- *New provenance column on the spec task* (`secrets_worker_id`) — rejected on
  review. The fact is already derivable from existing bot→project state; storing
  it adds a column, clone/clear bookkeeping, and a second source of truth that
  can drift from the live binding. The origin event (who spawned the task, via
  which API) is ephemeral; the standing project↔agent bond is what should gate
  access. "Reuse the existing spec task — MCP tools are just an addition."
- *Attach the Worker's own `helix-org` MCP URL/key to the task* — that backend
  authorizes on the Worker identity in the key's session; a task session isn't
  one. Making it work means full impersonation (every tool, audit logs as the
  Worker, no finer revocation) or re-implementing this spec's derivation inside
  the wrong backend with the whole bot surface exposed.
- *Per-task `secret_grants` argument* — rejected on review: ceremony without
  gain; the Worker is the trusted grant holder. Whole-set inheritance.

## User Stories

1. As a Worker with granted secrets, I want every spec task in my project to be
   able to discover and read my credentials with `list_secrets`/`get_secret`, so
   the task completes credentialed work without me pasting values anywhere.
2. As an org owner, I want a task to see **exactly and only** the bindings of
   the Agent bound to its own project — a project no Agent owns gets none — so
   access stays agent-specific while remaining derivable from data, not
   bookkept per task. No Bot's credential can ever reach a project it does not
   own.
3. As a task agent, I want secrets resolved at use time (never copied into my
   prompt, DB row, or container env), so rotating or revoking a Worker secret
   takes effect on my next `get_secret` call, and firing the Worker removes the
   tools at the next session refresh.
4. As a task agent, I want to discover what I can use first (`list_secrets` →
   names + usage metadata, values and backend source details never exposed),
   exactly like the Worker does — including an empty list when my Agent holds
   nothing yet.
5. As a task agent, I want tasks I spawn (including into other projects I
   manage) scoped by the same rule: each task shows its own project's Agent
   bindings — never the spawning task's — so delegation never smuggles extra
   credentials across project boundaries.
6. As an auditor, I want every credential read by a task logged with the task
   as actor and the source Worker + secret as subject, matching the audit trail
   Bots already produce.
7. As the human owner of a project, I want every spec task — spawned or
   UI-created, inside or outside an Agent-owned project — to remain a
   first-class, **independently usable** task. Nothing about this feature may
   couple a task's lifecycle to an Agent; a spawned task is a normal task that
   happens to see two extra MCP tools.

## Acceptance Criteria

- [ ] Schema and contract are untouched: no new DB column on spec tasks or
      sessions, no `create_spectask` argument, no task-creation/plumbing changes.
      "Receives secrets" is computed from existing org state on every relevant
      request.
- [ ] A task's secret access is derived: exactly one org Agent has the task's
      project as its own runtime project → that Agent's bindings apply; zero
      Agents → no secret tools, clean "not bound" error; ambiguous (>1) → fail
      closed as zero-Agents. Only an Agent's **own** project derives (never a
      managed/allowlisted project): a multi-project manager Bot leaks nothing.
- [ ] Where bound, the task's effective Helix MCP tools equal the pre-change
      surface **plus** exactly `list_secrets` + `get_secret` at every surface
      (MCP tools/list, the `helix-tasks` cache-bust rev, the REST agent-tools
      view, the planning prompt hint). Nothing existing is removed or
      reinterpreted; unbound tasks are byte-identical to today, including the
      helix-tasks mount decision.
- [ ] The secret pair alone mounts the `helix-tasks` channel for a bound task
      with no other agent tools, and never makes the delegation prompt section
      render — "credentials available" is its own section, gated on the same
      derivation, never enumerating delegation tools (`create_spectask` is not
      part of the pair).
- [ ] `list_secrets` from a task returns the bound Agent's current bindings
      (names + metadata), never values or backend source details; `get_secret`
      returns the same freshly-resolved value the Agent would get, scoped to its
      bindings.
- [ ] Tool arguments accept no worker id or task id; derivation is server-side
      only, and the task's own session is never marked with a Worker identity
      (stamping `Metadata.OrgWorkerID` on a spec-task session is prohibited —
      that flag authorizes the full bot surface on a different backend).
- [ ] Rotation/revocation/ungarnting takes effect on the next `get_secret`;
      firing/deleting the bound Agent unbinds the project — tools disappear at
      the next tool-surface refresh (restart/resume), the task itself stays
      fully usable throughout; if a new Agent is later bound, new config picks
      it up.
- [ ] A task in an Agent-owned project gets the pair regardless of creation
      path (spawned via MCP or created in the UI) — the project bond, not the
      origin event, is the authorization; this is intended behavior.
- [ ] Every successful and failed `get_secret` call from a task is recorded by
      the existing worker-secret audit recorder, attributed to the task id as
      actor and the bound Agent/secret as subject.
- [ ] Everything outside the secret pair treats every task identically:
      planning, revision, approval, implementation, agent start/stop/restart,
      PRs, labels, and the human UI work the same with or without a bound Agent
      (e2e-proven with an unbound task completing a full cycle).
- [ ] No secret values are written to the SpecTask row, the create call's
      logged args, or the sandbox environment.

## Open Questions

None — all resolved.

## Resolved questions

- **New task fields? — resolved by review: no.** Reuse the existing spec task
  unchanged; the owning Agent is derived from the existing bot→project mapping
  at request time (see Rejected alternatives).
- **Env vars?** — confirmed by review: no pre-materialized environment;
  `get_secret` + shell tools, per call, same pattern Workers use.
- **UI surfacing** — no detail-page display; in-task `list_secrets` is the
  surface.
- **Per-binding hiding flag** — no, now or later; whole-set inheritance for a
  bound project is the invariant.
- **Project-level "may use worker secrets" toggle** — no; the project↔Agent
  bond *is* the boundary ("the same project in reality anyway").
