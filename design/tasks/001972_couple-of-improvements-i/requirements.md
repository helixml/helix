# Requirements: Agent-Driven Multi-PR & Multi-Spec-Task Proposals

## Problem

Two related limitations in the spec task workflow:

1. **One PR per repo, one task lifecycle.** A spec task can only result in one PR per attached repository, and the orchestrator marks the task `done` when "all tracked PRs merged" — a heuristic that breaks when the agent legitimately needs to ship the work as a series of PRs (e.g., a refactor that splits cleanly into 3 chained PRs on different branches). The agent has no way to say "I'm done now" or "I want to open another PR for the next slice on a different branch".

2. **No way for an implementation agent to spawn follow-up spec tasks.** The Optimus / project-manager agent already has a `CreateSpecTask` tool (`api/pkg/agent/skill/project/spec_task_create_tool.go`), but the implementation-phase agent running inside a spec task has no equivalent. Discoveries during implementation — "this needs a follow-up", "we should also do X in a separate task" — must be recorded by the user out of band.

Both gaps share a constraint: any agent action that can fill the board with garbage or push to an arbitrary branch on a real GitHub/GitLab/ADO repo must be **proposed**, then approved by the user in the Helix UI. We do not give the agent unfettered write access to either the board or to arbitrary remote branches.

**Mechanism (matches existing patterns).** The agent makes an MCP call (e.g. `propose_pull_request`) which returns immediately with a pending proposal ID. The user clicks Approve / Reject / Mark Done / Send Back in the UI, and the decision is delivered back to the agent as a **plain text user-turn message rendered from a Go `text/template`** — the same mechanism used today to deliver review comments, revision requests, approval, and merge instructions (`api/pkg/services/agent_instruction_service.go`). No new transport, no long-running MCP requests; the agent sees the decision on its next turn just as if the user typed it.

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

### 2. Agent opens zero, one, or many PRs for the same spec task

**As an implementation agent**, I want to open any number of PRs for a spec task — including zero — so that the workflow matches the actual nature of the work.

**Acceptance Criteria:**

- The agent may call `propose_pull_request` zero or more times for the same spec task. Each call creates a separate proposal; each approved proposal results in a tracked PR appended to `task.RepoPullRequests`.
- **Zero-PR tasks are first-class.** Some tasks (research, analysis, doc-only updates that live in the spec branch) legitimately ship no code. These tasks complete via `mark_task_complete` (Story 4) without ever calling `propose_pull_request`. Nothing in the system requires a PR to exist for a task to reach `done`.
- The Kanban / task UI visibly lists all PRs attached to the task with their state (open / merged / closed). For zero-PR tasks the PR section is hidden or shows "No PRs (knowledge-only task)".
- The system never auto-creates a PR on its own. The existing "Open PR" button in the UI continues to work for the simple single-PR case; agent-driven proposals are an additive path.

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

### 4. Agent declares the task complete (the only path to `done`)

**As an implementation agent**, I want to declare that a spec task is finished, regardless of how many PRs exist or what state they're in, so that the task moves to `done` based on my judgment — and so the brittle "all PRs merged" heuristic stops cutting agents off mid-work.

**Acceptance Criteria:**

- The agent calls a new MCP tool `mark_task_complete(reason?)`.
- This creates a pending `mark_complete` proposal. The user clicks **Mark Done** (or **Send Back** with feedback) in the Helix UI to actually transition the task to `done`. Same agent-proposes, user-approves pattern as the other proposals.
- A spec task may have **any** number of PRs at the time `mark_task_complete` is called: zero, one, several, all merged, none merged, mixed states. None of that affects task completion logic. Completion is decoupled from PR state.
- **The orchestrator's auto-transition to `done` based on PR / branch merge detection is removed entirely.** The four current sites in `spec_task_orchestrator.go` (lines ~781, ~851, ~1080, ~1123) that move tasks to `done` when "all PRs merged" / "branch merged to main" / "externally-opened PR found merged" / "branch detected merged without PR" are deleted. The PR-polling loop continues to run — but only to update `RepoPR.PRState` for UI display, never to change `task.Status`.
- The user may still manually move a task to `done` from the UI (existing affordance unchanged). The two paths to `done` are: (a) the agent calls `mark_task_complete` and the user confirms, or (b) the user marks it done directly. **No third path.**

**Why kill the heuristic entirely** (not just gate it):

The current behaviour terminates spec task agents prematurely. An agent that has opened a PR and is now writing follow-up notes / answering review comments / preparing a second PR gets killed when the first PR happens to merge. The agent's session is shut down based on a guess at completion; real work in progress is lost. Removing the heuristic — not gating it on a flag — eliminates the premature-termination class of bug. There is no "legacy task" backwards-compatibility carve-out to preserve, because the heuristic is precisely what we're calling unreliable.

---

### 5. Approval security & branch-name policy

**As a project owner**, I want guardrails so the agent cannot push to an arbitrary branch or fill the board with garbage without my consent.

**Acceptance Criteria:**

- No `propose_pull_request`, `propose_spec_task`, or `mark_task_complete` call can complete an action against an external repo or modify the board without explicit user approval in the Helix UI.
- The default branch name for `propose_pull_request` is the **system-generated** branch (the existing `task.BranchName`). The agent may *request* a different branch name in the proposal; the user sees the requested name and can accept or override it during approval.
- An optional project-level setting `spec_task_proposals.auto_approve` (default: false) may, in trusted environments, allow proposals to be auto-approved. **Out of scope for this task** — listed for future work; default behaviour is always "user must approve".
- The audit log (`audit_log_service.go`) records: proposal created (by agent), proposal decided (approved/rejected, by user, with any edits), and resulting action (PR opened, task created).

---

### 6. Prompt updates so the agent knows about the new tools and the new completion model

**As an agent**, I need the planning and implementation prompts to tell me these proposal tools exist, when to use them, and that zero-PR completion is a legitimate outcome.

**Acceptance Criteria:**

- `api/pkg/services/spec_task_prompts.go` (planning prompt) is updated to mention that:
  - The agent may use `propose_spec_task` to propose follow-up tasks discovered during planning.
  - It must NOT use `CreateSpecTask` directly (that tool is only for Optimus chat sessions).
  - **Some tasks won't need any code changes** — research, analysis, and pure-knowledge tasks are valid and complete with `mark_task_complete` alone.

- `api/pkg/services/agent_instruction_service.go` (implementation/approval prompt) is updated to mention:
  - The agent may use `propose_pull_request` to open one or more PRs when there is code to ship. Opening **zero** PRs is a valid outcome.
  - **`mark_task_complete` is the only way the task moves to `done`.** There is no auto-completion based on PRs merging. The agent must call it explicitly when the work is judged finished, even for tasks with no PRs and even for tasks where every PR is already merged.
  - **Knowledge capture is encouraged whether or not there's a PR.** Two channels:
    - **Spec branch (`helix-specs`)** — forward-only, no PR needed. Push design notes, gotchas, architecture decisions, post-mortem learnings to `design/tasks/{task_dir}/design.md` or new files in the same directory. Pushes to this branch happen continuously throughout the task; one more push at the end with "what I learned" is the cheapest possible knowledge capture.
    - **Main repo markdown files** — for content that should live alongside the code (`README.md`, `docs/`, `ARCHITECTURE.md`, etc.), include the file in a `propose_pull_request` like any other code change. A doc-only PR is fine.
  - The current instruction "Do NOT create pull requests yourself" remains in force for `gh pr create` / GitHub MCP tools — `propose_pull_request` is the **only** sanctioned route.

---

### 7. Cleanup: rename `PlanningSessionID` → `AgentSessionID`

**As a developer reading the codebase**, I want the data model to reflect reality — there is one agent per spec task, not two — so the naming stops misleading me into thinking there are separate planning and implementation agent instances.

**Acceptance Criteria:**

- The struct field `SpecTask.PlanningSessionID`, the JSON/swagger field `planning_session_id`, the `SpecTaskFilters.PlanningSessionID` filter, and the store method `Store.GetPendingCommentByPlanningSessionID` are all renamed to use `AgentSessionID` / `agent_session_id` / `GetPendingCommentByAgentSessionID`.
- The DB column `spec_tasks.planning_session_id` is renamed via an explicit Postgres `ALTER TABLE ... RENAME COLUMN` migration (GORM AutoMigrate does not rename columns).
- The unused constants `AgentTypeSpecGeneration` and `AgentTypeImplementation` (which currently have **zero** non-definition usages anywhere in the codebase) are deleted.
- The frontend regenerates the API client (`./stack update_openapi`) and updates all TS references from `task.planning_session_id` to `task.agent_session_id` in lockstep.
- No backwards-compatibility aliasing (no dual-name JSON tags, no fallback lookups). One name, one path.
- The phase-named prompt builders (`BuildPlanningPrompt`, `planningPromptTemplate`) and the workflow status constants (`TaskStatusSpecGeneration`, `TaskStatusImplementation`) are **not** renamed — those describe phases of work, which remain a real distinction. Only the *agent/session* naming gets cleaned up.

This cleanup ships in the same PR as the new MCP tools because the new tool registration logic looks up the spec task from the session ID — adding three new call sites that filter by `PlanningSessionID` would double down on the wrong name at exactly the wrong moment.

---

## Out of Scope

- Renaming `BuildPlanningPrompt` / `planningPromptTemplate` / `TaskStatusSpecGeneration` / `TaskStatusImplementation` — these name *phases of work* and remain accurate; only the agent/session naming is cleaned up.
- Auto-approval of proposals (no `spec_task_proposals.auto_approve` policy yet).
- A separate "agent says it's done" → automatic merge / close flow that bypasses user confirmation.
- Deletion or editing of already-approved-and-opened PRs through proposals (the agent must use existing PR comment / close mechanisms for that).
- New MCP tools for editing files on remote repos directly (out of scope; the agent works in its sandbox and pushes branches as it does today).
- Changes to the existing Optimus `CreateSpecTask` tool (left as-is — it's used in an interactive chat where the user is already in the loop).
- Cross-project task spawning (proposals can only create tasks in the same project as the parent task).
