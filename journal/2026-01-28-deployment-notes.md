# Deployment Notes - 2026-01-28

## Issues to Address

### 1. Version Banner Shows False Upgrade Available

**Problem**: After upgrading from `2.5.60-rc5` to `2.5.60`, the UI shows:
> "A new version of Helix (2.5.60) is available! You are currently running version 2.5.60-rc5."

**Expected**: No banner should appear since `2.5.60` is the stable release of `2.5.60-rc5`.

**Root Cause**: Version comparison logic doesn't properly handle RC → stable upgrade detection. Likely comparing string versions without understanding semantic versioning with pre-release tags.

**Location to investigate**: Search for version comparison logic in the control plane, likely in the API or frontend code.

---

### 2. sandbox.sh Has Version in Two Places

**Problem**: In `/opt/HelixML/sandbox.sh`, the version `2.5.60` appears in two places:
1. `SANDBOX_TAG="2.5.60"` - variable at top
2. `registry.helixml.tech/helix/helix-sandbox:2.5.60` - hardcoded in docker run command

**Expected**: Version should only be defined once (in `SANDBOX_TAG`) and the docker run should use `$SANDBOX_TAG`.

**Fix**: Change the docker run line from:
```bash
registry.helixml.tech/helix/helix-sandbox:2.5.60-rc5
```
to:
```bash
registry.helixml.tech/helix/helix-sandbox:$SANDBOX_TAG
```

---

### 3. Emails with Trailing Spaces Fail Registration

**Problem**: User registration fails when email addresses have trailing spaces (e.g., `user@example.com `).

**Expected**: Email should be trimmed before validation and storage.

**Fix**: Add `strings.TrimSpace()` to email input in registration handler.

---

### 4. External Repo Git Integration Broken (CRITICAL BUG)

**Problem**: External GitHub repo integration has multiple failures:

**Issue A: Initial sync fails silently**
- Automatic sync shows `branch_count=0` and fails with "couldn't find remote ref refs/heads/main"
- Manual `git fetch --all -v` inside container works fine
- After manual fetch, task creation works

**Issue B: Push to external repo fails silently**
- Agent pushes to internal git server → SUCCESS
- Internal server should push to GitHub → FAILS SILENTLY
- Agent thinks push succeeded (sees "pushed to origin/helix-specs")
- But branch never appears on GitHub
- Task stays in `spec_generation` forever because the sync/transition expects the branch

**Observed on code.helix.ml**:
- Agent pushed design docs to `helix-specs` branch
- Push appeared successful in agent logs
- But `helix-specs` branch doesn't exist on GitHub (only `main`)
- Task stuck in `spec_generation` status

**Workaround for Issue A**: Manual fetch:
```bash
docker compose exec api sh -c "cd /filestore/git-repositories/<repo-id> && git fetch --all -v"
```

**No workaround for Issue B** - external push is broken.

**Root cause analysis** (2026-01-28):

Likely cause: **HTTP request context cancellation**

In `git_http_server.go:600`, the external push uses `r.Context()`:
```go
if err := s.gitRepoService.PushBranchToRemote(r.Context(), repoID, branch, false); err != nil {
```

The problem:
1. Git client (agent) sends push to internal git server
2. Server runs `git receive-pack`, streams output back to client
3. Once output is complete, git client closes connection (considers push successful)
4. HTTP request context is cancelled when connection closes
5. External push starts but context is already cancelled
6. `pushBranchNative()` at line 115-122 checks `ctx.Done()` and returns early with error
7. Error is logged at line 601, branch is rolled back at line 610-611
8. But agent never sees this - it already disconnected

This explains:
- Agent sees "push successful" (internal push worked)
- Branch doesn't appear on GitHub (external push cancelled)
- Task stuck (transition depends on external push)

**Comparison with prime**: On prime, the memos repo's helix-specs branch DID make it to GitHub. The difference may be:
- Network latency (faster connection = context didn't cancel in time)
- OAuth token valid vs invalid (invalid would fail at credentials phase, not context)
- Timing (context cancellation is a race condition)

**Fix needed**: Change line 600 to use `context.Background()` or a detached context instead of `r.Context()`:
```go
// Use background context since HTTP connection may close before external push completes
pushCtx := context.Background()
if err := s.gitRepoService.PushBranchToRemote(pushCtx, repoID, branch, false); err != nil {
```

**Location to fix**:
- `api/pkg/services/git_http_server.go:600` - change context to background
- Consider adding timeout to background context for safety

---

### 5. OAuth Provider Missing Default Scopes Configuration

**Problem**: OAuth provider configuration has no field to specify default scopes. When connecting GitHub, no scopes are requested, resulting in tokens with no `repo` access.

**Expected**: OAuth provider should have a `scopes` field (e.g., `["repo", "read:user"]` for GitHub) that gets requested during the OAuth flow.

**Location**: `api/pkg/types/oauth.go` - `OAuthProvider` struct needs a `Scopes []string` field.

**Workaround**: Manually update the connection scopes in database after connecting.

---

### 5. CLI Needs User Management Commands

**Problem**: No CLI command to create new users. Currently must be done manually via database or UI.

**Desired**: `helix user create --email foo@example.com --password xxx` or similar.

**Use case**: Provisioning accounts on deployments like code.helix.ml without UI access.

---

### 7. Task Doesn't Transition to Review After Push (BUG)

**Problem**: After agent pushes to branch, task status stays in `implementation` instead of transitioning to `implementation_review`.

**Root Cause Analysis**:

1. **Prompt says it happens but code doesn't do it**:
   - `spec_task_prompts.go:109`: "The backend detects your push and moves the task to review status."
   - But `git_http_server.go:900-912` only records the push, explicitly says "don't transition status"

2. **Branch handling**:
   - `helix-specs` branch → `processDesignDocsForBranch()` - updates design docs only
   - `feature/*` branches → `handleFeatureBranchPush()` - records push, NO transition

3. **Old polling mechanism removed?**: User mentioned there used to be polling on git repo that changed to push-based. The transition logic may have been removed during that change.

**Locations**:
- `api/pkg/services/git_http_server.go:handleFeatureBranchPush()` - needs to transition status
- `api/pkg/services/spec_task_prompts.go:109` - promises behavior that doesn't exist

**Possible fixes**:
1. Add status transition in `handleFeatureBranchPush()` when agent signals complete
2. Add agent tool/MCP method to explicitly mark implementation complete
3. Auto-transition when all implementation tasks in plan are done
4. Add UI button for user to manually move to review

---

### 8. Zed Agent Screen Slow to Load

**Problem**: Zed takes a long time to load the agent screen. Need to investigate what it's doing during startup.

**To investigate**: Profile Zed startup, check network calls, identify blocking operations.

---

## Deployments Done Today

- **code.helix.ml**: Upgraded control plane and sandbox from `2.5.60-rc5` to `2.5.60`
- **Azure customer (axa-private)**: Upgraded control plane (4.231.116.26) and sandbox (172.201.248.88) from `2.5.59-rc2` to `2.5.60`
- **prime**: Upgraded from `2.5.60-rc2` to `2.5.60` (for debugging)
