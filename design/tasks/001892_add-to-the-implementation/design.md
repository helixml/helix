# Design: Merge Default Branch Before PR

## Overview

Two changes to the agent instruction system:

1. **Implementation prompt** — add a step telling agents to merge the default branch into their feature branch before finishing work
2. **Approved push template** — add a merge-main step before pushing, and ensure the default branch name is available

## Key Files

| File | Purpose |
|------|---------|
| `api/pkg/services/agent_instruction_service.go` | Contains `approvalPromptTemplate` (the main implementation prompt) and `ApprovalPromptData` struct |
| `api/pkg/prompts/templates/agent_implementation_approved_push.tmpl` | Template sent when implementation is approved — tells agent to commit & push |
| `api/pkg/prompts/helix_code_prompts.go` | `ImplementationApprovedPushInstruction()` function that renders the push template |
| `api/pkg/server/spec_task_workflow_handlers.go` | Calls `ImplementationApprovedPushInstruction()` — needs to pass default branch |

## Changes

### 1. Implementation prompt (`agent_instruction_service.go`)

In the `approvalPromptTemplate`, add a step in the "Steps" section (between step 3 and step 4) telling the agent to merge the default branch before pushing code:

```
4. Before pushing code, merge the latest default branch into your feature branch in every repo:
   `cd /home/retro/work/<repo> && git fetch origin <baseBranch> && git merge origin/<baseBranch>`
   Resolve any conflicts, commit, then push.
```

The `BaseBranch` field is already available in `ApprovalPromptData` — it just isn't referenced in the Steps section yet.

### 2. Approved push template (`agent_implementation_approved_push.tmpl`)

Add a `BaseBranch` field to the template data and insert a merge step before each push:

```bash
# Merge latest default branch, then push
cd /home/retro/work/{{ .PrimaryRepoName }} && git fetch origin {{ .BaseBranch }} && git merge origin/{{ .BaseBranch }}
git add -A && git diff --cached --quiet || git commit -m "Complete implementation"
git push origin {{ .BranchName }}
```

Same pattern for each non-primary repo.

### 3. Function signature update (`helix_code_prompts.go`)

Add `baseBranch string` parameter to `ImplementationApprovedPushInstruction()` and pass it to the template data struct.

### 4. Caller update (`spec_task_workflow_handlers.go`)

Pass `repo.DefaultBranch` (or the project's default branch) when calling `ImplementationApprovedPushInstruction()`.

## Design Decisions

- **Merge, not rebase**: Consistent with the existing `agent_rebase_required.tmpl` which already uses `git merge`, not `git rebase`. Merge commits are simpler and less risky for automated agents.
- **Keep rebase_required template**: The existing reactive "rebase required" flow remains as a fallback. The new proactive merge is a best-effort step — if it fails (e.g., conflicts), the rebase_required flow will still catch it.
- **BaseBranch field**: Already available in `ApprovalPromptData` and used in the implementation prompt footer. Just needs to be threaded through to the push template too.
