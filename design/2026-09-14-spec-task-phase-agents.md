# Spec task planning and implementation agents

## Decision

A spec task owns two complete coding-agent configurations:

- `PlanningCodeAgentConfig` generates and revises the specification.
- `CodeAgentConfig` implements the approved specification.

Projects store the same pair as defaults. New tasks snapshot both so later
project-setting changes do not alter work already in progress. When no planning
configuration exists on a historical project or task, Helix copies the
implementation configuration at the existing start-time migration boundary.

## What we learned from fx

The premise needs one correction: as of `vercel-labs/fx` main at
`2bd8460fc08dc15793480ee734bcd0cc28140b13`, fx does not expose a native
planner/implementer split with separate phase models. Its
[model selection](https://fx.sh/docs/configure-fx/models) is a user/session
preference, and [subagents](https://fx.sh/docs/capabilities/subagents) inherit
the parent's model and reasoning effort.

The transferable ideas are the lifecycle boundaries around those features:

- fx is model-agnostic and can run as an
  [ACP server](https://fx.sh/docs/using-fx/acp), so harness and model are
  independent choices rather than an Agent identity.
- `/clear` starts a fresh conversation while preserving workspace background
  processes. ACP `session/resume` reconnects without replaying history. Both
  distinguish conversational state from workspace state.
- Subagents have isolated conversations but share the workspace, making their
  state visible without polluting the parent conversation.
- [Project instructions](https://fx.sh/docs/configure-fx/project-instructions)
  are assembled as bounded, target-scoped context instead of copying an entire
  prior transcript into every request.
- fx keeps a model preference per provider. Switching providers restores the
  last compatible model instead of treating provider and model as unrelated
  text fields.

Helix already owns a stronger planning primitive than fx: durable requirements,
design, and task documents with a human approval gate. We combine that with
fx's clean separation of conversation, workspace, harness, and model. We do not
model planning as a subagent, because it needs an independently selectable
harness/model and durable project/task defaults.

## Phase boundary

Planning and implementation keep the same Helix session, sandbox, and working
tree. Approval clears the ACP thread, applies the implementation configuration,
and sends the implementation prompt as the first turn on a fresh thread. The
planner transcript is not injected. The handoff consists of the original
request, approved `requirements.md`, `design.md`, and `tasks.md`, approval
comments, repository instructions, and the existing workspace.

Task status determines the active configuration. Spec generation, review, and
revision use planning; queued implementation and every later state use
implementation. Zed configuration, usage attribution, subscription preflight,
session forks, and task execution controls all resolve through this same phase
mapping.

Changing the inactive phase only updates its task snapshot. Changing the active
phase keeps the existing behavior: cancel the current turn and open a fresh ACP
thread when the sandbox is live.

## Product surface

Project Settings shows separate Planning and Implementation selectors with the
reasoning effort aligned at the far edge of each row. Hover guidance recommends
an intelligent, high-reasoning planner and a faster, lower-cost implementer.
The new task form starts from those defaults and permits a per-task override
for either phase.

The task page keeps the normal `AgentChat` in the left pane. The right workspace
opens Plan beside Desktop, Diff, Files, Agents, and Details during planning;
Browser returns when implementation begins and there is an application to
preview. Plan embeds the existing design-review document surface, so reading,
commenting, revising, and approving the plan no longer opens a separate
workspace tab. Diff, files, and subagents remain available throughout planning.
Before the first successful local push, Plan explains that documents are pending
instead of disappearing.

`helix-specs` is local-authoritative. Helix creates the review and serves its
documents from the local bare repository; publishing that branch to the external
VCS is best-effort and never gates planning on the acting user's repository
permissions. Failed external publication does not roll back the local branch,
and later external syncs exclude `helix-specs` so they cannot erase local plans.
Normal code branches remain mirrored and keep their existing rollback behavior.

Rendered plan documents use the same Markdown component and typography tokens
as `AgentChat`. A source toggle opens the underlying Markdown as editable plain
text, with preview, save, cancel, and `Cmd/Ctrl+S`. Saves commit the canonical
file on `helix-specs` and update the review/task snapshots. The client sends the
content it started from, and the API returns a conflict if the planning agent
changed that document in the meantime; reviewer edits never silently overwrite
an agent push. Inline comments remain anchored and editable from rendered mode.

The project chat composer remembers Plan versus Build per user and project.
Selecting Plan switches its single runtime selector to the project's planning
configuration; selecting Build switches it back to the implementation
configuration. A later manual selector change remains task-local and is not
reset while the selected mode stays active.

## Compatibility

The API adds `planning_code_agent_config` to project and task create/update
payloads and `phase` to task execution-configuration updates. Omitting `phase`
continues to edit the currently active configuration. Existing clients that
only send `code_agent_config` retain the old behavior because planning defaults
to the same configuration.
