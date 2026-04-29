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
ParentTaskID string `json:"parent_task_id,omitempty" gorm:"size:255;index"` // set when task was spawned via spec_task proposal
```

`ParentTaskID` enables lineage display ("spawned from task #1971") in the UI.

(There is intentionally **no** `AgentMarkedCompleteAt` field. The "agent thinks the task is complete" UI state is derived from `SELECT FROM spec_task_proposals WHERE spec_task_id=? AND kind='mark_complete' AND status='pending'` — see the orchestrator section.)

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

### Awaiting the user's decision — async, via prompt templates

**Yes, this is exactly the same pattern as every other user-action-back-to-agent handoff.** The MCP tool returns immediately with `{proposal_id, status: "pending"}`. The agent does not block on the MCP call. When the user clicks Approve / Reject / Mark Done / Send Back in the UI, the decision flows back to the agent as a **plain text user-turn message** rendered from a Go `text/template`, sent via the existing `SendInstructionToAgent` path — identical to how `revisionPromptTemplate`, `mergePromptTemplate`, `commentPromptTemplate`, and `implementationReviewPromptTemplate` already work today in `api/pkg/services/agent_instruction_service.go`.

Concretely, four new templates are added to `agent_instruction_service.go` alongside the existing ones, and a single new entry point `SendProposalDecisionInstruction(ctx, task, proposal)` selects the right template by `proposal.Kind` and `proposal.Status`:

```go
// PR proposal — approved
var prProposalApprovedPromptTemplate = template.Must(template.New("prProposalApproved").Parse(`# PR Proposal Approved

Speak English.

Your proposal to open a pull request was approved by {{.DecidedByEmail}}.

**Branch:** {{.HeadBranch}} → {{.BaseBranch}}
**PR:** {{.PRURL}} (#{{.PRNumber}})
{{if .UserComment}}
**Reviewer note:** {{.UserComment}}
{{end}}{{if .UserEdits}}
The reviewer adjusted your proposal before approving:
{{.UserEdits}}
{{end}}
You may continue working. If you want to open more PRs for this task, use ` + "`propose_pull_request`" + ` again.
`))

// PR proposal — rejected
var prProposalRejectedPromptTemplate = template.Must(template.New("prProposalRejected").Parse(`# PR Proposal Rejected

Speak English.

Your proposal to open a pull request was rejected by {{.DecidedByEmail}}.

**Branch you proposed:** {{.HeadBranch}} → {{.BaseBranch}}
**Reason:** {{.UserComment}}

Do not retry the same proposal. Address the feedback (in the design docs and in your code if relevant), then either propose a corrected PR or continue with the existing approach.
`))

// Spec task proposal — approved
var specTaskProposalApprovedPromptTemplate = template.Must(template.New("specTaskProposalApproved").Parse(`# Spec Task Proposal Approved

Speak English.

Your proposal to create a follow-up task was approved by {{.DecidedByEmail}}.

**New task:** {{.ResultTaskID}} — {{.TaskName}}
{{if .UserEdits}}
The reviewer adjusted your proposal before approving:
{{.UserEdits}}
{{end}}
The task is now in the project backlog. Continue with your current task.
`))

// Spec task proposal — rejected
var specTaskProposalRejectedPromptTemplate = template.Must(template.New("specTaskProposalRejected").Parse(`# Spec Task Proposal Rejected

Speak English.

Your proposal to create a new task ({{.TaskName}}) was rejected by {{.DecidedByEmail}}.

**Reason:** {{.UserComment}}

Continue with your current task. Do not re-propose the same follow-up.
`))

// Mark-complete — confirmed (task moved to done)
var markCompleteConfirmedPromptTemplate = template.Must(template.New("markCompleteConfirmed").Parse(`# Task Marked Done

Speak English.

{{.DecidedByEmail}} confirmed your mark-complete proposal. The task has been moved to **done**.

No further action required. Do not push more changes.
`))

// Mark-complete — sent back
var markCompleteSentBackPromptTemplate = template.Must(template.New("markCompleteSentBack").Parse(`# Mark-Complete Sent Back

Speak English.

{{.DecidedByEmail}} reviewed your mark-complete proposal and sent it back with feedback:

{{.UserComment}}

Address the feedback. When you are ready, you may call ` + "`mark_task_complete`" + ` again.
`))
```

The templates live next to the existing ones (`revisionPromptTemplate`, `mergePromptTemplate`, etc.) so the pattern is unmistakable to anyone reading the file. `ProposalDecisionPromptData` is the analogue of `ApprovalPromptData` / `RevisionPromptData`.

**Delivery path** (unchanged from existing flows): the rendered template string is passed to whatever `agent_instruction_service` already uses to insert a user-turn message into the active spec task session — the same call site that delivers review comments today. The agent sees it on its next turn just as if the user typed it into the chat.

This means: no new transport, no long-polling MCP request, no special "wait for decision" behaviour anywhere. The agent calls the MCP tool, gets an immediate "queued" response, keeps working (or pauses if it has nothing to do), and the decision arrives later as a normal session turn. Existing pattern, end-to-end.

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

## Orchestrator Changes — Delete the "all PRs merged → done" heuristic outright

The current orchestrator transitions tasks to `done` from **five** different code paths, all triggered by PR / branch merge detection (four in the orchestrator itself, one in the git HTTP server):

| File:Line | Trigger | Action |
|---|---|---|
| `services/spec_task_orchestrator.go:~781` | `allMerged && len(task.RepoPullRequests) > 0` | `task.Status = Done` |
| `services/spec_task_orchestrator.go:~851` | branch detected merged to main, no PRs tracked | `task.Status = Done` |
| `services/spec_task_orchestrator.go:~1080` | externally-opened PR found and already merged | `task.Status = Done` |
| `services/spec_task_orchestrator.go:~1123` | branch merged to main (no PR found), fallback check | `task.Status = Done` |
| **`services/git_http_server.go:~1008-1051`** (`handleMainBranchPush`) | direct push to main detected; transitions any `implementation_review` task whose branch was merged into main | `task.Status = Done` |

**All five are deleted.** No code path remaining anywhere transitions a task to `done` on PR or branch state. The PR polling loop (`prPollLoop` / `pollPullRequests`) **continues to run** and continues to update `RepoPR.PRState` for every tracked PR — that's still valuable for UI display and for the `result_pr_*` fields on proposals — but it never modifies `task.Status` or `task.MergedAt` / `task.CompletedAt`. Likewise `handleMainBranchPush` either keeps its other side effects (audit-log push events, etc.) and just stops touching `task.Status`, or is reduced to a no-op for spec tasks.

### Why delete, not gate

The original design here gated the heuristic on `task.AgentMarkedCompleteAt == nil` to preserve "legacy" behaviour. Per follow-up requirements: **there is no legacy behaviour worth preserving**. The heuristic is the bug. It cuts agents off mid-work whenever an unrelated PR happens to merge while the agent is doing follow-up work, knowledge capture, or preparing the next slice. Gating it would leave the bug live for any task that doesn't happen to call `mark_task_complete`. Deleting it makes the model uniform.

### The two — and only two — paths to `done`

1. **Agent proposes + user confirms** — agent calls `mark_task_complete`, user clicks **Mark Done** in the UI, the proposal-decision handler sets `task.Status = Done`, `task.CompletedAt = now`. (See "API Surface" section.)
2. **User marks done directly** — the existing manual UI affordance to set status. Unchanged.

Nothing else writes `task.Status = TaskStatusDone` from the orchestrator going forward. (Other writers — e.g. `handleSpecApproved` setting other statuses — are untouched.)

### `AgentMarkedCompleteAt` is no longer needed

Earlier drafts of this design added `AgentMarkedCompleteAt *time.Time` to `SpecTask` as a "gate" against auto-completion. With the heuristic deleted there is nothing to gate. **Removed from the schema.**

The "agent thinks the task is complete, awaiting your confirmation" UI state is derived purely from the existence of a `pending` `mark_complete` proposal in `spec_task_proposals` — no denormalised flag on `SpecTask`.

### `task.MergedToMain` / `task.MergedAt`

These remain useful as informational fields (displayed in UI, used for filtering/sorting). They get set from the PR polling loop when PR state transitions to `merged`, but **only as informational metadata**, not as a trigger for any status transition. They no longer imply task completion.

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

## Full Catalog of Spec-Task Agent Prompts (and Disposition)

A code-wide audit found **8 Go `text/template` prompts + 2 `.tmpl` files + 8 builder/sender functions** that send instructions to a spec task agent. Some of the prompts I didn't initially catalog are part of the merge/push lifecycle that is being significantly rewritten by this task. Below is the complete list with disposition.

### Templates

| File:Line | Variable / Filename | Triggered when | Disposition |
|---|---|---|---|
| `services/spec_task_prompts.go:28` | `planningPromptTemplate` | Task starts spec generation | **EDIT** — add "Spawning Follow-Up Tasks" + "Not Every Task Needs Code" sections (covered above) |
| `services/agent_instruction_service.go:126` | `approvalPromptTemplate` | Design approved → implementation begins | **EDIT** — replace step 5 with the three new steps (PRs / knowledge / mark complete) covered above |
| `services/agent_instruction_service.go:370` | `commentPromptTemplate` | Reviewer leaves an inline comment on a design doc | **NO CHANGE** — already minimal, just instructs agent to update design docs and push |
| `services/agent_instruction_service.go:390` | `implementationReviewPromptTemplate` | Agent has pushed code; task moves to `implementation_review` | **NO CHANGE** — generic "code pushed, user will test" message; works fine for tasks that opened a PR. For zero-PR tasks this prompt simply never fires (no code push happens). |
| `services/agent_instruction_service.go:402` | `revisionPromptTemplate` | Reviewer requests changes during spec review | **NO CHANGE** — operates on design docs in spec branch, not on PR/completion lifecycle |
| `services/agent_instruction_service.go:420` | `mergePromptTemplate` | After implementation approval — tells the agent to `git merge && git push` to base | **DELETE** — see "Merge instruction: delete it" below |
| `prompts/helix_code_prompts.go:25` | `tmpl` (loaded from `templates/agent_implementation_approved_push.tmpl`) | After implementation approval — tells the agent to commit/push and notes "the Pull Request has been opened automatically" | **EDIT** — see "ImplementationApprovedPush: edit, don't delete" below |
| `prompts/helix_code_prompts.go:51` | `tmpl` (loaded from `templates/agent_rebase_required.tmpl`) | Detected branch divergence; agent must rebase before merge can complete | **EDIT** — strip references to "merge" / "PR will close on merge" if any; the agent's job becomes "rebase + push to keep the open PR(s) up to date", and `mark_task_complete` remains the only path to `done` |

### Builder / sender functions

| File:Line | Function | Calls template | Disposition |
|---|---|---|---|
| `services/spec_task_prompts.go:152` | `BuildPlanningPrompt` | `planningPromptTemplate` | **EDIT** wording (template change covers this) |
| `services/agent_instruction_service.go:440` | `BuildApprovalInstructionPrompt` | `approvalPromptTemplate` | **EDIT** wording (template change covers this) |
| `services/agent_instruction_service.go:522` | `BuildCommentPrompt` | `commentPromptTemplate` | NO CHANGE |
| `services/agent_instruction_service.go:554` | `BuildImplementationReviewPrompt` | `implementationReviewPromptTemplate` | NO CHANGE |
| `services/agent_instruction_service.go:571` | `BuildRevisionInstructionPrompt` | `revisionPromptTemplate` | NO CHANGE |
| `services/agent_instruction_service.go:588` | `BuildMergeInstructionPrompt` | `mergePromptTemplate` | **DELETE** (see below) |
| `services/agent_instruction_service.go:786` | `SendMergeInstruction` | calls `BuildMergeInstructionPrompt` and dispatches to agent | **DELETE** (see below) |
| `prompts/helix_code_prompts.go:11` | `ImplementationApprovedPushInstruction` | wraps `agent_implementation_approved_push.tmpl` | **EDIT** wording |
| `prompts/helix_code_prompts.go:36` | `RebaseRequiredInstruction` | wraps `agent_rebase_required.tmpl` | **EDIT** wording |

### `.tmpl` files

| File | Disposition |
|---|---|
| `prompts/templates/agent_implementation_approved_push.tmpl` | **EDIT** — currently says *"the Pull Request has been opened automatically"*. Under the new model: PRs are opened via `propose_pull_request` (agent-driven) or via the legacy "Open PR" button (user-driven). Reword to: push your remaining changes; if you used `propose_pull_request`, the PR is now open and tracked; **call `mark_task_complete` when the work is finished — pushing alone does not move the task to done**. |
| `prompts/templates/agent_rebase_required.tmpl` | **EDIT** — strip any "task will close on merge" wording; clarify that rebase keeps existing PRs current but does not affect task status. `mark_task_complete` remains the only path to `done`. |

### Plus: callers that fire prompts at agents

These are not templates themselves but they're the wiring that delivers prompts to the agent. Audit found these ones; all keep working (no signature changes), but several call the templates we're editing/deleting:

| File:Line | What it does |
|---|---|
| `services/spec_driven_task_service.go:359` | Sends planning prompt on task start — uses edited `BuildPlanningPrompt` |
| `services/spec_driven_task_service.go:1340` | Sends approval/implementation prompt on design approval — uses edited `BuildApprovalInstructionPrompt` |
| `services/spec_driven_task_service.go:1409` | Sends revision prompt — `BuildRevisionInstructionPrompt`, no change |
| `services/agent_instruction_service.go:608` | `SendApprovalInstruction` — uses edited `BuildApprovalInstructionPrompt` |
| `services/agent_instruction_service.go:746` | `SendImplementationReviewRequest` — no change |
| `services/agent_instruction_service.go:766` | `SendRevisionInstruction` — no change |
| `services/agent_instruction_service.go:786` | `SendMergeInstruction` — **DELETE** entirely (see below); audit all callers and remove |
| `server/spec_task_design_review_handlers.go:377` | Design rejection handler — `BuildRevisionInstructionPrompt`, no change |
| `server/spec_task_design_review_handlers.go:1015` | Comment reply — `BuildCommentPrompt`, no change |
| `server/spec_task_design_review_handlers.go:1570` | Approval endpoint — `BuildApprovalInstructionPrompt`, edited |

### Merge instruction: delete it

`SendMergeInstruction` / `BuildMergeInstructionPrompt` / `mergePromptTemplate` exists to tell the agent *"your implementation was approved — now run `git merge && git push origin BASE_BRANCH`"*. Under the new model this is wrong on two counts:

1. **For external repos (GitHub / GitLab / ADO / Gitea):** merging happens on the external platform, not via the agent's local git. The user clicks Merge on GitHub (or via Helix's "Open PR" / proposal-driven PR flow). The agent has no business pushing directly to `main`.
2. **For internal-only repos:** merging directly is theoretically OK, but it would *also* trigger the now-deleted `handleMainBranchPush` auto-transition. With that gone, the merge instruction has no follow-on effect — and `mark_task_complete` is the explicit signal anyway.

**Decision: delete the entire merge-instruction code path.** Remove `mergePromptTemplate`, `BuildMergeInstructionPrompt`, `SendMergeInstruction`, the `.tmpl` (none here — it's inline), and every caller. Audit all call sites of `SendMergeInstruction` (search the repo) and replace the contract: implementation approval no longer sends *any* merge prompt; the agent learns about approval through the `propose_pull_request`-decision flow (PR was approved/opened) or simply via the existing comment/notification channels. Completion still requires `mark_task_complete`.

### `ImplementationApprovedPush`: edit, don't delete

This one is genuinely useful — it's a "you've been approved, push your remaining uncommitted changes" nudge. We keep the push behaviour but rewrite the message to:

- Drop the "the Pull Request has been opened automatically" line (no longer accurate in the proposal-driven world).
- Add a clear instruction that pushing alone does **not** complete the task — the agent must call `mark_task_complete` when the work is judged done.
- Mention that `propose_pull_request` is the route to open additional PRs.

### Audit result for completion sites

Code-wide search confirmed FIVE sites that auto-transition a task to `TaskStatusDone` based on PR/branch state (one more than my earlier draft caught):

| File:Line | Trigger | Status |
|---|---|---|
| `services/spec_task_orchestrator.go:~781` | `allMerged && len(RepoPullRequests) > 0` | **DELETE** |
| `services/spec_task_orchestrator.go:~851` | branch detected merged to main, no PRs tracked | **DELETE** |
| `services/spec_task_orchestrator.go:~1080` | externally-opened PR found and already merged | **DELETE** |
| `services/spec_task_orchestrator.go:~1123` | branch merged to main fallback | **DELETE** |
| **`services/git_http_server.go:~1008-1051`** (`handleMainBranchPush`) | **direct push to main detected; transitions `implementation_review` task → done** | **DELETE** (was missing from earlier draft of this design) |

After all five deletions, `task.Status = TaskStatusDone` is written from exactly two places in the entire codebase: (a) the new proposal-decision handler when a `mark_complete` proposal is approved, and (b) any pre-existing manual user "set status to done" handler. The PR audit task in tasks.md requires this to be documented in the PR description as proof.

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

## Not Every Task Needs Code

Some tasks legitimately produce zero pull requests — research, analysis, documentation
updates that live in the spec branch, knowledge consolidation. That's fine. The
implementation phase will still happen (so you have an environment to investigate in),
but you may finish without ever opening a PR.
```

### Implementation prompt (`api/pkg/services/agent_instruction_service.go`)

Replace the current `5.` step:

> 5. **Do NOT create pull requests yourself** (no `gh pr create`, no GitHub MCP tools). Pushing to the branch is sufficient — the Helix platform creates the GitHub PR automatically when the user clicks "Open PR" in the UI.

with:

```markdown
5. **Opening pull requests (zero, one, or many)**

   If your task produces code changes, open one or more PRs via the `propose_pull_request`
   MCP tool. Each call creates a pending proposal for the user to approve in the UI. You
   may call it multiple times per task to ship work as a series of reviewable slices, and
   you may request a non-default branch name; the user can override it during approval.

   The simple "click Open PR in the UI" path still works for single-PR tasks — you don't
   have to use `propose_pull_request` if there's only one PR and it goes on the default
   branch.

   **Opening zero PRs is a valid outcome.** Some tasks (research, analysis, knowledge work,
   doc-only updates that live in the spec branch) finish without any code changes. That's
   fine. Just call `mark_task_complete` when you're done — see step 7.

   Do NOT use `gh pr create`, the GitHub MCP tools, or any other direct route to open PRs —
   `propose_pull_request` is the only sanctioned mechanism.

6. **Capture knowledge as you go**

   You have two channels for writing down what you learned. Use both as appropriate:

   - **Spec branch (`helix-specs`) — no PR needed, push freely.** This branch is forward-only
     and you push to it constantly throughout the task anyway. At minimum, update
     `design/tasks/{task_dir}/design.md` with: gotchas you hit, design decisions you made,
     why you picked approach A over B, things future agents on similar tasks should know.
     You can also add new files (`learnings.md`, `architecture-notes.md`) in the same task
     directory. None of this needs a PR — it's already pushed and visible.

   - **Main repo markdown files — use `propose_pull_request` like any code change.** For
     content that should live next to the code (`README.md`, `docs/`, `ARCHITECTURE.md`,
     etc.), include the file in a regular PR proposal. Doc-only PRs are valid.

   When in doubt, prefer the spec branch — it's friction-free and the knowledge is
   guaranteed to be captured.

7. **Declaring the task done — REQUIRED**

   `mark_task_complete` is the **only** way the task moves to `done`. There is no
   automatic completion based on PRs merging (the old "all PRs merged → done" heuristic
   has been removed because it cut agents off mid-work). You must call `mark_task_complete`
   explicitly when the work is finished, regardless of how many PRs you opened or what
   state they're in.

   - Zero PRs and you've captured what you needed → call `mark_task_complete`.
   - One PR open and waiting for review → call `mark_task_complete` after pushing.
   - All PRs merged → call `mark_task_complete`.

   The user clicks Mark Done (or Send Back with feedback) in the UI to confirm. Without
   your explicit call the task stays in its current state forever.

   If during implementation you discover follow-up work that should be its own task, use
   `propose_spec_task` to propose it before calling `mark_task_complete`.
```

---

## Cleanup: One Agent, Not Two — Rename `PlanningSessionID` → `AgentSessionID`

This task is a good moment to clear out the leftover two-agent naming. Today the codebase **already** runs a single agent instance for both planning and implementation phases — the only difference between phases is which prompt template (`planningPromptTemplate` vs `approvalPromptTemplate`) is sent to it. But the data model still names everything as if there were a separate planning agent.

**Evidence the cleanup is overdue:**

- `api/pkg/types/simple_spec_task.go:127` literally comments: *"Session tracking (single Helix session for entire workflow - planning + implementation). The same external agent/session is reused throughout the entire SpecTask lifecycle"* — but the field below it is still called `PlanningSessionID`.
- `AgentTypeSpecGeneration = "spec_generation"` and `AgentTypeImplementation = "implementation"` constants exist (`simple_spec_task.go:318-319`) with **zero non-definition usages** in the codebase. Pure dead code.
- 220 occurrences of `PlanningSessionID` / `planning_session_id` / `planningSessionId` across non-test code (Go + frontend + swagger).
- The store interface has a method named `GetPendingCommentByPlanningSessionID` even though the comment it's looking up is just a comment on a session — nothing planning-specific about it.
- The MCP-tool registration logic in this very design has to filter by `PlanningSessionID` to find the spec task — which makes "planning" sound like a phase predicate when it isn't.

### Renames

| From | To | Notes |
|---|---|---|
| `SpecTask.PlanningSessionID` (Go field) | `SpecTask.AgentSessionID` | The single agent session for the whole task lifecycle |
| `planning_session_id` (DB column, JSON field) | `agent_session_id` | DB rename via explicit GORM migration (AutoMigrate doesn't rename columns) |
| `SpecTaskFilters.PlanningSessionID` | `SpecTaskFilters.AgentSessionID` | Filter struct rename |
| `Store.GetPendingCommentByPlanningSessionID` | `Store.GetPendingCommentByAgentSessionID` | Mock + memorystore + postgres impl + all 6+ callers |
| `sessionCommentTimeout` map key comment "planning_session_id -> ..." (`server.go:112`) | "agent_session_id -> ..." | Comment-only |
| Frontend `task.planning_session_id` (TS) | `task.agent_session_id` | Regenerated by `./stack update_openapi` after the swagger field rename |

**Why `AgentSessionID` and not `SessionID`?** `SessionID` is too generic — the codebase has many session concepts (Helix sessions, Zed sessions, work sessions). `WorkSessionID` is already taken (by `SpecTaskWorkSession`, a separate multi-session feature). `AgentSessionID` makes it unambiguous: this is the single Helix session backing the agent that owns this spec task.

`ExternalAgentID` (the desktop-container ID — a sibling field) keeps its name; it's already accurate.

### Dead-code removal

- Delete `AgentTypeSpecGeneration` and `AgentTypeImplementation` constants from `simple_spec_task.go`. Verified zero non-definition usages.
- Delete the comment block above `PlanningSessionID` that exists only to apologise for the misleading name; the new name is self-documenting.

### DB migration

GORM's `AutoMigrate` does not rename columns. We need an explicit one-shot migration. Options:

1. **Postgres `ALTER TABLE ... RENAME COLUMN`** — safe, atomic, preserves data. Add as a numbered SQL migration alongside any existing migrations the repo uses, or as a Go-coded migration step in `store_postgres.go` gated on schema version.
2. Drop-and-recreate (destructive) — **NOT acceptable**, would lose data.

We use option 1. Migration script:

```sql
ALTER TABLE spec_tasks RENAME COLUMN planning_session_id TO agent_session_id;
-- The existing index on planning_session_id is renamed automatically by Postgres.
```

If the repo doesn't already have a numbered-migration system (need to verify), the migration can be issued at startup before `AutoMigrate` runs, gated by a check `IF EXISTS (... column_name = 'planning_session_id' ...)` so it's idempotent and safe to ship.

### Backwards compatibility for the JSON API

The `planning_session_id` JSON field is part of the public REST/swagger API. To minimise surprise:

- **Same release:** swagger emits `agent_session_id`. The frontend is regenerated and updated in lockstep (single PR / single deployment).
- **No JSON-tag aliasing.** No struct with both `json:"agent_session_id" alt:"planning_session_id"` shenanigans — that would re-introduce exactly the kind of "fallback path" the project's CLAUDE.md forbids ("**NO FALLBACKS** — one approach, fix properly, no dead code paths").
- **External integrations:** if any exist, they break on this release. Acceptable, given the field is an internal-implementation detail and the user has explicitly asked for the cleanup.

### Why include this in this task

The new MCP-tool registration logic explicitly looks up the spec task from the session ID. Adding three new tools that filter by `PlanningSessionID` (when the session is no longer "planning-specific") would be doubling down on the wrong name at exactly the wrong moment. Cleaning up first means the new MCP code reads naturally — `findSpecTaskByAgentSession(sessionID)` instead of `findSpecTaskByPlanningSession(sessionID)`.

### Scope of this cleanup

**In scope:**
- The renames in the table above
- DB column rename via migration
- Removal of the two dead `AgentType*` constants
- Frontend regeneration via `./stack update_openapi` and search-and-replace of `planning_session_id` → `agent_session_id` in TS sources

**Out of scope (explicitly):**
- Renaming `BuildPlanningPrompt` or `planningPromptTemplate` — those are accurate names for the *prompts* (one prompt is for the planning phase, one is for implementation; the agent receiving them is single, but the prompts are not).
- Renaming the workflow status constants `TaskStatusSpecGeneration` / `TaskStatusImplementation` — those describe the *task's lifecycle phase*, which remains a real distinction.
- Renaming `AgentTypeHelixAgent` (used) or any other `AgentType*` constants that are actually referenced.
- Restructuring `SpecTaskWorkSession` (a separate, real multi-session concept).

The line is: **the agent is one thing**, so its session ID and any constants that pretended otherwise get renamed/deleted. **The phases are real** (planning produces specs; implementation produces code), so phase-named prompts and statuses stay.

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
