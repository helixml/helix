# Design: Fix Commit Ownership to Match Approving User

## Root Cause

The agent container is created during `StartSpecGeneration()` with `UserID: task.CreatedBy` (`spec_driven_task_service.go:574`). This `UserID` flows to `hydra_executor.go:227-254` which fetches the user's name/email and sets `GIT_USER_NAME`/`GIT_USER_EMAIL` env vars. The container is never restarted when the task transitions to implementation, so the creator's identity persists for all commits.

**Key code path:**
1. `spec_driven_task_handlers.go:394` — `approveSpecs` HTTP handler sets `task.SpecApprovedBy = user.ID`
2. `spec_driven_task_service.go:1118` — `ApproveSpecs()` processes approval, sends implementation instruction to existing agent session
3. The agent (already running in the creator's container) starts coding and committing — with the **creator's** git identity

## Solution

Update git author identity inside the running container when transitioning to implementation, using the approving user's name and email. The container doesn't need to restart.

### Approach: Update git config via the approval instruction

When `ApproveSpecs()` sends the implementation instruction to the agent, also update the git config inside the container. Two parts:

**Part 1: Validate OAuth and get approver identity at approval time**

In `approveSpecs` handler (`spec_driven_task_handlers.go`), before transitioning to `spec_approved`:
- Validate the approving user has GitHub OAuth (same as `approveImplementation` does at line 141 of `spec_task_workflow_handlers.go`)
- This ensures the approver's credentials are available for both commits and PR creation

**Part 2: Pass approver's identity through to the agent container**

Option A (Preferred): **Execute `git config` in the running container via Hydra exec.**
- Add a method to update git identity in a running container (e.g., `UpdateGitIdentity(ctx, sessionID, userName, userEmail)`)
- Call it from `ApproveSpecs()` after setting the task status to implementation
- This directly runs `git config --global user.name "X"` and `git config --global user.email "Y"` inside the container

Option B (Simpler fallback): **Include git config commands in the approval instruction prompt.**
- The approval instruction sent to the agent already contains setup commands
- Prepend `git config --global user.name "Approver Name"` and `git config --global user.email "approver@example.com"` to the instruction
- Relies on the AI agent executing these commands, which is less reliable

**Decision: Option A** — direct container exec is deterministic and doesn't depend on AI behavior.

**Part 3: Update push credentials to use approver's OAuth**

The push path (`PushBranchToRemote`) already accepts an optional `userID` parameter. Currently the agent pushes via its API key (tied to creator). We need to:
- Store the approver's user ID on the task (already done: `task.SpecApprovedBy`)
- When the agent pushes, use `SpecApprovedBy` as the acting user for credential lookup
- The `getCredentialsForRepo()` function already prioritizes user OAuth when `actingUserID` is provided

**Part 4: Update session API key to approver's identity**

The ephemeral session API key minted in `OnBeforeCreate` uses `task.CreatedBy`. When the task transitions to implementation:
- Mint a new session API key for the approver (`task.SpecApprovedBy`)
- Update the agent's `USER_API_TOKEN` env var or create a new key that the push path can use
- Alternative: since push goes through the API server (not directly from the container), pass the approver's userID through the push request context

## Codebase Patterns Discovered

- **Credential hierarchy** (`git_repository_service.go:2634-2700`): User OAuth > Repo OAuth > PAT > fallback. Passing the right `actingUserID` is sufficient to use the approver's credentials.
- **Container env vars are immutable after creation** — Hydra containers can't have env vars updated. Must use exec or API-level overrides.
- **`SpecApprovedBy` already exists** on the `SpecTask` type (`simple_spec_task.go:162`) and is set by the approval handler. This field is the source of truth for who should own implementation commits.
- **OAuth validation pattern** exists in `spec_task_workflow_handlers.go:141-152` (`ValidateUserGitHubOAuth`) — reuse this for spec approval.
- **The agent container uses `helix-workspace-setup.sh`** to apply `GIT_USER_NAME`/`GIT_USER_EMAIL` via `git config --global`. Overwriting these values at runtime via exec is safe and immediate.

## Scope

- **In scope**: Fixing commit authorship and push credentials when transitioning from spec to implementation
- **Out of scope**: Changing PR creation (already correct), changing planning phase authorship (commits during planning should remain as creator)
