# Spec-task agent tools: project grant + per-task extras

Gives spec tasks a configurable Helix MCP tool surface, so a task can create and
steer other spec tasks as sub-agents. Two scopes, union semantics:

1. **Project** grants a tool set to every spec task in it.
2. **Task** adds extra tools on top, for that task's lifetime.
3. **Effective = project ∪ task.** The task picker shows the project's tools
   checked and read-only, chipped `From project`.

The agent authenticates with the session-scoped api key its sandbox already
holds. Identity comes only from the stored session that key names.

## What already existed

The tool registry, the per-caller MCP server, the gateway, and
`ToolPickerDialog` are all reused as-is. The only gap was that
`zed_config.go` gated the org tool surface on `orgWorkerID`, which spec tasks
never have — so they got no Helix MCP at all.

## Changes

**Caller identity.** Org tools resolve their project from per-Worker runtime
state (`LoadState`), which a spec task has none of. `runtime.ProjectPrincipal`
carries the project + acting user on the context instead — the same mechanism
this runtime already uses for caller identity (`WithHelixIdentity`). Two
branches consume it: `resolveProject` (helix/spectasks.go) and `callerIdentity`
(application/spectasks). A principal is pinned to one project; naming any other
is refused, not redirected.

**Eligibility is data.** `mcptools.SpecTaskAgentTools` lists the 14 tools that
work for a project principal. Selections are intersected with it on save *and*
at call time, so a stale or hand-crafted name can never widen the surface.
Adding a tool is a one-line data edit.

**Storage.** `Project.AgentTools []string` and `SpecTask.AgentTools []string`
(both jsonb). Empty = no MCP surface at all, so this is opt-in and existing
projects are unchanged. GORM AutoMigrate.

**Enforcement.** `SpecTaskMCPBackend` (`helix-tasks` on the gateway) resolves
session → task → project, authorizes, computes the tool list and delegates to
`Server.ServeMCPForCaller` — the same registry, transport, and audit path Bots
use. Spec-task callers declare `AuditActorType() = spec_task` so the org audit
log does not record them as Bots.

**Sandbox.** `GenerateZedMCPConfig` adds context server `helix-tasks` when the
list is non-empty, with `?rev=<hash>`. Zed caches `tools/list` from initialize,
so the rev is what makes an edit land mid-session: list changes → settings.json
changes → Zed restarts that one context server. `helix-tasks` is in the
daemon's `HELIX_OWNED_CONTEXT_SERVERS` so a stale on-disk entry cannot win.

**Discovery.** The tools reach the model through MCP `tools/list`, so they show
up in its tool list like any other tool. That is enough to *use* them but not
enough to *reach for* them — nothing in the spec-task prompt said delegation was
an option, so unprompted an agent never did. `BuildAgentToolsSection` adds a
short "Delegating to other spec tasks" block to the planning and implementation
prompts when the grant is non-empty, following the existing
kodit/repo/attachments section convention. It carries the two rules the tool
descriptions cannot convey: a new task sits in backlog until
`start_spectask_planning`, and the sub-agent cannot see the parent's
conversation. Empty grant → no section at all.

**API.** `GET /api/v1/agent-tools` returns the catalogue; `agent_tools` is a
field on the existing project and spec-task update requests. No new write
endpoints.

**UI.** `ToolPickerDialog` gained `lockedTools` (checked, disabled, chipped,
excluded from Enable-all/Clear-all and never returned by `onApply`). A shared
`AgentToolsPicker` renders chips + Edit at both scopes: project settings →
Skills tab, and the task detail Task-setup panel under Execution.

## Verified live (100.108.100.25:8080, unmanned-org)

- Catalogue endpoint returns the 14 tools.
- Save-time sanitization drops `delete_bot` / `publish`.
- A real headless spec task's `zed-config` and in-sandbox `settings.json` both
  carry `helix-tasks` with the session key and rev.
- MCP `initialize` + `tools/list` over the session key returns exactly the
  project's 3 tools; adding 2 task tools makes it 5 with no restart, and the
  rev changes (`1b87f419` → `7a47869e`).
- `create_spectask` over MCP creates a correctly scoped, correctly attributed
  task. Naming a sibling project is refused.
- **The live agent in the sandbox listed `create_spectask, get_spectask,
  list_spectasks` and then used `create_spectask` to spawn a sub-task**
  (`spt_01m0k43yqc408c9zndsjsren13`).
- Deny-by-default: no tools → no context server, and MCP 403.
- A plain user api key is rejected; another task's session key resolves only to
  its own task.
- Audit rows land as `spec_task | spt_… | create_spectask | succeeded`.
- An **already-running** task (`spt_01m0jzg53…`, helix-next) picked up all 14
  tools after they were enabled in the UI — no restart, rev `d681819f`.

## Not verified

- The two UI surfaces were typechecked, unit-tested, and built, but not
  visually confirmed in a browser.
- Only `zed_agent` was exercised live. Claude Code / Codex / opencode read the
  same `context_servers` map, but were not run (this environment has no ChatGPT
  subscription for codex).

## Follow-ups worth considering

- `SpecTask.ParentTaskID` for nesting in the UI and a recursion depth cap.
  Project WIP limits already bound auto-start fan-out.
- Whether agent-created tasks should get a different WIP limit than human ones.
