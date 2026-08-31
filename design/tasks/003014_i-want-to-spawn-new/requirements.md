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

## User Stories

1. As a Worker with a granted secret (e.g. `SLACK_TOKEN`), I want to spawn a
   spec task and let it read that credential with `list_secrets`/`get_secret`,
   so the task can complete the credentialed part of the job without me pasting
   values into the task description.
2. As an org owner, I want the task to see **only** the secrets I granted the
   spawning Worker — and only the names the Worker chose to pass at creation —
   so a spawned task can never widen its credential access or borrow another
   Worker's secrets.
3. As a task agent, I want secrets resolved at use time (not copied into my
   prompt, DB row, or container env), so that rotating or revoking a Worker
   secret takes effect on my next `get_secret` call.
4. As a task agent that was spawned with credentials available, I want to
   discover them (`list_secrets` → names + usage metadata, values never
   exposed) before fetching, exactly like the Worker did.
5. As a task agent, I want to spawn my own sub-tasks with at most the grants I
   myself hold, so delegation cannot escalate credentials down the task tree.
6. As an auditor, I want every credential read by a task logged with the task
   as actor and the source Worker + secret as subject, matching the audit trail
   Bots already produce.

## Acceptance Criteria

- [ ] `create_spectask` accepts an optional `secret_grants` list of secret names;
      names not bound to the calling Worker are rejected at creation time with a
      clear error.
- [ ] The created task stores the spawning Worker id and the granted names;
      tasks created via REST/UI (no Worker) store neither and are unaffected.
- [ ] A task with grants receives `list_secrets` and `get_secret` on its
      `helix-tasks` MCP surface without requiring the project allowlist to name
      those tools; `list_secrets` returns exactly the granted names with the
      Worker binding's metadata and never values or backend source details.
- [ ] `get_secret` from a task returns the same freshly-resolved value the
      Worker would get; resolving a name outside its grants returns an error.
- [ ] Tasks with no grants see neither tool.
- [ ] A secret revoked/ungaranted on the Worker causes the next `get_secret`
      from the task to fail immediately; already-running shell work is
      unaffected (per-call resolution).
- [ ] A sub-task created by a task agent inherits the parent's source Worker and
      may only request grants that are a subset of the parent's grants; a
      non-subset request is rejected.
- [ ] A task cannot address another task's or Worker's secrets: tool arguments
      accept no worker/task id for secret resolution; provenance comes only from
      the stored task row behind the authenticated session.
- [ ] Every successful and failed `get_secret` call from a task is recorded by
      the existing worker-secret audit recorder, attributed to the task id as
      actor and the source Worker/secret as subject.
- [ ] When Zed/agents cache the tool list, the cache-busting rev for the
      `helix-tasks` server changes when a task's grants are added, so a running
      session's tool surface updates like other AgentTools edits.
- [ ] The task's planning prompt gains a short "credentials available" hint
      (names + usage only) when grants exist, mirroring the existing
      `BuildAgentToolsSection` delegation hint.
- [ ] No secret values are written to the SpecTask row, the create call's
      logged args, or the sandbox environment.

## Open Questions

1. **Grant default:** Should omitting `secret_grants` on `create_spectask` pass
   no secrets (my assumption — explicit opt-in) or inherit all of the Worker's
   bindings? Inheriting-all is more convenient but silently ships every Worker
   credential into every spawned sandbox.
2. **Project-level gate:** Is Worker-side choice plus tool-level scoping enough,
   or do project admins also need a hard "spec tasks may borrow worker secrets"
   project toggle (more UI/Config work; the org philosophy prefers trusting the
   Worker + enforcing at the secret boundary)?
3. **Post-creation grants:** Should `update_spectask` allow adding/removing
   grants after creation? Assumed no for v1 (recreate the task instead).
4. **UI surfacing:** Should the spec-task detail page show "credentials
   inherited from worker X" in the UI? Assumed no for v1 (MCP-only path).
5. **Shell ergonomics:** Task agents will still fetch the value via `get_secret`
   and then use it with shell tools (same pattern Workers use — never via boot
   env vars). Confirm no task needs the values pre-materialized as env vars in
   the container.
