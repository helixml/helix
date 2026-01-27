# Helix Git System Architecture

**Date:** 2026-01-27
**Author:** Claude Opus 4.5
**Status:** Implemented

## Overview

Helix implements a custom git infrastructure that:
1. Hosts internal bare repositories for each project
2. Syncs with external upstream repositories (GitHub, GitLab, etc.)
3. Provides HTTP-based git access for agents (Zed IDE, Qwen Code)
4. Handles post-push hooks for SpecTask workflow automation

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           External Git Providers                             │
│  (GitHub, GitLab, Azure DevOps, Bitbucket, etc.)                           │
└─────────────────────────────────────────────────────────────────────────────┘
                                    ▲
                                    │ HTTPS (with OAuth token)
                                    │ Push/Pull sync
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Helix API Server                                     │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │ GitRepositoryService                                                     ││
│  │ - CreateRepository (clones external, creates internal)                  ││
│  │ - SyncBaseBranch (fetch from upstream before SpecTask)                  ││
│  │ - PushBranchToRemote (push to upstream after local changes)             ││
│  │ - PullFromRemote (fast-forward merge from upstream)                     ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │ GitHTTPServer                                                            ││
│  │ - handleInfoRefs (git-upload-pack/receive-pack discovery)               ││
│  │ - handleUploadPack (git clone/fetch)                                    ││
│  │ - handleReceivePack (git push)                                          ││
│  │ - Post-push hooks (design doc processing, upstream sync)                ││
│  └─────────────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────────────┘
                                    ▲
                                    │ HTTP (git protocol)
                                    │ With API key auth
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Desktop Container (Sandbox)                          │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐          │
│  │     Zed IDE      │  │   Qwen Code      │  │   User Agent     │          │
│  │  (clones repos)  │  │ (reads/writes)   │  │  (git commands)  │          │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘          │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Repository Types

### Internal Repositories
- Created fresh without external URL
- Stored as bare repos at `/filestore/git-repositories/{repo_id}/`
- No upstream sync needed
- Used for projects without GitHub integration

### External Repositories
- Cloned from external URL (e.g., GitHub)
- Stored as bare repos with `origin` remote pointing to external
- Require OAuth token for authenticated operations
- Support bidirectional sync (push/pull)

## Flow Diagrams

### 1. Repository Creation Flow

```
User clicks "Create Project" → Selects GitHub repo
                                      │
                                      ▼
                    ┌─────────────────────────────────────┐
                    │ SimpleForkSampleProject endpoint    │
                    │ - Validate OAuth token              │
                    │ - Create project in DB              │
                    └─────────────────────────────────────┘
                                      │
                                      ▼
                    ┌─────────────────────────────────────┐
                    │ GitRepositoryService.CreateRepository│
                    │ 1. Generate repo ID                 │
                    │ 2. Create DB record (status=cloning)│
                    │ 3. Build authenticated URL          │
                    └─────────────────────────────────────┘
                                      │
                                      ▼
                    ┌─────────────────────────────────────┐
                    │ Clone external repo (async)         │
                    │ - git clone --mirror {auth_url}     │
                    │ - Parse progress output             │
                    │ - Update clone_progress in DB       │
                    └─────────────────────────────────────┘
                                      │
                                      ▼
                    ┌─────────────────────────────────────┐
                    │ Post-clone setup                    │
                    │ - Detect default branch             │
                    │ - List branches                     │
                    │ - Update status=active              │
                    │ - Register with Kodit (optional)    │
                    └─────────────────────────────────────┘
```

### 2. Agent Push Flow (Minimizing Conflicts)

```
Agent makes changes in Zed IDE
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ Agent runs: git push origin feature/spec_task_xyz                            │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ GitHTTPServer.handleReceivePack                                              │
│ 1. Record branch hashes BEFORE receive-pack                                  │
│    branchesBefore = getBranchHashes(repoPath)                               │
│                                                                              │
│ 2. Execute: git receive-pack --stateless-rpc                                │
│    (accepts pack file, updates refs)                                         │
│                                                                              │
│ 3. Record branch hashes AFTER                                               │
│    branchesAfter = getBranchHashes(repoPath)                                │
│    pushedBranches = detectChangedBranches(before, after)                    │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ Branch Restriction Check (for agent API keys)                                │
│ - Get API key from Authorization header                                      │
│ - Lookup branch restrictions for this key                                    │
│ - If branch not in allowed list → ROLLBACK refs                             │
│   rollbackBranchRefs(repoPath, branchesBefore, pushedBranches)              │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ▼ (if external repo)
┌─────────────────────────────────────────────────────────────────────────────┐
│ SYNCHRONOUS Push to Upstream                                                 │
│ For each pushed branch:                                                      │
│ 1. Build authenticated URL with OAuth token                                  │
│ 2. giteagit.Push(repoPath, {Remote: "origin", Branch: refspec})             │
│ 3. If push fails → ROLLBACK local refs                                      │
│    rollbackBranchRefs(repoPath, branchesBefore, pushedBranches)             │
│                                                                              │
│ Why SYNCHRONOUS?                                                             │
│ - Ensures upstream is updated before client sees success                     │
│ - Allows rollback if upstream rejects push                                   │
│ - Prevents divergence between local and upstream                             │
│                                                                              │
│ KNOWN ISSUE: If branch A pushes to upstream but branch B fails,             │
│ we rollback locally but A is already on upstream = DIVERGENCE               │
│ TODO: Track which branches were successfully pushed and only rollback those │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ▼ (if successful)
┌─────────────────────────────────────────────────────────────────────────────┐
│ ASYNCHRONOUS Post-Push Hooks                                                 │
│ - handleFeatureBranchPush: SpecTask workflow                                │
│ - handleMainBranchPush: Design doc archival                                 │
│ - processDesignDocsForBranch: Parse and index design docs                   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3. Sync from Upstream Flow (Before SpecTask)

```
User starts new SpecTask
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ SyncBaseBranch(repoID, baseBranch)                                           │
│ Purpose: Ensure local base branch is up-to-date before branching            │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. Fetch from upstream                                                       │
│    Fetch(repoPath, {                                                         │
│        Remote: authenticatedURL,                                             │
│        RefSpecs: ["+refs/heads/{branch}:refs/remotes/origin/{branch}"],     │
│        Force: true,                                                          │
│        Timeout: 2 minutes                                                    │
│    })                                                                        │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 2. Get local and remote commit IDs                                           │
│    localCommit = GetBranchCommitID(repoPath, branch)                        │
│    remoteCommit = getRemoteTrackingCommit(repoPath, branch)                 │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ 3. Check divergence                                                          │
│    diverge = GetDivergingCommits(repoPath, remoteBranch, localBranch)       │
│                                                                              │
│    If diverge.Ahead > 0 AND diverge.Behind > 0:                             │
│        Return BranchDivergenceError (requires manual resolution)             │
│                                                                              │
│    If diverge.Ahead == 0 AND diverge.Behind > 0:                            │
│        Fast-forward: updateBranchRef(repoPath, branch, remoteCommit)        │
│        (No merge needed - just move ref forward)                             │
│                                                                              │
│    If diverge.Ahead > 0 AND diverge.Behind == 0:                            │
│        Local is ahead - no sync needed                                       │
│        (Will push to upstream after SpecTask completes)                      │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4. Post-Push Hook Flow

```
Push completed successfully
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ handlePostPushHook (runs ASYNCHRONOUSLY)                                     │
│ For each pushed branch:                                                      │
└─────────────────────────────────────────────────────────────────────────────┘
           │
           ├─────────────────────────────────────────────┐
           ▼                                             ▼
┌───────────────────────────┐              ┌───────────────────────────┐
│ Feature branch push       │              │ Main branch push          │
│ (feature/*, helix-specs)  │              │ (main/master)             │
├───────────────────────────┤              ├───────────────────────────┤
│ 1. Find associated        │              │ 1. Archive completed      │
│    SpecTask by branch     │              │    SpecTasks              │
│                           │              │                           │
│ 2. Parse design docs      │              │ 2. Update design doc      │
│    from commit            │              │    indices                │
│                           │              │                           │
│ 3. Update SpecTask state  │              │ 3. Trigger post-merge     │
│    (planning→implementing)│              │    workflows              │
│                           │              │                           │
│ 4. Send message to agent  │              │                           │
│    about new docs         │              │                           │
└───────────────────────────┘              └───────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ processDesignDocsForBranch                                                   │
│ 1. Get changed files in latest commit                                       │
│    files = GetCommitFileStatus(repoPath, commitHash)                        │
│                                                                              │
│ 2. Filter for design doc files                                              │
│    (design/tasks/*.md, .helix/startup.sh)                                   │
│                                                                              │
│ 3. Extract SpecTask ID from path                                            │
│    design/tasks/{task_id}/plan.md → task_id                                 │
│                                                                              │
│ 4. Update SpecTask with new design doc content                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Conflict Minimization Strategy

### Why Conflicts Occur
1. **Concurrent edits**: Human and agent editing same file
2. **Divergent history**: Upstream changes while agent working
3. **Branch confusion**: Agent pushes to wrong branch

### How Helix Minimizes Conflicts

1. **Pre-SpecTask sync**: Always fetch and fast-forward base branch before creating feature branch

2. **Branch restrictions**: Agent API keys are restricted to specific branches (feature/spec_task_*)

3. **Synchronous upstream push**: Push to upstream immediately after local update, rollback if rejected

4. **Detect divergence before merge**: Use `GetDivergingCommits` to detect if merge would be non-trivial

5. **Fail-fast on divergence**: Return `BranchDivergenceError` instead of attempting complex merge

6. **Separate branches per task**: Each SpecTask gets its own feature branch, preventing cross-task conflicts

## Implementation Notes

### gitea Module Usage

The implementation uses gitea's git module (`code.gitea.io/gitea/modules/git`) with two levels:

**High-Level APIs** (preferred where available):
- `giteagit.OpenRepository()` - Open a repository
- `repo.GetCommit()` - Get commit info
- `repo.GetBranchCommitID()` - Get commit ID for branch
- `repo.GetBranchNames()` - List branches
- `repo.GetRefCommitID()` - Get commit ID for any ref
- `repo.DeleteBranch()` - Delete branch
- `repo.CreateBranch()` - Create branch
- `repo.RenameBranch()` - Rename branch
- `repo.CommitsByRange()` - List commits with pagination/filters
- `repo.CommitsByFileAndRange()` - List commits touching a file
- `giteagit.GetCommitFileStatus()` - Get files changed in commit
- `giteagit.GetDivergingCommits()` - Compare two branches
- `giteagit.Push()` - Push to remote
- `giteagit.InitRepository()` - Create new repo
- `giteagit.AddChanges()` - Stage files
- `giteagit.CommitChanges()` - Create commit

**Low-Level gitcmd** (when no high-level alternative):
- `git merge-base --is-ancestor` - Check commit ancestry
- `git update-ref` - Update ref to point to commit
- `git symbolic-ref` - Get/set HEAD
- `git remote add/set-url/remove` - Remote management
- `git checkout` - Switch branches in working copy
- `git fetch` with custom env - Fetch with auth
- `git push` to path - Push to local bare repo
- `git upload-pack/receive-pack` - HTTP protocol

### HTTP Protocol Implementation

The GitHTTPServer implements Smart HTTP protocol:

1. **Discovery** (`GET /info/refs?service=git-upload-pack`):
   - Client requests available refs
   - Server runs `git upload-pack --stateless-rpc --advertise-refs`
   - Returns ref list in pkt-line format

2. **Fetch** (`POST /git-upload-pack`):
   - Client sends wanted/have objects
   - Server runs `git upload-pack --stateless-rpc`
   - Streams pack file to client

3. **Push** (`POST /git-receive-pack`):
   - Client sends pack file with new objects
   - Server runs `git receive-pack --stateless-rpc`
   - Server validates branch restrictions
   - Server pushes to upstream (for external repos)
   - Server runs post-push hooks

## Edge Cases Handled

1. **Initial clone progress**: Clone progress is tracked via regex parsing of git output, stored in DB, polled by frontend

2. **OAuth token refresh**: Tokens are embedded in clone/push URLs; if expired, operation fails with auth error

3. **Upstream push failure**: Rollback local refs to previous state using `update-ref`

4. **Branch divergence**: Return structured error with ahead/behind counts for UI display

5. **Empty commits**: `GetCommitFileStatus` handles commits with no parent (initial commits)

6. **Missing branch**: Graceful fallback to default branch or error

7. **Concurrent pushes**: Last-write-wins for same branch (git's native behavior)

## Security Considerations

1. **API key scoping**: Agent API keys only have access to their assigned branches

2. **OAuth token handling**: Tokens embedded in URLs for clone/push, not stored in git config

3. **Path traversal**: All repo paths validated against `gitRepoBase`

4. **Command injection**: Use `gitcmd.AddDynamicArguments()` which validates input
