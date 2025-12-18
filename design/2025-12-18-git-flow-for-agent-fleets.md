# Git Flow for Agent Fleets: How We Solved Branch Sync for Parallel AI Coding Agents

**Date:** 2025-12-18
**Author:** Luke Marsden

## The Problem Nobody Talks About

Everyone's building AI coding agents. What nobody's talking about is what happens when you have *multiple* agents working on the same codebase simultaneously.

At Helix, we're building an enterprise platform where teams can run fleets of AI agents working in parallel on different tasks. Think 10 agents, each working on separate features, all pushing to the same repository that's connected to Azure DevOps or GitHub.

The naive approach fails immediately:

```
Agent 1: git checkout main && git checkout -b feature/add-login
Agent 2: git checkout main && git checkout -b feature/add-signup
Agent 3: git checkout main && git checkout -b feature/fix-auth

# Meanwhile, a human merges a PR on GitHub...

Agent 4: git checkout main  # Wait, which main? Local or remote?
```

## The Single-Direction Branch Strategy

After weeks of debugging weird divergence errors and race conditions, we landed on a simple principle: **every branch type has exactly ONE direction**.

| Branch Type | Direction | Why |
|-------------|-----------|-----|
| Main/default | PULL-ONLY | External repo is source of truth. Protected on enterprise repos. |
| Config branch (`helix-specs`) | PUSH-ONLY | Helix owns this branch. Never pull from upstream. |
| Feature branches | PUSH-ONLY | Created by agents. Don't pull upstream changes mid-work. |

This sounds obvious in retrospect, but it took us a while to get here.

## The Divergence Problem

Here's the scenario that kept breaking:

1. Import external repo from Azure DevOps
2. Agent creates feature branch from main
3. Human merges a different PR on ADO (main advances)
4. Agent finishes work, tries to push
5. Next task starts, tries to sync main, gets **divergence error**

The divergence happens because we had a stale local `main` and the upstream `main` had moved. Git's merge-base algorithm detects this correctly, but the error message was useless:

```
error: cannot sync - local has 1 commit not in upstream
```

## Fast-Forward Only for Main

Our solution: main branch is **pull-only** and must always fast-forward.

```go
// SyncBaseBranch - runs before starting any new task
func (s *GitRepositoryService) SyncBaseBranch(ctx context.Context, repoID, branchName string) error {
    // Fetch to remote-tracking ref first (non-destructive)
    fetchRefSpec := fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branchName, branchName)
    repo.Fetch(fetchOpts)

    // Get local and remote refs
    localRef, _ := repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
    remoteRef, _ := repo.Reference(plumbing.NewRemoteReferenceName("origin", branchName), true)

    // Check for divergence
    ahead, behind, _ := s.countCommitsDiff(repo, localRef.Hash(), remoteRef.Hash())

    // If local has commits not in remote = DIVERGENCE
    if ahead > 0 {
        return &BranchDivergenceError{
            BranchName:  branchName,
            LocalAhead:  ahead,
            LocalBehind: behind,
        }
    }

    // Fast-forward: just move the ref pointer
    newLocalRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branchName), remoteRef.Hash())
    repo.Storer.SetReference(newLocalRef)
}
```

## The Orphan Branch Trick: A Forward-Only Lab Notebook

Another problem: startup scripts and agent config need to live *somewhere*, but enterprises protect their main branch. You can't push to main on Azure DevOps without a PR.

But there's a deeper issue. We're doing **spec-driven development** - similar to AWS's [Kiro](https://kiro.dev/) approach where agents first write requirements, design docs, and implementation plans before coding. These specs need to live somewhere persistent.

The problem with putting specs in feature branches:

```
Agent works on feature/add-auth:
  - Writes requirements.md, design.md, tasks.md
  - Implements the feature
  - PR gets merged, feature branch deleted
  - 💨 Design docs gone forever
```

Development involves constantly jumping between branches. You might have 5 agents on 5 different feature branches. But your **design documentation should be a forward-only lab notebook** - an append-only record of all the thinking, decisions, and learnings that happened across all tasks.

Solution: an **orphan branch** called `helix-specs` that has no shared history with main.

```go
// Create empty tree (no files)
emptyTree := object.Tree{}
emptyTreeObj := repo.Storer.NewEncodedObject()
emptyTree.Encode(emptyTreeObj)
emptyTreeHash, _ := repo.Storer.SetEncodedObject(emptyTreeObj)

// Create orphan commit (no parents)
commit := &object.Commit{
    Message:  "Initialize helix-specs branch",
    TreeHash: emptyTreeHash,
    // Note: no ParentHashes - this is an orphan commit
}
```

This gives us:

1. **A branch we fully control** - agents push directly, no PR review needed
2. **Persistent design history** - specs survive feature branch deletion
3. **Forward-only append** - helix-specs only moves forward, never syncs from upstream
4. **Separation of concerns** - code history stays clean, design history is separate

The structure inside helix-specs:

```
helix-specs/
├── .helix/
│   └── startup.sh           # Environment setup script
└── design/
    └── tasks/
        ├── 2025-12-18_add-auth_42/
        │   ├── requirements.md
        │   ├── design.md
        │   └── tasks.md
        └── 2025-12-18_fix-login_43/
            ├── requirements.md
            ├── design.md
            └── tasks.md
```

Each task gets a dated directory. When an agent marks a task complete in `tasks.md`, it commits and pushes to helix-specs. The lab notebook grows monotonically - you never lose the record of what the agent was thinking when it made decisions.

## Feature Branches: Don't Pull Mid-Work

The trickiest insight was about feature branches. When Agent A is building on top of Agent B's WIP branch:

```
Agent B: feature/add-auth (commits A, B, C)
Agent A: git checkout feature/add-auth && git checkout -b feature/add-login
Agent A: (makes commits D, E, F)

# Meanwhile Agent B pushes more commits (G, H) to feature/add-auth

# Should Agent A pull? NO!
```

If Agent A pulls mid-work, you get merge conflicts in the middle of an AI's work session. The agent isn't equipped to resolve merge conflicts intelligently.

Instead:
1. Agent A pushes their feature branch when done
2. Merge happens at PR merge time (GitHub/ADO handles this)
3. If conflicts, the human resolves them in the PR UI

```go
// Only sync if this is the default branch
if branchToSync != repo.DefaultBranch {
    log.Info().
        Str("branch", branchToSync).
        Msg("Skipping sync for non-default branch (feature branches are PUSH-ONLY)")
    continue
}
```

## Bare Repositories as the Source of Truth

We store external repos as **bare git repositories** on our server:

```
External Repo (ADO/GitHub)     Helix Bare Repo        Agent Sandbox
        │                            │                      │
        │◄────── initial clone ──────┤                      │
        │                            │◄──── agent pushes ───┤
        │◄────── helix pushes ───────┤                      │
```

Bare repos have no working directory, which means:
- No working directory conflicts
- Multiple agents can push via HTTP simultaneously
- We control when to sync with upstream

## The Stats

After implementing this:
- Zero divergence errors in production (was ~5% of task starts before)
- 10+ agents can work on same repo simultaneously
- Average sync time: 200ms for repos up to 1GB

## What's Next

We're still figuring out:
1. What to do when upstream renames their default branch (master → main)
2. Better UX for showing divergence state to users
3. Automatic conflict resolution for simple cases

## Try It

Helix is open source: https://github.com/helixml/helix

The git sync code is in `api/pkg/services/git_repository_service_pull.go` if you want to steal our approach.

---

*Luke Marsden is the founder of Helix. Previously he worked on container orchestration at Weaveworks and storage at ClusterHQ.*
