# Design: Agent-Driven Multi-PR & Multi-Spec-Task Proposals

## Overview

Add a new MCP tool surface — exposed through the existing `HelixMCPBackend` at `/api/v1/mcp/helix` — that the implementation-phase agent can call to *propose* PR creation, *propose* new spec task creation, and *declare* its task complete. Each call writes a row to a new `spec_task_proposals` table. The Helix UI surfaces pending proposals on the task card; the user approves/edits/rejects, and the existing PR/task creation code paths execute on approval. No agent action against an external repo or the project board ever happens without user approval.

The "all PRs merged → done" heuristic in `spec_task_orchestrator.go` is preserved for backwards compatibility, but is **bypassed** for any task where the agent has called `mark_task_complete` (replaced by user confirmation).

---

## Architecture

```
Implementation agent (Zed/Claude in sandbox)
       │
       ▼ (MCP call)
HelixMCPBackend  ────►  spec_task_proposals table  ────►  Helix UI (task card surfaces pending proposals)
                                                                     │
                                                                     ▼ (user approves / edits / rejects)
                                                          ProposalDecisionHandler
                                                          ├─ PR proposal:    existing ensurePRs / Open PR flow
                                                          ├─ Task proposal:  existing CreateSpecTask flow
                                                          └─ Mark-complete:  set task.Status = Done
```

---

## Data Model

### New table: `spec_task_proposals`

One unified table covers all three proposal kinds (PR, task, mark-complete). Discriminated by `kind`. Payload columns are nullable; only the columns relevant to the kind are populated.

```go
// api/pkg/types/spec_task_proposal.go
type SpecTaskProposal struct {
    ID         string `json:"id" gorm:"primaryKey;size:255"`
    SpecTaskID string `json:"spec_task_id" gorm:"not null;size:255;index"`
    ProjectID  string `json:"project_id" gorm:"not null;size:255;index"`

    Kind   SpecTaskProposalKind   `json:"kind" gorm:"not null;size:50;index"`
    Status SpecTaskProposalStatus `json:"status" gorm:"not null;size:50;default:pending;index"`

    // Created-by-agent metadata
    ProposedBySession string `json:"proposed_by_session,omitempty" gorm:"size:255"` // session_id
    AgentReason       string `json:"agent_reason,omitempty" gorm:"type:text"`       // free-form why-we-want-this

    // PR proposal payload (kind = "pull_request")
    PRRepositoryID string `json:"pr_repository_id,omitempty" gorm:"size:255"`
    PRHeadBranch   string `json:"pr_head_branch,omitempty" gorm:"size:255"`
    PRBaseBranch   string `json:"pr_base_branch,omitempty" gorm:"size:255"`
    PRTitle        string `json:"pr_title,omitempty" gorm:"type:text"`
    PRBody         string `json:"pr_body,omitempty" gorm:"type:text"`

    // Task proposal payload (kind = "spec_task")
    TaskName           string                  `json:"task_name,omitempty" gorm:"size:500"`
    TaskDescription    string                  `json:"task_description,omitempty" gorm:"type:text"`
    TaskType           string                  `json:"task_type,omitempty" gorm:"size:50"`
    TaskPriority       types.SpecTaskPriority  `json:"task_priority,omitempty" gorm:"size:50"`
    TaskOriginalPrompt string                  `json:"task_original_prompt,omitempty" gorm:"type:text"`

    // Mark-complete payload (kind = "mark_complete")
    CompleteReason string `json:"complete_reason,omitempty" gorm:"type:text"`

    // Decision tracking
    DecidedBy        string         `json:"decided_by,omitempty" gorm:"size:255;index"` // user ID
    DecidedAt        *time.Time     `json:"decided_at,omitempty"`
    DecisionComment  string         `json:"decision_comment,omitempty" gorm:"type:text"`
    EditedPayload    datatypes.JSON `json:"edited_payload,omitempty" gorm:"type:jsonb"` // user's edits to the payload, if any

    // Result tracking — what actually happened on approval
    ResultPRID         string `json:"result_pr_id,omitempty"  gorm:"size:255"` // for PR kind
    ResultPRURL        string `json:"result_pr_url,omitempty" gorm:"size:1024"`
    ResultTaskID       string `json:"result_task_id,omitempty" gorm:"size:255"` // for task kind
    ResultError        string `json:"result_error,omitempty"   gorm:"type:text"`

    CreatedAt time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP;index"`
    UpdatedAt time.Time `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
}

type SpecTaskProposalKind string
const (
    ProposalKindPullRequest  SpecTaskProposalKind = "pull_request"
    ProposalKindSpecTask     SpecTaskProposalKind = "spec_task"
    ProposalKindMarkComplete SpecTaskProposalKind = "mark_complete"
)

type SpecTaskProposalStatus string
const (
    ProposalStatusPending  SpecTaskProposalStatus = "pending"
    ProposalStatusApproved SpecTaskProposalStatus = "approved"
    ProposalStatusRejected SpecTaskProposalStatus = "rejected"
    ProposalStatusFailed   SpecTaskProposalStatus = "failed" // approval succeeded but action failed
)
```

GORM AutoMigrate handles the schema. New file: `api/pkg/store/spec_task_proposal_store.go`.

### New field on `SpecTask`

```go
// api/pkg/types/simple_spec_task.go
AgentMarkedCompleteAt *time.Time `json:"agent_marked_complete_at,omitempty"` // set when agent calls mark_task_complete
ParentTaskID          string     `json:"parent_task_id,omitempty" gorm:"size:255;index"` // set when task was spawned via spec_task proposal
```

`ParentTaskID` enables lineage display ("spawned from task #1971") in the UI.

---

## MCP Tool Layer

The implementation-phase agent already connects to the Helix MCP gateway at `/api/v1/mcp/helix?app_id=...&session_id=...` (see `api/pkg/external-agent/zed_config.go:189-200`). Today this gateway exposes APIs / Knowledge / Zapier tools registered on the assistant. We add **three** built-in tools that are always exposed when the session is a spec task session.

### Where to register

`api/pkg/server/mcp_backend_helix.go` — extend `addToolsFromAssistant()` to also call a new `addSpecTaskProposalTools(ctx, mcpServer, user, sessionID)` when the request's `session_id` query param resolves to a spec task session.

The session→spec-task lookup uses `store.ListSpecTasks(ctx, &SpecTaskFilters{PlanningSessionID: sessionID})`, which is already an indexed query (see `simple_spec_task.go:265`).

If the session is **not** a spec task session, these tools are not registered (Optimus chat sessions, ad-hoc app sessions, etc., still see only their configured tools). This keeps the surface area minimal.

### Tool specs

```go
// 1. propose_pull_request
{
  Name: "propose_pull_request",
  Description: "Propose opening a pull request from the given branch. Requires user approval via the Helix UI before any push/PR happens. Use this when you want to ship a slice of work — you may call it more than once per task.",
  Parameters: {
    "repository_id":  { type: "string", description: "Project repo ID. Defaults to the project's primary repo.", required: false },
    "head_branch":    { type: "string", description: "Branch to open the PR from. Defaults to the system-generated branch for this task.", required: false },
    "base_branch":    { type: "string", description: "Target branch. Defaults to the repo's default branch.", required: false },
    "title":          { type: "string", description: "PR title. Defaults to the contents of pull_request.md or pull_request_<repo>.md.", required: false },
    "body":           { type: "string", description: "PR body markdown. Defaults to the contents of pull_request.md.", required: false },
    "reason":         { type: "string", description: "Why you're proposing this PR (shown to the user).", required: true },
  },
  Returns: "Proposal ID; tells the agent the proposal is pending user approval and will produce a follow-up message when decided."
}

// 2. propose_spec_task
{
  Name: "propose_spec_task",
  Description: "Propose creating a new spec task in this project. Requires user approval via the Helix UI before the task appears on the board. Use this for follow-ups discovered during implementation that should be tracked separately.",
  Parameters: {
    "name":            { type: "string", required: true },
    "description":     { type: "string", required: true },
    "type":            { type: "string", enum: ["feature","bug","refactor"], required: false },
    "priority":        { type: "string", enum: ["low","medium","high","critical"], required: false },
    "original_prompt": { type: "string", required: false },
    "reason":          { type: "string", description: "Why you're proposing this task (shown to the user).", required: true },
  },
  Returns: "Proposal ID; pending user approval."
}

// 3. mark_task_complete
{
  Name: "mark_task_complete",
  Description: "Declare that you believe this spec task is finished. The user must confirm via the UI to actually move the task to done. Use this instead of relying on the 'all PRs merged' heuristic when you have judged the work done.",
  Parameters: {
    "reason": { type: "string", description: "Brief summary of what was accomplished.", required: true },
  },
  Returns: "Proposal ID; pending user confirmation."
}
```

The MCP tool handlers live in a new file `api/pkg/server/mcp_backend_helix_proposals.go`. Each handler:
1. Resolves the spec task from `session_id` (already authenticated upstream by the MCP gateway).
2. Validates inputs (e.g., `repository_id` must be attached to the project).
3. Inserts a `SpecTaskProposal` row.
4. Emits a pubsub event `spec_task.proposal.created` so the frontend updates without polling (existing pubsub infrastructure in `api/pkg/pubsub/`).
5. Returns a structured success payload to the agent.

### Awaiting the user's decision (out of scope for v1)

The MCP tool returns immediately with `proposal_id` and a status of "pending". The agent does not block. When the user decides, the decision is delivered to the agent as a follow-up message in its session via the existing `agent_instruction_service` mechanism (the same channel used to send approval/revision/comment prompts today). Message text:

> Your proposal `<id>` was **approved** (PR opened: `<url>`).
> *(or)*
> Your proposal `<id>` was **rejected**: `<reason>`.

This is consistent with the existing async pattern (review comments, approval, revision requests are all delivered as messages, not synchronous returns).

---

## API Surface

New REST endpoints (added to `api/pkg/server/spec_task_proposal_handlers.go`):

| Method | Path | Purpose |
|---|---|---|
| GET    | `/api/v1/spec-tasks/{id}/proposals` | List proposals for a task (frontend uses this to render pending-proposal cards) |
| GET    | `/api/v1/projects/{id}/proposals?status=pending` | Project-level pending proposal count (for the board badge) |
| POST   | `/api/v1/proposals/{id}/decide` | Body: `{ decision: "approve"\|"reject", edited_payload?: {...}, comment?: string }` |

`POST /proposals/{id}/decide` is the single execution point. It:
1. Loads the proposal, verifies it's pending and the caller has rights on the project (reuses existing project-RBAC `authorizeUserToResource()`).
2. Applies any `edited_payload` over the original payload (e.g., user changed branch name).
3. Sets `Status = approved` / `rejected`, `DecidedBy`, `DecidedAt`, `DecisionComment`.
4. On approve, dispatches by kind:
   - **pull_request** → calls existing `ensurePRs`-style flow with the (possibly edited) `head_branch` / `base_branch` / `title` / `body`. The work happens on `git_repository_service_pull_requests.go` and reuses the auth resolution there. On success, appends a `RepoPR` to `task.RepoPullRequests` and stores `result_pr_id` / `result_pr_url`.
   - **spec_task** → calls a refactored helper extracted from `api/pkg/agent/skill/project/spec_task_create_tool.go:Execute` that creates the new `SpecTask` (sets `ParentTaskID = proposal.SpecTaskID`). Stores `result_task_id`.
   - **mark_complete** → sets `task.AgentMarkedCompleteAt = now`. The actual transition to `Done` happens here too if the user clicked "Approve" — i.e., the user's approval IS the confirmation.
5. On any execution failure, sets `Status = failed` and stores `result_error`. The proposal stays visible so the user can retry.
6. Sends the agent the follow-up message via `agent_instruction_service`.
7. Audit-logs the event via the existing `audit_log_service.go`.

---

## Orchestrator Changes

`api/pkg/services/spec_task_orchestrator.go` — `pollPullRequests` / `handlePullRequest`:

```go
// Existing logic: if allMerged && len(task.RepoPullRequests) > 0 → task.Status = Done
// New logic:
//   - If task.AgentMarkedCompleteAt != nil → DO NOT auto-transition on PR merge.
//     The user is in control via the mark-complete proposal flow.
//   - Else → existing behaviour (backwards compat for tasks not using new tools).
```

This is a ~5-line change: gate the `if allMerged` block on `task.AgentMarkedCompleteAt == nil`.

No changes needed to the PR polling itself — it still polls all tracked PRs and updates their `PRState`.

---

## Frontend Changes

The task card (`frontend/src/components/tasks/SpecTaskActionButtons.tsx` and the kanban / detail views) gets a new "Pending Proposals" panel when the task has any `pending` proposals. For each proposal:

- **PR proposal**: shows head/base branches (editable inline), title (editable), body (editable, expandable), agent reason. Buttons: **Approve & Open PR**, **Reject**.
- **Task proposal**: shows name (editable), description (editable), type/priority dropdowns, agent reason. Buttons: **Approve & Create Task**, **Reject**.
- **Mark-complete proposal**: shows agent's summary. Buttons: **Mark Done**, **Send Back** (with feedback box).

A new hook `useSpecTaskProposals(taskId)` wraps `v1SpecTasksProposalsList` (generated client) with React Query. Pubsub event `spec_task.proposal.created` invalidates the query.

A subtle badge appears on the project board next to any task with pending proposals. The board page uses `v1ProjectsProposalsPendingCount` to render an aggregated count.

**No new pages.** Everything renders inside the existing task card / detail surface.

---

## Prompt Updates

### Planning prompt (`api/pkg/services/spec_task_prompts.go`)

Add a short section before "Document Your Learnings":

```markdown
## Spawning Follow-Up Tasks (Optional)

If during planning you discover that a related but separable piece of work should be tracked
as its own spec task, propose it via the `propose_spec_task` MCP tool. The user must approve
it before it appears on the board. Do NOT use `CreateSpecTask` — that tool is reserved for
chat sessions with the project manager agent.
```

### Implementation prompt (`api/pkg/services/agent_instruction_service.go`)

Replace the current `5.` step:

> 5. **Do NOT create pull requests yourself** (no `gh pr create`, no GitHub MCP tools). Pushing to the branch is sufficient — the Helix platform creates the GitHub PR automatically when the user clicks "Open PR" in the UI.

with:

```markdown
5. **Opening pull requests**

   The user can click "Open PR" in the UI as before — that still works for the default branch.

   If you need to ship work as a series of PRs (e.g., a refactor split into reviewable slices),
   use the `propose_pull_request` MCP tool. You may call it multiple times per task. Each call
   creates a pending proposal for the user to approve in the UI. You may request a non-default
   branch name; the user can override it.

   Do NOT use `gh pr create`, the GitHub MCP tools, or any other direct route to open PRs —
   `propose_pull_request` is the only sanctioned mechanism.

6. **Declaring the task done**

   When you believe the work is complete, call `mark_task_complete` with a brief summary.
   The user will confirm in the UI. Without this call, the task moves to "done" only when
   all tracked PRs are merged (the legacy heuristic).

   If during implementation you discover follow-up work that should be its own task, use
   `propose_spec_task` to propose it.
```

---

## Reused Internals

| Need | Existing thing reused |
|---|---|
| Spec task → session lookup | `store.ListSpecTasks` filter `PlanningSessionID` (`simple_spec_task.go:265`) |
| Spec task creation logic | extract from `api/pkg/agent/skill/project/spec_task_create_tool.go:Execute` into `services.CreateSpecTaskFromProposal` (so the proposal handler and the existing Optimus tool both call it) |
| PR opening logic | existing `EnsurePRsFunc` callback + `git_repository_service_pull_requests.go` (multi-auth: GitHub App > OAuth > PAT > password). For non-default branch names, pass through to the same code paths — the head branch is just a parameter. |
| MCP gateway / auth | `HelixMCPBackend` (`mcp_backend_helix.go`) — already authenticates the user from the bearer token and resolves the app/session |
| Pubsub for UI updates | existing `pubsub` package — already used for review comments, agent activity |
| Audit log | `audit_log_service.go` — already records spec task lifecycle events |
| RBAC on decision endpoint | `authorizeUserToResource()` |
| Sending follow-up message to agent | `agent_instruction_service.go` (same channel as comment / approval / revision messages) |

---

## Design Decisions

- **One unified `spec_task_proposals` table, not three.** Three kinds share lifecycle (pending/approved/rejected/failed), audit pattern, agent-message-on-decision, and frontend rendering shape. Splitting into three tables would triple the wiring with no benefit. Discriminated by `kind`; nullable payload columns.

- **Agent gets immediate proposal ID, not a blocking call.** The MCP returns synchronously with `{proposal_id, status: "pending"}`. The user's decision arrives later as a session message. This matches the existing async pattern (the agent doesn't block waiting for a review comment either) and avoids needing to thread long-lived MCP requests through the gateway.

- **Reuse `CreateSpecTask` logic by extraction, not by calling the tool.** The Optimus skill's `Execute` method does input parsing + project lookup + task creation. We extract the project-lookup-and-create steps into a `services.CreateSpecTaskFromProposal(ctx, ProposalRequest)` so the proposal handler and the existing Optimus tool share the same code path. This avoids drift.

- **`mark_task_complete` requires user confirmation, not auto-transition.** The user told us this is for security: agents don't get unfettered ability to mark things done either. Two-step (agent proposes, user confirms) is consistent with the rest of the design.

- **Backwards compatibility: heuristic preserved for legacy tasks.** Tasks created today don't use the new tools. Their behaviour is unchanged. New tasks may opt in by the agent calling `mark_task_complete` (which sets `AgentMarkedCompleteAt`, gating the auto-transition).

- **Branch name policy.** The agent may *propose* any branch name. The user *sees* the proposed name during approval and can edit it before the push. There is no allowlist; the user is the gate. This matches the user's stated requirement: "we don't want to give unfettered access to any branch".

- **Out of scope: auto-approval.** A future `spec_task_proposals.auto_approve` project setting could let trusted teams skip the UI gate. Designed-in by separating proposal-creation from decision-execution, but no UI / behaviour flips ship in this task.

- **Out of scope: cross-project task spawning.** Proposed tasks land in the same project as the parent task. Multi-project orchestration is a separate concern (Optimus already serves it via interactive chat).

---

## Testing Approach

- **Unit:** mock `store.SpecTaskProposalStore`; verify `propose_pull_request` writes a proposal with the right defaults; verify `decide` dispatches to the right kind and applies edits.
- **Integration:** use the Postgres-backed test setup (same pattern as `spec_driven_task_service_test.go`) to round-trip a proposal through approve → PR creation → audit log entry.
- **Frontend:** Storybook story for the pending-proposal card; vitest test for the `useSpecTaskProposals` hook.
- **End-to-end (manual):** run a spec task in helix-in-helix, have the agent call `propose_pull_request` via the Helix MCP, approve in the UI, verify the PR opens on GitHub. Then have the agent call `propose_spec_task`, approve, verify a child task appears on the board with `parent_task_id` set.
