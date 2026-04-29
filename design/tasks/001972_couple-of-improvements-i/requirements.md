# Requirements: Agent-Driven Multi-PR & Multi-Spec-Task Proposals

## Problem

Two related limitations in the spec task workflow:

1. **One PR per repo, one task lifecycle.** A spec task can only result in one PR per attached repository, and the orchestrator marks the task `done` when "all tracked PRs merged" — a heuristic that breaks when the agent legitimately needs to ship the work as a series of PRs (e.g., a refactor that splits cleanly into 3 chained PRs on different branches). The agent has no way to say "I'm done now" or "I want to open another PR for the next slice on a different branch".

2. **No way for an implementation agent to spawn follow-up spec tasks.** The Optimus / project-manager agent already has a `CreateSpecTask` tool (`api/pkg/agent/skill/project/spec_task_create_tool.go`), but the implementation-phase agent running inside a spec task has no equivalent. Discoveries during implementation — "this needs a follow-up", "we should also do X in a separate task" — must be recorded by the user out of band.

Both gaps share a constraint: any agent action that can fill the board with garbage or push to an arbitrary branch on a real GitHub/GitLab/ADO repo must be **proposed**, then approved by the user in the Helix UI. We do not give the agent unfettered write access to either the board or to arbitrary remote branches.

---

## User Stories

### 1. Agent proposes opening a PR from a chosen branch

**As an implementation agent**, I want to propose to the user that I open a pull request from a specific branch (which may differ from the system-generated default branch for this task) into a specific base branch, so that I can ship work in slices.

**Acceptance Criteria:**

- The agent calls a new MCP tool `propose_pull_request(repository_id, head_branch, base_branch?, title?, body?)` exposed through the Helix MCP gateway.
- `head_branch` defaults to the system-generated branch for the task (`task.BranchName`); the agent may override it.
- `base_branch` defaults to the repository's default branch.
- `title` / `body` default to the contents of the relevant `pull_request*.md` file in the design docs, when present.
- The proposal is **not** auto-executed. It is persisted as a pending `PRProposal` linked to the spec task and surfaced in the Helix UI on the task card.
- The user sees the proposed branch + base + title/body in the UI and can: **Approve**, **Approve with edits** (modify branch name, base, title, body), or **Reject** with a reason.
- On Approve, Helix performs the existing push + open-PR flow against the configured external repo (GitHub / GitLab / ADO / Gitea), using the approver's OAuth identity (same as today's "Open PR" button).
- On Reject, the proposal is closed and the rejection reason is sent back to the agent as a tool follow-up message.
- The proposal records who approved/rejected it and when, for the audit log.

---

### 2. Agent opens multiple PRs for the same spec task

**As an implementation agent**, I want to open more than one PR for the same spec task (each from a different branch, optionally targeting different bases), so that I can split work into reviewable slices without inventing a new spec task per slice.

**Acceptance Criteria:**

- The agent may call `propose_pull_request` multiple times for the same spec task. Each call creates a separate proposal; each approved proposal results in a tracked PR appended to `task.RepoPullRequests`.
- The existing "all PRs merged → task done" heuristic is **disabled** for tasks where the agent has used `propose_pull_request` (or, equivalently, where the agent has called `mark_task_complete` — see Story 4). Falls back to existing behaviour for legacy tasks that do not use the new tools.
- The Kanban / task UI visibly lists all PRs attached to the task with their state (open / merged / closed).
- The system never auto-creates a PR on its own once the agent is using the proposal flow. The current implicit "Open PR" button still works; using the agent-driven flow does not break it.

---

### 3. Agent proposes spawning a follow-up spec task

**As an implementation agent**, I want to propose that one or more new spec tasks be created on the same project's board (e.g., "follow-up: also rewrite the Bitbucket adaptor for symmetry"), so that work I discover but should not do inline gets captured.

**Acceptance Criteria:**

- The agent calls a new MCP tool `propose_spec_task(name, description, type?, priority?, original_prompt?)` exposed through the Helix MCP gateway.
- The proposal is persisted as a pending `SpecTaskProposal` linked to the parent spec task, and surfaced in the Helix UI on the parent task card and on a project-level "pending proposals" indicator.
- The user can **Approve**, **Approve with edits** (modify name/description/type/priority), or **Reject** in the UI.
- On Approve, a real `SpecTask` is created in the project's `backlog` column via the **same logic** that the existing `CreateSpecTask` tool (`api/pkg/agent/skill/project/spec_task_create_tool.go`) uses. The new task records `parent_task_id` = the proposing task's ID for traceability.
- On Reject, the proposal is closed and the rejection reason is sent back to the agent as a tool follow-up message.
- The Optimus / project-manager agent's existing `CreateSpecTask` tool is **unchanged** — it already runs in an explicit chat with the user, where the user is implicitly approving by asking. The new MCP tool is only for the autonomous implementation-phase agent.

---

### 4. Agent declares the task complete

**As an implementation agent**, I want to declare that a spec task is finished (independently of how many PRs are merged), so that the task moves to `done` based on my judgment, not the brittle "all PRs merged" heuristic.

**Acceptance Criteria:**

- The agent calls a new MCP tool `mark_task_complete(reason?)`.
- This marks the task as **agent-claims-complete** internally and surfaces a UI affordance for the user to confirm. The task does **not** automatically move to `done` — the user clicks **Mark Done** (or **Send Back** with feedback). This keeps the user in the loop and consistent with the "agent proposes, user approves" pattern.
- A spec task may have any number of open / merged / closed PRs at the time `mark_task_complete` is called; the orchestrator no longer requires "all PRs merged" to allow completion.
- For tasks where the agent never calls `mark_task_complete`, the existing "all PRs merged → done" behaviour is preserved (backwards-compatible).

---

### 5. Approval security & branch-name policy

**As a project owner**, I want guardrails so the agent cannot push to an arbitrary branch or fill the board with garbage without my consent.

**Acceptance Criteria:**

- No `propose_pull_request`, `propose_spec_task`, or `mark_task_complete` call can complete an action against an external repo or modify the board without explicit user approval in the Helix UI.
- The default branch name for `propose_pull_request` is the **system-generated** branch (the existing `task.BranchName`). The agent may *request* a different branch name in the proposal; the user sees the requested name and can accept or override it during approval.
- An optional project-level setting `spec_task_proposals.auto_approve` (default: false) may, in trusted environments, allow proposals to be auto-approved. **Out of scope for this task** — listed for future work; default behaviour is always "user must approve".
- The audit log (`audit_log_service.go`) records: proposal created (by agent), proposal decided (approved/rejected, by user, with any edits), and resulting action (PR opened, task created).

---

### 6. Prompt updates so the agent knows about the new tools

**As an agent**, I need the planning and implementation prompts to tell me these proposal tools exist and when to use them.

**Acceptance Criteria:**

- `api/pkg/services/spec_task_prompts.go` (planning prompt) is updated to mention that:
  - The agent may use `propose_spec_task` to propose follow-up tasks discovered during planning.
  - It must NOT use `CreateSpecTask` directly during implementation (that tool is only for Optimus chat sessions).
- `api/pkg/services/agent_instruction_service.go` (implementation/approval prompt) is updated to mention:
  - The agent may use `propose_pull_request` to open additional PRs (e.g., when splitting work).
  - The agent may use `mark_task_complete` to declare the task done; otherwise the existing "all PRs merged" heuristic applies.
  - The current instruction "Do NOT create pull requests yourself" remains in force for `gh pr create` / GitHub MCP tools — `propose_pull_request` is the **only** sanctioned route.

---

## Out of Scope

- Auto-approval of proposals (no `spec_task_proposals.auto_approve` policy yet).
- A separate "agent says it's done" → automatic merge / close flow that bypasses user confirmation.
- Deletion or editing of already-approved-and-opened PRs through proposals (the agent must use existing PR comment / close mechanisms for that).
- New MCP tools for editing files on remote repos directly (out of scope; the agent works in its sandbox and pushes branches as it does today).
- Changes to the existing Optimus `CreateSpecTask` tool (left as-is — it's used in an interactive chat where the user is already in the loop).
- Cross-project task spawning (proposals can only create tasks in the same project as the parent task).
