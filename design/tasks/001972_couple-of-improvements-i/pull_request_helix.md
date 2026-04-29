# Agent-driven multi-PR & multi-spec-task proposals

## Summary

Adds three MCP tools the spec-task agent can call to propose actions that need user approval in the Helix UI:

- `propose_pull_request(repository_id?, head_branch?, base_branch?, title?, body?, reason)` — open one or more PRs per task, possibly from non-default branches. User sees and can override the branch/title/body before approving.
- `propose_spec_task(name, description, type?, priority?, reason)` — spawn follow-up tasks; new task lands in the project backlog with `parent_task_id` linking it back.
- `mark_task_complete(reason)` — declare the task finished; user clicks Mark Done (or Send Back with feedback) in the UI.

Replaces the brittle "all PRs merged → done" heuristic with explicit agent-declared completion.

## Changes

**New backend:**
- `types.SpecTaskProposal` — unified table for the three proposal kinds (PR / spec_task / mark_complete). Discriminated by `kind`; nullable payload columns.
- `services.CreateSpecTaskFromProposal` — extracted from the Optimus tool's `Execute`; now shared between the chat tool and the proposal-decision handler.
- Three MCP tools registered on `HelixMCPBackend` when the session is the agent session of a spec task. Cache key extended to `(user, app, session)` so registration is correctly scoped.
- REST API: `GET /api/v1/spec-tasks/{taskId}/proposals`, `GET /api/v1/projects/{projectId}/proposals`, `POST /api/v1/proposals/{proposalId}/decide`.
- Six decision-prompt templates in `services/spec_task_proposal_prompts.go` (PR approved/rejected, spec_task approved/rejected, mark_complete confirmed/sent_back), delivered to the agent's session via the existing `sendMessageToSpecTaskAgent` channel — same mechanism used today for review comments and revision requests.

**Auto-transitions to `done` removed (5 sites):**
- 4 sites in `services/spec_task_orchestrator.go` (allMerged, branch-merged-no-PR, externally-opened-PR-merged, branch-merged fallback)
- 1 site in `services/git_http_server.go:handleMainBranchPush`

These now record metadata (`MergedToMain`, `MergedAt`, `RepoPR.PRState`) only — they no longer touch `task.Status`. The two paths to `done` are now: (a) approved `mark_complete` proposal, (b) the existing user-initiated "Approve Implementation" handler for internal repos.

**Merge-instruction code deleted entirely:**
- `mergePromptTemplate` + `MergePromptData` + `BuildMergeInstructionPrompt` + `SendMergeInstruction` — zero callers anywhere in the codebase, fully removed.

**Prompt edits:**
- `planningPromptTemplate` gains "Spawning Follow-Up Tasks" + "Not Every Task Needs Code" sections.
- `approvalPromptTemplate`'s old single step 5 ("Do NOT create pull requests yourself") replaced with three steps: opening PRs (zero/one/many via `propose_pull_request`), capturing knowledge (spec branch preferred, no PR needed), declaring done via `mark_task_complete`.
- `agent_implementation_approved_push.tmpl`: drop "Pull Request opened automatically" line; add "pushing alone does NOT complete the task".
- `agent_rebase_required.tmpl`: clarify rebasing keeps PRs current but does not affect task status.

**Cleanup (separate concern, shipped together to avoid touching same call sites twice):**
- `PlanningSessionID` → `AgentSessionID` rename across 220+ call sites (Go + frontend + swagger). Reflects reality: there is one agent per spec task for the whole lifecycle, not separate planning/implementation agents.
- Idempotent GORM `Migrator().RenameColumn` in `postgres.go` handles three states (old-only, both, new-only) so it's safe across deployment orderings.
- Dead constants `AgentTypeSpecGeneration` and `AgentTypeImplementation` removed (verified zero non-definition references).

**Frontend:**
- `useSpecTaskProposals` React Query hook (5s polling, decideMutation).
- `PendingProposalsPanel` component with three editable cards (PR / SpecTask / MarkComplete), mounted in `SpecTaskDetailContent`.

## Audit of `task.Status = TaskStatusDone` writes after this PR

```
api/pkg/server/spec_task_workflow_handlers.go:349  → user-initiated "Approve Implementation" for internal repos
api/pkg/server/spec_task_proposal_handlers.go:351  → approved mark_complete proposal
```

These are the only two places remaining. Documented as the two valid paths in code comments.

## Testing

End-to-end smoke test against the inner Helix stack:
- ✅ DB schema migration (rename + new table + new column)
- ✅ Both-columns-exist migration recovery (GORM AutoMigrate created the new column on a previous startup; rename now drops the empty new column first, then renames)
- ✅ List proposals endpoint
- ✅ Reject decision (status → rejected, decided_by/at/comment captured)
- ✅ Approve mark_complete (task → done, completed_at set)
- ✅ All endpoints 401 without auth (RBAC wired)

## Notes

- The agent receives decision results as a plain text user-turn message rendered from a Go `text/template` — same async pattern as review comments / revision / approval today. No new transport, no long-poll MCP requests.
- v1 has no auto-approval policy — every proposal requires explicit user approval. Designed-in by separating proposal-creation from decision-execution; future `auto_approve` setting would be a small addition.
