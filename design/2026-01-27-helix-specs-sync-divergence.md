# helix-specs Branch Sync Divergence Analysis

**Date:** 2026-01-27
**Status:** Investigation
**Issue:** Multi-agent push race condition causes lost commits

## Problem Statement

**Core Issue:** Two agents pushed design docs to the same helix-specs branch, and **both pushes succeeded** even though neither agent had pulled the latest changes. Git should have rejected the second push because the old SHA didn't match the current ref, but somehow it was accepted.

**Observed behavior:**
```
Agent 001022: 4fca338ce..362140a75 (succeeded)
Agent 001023: 362140a75..1a8a3cdb8 (agent thought it succeeded, but commit was lost!)
Agent 001024: 362140a75..04e15b55d (succeeded - note parent is 362140a75, NOT 1a8a3cdb8)
```

Agent 001024's push used `362140a75` as parent, which means Agent 001023's commit `1a8a3cdb8` was already gone. The Helix git server should have rejected 001024's push with "failed to update ref" because the server's current ref was `1a8a3cdb8`, not `362140a75`.

**Expected behavior (from manual testing):**
When two git clones make divergent changes and push sequentially, the second push is rejected:
```
$ git push origin master  # Second push from diverged clone
 ! [rejected]        master -> master (fetch first)
error: failed to push some refs...
hint: Updates were rejected because the remote contains work that you do not have locally.
```

**Mystery:** Manual testing confirms git correctly rejects diverged pushes. So why did the Helix git server accept them?

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

### Phase 3b - ROOT CAUSE FOUND

**The Problem:**
Both agents pushed to helix-specs, both saw success from their perspective, but one agent's specs never showed up!

**Agent push logs (from agent tool call history):**

| Task # | Task ID | Commit Range | Agent Status | Specs Detected? |
|--------|---------|--------------|--------------|-----------------|
| 001021 | ? | `f58d34187..4fca338ce` | Completed | ? |
| 001022 | ? | `4fca338ce..362140a75` | Completed | ? |
| 001023 | spt_01kg04dv3zfec3t9zap8wmnmn3 | `362140a75..1a8a3cdb8` | Completed | **NO - LOST!** |
| 001024 | spt_01kg04npzjy912y5jnjp3nc59j | `362140a75..04e15b55d` | Completed | Yes |

**Raw agent logs:**
```
# Task 001021
[helix-specs 4fca338ce] Design docs for In api/pkg/services/git_http_server.go...
To http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-helix-4
   f58d34187..4fca338ce  helix-specs -> helix-specs

# Task 001022
[helix-specs 362140a75] Design docs for Connect to Azure DevOps...
To http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-helix-4
   4fca338ce..362140a75  helix-specs -> helix-specs

# Task 001023 - SPECS NEVER SHOWED UP!
[helix-specs 1a8a3cdb8] Design docs for chat widget O(n²) streaming performance fix
To http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-helix-4
   362140a75..1a8a3cdb8  helix-specs -> helix-specs

# Task 001024 - NOTE: Same parent as 001023!
[helix-specs 04e15b55d] Design docs for install go in the helix startup script...
To http://api:8080/git/prj_01kg02vqqyg178c1n2ydscn5fb-helix-4
   362140a75..04e15b55d  helix-specs -> helix-specs
```

**Task creation times (from API logs):**
```
16:23:45Z - spt_01kg049a7rv74y916vxn0y8fpk (also failed - specs not detected)
16:26:14Z - spt_01kg04dv3zfec3t9zap8wmnmn3 (001023 - specs not detected)
16:30:31Z - spt_01kg04npzjy912y5jnjp3nc59j (001024 - specs detected)
```

**SMOKING GUN: Both 001023 and 001024 have the same parent `362140a75`**

This is impossible under normal git operation:
- 001023 pushed `362140a75..1a8a3cdb8` and agent saw success
- 001024 pushed `362140a75..04e15b55d` and succeeded
- If 001023's push truly succeeded, 001024's parent should be `1a8a3cdb8`
- The fact that 001024's parent is `362140a75` proves the bare repo was rolled back

**Root Cause: Rollback after GitHub push failure creates a silent data loss window**

The sequence that causes lost commits:

```
Timeline:

Agent A                    Helix Bare Repo              GitHub
   │                            │                          │
   │ git push (base..A)         │                          │
   │ ─────────────────────────> │                          │
   │                            │ receive-pack succeeds    │
   │                            │ (bare repo: base → A)    │
   │ <───────────────────────── │                          │
   │ (Agent A sees SUCCESS)     │                          │
   │                            │ post-push to GitHub...   │
   │                            │ ─────────────────────────>│
   │                            │                  FAILS!   │
   │                            │ <─────────────────────────│
   │                            │ ROLLBACK!                 │
   │                            │ (bare repo: A → base)     │
   │                            │                          │
   │        AGENT A's COMMIT IS NOW GONE!                  │
   │        (but agent thinks it succeeded)                │
   │                            │                          │
Agent B                         │                          │
   │ git push (base..B)         │                          │
   │ ─────────────────────────> │                          │
   │                            │ old value = base ✓        │
   │                            │ receive-pack succeeds    │
   │                            │ (bare repo: base → B)    │
   │ <───────────────────────── │                          │
   │ (Agent B sees SUCCESS)     │                          │
   │                            │ post-push to GitHub...   │
   │                            │ ─────────────────────────>│
   │                            │                  SUCCESS  │
   │                            │ <─────────────────────────│
   │                            │                          │
   │        AGENT B's COMMIT PERSISTS                      │
   │        AGENT A's COMMIT IS LOST FOREVER               │
```

**Why this happens:**
1. The HTTP response is streamed during `receive-pack` - agent sees success immediately
2. Post-push to GitHub happens AFTER the response is sent
3. If GitHub push fails, we rollback bare repo refs
4. Agent never knows their commit was rolled back
5. Next agent's push succeeds because `old_value` matches the rolled-back state

### Reproduction Test Results (2026-01-27)

**Test repo:** `code-test1-2-1769535450` (connected to GitHub)

**Test 1: Sequential pushes - Git correctly rejects diverged push**
```
=== Push from Clone 1 (Agent A) ===
To http://localhost:8080/git/code-test1-2-1769535450
   a028729..cec29f2  helix-specs -> helix-specs
Exit: 0

=== Push from Clone 2 (Agent B) - SHOULD FAIL ===
To http://localhost:8080/git/code-test1-2-1769535450
 ! [rejected]        helix-specs -> helix-specs (non-fast-forward)
error: failed to push some refs
Exit: 1
```
✅ Git correctly rejects the diverged push.

**Test 2: Concurrent pushes - Git correctly rejects with atomic check**
```
=== Concurrent pushes ===
Push 1: Exit: 0 (succeeded)
Push 2: ! [remote rejected] helix-specs -> helix-specs (incorrect old value provided)
         Exit: 1
```
✅ Git atomic check prevents concurrent overwrites.

**Conclusion:** The git server is working correctly. The issue is the **rollback after GitHub push failure**.

### Proposed Fixes

**Option A: Don't rollback on GitHub push failure (keep local, retry async)**
- Accept the push locally (agent sees success)
- If GitHub push fails, keep the local commit and queue async retry
- Pros: No silent data loss
- Cons: Local and GitHub can diverge temporarily

**Option B: Validate before receive-pack (pre-push GitHub check)**
- Before running receive-pack, check if GitHub has the expected parent
- If not, reject the push immediately with clear error
- Pros: Agent sees failure before committing
- Cons: Adds latency, GitHub could change between check and push

**Option C: Lock helix-specs during push (serialize pushes)**
- Use a distributed lock on helix-specs branch during push operation
- Only one agent can push at a time
- Pros: Eliminates race conditions
- Cons: Limits concurrency, adds complexity

**Option D: Merge-based helix-specs (auto-merge non-conflicting)**
- Instead of rejecting diverged pushes, auto-merge if no conflicts
- Each task writes to its own directory, so conflicts are rare
- Pros: Best UX, maximizes concurrency
- Cons: Complex to implement, edge cases with same-file edits

**Recommended approach: Option A (no rollback)**
- Simplest fix with lowest risk
- The local commit is valid; GitHub sync is a secondary concern
- Add logging/alerting for failed GitHub syncs
- Implement async retry queue for failed pushes

### Proposed Future Work (Phase 3c)

**Agent retry logic:**
- When push fails, agent should pull/rebase and retry
- Optionally: send agent a message about the conflict via ACP
- Could implement automatic rebase for non-conflicting changes (different task directories)

**Long-term considerations:**
- Consider making helix-specs a merge-based branch
- Automatic merge for non-conflicting changes (different task directories)
- Conflict detection and resolution for same-file changes

---

## Phase 4: Evidence from API Server Logs (2026-01-27)

### Full Timeline from API Logs

| Time (UTC) | Event | Commit | Details |
|------------|-------|--------|---------|
| 16:26:14Z | Task 001022 receive-pack | → `362140a75` | `Receive-pack completed pushed_branches=["helix-specs"]` |
| 16:26:14Z | Task 001022 upstream push | | `Pushing branch to upstream (synchronous)` |
| 16:26:16Z | Task 001022 upstream success | | `Successfully pushed branch to upstream branch=helix-specs` |
| 16:26:16Z | Task 001022 post-push hook | `362140a75` | `Processing pushed branch branch=helix-specs commit=362140a7...` |
| 16:29:11-14Z | Force sync from GitHub | | Multiple `Syncing ALL branches from external repository` with `force=true` |
| 16:29:22Z | Task 001023 receive-pack | → `1a8a3cdb8` | `Receive-pack completed pushed_branches=["helix-specs"]` |
| 16:29:22Z | Task 001023 upstream push | | `Pushing branch to upstream (synchronous)` |
| 16:29:23Z | Task 001023 upstream success | | `Successfully pushed branch to upstream branch=helix-specs` |
| 16:29:23Z | **BUG**: Post-push hook wrong commit | `362140a75` | `Processing pushed branch branch=helix-specs commit=362140a7...` ← **SHOULD BE 1a8a3cdb8!** |
| 16:32:03Z | Task 001024 receive-pack | → `04e15b55d` | `Receive-pack completed pushed_branches=["helix-specs"]` |
| 16:32:03Z | Pre-push sync | | `Successfully synced from upstream before push` |
| 16:32:04Z | Task 001024 upstream push | | `Pushing branch to upstream (synchronous)` |
| 16:32:05Z | Task 001024 upstream success | | `Successfully pushed branch to upstream branch=helix-specs` |
| 16:32:05Z | Post-push hook | `04e15b55d` | `Processing pushed branch branch=helix-specs commit=04e15b55...` |
| 16:32:05Z | Task detected | | `Design docs detected in push repo_id=... task_ids=["spt_01kg04npzjy912y5jnjp3nc59j"]` |

### Critical Observation

**At 16:29:23**, the post-push hook for task 001023 shows:
```
Processing pushed branch branch=helix-specs commit=362140a7539f01aa70462cdf0ebbb099b7e3a868
```

But the agent pushed `1a8a3cdb8`! The post-push hook is reading the **WRONG COMMIT**.

### No Rollback Log Entry

**Crucially, there is NO "Failed to push branch to upstream - rolling back" log entry** in the API logs around 16:29. This suggests:

1. The upstream push to GitHub DID succeed for task 001023
2. But something else caused the branch to point to the old commit
3. OR the post-push hook simply reads the wrong commit (the bug we identified)

### The Real Root Cause: Post-Push Hook Race Condition

Looking at the logs more carefully:

1. At 16:29:11-14, there were **force syncs** happening (`Syncing ALL branches from external repository` with `force=true`)
2. These syncs were triggered by READ operations (not writes)
3. The `force=true` sync OVERWROTE local helix-specs with whatever GitHub had at that moment
4. If the sync happened AFTER receive-pack but BEFORE post-push to GitHub, this would:
   - Overwrite the local commit `1a8a3cdb8` with the old GitHub commit `362140a75`
   - The upstream push would then push `362140a75` (no-op since GitHub already has it)
   - The post-push hook would see `362140a75`

### Sequence Diagram

```
Agent 001023                    Helix API                     GitHub
     │                              │                            │
     │ git push (1a8a3cdb8)         │                            │
     │ ───────────────────────────> │                            │
     │                              │ receive-pack               │
     │                              │ local: 362... → 1a8...     │
     │                              │                            │
     │                              │ (Before upstream push...)  │
     │                              │                            │
     │              CONCURRENT      │                            │
     │              READ REQUEST → │ SyncAllBranches(force=true) │
     │                              │ ───────────────────────────>│
     │                              │ GitHub still has 362...    │
     │                              │ <───────────────────────────│
     │                              │ FORCE overwrites local!    │
     │                              │ local: 1a8... → 362...     │
     │                              │                            │
     │                              │ (Now upstream push...)     │
     │                              │ PushBranch(helix-specs)    │
     │                              │ ───────────────────────────>│
     │                              │ (no-op, already 362...)    │
     │                              │                            │
     │                              │ post-push hook             │
     │                              │ reads: 362... (WRONG!)     │
     │                              │                            │
     │ <─────────────────────────── │ (agent sees success)       │
     │                              │                            │
     │        COMMIT 1a8a3cdb8 IS NOW LOST!                      │
```

### The Bug

The `SyncAllBranches(force=true)` is being called by read operations (e.g., viewing the repo), and it **force-overwrites** local refs. If this happens in the window between `receive-pack` and `PushBranchToRemote`, the commit is lost.

### Fix Required

1. **Don't use force=true for SyncAllBranches during reads** - force sync should only be used for explicit user action
2. **Lock the repository during push operations** - prevent concurrent sync operations
3. **Push immediately after receive-pack** - minimize the window where commits can be lost
4. **Don't force-sync helix-specs** - Helix is the source of truth for this branch
