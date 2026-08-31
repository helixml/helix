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
must be derived per spawning-agent, never global.

A spec task's coding agent already has a scoped MCP channel back into the org
tool registry (`SpecTaskMCPBackend`, `/api/v1/mcp/helix-tasks`, authenticated by
its session-scoped API key, with a `ProjectPrincipal` identity). Today that
surface is intersected with a catalogue that contains only spec-task CRUD tools.

**Two rejected alternatives, recorded deliberately:**

- *Attach the Worker's own `helix-org` MCP URL/key to the task.* The
  `helix-org` backend resolves the Worker only from `OrgWorkerID` on the key's
  session; a spec-task session has none. Making it work means either handing the
  task the Worker's own desktop API key (full impersonation: `chat`,
  `create_bot`, org mutations, sandbox controls — not just secrets, audit logs
  show the Worker, and revoking the task can never revoke less than the whole
  Worker) or pushing task→worker impersonation into that shared backend, which
  is the same provenance mechanism done below the abstraction boundary and with
  a much larger tool surface than the need.
- *A per-task `secret_grants` argument.* Rejected on review: Workers spawn work
  frequently and hold their grants as a trusted set; pasting values into task
  descriptions is already possible today, so per-task name selection adds
  ceremony without a real security gain. Inheritance is whole-set and automatic.

## User Stories

1. As a Worker with granted secrets, I want every spec task I spawn to be able
   to discover and read my credentials with `list_secrets`/`get_secret`, so the
   task can complete the credentialed part of the job without me pasting values
   into the task description and without naming grants at creation time.
2. As an org owner, I want a spawned task to see **exactly and only** the
   secrets bound to its spawning Worker — never another Worker's — so access
   remains agent-specific.
3. As a task agent, I want secrets resolved at use time (not copied into my
   prompt, DB row, or container env), so that rotating or revoking a Worker
   secret takes effect on my next `get_secret` call.
4. As a task agent, I want to discover what I can use first (`list_secrets` →
   names + usage metadata, values and backend source details never exposed),
   exactly like the Worker does — including an empty list when my Worker holds
   nothing yet.
5. As a task agent, I want tasks I spawn myself to inherit the same
   credentials, so delegation keeps working down the task tree without
   escalating to other Workers' secrets.
6. As an auditor, I want every credential read by a task logged with the task
   as actor and the source Worker + secret as subject, matching the audit trail
   Bots already produce.
7. As the human owner of a project, I want a Worker-spawned task to remain a
   first-class, **independently usable** spec task — a spawned task is a normal
   task that happens to carry extra MCP tools. It must behave identically to a
   UI-created task everywhere outside the secret pair, including after the
   spawning Worker is renamed, stopped, or fired.

## Acceptance Criteria

- [ ] `create_spectask`'s contract is unchanged: no secret-related argument. When
      called by a Worker session, the spawned task stores provenance (the
      spawning Worker); tasks created via REST/UI store none and are unaffected.
- [ ] The secret pair is **strictly additive**: at every surface that computes a
      task's tool set (MCP tools/list, the `helix-tasks` cache-bust rev, the REST
      agent-tools view, the planning prompt), a provenance-bearing task shows
      its pre-change surface **plus** exactly `list_secrets` + `get_secret`. No
      existing tool is removed, hidden, or given new behaviour, and a
      provenance-less task's surfaces are unchanged from today.
- [ ] Everything outside the secret pair treats a spawned task exactly like a
      UI-created task: planning, revision, approval, implementation, agent
      start/stop/restart, PRs, labels, and the human UI work identically with or
      without provenance; an end-to-end run of a Worker-spawned task through the
      normal UI (no org session involved) completes a full planning →
      implementation → PR cycle.
- [ ] If the spawning Worker is fired/deleted, the task remains fully usable:
      every non-secret capability is unaffected, and `list_secrets`/`get_secret`
      calls return a clear error rather than hanging or breaking the session.
- [ ] A task spawned by a Worker sees `list_secrets` and `get_secret` on its
      `helix-tasks` MCP surface — mirroring the Worker baseline where the pair
      is in `BaseReadTools` regardless of current bindings — without requiring
      the project allowlist to name those tools.
- [ ] `list_secrets` from a task returns exactly the spawning Worker's current
      bindings (names + metadata), never values or backend source details; an
      empty list when the Worker holds none.
- [ ] `get_secret` from a task returns the same freshly-resolved value the
      Worker would get, scoped to the Worker's bindings.
- [ ] A task with no Worker provenance (REST/UI-created) sees neither tool.
- [ ] A secret rotated or ungranted on the Worker takes effect on the task's
      next `get_secret` immediately; already-running shell work is unaffected
      (per-call resolution).
- [ ] A sub-task created by a task agent's `create_spectask` inherits the
      parent task's provenance; a task can never name another Worker or task to
      borrow its secrets — tool arguments accept no worker/task id, and
      provenance comes only from stored rows behind the authenticated session.
- [ ] Every successful and failed `get_secret` call from a task is recorded by
      the existing worker-secret audit recorder, attributed to the task id as
      actor and the source Worker/secret as subject.
- [ ] When Zed/agents cache the tool list, the cache-busting rev for the
      `helix-tasks` server changes when hidden-to-visible flips for a
      provenance-bearing task, so a running session's tool surface updates like
      other AgentTools edits.
- [ ] The task's planning prompt gains a short "credentials inherited from the
      spawning agent — use list_secrets/get_secret" hint when the task has
      provenance. The hint is its own section: the existing "Delegating to other
      spec tasks" section still renders from delegation/lifecycle tools only, so
      a task granted just the secret pair is never told it can create or steer
      other tasks. No credential values in prompts.
- [ ] No secret values are written to the SpecTask row, the create call's
      logged args, or the sandbox environment anywhere in this design.

## Open Questions

1. **Project-level gate:** Is Worker-side spawning plus provenance-scoped
   resolution enough, or do project admins also need a hard "spec tasks may use
   worker secrets" project toggle (more UI/Config work; the org philosophy
   prefers trusting the Worker + enforcing at the secret boundary)?
2. **Per-binding hiding:** Later, should a Worker be able to exempt a specific
   binding from tasks it spawns (e.g. an "inheritable" flag on the grant)
   without un-granting it for itself? Assumed no for v1.
3. **UI surfacing:** Should the spec-task detail page show "credentials
   inherited from agent X"? Assumed no for v1 (MCP-only path).
4. **Shell ergonomics:** Task agents will fetch the value via `get_secret` and
   then use it with shell tools (same pattern Workers use — never via boot env
   vars). Confirm no task needs the values pre-materialized as env vars in the
   container.
