# Design: Enforce Agent Branch Restrictions via Pre-Receive Hook

## Architecture Overview

The solution moves branch restriction enforcement from post-receive rollback to pre-receive rejection using Git's native hook mechanism.

```
Current (broken):
  Agent Push → receive-pack (accepts) → check branches → rollback → client sees "success"

Fixed:
  Agent Push → pre-receive hook (rejects) → client sees "error: branch not allowed"
```

## Key Design Decisions

### Decision 1: Environment Variable for Allowed Branches

Pass allowed branches to the pre-receive hook via `HELIX_ALLOWED_BRANCHES` environment variable.

**Rationale:**
- Git hooks inherit the parent process environment
- Already passing `GIT_PROTOCOL` env var in the same code path
- Clean separation: HTTP handler determines permissions, hook enforces them
- No need for hook to make API calls or read files

**Format:** Comma-separated branch names, e.g., `helix-specs,feature/001062-my-task`

### Decision 2: Single Combined Pre-Receive Hook

Extend the existing pre-receive hook (currently only protects `helix-specs` from force-push) to also handle branch restrictions.

**Rationale:**
- Only one pre-receive hook can exist per repository
- Avoids complexity of chaining hooks
- Both checks operate on the same input (ref updates from stdin)

### Decision 3: Fail-Open for Missing Env Var

If `HELIX_ALLOWED_BRANCHES` is not set, allow all branches (normal user behavior).

**Rationale:**
- Backward compatible with existing behavior
- Normal users (non-agent API keys) don't have restrictions
- Only agent API keys trigger the env var to be set

## Implementation Details

### Modified Files

1. **`api/pkg/services/git_http_server.go`**
   - In `handleReceivePack()`: Get branch restriction BEFORE running receive-pack
   - Pass `HELIX_ALLOWED_BRANCHES` in the `environ` slice
   - Remove post-receive rollback logic for branch restrictions

2. **`api/pkg/services/gitea_git_helpers.go`**
   - Update `preReceiveHookScript` to check `HELIX_ALLOWED_BRANCHES`
   - Increment `PreReceiveHookVersion` to trigger hook updates

### Pre-Receive Hook Logic

```bash
# Pseudocode for updated hook
ALLOWED_BRANCHES="$HELIX_ALLOWED_BRANCHES"  # e.g., "helix-specs,feature/001062"

while read oldrev newrev refname; do
    branch="${refname#refs/heads/}"
    
    # Existing: force-push protection for helix-specs
    if [ "$branch" = "helix-specs" ] && is_force_push; then
        reject "refusing to force-push to protected branch 'helix-specs'"
    fi
    
    # New: branch restriction for agents
    if [ -n "$ALLOWED_BRANCHES" ]; then
        if ! branch_in_list "$branch" "$ALLOWED_BRANCHES"; then
            reject "refusing to update '$branch' - allowed branches: $ALLOWED_BRANCHES"
        fi
    fi
done
```

### Error Message Format

Following GitHub's branch protection error style:

```
remote: error: refusing to update refs/heads/main
remote: hint: This push is restricted to: helix-specs, feature/001062-agent-was-able-to-push
remote: hint: Push to your assigned feature branch instead.
error: failed to push some refs to 'http://api:8080/git/...'
```

## Testing Strategy

1. **Unit Test:** Hook script logic with various branch/permission combinations
2. **Integration Test:** End-to-end push from agent API key to unauthorized branch
3. **Regression Test:** Normal users can still push to any branch
4. **Regression Test:** Force-push protection for helix-specs still works

## Rollout

1. Increment `PreReceiveHookVersion` to trigger automatic hook updates
2. Existing repos get updated hook on next `receive-pack` call (hook is checked/updated in `handleReceivePack`)
3. No migration needed - hooks auto-update on API startup via `InstallPreReceiveHooksForAllRepos()`

## Implementation Notes

### Files Modified

1. **`api/pkg/services/gitea_git_helpers.go`** (lines 319-390):
   - Changed `PreReceiveHookVersion` from `"1"` to `"2"`
   - Rewrote `preReceiveHookScript` to include `branch_allowed()` helper function
   - Hook now reads `HELIX_ALLOWED_BRANCHES` env var and rejects unauthorized branches
   - Combined force-push protection and branch restriction into single loop

2. **`api/pkg/services/git_http_server.go`** (lines 539-561 and 584-586):
   - Moved `getBranchRestrictionForAPIKey()` call BEFORE `cmd.Run()` (was after)
   - Added env var: `environ = append(environ, "HELIX_ALLOWED_BRANCHES="+strings.Join(restriction.AllowedBranches, ","))`
   - Removed ~30 lines of post-receive rollback logic for branch restrictions
   - Kept upstream push failure rollback (different code path, still needed)

### Shell Script Gotcha

The `branch_allowed()` function uses a subshell pattern with `echo | while read` and `exit 0` inside the loop. This works because we check `return 1` after the subshell - if the subshell exited with 0 (found), the return value propagates. This is a common shell idiom for "find in list".

### Existing Pattern Reused

The codebase already had:
- Pre-receive hook infrastructure (`InstallPreReceiveHook`, `PreReceiveHookVersion`)
- Environment variable passing in `handleReceivePack()` (used for `GIT_PROTOCOL`)
- `getBranchRestrictionForAPIKey()` function for looking up agent restrictions

No new infrastructure needed - just rewired existing components.
