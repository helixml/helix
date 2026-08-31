# PR: Give Spawned Spec Tasks Their Agent's Tool Access

Repo: helixml/helix — branch `feature/003014-give-spawned-spec-tasks`

Spec: helixml/helix-specs `design/tasks/003014_i-want-to-spawn-new/`
(requirements.md / design.md / tasks.md). No schema changes, no new
config, no DTO changes (no openapi regen needed).

## What this does

A spec task's coding agent now operates with the tools of the helix-org
Agent whose runtime home project is the task's project. The task's org
tool surface, computed identically at both server choke points, is:

    (task's own allowlisted catalogue grants)  ∪  (live Node.Tools of the
    Agent bound to the task's project)  −  (delegation-blocked admin tools)

- "Bound" = exactly one non-human org Node whose `helix` runtime state
  names the task's project as home (`BoundAgentForProject`). Zero or two
  such Agents → no bond, fail closed, WARN on ambiguity. Managed
  `ProjectIDs` allowlist membership never counts.
- Live means live: attach_tool/detach_tool on the Agent, or removing the
  bond (fire/re-home), change what the next surface resolution shows —
  nothing about the grant is persisted on the task.
- The existing mount rule (`len(specTaskTools) > 0` in
  external-agent GenerateZedMCPConfig, rev = `AgentToolsRev`) decides the
  `helix-tasks` context-server mount/revision for free; no external-agent
  changes.
- On-behalf-of tools (chat, dm, managers, reports, read_events, bot_log,
  ask_human, all asset/server tools, get_secret, list_secrets) act AS the
  bound Agent via `mcptools.SubjectForCaller`; audit attribution stays
  with the task (verified live: `org_audit_logs.actor = spt id,
  actor_type = spec_task, action = get_secret, succeeded` — the bot's
  project secret read via the task's own key on
  `/api/v1/mcp/helix-tasks`).
- Delegation-incompatible org-admin tools are filtered out of the surface
  at the shared choke point, so they are absent from tools/list AND from
  the callable registry (dispatch authorizes by served-list membership):
  a task calling one gets an unknown-tool error with zero side effects.
- Guardrail intact: task sessions never carry `Metadata.OrgWorkerID`, so
  task keys still 403 on `/api/v1/mcp/helix-org` (live-verified).

## Tool identity classification (merge-blocking deliverable)

(a) on-behalf-of — converted to `SubjectForCaller`; (b) task/resource-
keyed — correct for tasks as-is; (c) blocked for task callers
(`mcptools.SpecTaskBlockedTools`).

| Class | Tools | Notes |
|---|---|---|
| (a) converted | `chat`, `dm`, `managers`, `reports`, `read_events`, `bot_log`, `ask_human`, `list_assets`, `get_asset`, `server_run_command`, `server_list_commands`, `server_get_command`, `server_kill_command`, `server_list_files`, `server_read_file`, `server_write_file`, `server_ssh_access`, `get_secret`*, `list_secrets`* | *done pre-sweep. `bot_log` attaches the transcript to the Agent but records `created_by` = the task id. `server_ssh_access` kept delegable: its 1h cert is the same capability class as `server_run_command`, which a bound Agent already grants. |
| (b) as-is | `list_bots`, `get_bot`, `list_triggers`, `get_trigger`, `list_trigger_events`, `trigger_members`, `list_processors`, `get_processor`, `list_projects`, `get_project`, `list_sandboxes`, `get_sandbox`, `list_sandbox_runtimes`, `list_org_assets`, `get_org_asset`, `list_asset_links`, `get_asset_health`, `list_repositories`, `list_bot_repositories`, `get_bot_project`, all 14 `*_spectask*` tools | Reads keyed on org + args. The service-gated ones (projects/sandboxes/spectask membership) fail closed with clear errors for unbound callers — strictly narrower than the bound Agent's reach, never wider. Spec-task CRUD already had the ProjectPrincipal branch. |
| (c) blocked | `create_bot`, `delete_bot`, `set_bot_content`, `attach_tool`, `detach_tool`, `start_bot`, `stop_bot`, `restart_bot`, `attach_worker`, `detach_worker`, `create_trigger`, `create_processor`, `update_processor`, `delete_processor`, `set_human_contact`, `configure_bot_project`, `attach_repository`, `detach_repository`, `create_server_asset`, `update_server_asset`, `delete_asset`, `link_asset`, `unlink_asset`, `create_sandbox`, `update_sandbox`, `delete_sandbox`, `sandbox_ssh_access` | Org-graph mutations, tool/grant edges, inbound endpoints, org compute admin. `attach_tool` on the task's own Agent was the escalation vector this list closes. |

Silent-wrong-answer fixes that fall out of the (a) conversions: `chat`
previously posted as `spt-…`, `managers`/`reports`/`list_assets`
returned empty, `read_events` returned empty — all now resolve the
subject or fail loudly with zero events (unit-tested:
`TestChatFailsClosedWhenUnbound`, `TestManagersDelegatedWalksAgentLine`).

## Key code

| File | Change |
|---|---|
| `api/pkg/org/infrastructure/runtime/helix/agent_binding.go` | `BoundAgentForProject`, `AgentToolNames`, `ErrNoBoundAgent` (fail closed on 0/>1 owners; never keyed on `ProjectIDs`) + tests |
| `api/pkg/org/infrastructure/runtime/principal.go` | `WithBoundWorker`/`BoundWorkerFromContext` (ctx bond, mirrors `WithProjectPrincipal`) |
| `api/pkg/org/interfaces/mcptools/subject.go` | `SubjectForCaller` + `ErrNoBoundWorker` with the (a)/(b)/(c) contract documented (helper lives in mcptools, the layer both tool impls and the org server dispatch already share) |
| `api/pkg/org/interfaces/mcptools/defaults.go` | `SpecTaskBlockedTools` / `IsSpecTaskBlockedTool` (policy data beside the existing `SpecTaskAgentTools` catalogue) |
| `api/pkg/server/mcp_backend_spectask.go` | `specTaskToolSurface` — ONE function the mount/rev view and the MCP backend share; drops blocked names; stashes the bond on ctx |
| `api/pkg/server/agent_tools_handlers.go` | `specTaskAgentTools` = surface as `[]string` (REST view ≡ rev ≡ tools/list by construction) |
| `api/pkg/org/interfaces/mcptools/{chat,dm,managers,reports,read_events,bot_log,ask_human,assets,get_secret,list_secrets}.go` | class-(a) conversions |

Tests: helpers/bonding (0/1/2/human/managed-list), surface (unbound ≡
today, union, ambiguity fail-closed, blocked-name exclusion),
specTaskAgentTools table incl. non-task nil, secret-tool subject cases
(never falls back to task-id bindings), delegation representatives,
helix-org backend 403 suite, plus the existing prompt golden (untouched)
and external-agent mount/rev suites.

## Verification

- `CGO_ENABLED=1 go test ./pkg/org/... ./pkg/server/ ./pkg/services/`
  plus `./pkg/external-agent` — all green.
- Live inner-Helix e2e (see tasks.md item 12 for the full record):
  real Bot hire → real project bond → real task + task api key;
  tools/list = Agent surface with own grants empty; `get_secret` read the
  Agent's secret; audit rows attributed to the task id; block-listed
  names never served even while the Agent held them; live attach/detach
  propagation; unbundle → 403 "no Helix tools are enabled for this
  task"; `/mcp/helix-org` stays 403 for task keys.

## Consequences reviewers should know

- A task in an Agent-owned project is silently entitled; capability is
  discoverable through tools/list and audit, not through the task row.
- `create_trigger`, processors, and sandbox admin are class (c) (judgment
  calls recorded in `SpecTaskBlockedTools` doc).
- Known env issue surfaced during e2e (pre-existing, not this PR): bot
  DELETE on a stale dev DB errors on missing `org_subscriptions`.
