# helix-specs Branch Sync Divergence Analysis

**Date:** 2026-01-27
**Status:** Investigation
**Issue:** 409 errors when agents push to external repos (Azure DevOps)

## Problem Statement

When an agent tries to push to an external repository's helix-specs branch, we're getting:
```
error fetching from remote: failed to update 'refs/heads/helix-specs' (non-fast-forward)
```

This happens because our local (middle) helix-specs branch is **ahead** of the upstream (remote) helix-specs branch.

## User's Scenario

1. Attach repo to project - all branches get pulled down (including helix-specs from upstream)
2. Start a SpecTask (NOT a cloned task)
3. At some point, specs for this task get written to helix-specs in the middle repo
4. User can see new specs MIXED with old ones in the repo viewer
5. Agent tries to push, git server does pre-push sync
6. Pre-push sync FAILS because local is ahead of remote

## Architecture: External Repo Flow

```
┌─────────────┐    push    ┌─────────────┐    push    ┌─────────────┐
│   Agent     │ ─────────> │ Helix Git   │ ─────────> │  Upstream   │
│             │            │  Server     │            │  (ADO/GH)   │
└─────────────┘            │ (Middle)    │            └─────────────┘
                           └─────────────┘
                                 │
                                 │ sync (fetch)
                                 ▼
                           ┌─────────────┐
                           │  Upstream   │
                           └─────────────┘
```

**Flow for agent push (git_http_server.go:510-630):**
1. Agent does `git push` to Helix git server
2. Git server receives push request
3. **Pre-push sync**: `SyncAllBranches()` fetches ALL branches from upstream
4. `git receive-pack` accepts the push from agent
5. **Post-push**: `PushBranchToRemote()` pushes changed branches to upstream

## Key Code Paths

### 1. Pre-push Sync (git_http_server.go:514-523)
```go
if repo != nil && repo.ExternalURL != "" {
    if err := s.gitRepoService.SyncAllBranches(r.Context(), repoID, false); err != nil {
        // Don't reject push on sync errors (changed from error to warning)
        log.Warn().Err(err).Msg("Pre-push sync from upstream had errors (continuing with push)")
    }
}
```

### 2. SyncAllBranches (git_repository_service_pull.go:416-434)
```go
refSpec := "refs/heads/*:refs/heads/*"  // Without + prefix = non-force
if force {
    refSpec = "+" + refSpec
}
err = Fetch(ctx, gitRepo.LocalPath, FetchOptions{
    Remote:   fetchURL,
    RefSpecs: []string{refSpec},
    Force:    force,
    Prune:    true,
    Timeout:  5 * time.Minute,
})
```

**Problem:** Without force (`+`), git fetch fails if any local branch is ahead of remote.

### 3. Post-push to Upstream (git_http_server.go:612-629)
```go
for _, branch := range pushedBranches {
    if err := s.gitRepoService.PushBranchToRemote(r.Context(), repoID, branch, false); err != nil {
        log.Error().Err(err).Msg("Failed to push branch to upstream - rolling back")
        upstreamPushFailed = true
        break
    }
}
if upstreamPushFailed {
    s.rollbackBranchRefs(repoPath, branchesBefore, pushedBranches)
}
```

## Possible Causes of Divergence

### Theory 1: Post-push to Upstream Failed (Most Likely)

1. Agent pushes specs to middle repo
2. Pre-push sync succeeds (local == remote)
3. Push accepted (local now has new commit)
4. Post-push to upstream **FAILS** (branch protection, auth issue, network, etc.)
5. We rollback the local refs, but...
   - **Wait, we DO rollback on failure (line 627)**
   - So if upstream push fails, local should be rolled back

### Theory 2: Race Condition

1. Agent 1 pushes to middle repo
2. Pre-push sync succeeds
3. Push accepted, local updated
4. Post-push to upstream in progress...
5. **Agent 2** starts a push
6. Agent 2's pre-push sync sees local ahead of remote (upstream hasn't received Agent 1's push yet)

### Theory 3: API Server Writes to helix-specs Without Pushing

Code paths that write to helix-specs in the middle repo:

| Function | File | Pushes to Upstream? |
|----------|------|---------------------|
| `prepopulateClonedSpecs` | spec_driven_task_service.go:1630 | **NO** (bug - fixed today) |
| `SaveStartupScriptToHelixSpecs` | project_internal_repo_service.go:479 | Yes (async, line 437) |
| `createHelixSpecsBranch` | project_internal_repo_service.go:543 | No (only writes to local bare) |
| `createOrphanHelixSpecsBranch` | project_internal_repo_service.go:319 | No (only writes to local bare) |

**Problem with `prepopulateClonedSpecs`:**
- Called when starting a CLONED task
- Writes specs to helix-specs branch in middle repo
- Did NOT push to upstream (fixed today with push after writing)

**Problem with `createHelixSpecsBranch`:**
- Called when SaveStartupScriptToHelixSpecs can't find helix-specs locally
- Creates an ORPHAN branch (no connection to any existing upstream helix-specs)
- Only writes to local bare repo
- The caller (SaveStartupScriptToHelixSpecs) will push afterward, but if upstream already has helix-specs with different history, push will fail

### Theory 4: createHelixSpecsBranch Creates Orphan Without Checking Remote (Most Likely for User's Case)

1. Attach external repo (all branches cloned including helix-specs from upstream)
2. Somehow helix-specs gets deleted locally OR wasn't included in the clone
3. User updates startup script via UI
4. `SaveStartupScriptToHelixSpecs` checks if helix-specs exists **locally**
5. It doesn't exist locally → calls `createHelixSpecsBranch`
6. Creates new ORPHAN helix-specs with placeholder commit
7. Writes startup script
8. Tries to push to upstream
9. Push fails because upstream helix-specs has different (non-ancestor) commits
10. Local now has orphan helix-specs, upstream has different helix-specs
11. Any future sync will fail with non-fast-forward

## User's Specific Case Analysis

User said:
- "this is a repo that i reattached just a second ago"
- "i wasn't using the clone feature"
- "specs for this task get written to helix-specs in our middle repo, i see it mixed with the old ones"

This suggests:
1. helix-specs DID exist and had old design docs
2. New specs were written successfully
3. They're mixed together in the same branch
4. But push to upstream is failing

This points to **Theory 3** - something wrote to local helix-specs without pushing to upstream.

For a NON-cloned task, what writes specs to helix-specs?

**Answer:** The AGENT does. When the agent finishes generating specs, it does `git push` which:
1. Pushes to middle repo via git server
2. Git server pushes to upstream

But what if step 2 (push to upstream) failed silently?

Looking at git_http_server.go:617-629:
```go
if err := s.gitRepoService.PushBranchToRemote(r.Context(), repoID, branch, false); err != nil {
    log.Error().Err(err).Str("repo_id", repoID).Str("branch", branch).Msg("Failed to push branch to upstream - rolling back")
    upstreamPushFailed = true
    break
}
```

If push fails, we rollback. But the rollback only happens AFTER the HTTP response has been sent (or is it?).

Actually looking more carefully:
- Line 541-545: Set response headers, write response
- Line 562-573: Run receive-pack (streams response to client)
- Line 612-629: Push to upstream (AFTER response started streaming)

So the push to upstream happens AFTER we've already started sending the response. If it fails, we rollback the refs, but... **does git receive-pack already "accept" the push from the agent's perspective?**

Yes! The agent sees success because receive-pack completes. But then we rollback the refs. So the agent THINKS the push succeeded, but it actually didn't persist.

Wait, but that would mean local is NOT ahead - we rolled back.

Unless... the rollback itself fails?

## Next Steps

1. Check if rollback is working correctly
2. Add more logging around push-to-upstream failures
3. Consider: should we NOT rollback and instead just log a warning? The local change is valid, we just couldn't sync to upstream.
4. Consider: for helix-specs specifically, should we treat Helix as source of truth and force-push if needed?

## Proposed Fix

### Option A: Force-fetch for helix-specs only
In SyncAllBranches, use force (`+`) for helix-specs branch specifically:
```go
refSpec := "refs/heads/*:refs/heads/*"
if force {
    refSpec = "+" + refSpec
}
// Always force helix-specs - Helix is source of truth
helixSpecsRefSpec := "+refs/heads/helix-specs:refs/heads/helix-specs"
```

**Problem:** This would LOSE local changes if remote is ahead.

### Option B: Skip helix-specs in pre-push sync
Don't try to sync helix-specs from upstream before a push - just push.
```go
// In SyncAllBranches, exclude helix-specs
refSpec := "refs/heads/*:refs/heads/*"
// But with a refspec that excludes helix-specs
```

**Problem:** git doesn't support exclusion in refspecs easily.

### Option C: Separate sync for code branches vs helix-specs
1. For code branches (main, feature/*): sync from upstream (upstream is source of truth)
2. For helix-specs: DON'T sync from upstream (Helix is source of truth)

This makes sense because:
- Code branches: developers push to GitHub/ADO, we pull from there
- helix-specs: Helix generates design docs, we push to GitHub/ADO

### Option D: Don't sync at all before push, only after
Remove pre-push sync entirely. Only sync after successful push.

**Problem:** This could cause conflicts if someone else pushed to the branch.

## Key Logical Constraint

**If Helix is the ONLY source of changes to helix-specs, and we ALWAYS sync from upstream before pushing, we should NEVER be ahead of upstream.**

The only ways we could be ahead:
1. We wrote something locally that wasn't pushed to upstream
2. There's a bug in the sync logic that caused us to not fetch when we should have
3. There's a race condition between fetch and push

## Detailed Trace: Agent Push Flow

```
Agent                    Git HTTP Server                 Middle Repo         Upstream
  │                            │                             │                  │
  │ git push ────────────────> │                             │                  │
  │                            │                             │                  │
  │                            │ SyncAllBranches() ─────────────────────────────>│
  │                            │                             │    fetch         │
  │                            │ <────────────────────────────────────────────── │
  │                            │                             │                  │
  │                            │ branchesBefore = getBranchHashes()              │
  │                            │                             │                  │
  │                            │ receive-pack ──────────────>│                  │
  │ <──────────────────────────│                             │ (LOCAL UPDATED)  │
  │ (push "succeeded")         │                             │                  │
  │                            │                             │                  │
  │                            │ branchesAfter = getBranchHashes()               │
  │                            │ pushedBranches = diff                           │
  │                            │                             │                  │
  │                            │ PushBranchToRemote() ──────────────────────────>│
  │                            │                             │                  │
```

**Key observation:** After `receive-pack` completes, the LOCAL repo is updated with the agent's changes. The HTTP response has already started streaming to the agent. THEN we try to push to upstream.

If the push to upstream fails:
1. We log an error
2. We call `rollbackBranchRefs()` to restore the original commit hashes
3. BUT the HTTP response already indicated success to the agent!

**Question:** Does rollbackBranchRefs actually work? Let's check...

## Investigating rollbackBranchRefs

```go
// rollbackBranchRefs reverts branch refs to their original values after a failed upstream push
func (s *GitHTTPServer) rollbackBranchRefs(repoPath string, originalHashes map[string]string, branchesToRollback []string) {
    for _, branch := range branchesToRollback {
        if originalHash, exists := originalHashes[branch]; exists {
            // Branch existed before - restore to original hash
            gitcmd.NewCommand("update-ref").
                AddDynamicArguments("refs/heads/"+branch, originalHash).
                RunStdString(context.Background(), &gitcmd.RunOpts{Dir: repoPath})
        } else {
            // Branch was newly created - delete it
            gitcmd.NewCommand("branch").
                AddArguments("-D").
                AddDynamicArguments(branch).
                RunStdString(context.Background(), &gitcmd.RunOpts{Dir: repoPath})
        }
    }
}
```

**This should work.** If push to upstream fails, we restore the local refs to what they were before.

## The Puzzle

Given:
1. Clone uses `Mirror: true` - ALL branches fetched
2. Pre-push sync fetches ALL branches before accepting push
3. Post-push pushes to upstream, and if it fails, we rollback

**How can local ever be ahead of remote?**

### Hypothesis 1: Push to upstream succeeds, but we don't update our tracking

After a successful push to upstream, both local and remote should have the same commit. But what if:
- We push successfully
- Later, we fetch from upstream again
- The fetch FAILS to update local for some reason
- Local is now "behind" in git's view, but actually has the same content?

No, this doesn't make sense. If we pushed successfully, both should have the same commit SHA.

### Hypothesis 2: Multiple concurrent pushes

1. Agent A pushes to helix-specs
2. Pre-push sync succeeds (local == remote)
3. receive-pack updates local with A's changes
4. While post-push is in progress...
5. Agent B pushes to helix-specs
6. B's pre-push sync: local (with A's changes) is ahead of remote (still syncing)
7. Sync fails with non-fast-forward

**This is a race condition!** If two agents push at roughly the same time:
- The first agent's post-push to upstream hasn't completed yet
- The second agent's pre-push sync sees local ahead of remote

### Hypothesis 3: Something else writes to helix-specs outside the push flow

Looking at code paths that write to helix-specs:

| Function | When Called | Pushes to Upstream? |
|----------|-------------|---------------------|
| `SaveStartupScriptToHelixSpecs` | User updates startup script in UI | Yes (async) |
| `prepopulateClonedSpecs` | Starting a CLONED task | **NO** (bug - fixed today) |
| `createHelixSpecsBranch` | First time saving startup script | No (caller pushes) |
| Agent via git push | Agent pushes specs | Yes (post-push flow) |

For user's case (NOT a cloned task), `prepopulateClonedSpecs` shouldn't be called.

**BUT WAIT:** User said "specs for this task get written to helix-specs in our middle repo".

For a non-cloned task, the ONLY thing that writes specs is the agent's git push. So the agent must have pushed at least once successfully.

Then why is pre-push sync failing on a subsequent push?

### Hypothesis 4: The push to upstream failed on the first push, we rolled back, but the agent retried

1. Agent pushes specs (first time)
2. Pre-push sync succeeds (local == remote, both don't have specs)
3. receive-pack updates local with specs
4. Post-push to upstream FAILS (branch protection, network, etc.)
5. We rollback local refs
6. Agent sees success (HTTP response already sent)
7. Agent does another push (e.g., updates to specs, or implementation)
8. **BUT WAIT** - if we rolled back, local == remote still, so sync should work

Unless... the agent is pushing the SAME commit that we just rolled back. The agent's local still has the commit. If the agent pushes again:
- Pre-push sync: local == remote (both old)
- receive-pack: updates local with agent's commit (same as before)
- Post-push: tries to push to upstream... same failure?

Actually this should work on retry if the issue was transient.

### Hypothesis 5: Pre-push sync is not actually fetching helix-specs

Let me check if SyncAllBranches fetches ALL branches or just specific ones.

```go
refSpec := "refs/heads/*:refs/heads/*"
```

This should fetch ALL branches. But wait, what if there's a branch that only exists locally and not remotely? That's fine for fetch (just won't update it).

What if a branch exists remotely but not locally? The refspec should create it.

What if a branch exists both places with divergent commits? That's the non-fast-forward case.

## The Answer (?)

I think the issue is simpler than we thought. Let me re-read the user's exact scenario:

> 1. attach repo, all branches get pulled down
> 2. start spectask
> 3. specs for this task get written to helix-specs in our middle repo, i see it mixed with the old ones
> 4. agent tries to push, can't pull-on-push

In step 3, the user says specs are ALREADY in the middle repo. But for a non-cloned task, specs would only be there after the agent pushes.

**Key question:** How did specs get into the middle repo BEFORE the agent tries to push?

For a cloned task, `prepopulateClonedSpecs` would write them. But user says it's not a cloned task.

For a non-cloned task... let me check if there's any other code path.

Actually wait - the user might be describing what they SEE, not the temporal order. They see specs in the repo viewer, which might be AFTER the first successful push, and the error is on a SUBSEQUENT push.

Let me ask: **Is this the FIRST push for this task, or a subsequent one?**

## ROOT CAUSE FOUND

### Timeline from API logs (code-backtester-2-1769520446):

```
13:27:26 - Repo cloned with Mirror=true (4 branches including helix-specs)
13:27:31 - Multiple syncs all SUCCESS - helix-specs synced fine
13:27:47 - "Startup script saved to helix-specs branch [commit=af29d98e]" ← PROBLEM!
13:28:05 - Sync FAILS with non-fast-forward on helix-specs
```

### The Bug

In `createProject` (project_handlers.go:248-265):
1. `InitializeStartupScriptInCodeRepo` writes startup.sh to helix-specs locally
2. **NO push to upstream** for external repos!
3. Local helix-specs is now ahead of remote
4. All subsequent syncs fail with non-fast-forward

This is different from `updateProject` which DOES push to upstream after saving (lines 429-451).

### Fixes Applied (Phase 1 - Post-Push)

1. **project_handlers.go:267-291** - Push helix-specs to upstream after initializing startup script in `createProject` for external repos

2. **spec_driven_task_service.go:1718-1739** - Push helix-specs to upstream after pre-populating cloned specs for external repos

3. **git_http_server.go:516-520** - Keep pre-push sync as a hard error (reject with 409). If sync fails, something wrote to helix-specs without pushing - this is a bug.

## Comprehensive Audit (Phase 2)

After fixing the initial issue, we audited ALL code paths that write to the middle repo for external repos. The principle is:

**For ANY write to the middle repo (external), we MUST:**
1. **Pre-sync**: Fetch from upstream BEFORE writing - if conflict, FAIL the operation
2. **Write**: Commit the change locally
3. **Post-push**: Push to upstream AFTER writing

### Write Paths Audited

| Path | Description | Pre-sync? | Post-push? | Status |
|------|-------------|-----------|------------|--------|
| `handleReceivePack` (git_http_server.go) | Agent git push | ✅ | ✅ | OK |
| `createProject` (project_handlers.go) | Init startup script | ✅ Added | ✅ Added | Fixed |
| `updateProject` (project_handlers.go) | Update startup script | ✅ Added | ✅ Added | Fixed |
| `prepopulateClonedSpecs` (spec_driven_task_service.go) | Clone task specs | ✅ Added | ✅ Added | Fixed |
| `createOrUpdateGitRepositoryFileContents` (git_repository_handlers.go) | UI file edit | ✅ Added | ✅ Added | Fixed |

### Fixes Applied (Phase 2 - Pre-Sync + Post-Push)

1. **project_handlers.go (createProject)** - Added pre-sync before `InitializeStartupScriptInCodeRepo`
   - Handles case where repo already exists in Helix but project is new
   - Warning on failure (don't block project creation)

2. **project_handlers.go (updateProject)** - Added pre-sync before `SaveStartupScriptToHelixSpecs`
   - Hard error on sync failure - user must resolve
   - Returns 500 error with clear message

3. **spec_driven_task_service.go (prepopulateClonedSpecs)** - Added pre-sync before writing specs
   - Hard error on sync failure - task start fails
   - Prevents divergence before agent work begins

4. **git_repository_handlers.go (createOrUpdateGitRepositoryFileContents)** - Added both pre-sync and post-push
   - Pre-sync: 409 Conflict on failure
   - Post-push: 500 error on failure (file saved locally)
   - Critical for any UI file edits to external repos

**Principle (Updated):**
- Before ANY write to an external repo, sync from upstream first
- If sync fails (local ahead of remote), FAIL the operation with a clear error
- After ANY write to an external repo, push to upstream immediately
- User must be informed when saves fail due to sync issues

---

## Phase 3: Multi-Writer Race Condition (2026-01-27)

### New Bug Discovered

When multiple agents push to helix-specs concurrently, one agent's commit can be lost due to a race condition.

### Root Cause Analysis

**Scenario:**
1. Agent A creates commit A (parent: base) at 16:29:19
2. Agent B creates commit B (parent: base) at 16:32:03
3. Both A and B are based on the same parent (base), not on each other

**Timeline:**
1. Agent A pushes commit A to Helix server
2. Server's helix-specs = A
3. Server pushes A to GitHub → GitHub helix-specs = A
4. Post-push hook runs for A (in goroutine)
5. Agent B pushes commit B to Helix server
6. Server syncs from GitHub (gets A)
7. BUT: Helix bare repo has `receive.denyNonFastForwards` NOT SET
8. Server accepts B as a **non-fast-forward push** (overwrites A)
9. Server helix-specs = B, commit A is now dangling!
10. Server pushes B to GitHub (helix-specs not protected)
11. GitHub helix-specs = B, commit A is lost!
12. Post-push hook for A runs but looks at wrong commit (fix applied in Phase 3a)

### Bugs Found

1. **Bug 1**: `getTaskIDsFromPushedDesignDocs` reads current branch tip instead of pushed commit
   - **Fix Applied**: Pass `commitHash` parameter and use `GetChangedFilesInCommit(commitHash)`
   - **Impact**: Prevents wrong task from being detected if branch moved

2. **Bug 2**: Helix bare repos allow non-fast-forward pushes
   - **Issue**: `receive.denyNonFastForwards` is not set
   - **Impact**: One agent's commit can overwrite another's
   - **Fix Required**: See options below

3. **Bug 3**: GitHub helix-specs branch is not protected
   - **Issue**: Non-fast-forward pushes to GitHub are allowed
   - **Impact**: Lost commits are not recoverable from GitHub
   - **Fix Required**: Enable branch protection OR force-push strategy

### Evidence from Investigation

Commit `1a8a3cdb8` (Agent A's design docs):
- Parent: `362140a75`
- Content: Design docs for task 001023
- Created at: 16:29:19
- **Status**: Exists on server as DANGLING commit (not in any branch)

Commit `04e15b55d` (Agent B's design docs):
- Parent: `362140a75` (same parent as A!)
- Content: Design docs for task 001024
- Created at: 16:32:03
- **Status**: Currently on helix-specs (both server and GitHub)

### Fix Applied (Phase 3a)

**File: git_http_server.go**

Changed `getTaskIDsFromPushedDesignDocs` to accept `commitHash` parameter:
```go
// Before (bug):
files, err := gitRepo.GetChangedFilesInBranch("helix-specs")  // reads current tip

// After (fixed):
files, err := gitRepo.GetChangedFilesInCommit(commitHash)  // reads specific commit
```

This prevents the race condition where the post-push hook looks at the wrong commit.

### Phase 3b Investigation - REVERTED

**Initial approach (reverted):** Set `receive.denyNonFastForwards = true` via git config.

This was reverted because:
1. `receive.denyNonFastForwards` blocks **force pushes** (`git push --force`)
2. But **diverged pushes are already rejected by git natively** - the atomic update check

**Key insight from manual testing:**

When testing manually (cloning repo twice, making changes from same base, pushing sequentially):
- Git correctly rejected the second push with "failed to update ref"
- This happened WITHOUT `receive.denyNonFastForwards` being set
- The atomic update check (`old-sha must match current ref`) handles this

**Question:** If git natively rejects diverged pushes, why did the agents' pushes succeed?

### Phase 3b - Continued Investigation

**Agent push logs showing the problem:**

```
Agent 001022: 4fca338ce..362140a75 (succeeded)
Agent 001023: 362140a75..1a8a3cdb8 (agent thought it succeeded)
Agent 001024: 362140a75..04e15b55d (agent thought it succeeded, push handler picked it up)
```

**Critical observation:** Agent 001024's push had parent `362140a75`, NOT `1a8a3cdb8`.
This means Agent 001023's commit was already gone when 001024 pushed.

**Difference between manual testing and agent pushes:**

| Aspect | Manual Testing | Agent Pushes |
|--------|---------------|--------------|
| Pre-push sync | None | Yes - `SyncAllBranches(force=false)` |
| Branch | Tested on main? | helix-specs (orphan branch) |
| Timing | Sequential | Possibly concurrent |
| Git client | Standard git CLI | Agent's embedded git |

**Hypotheses to test:**

1. **Pre-push sync overwrites local**: Even with `force=false`, does the pre-push sync somehow reset local refs?

2. **Concurrent pre-push syncs**: If Agent B's pre-push sync runs BEFORE Agent A's receive-pack completes, both might see the same starting state.

3. **Orphan branch behavior**: Does helix-specs being an orphan branch change how git validates pushes?

4. **Rollback timing**: If Agent A's push to GitHub failed and triggered rollback BEFORE Agent B's push, local would be back at old state.

### Reproduction Test Plan

**Setup:**
1. Create a test repo with helix-specs branch
2. Clone it to two working copies (simulating two agents)
3. Make changes in both from the same base commit
4. Push sequentially through the Helix git HTTP server

**Test 1: Manual git push (no Helix server)**
```bash
# Clone 1
cd /tmp/clone1
git checkout helix-specs
echo "change A" >> test.txt && git commit -am "Agent A"

# Clone 2
cd /tmp/clone2
git checkout helix-specs
echo "change B" >> test.txt && git commit -am "Agent B"

# Push sequentially to origin
cd /tmp/clone1 && git push origin helix-specs  # Should succeed
cd /tmp/clone2 && git push origin helix-specs  # Should FAIL with non-fast-forward
```

**Test 2: Through Helix git server (with pre-push sync)**
Same as above, but push to Helix's git server URL instead of origin.
Expected: Second push should still fail (git atomic check).
Actual: TBD

**Test 3: Concurrent pushes**
Use two terminals to push simultaneously.
Check: Does the pre-push sync create a race window?

### Log analysis needed

To understand what happened with the original bug, we need:

1. **Exact timestamps** for each agent's push request
2. **Pre-push sync logs** - did each agent's sync succeed? What refs were updated?
3. **receive-pack output** - did it accept or reject each push?
4. **Post-push logs** - did push to GitHub succeed or fail for each?
5. **Rollback logs** - was `rollbackBranchRefs` called?

### Proposed Future Work (Phase 3c)

**Agent retry logic:**
- When push fails, agent should pull/rebase and retry
- Optionally: send agent a message about the conflict via ACP
- Could implement automatic rebase for non-conflicting changes (different task directories)

**Long-term considerations:**
- Consider making helix-specs a merge-based branch
- Automatic merge for non-conflicting changes (different task directories)
- Conflict detection and resolution for same-file changes
