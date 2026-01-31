# Every Branch Has a Direction: Git Flow for AI Agent Fleets

*What happens when you can't just give AI agents your Azure DevOps credentials*

---

We're [Helix](https://github.com/helixml/helix), a bootstrapped team selling AI agent infrastructure to enterprises. The pitch: run 10 coding agents in parallel on your codebase, connected to Azure DevOps, behind your firewall.

The constraint: enterprise security won't give AI agents direct git credentials. This isn't paranoia - [GitGuardian's 2025 report](https://blog.gitguardian.com/agentic-ai-secdays-france/) found repositories with Copilot active have 40% higher incidence of secret leaks. One hallucinating agent with push access to production is one too many.

So we built a middle layer:

```
Customer's Repo (ADO/GitHub)  ←→  Helix Bare Repo  ←→  Agent Sandboxes
            │                           │                     │
       source of truth           we control this         no git creds
```

Agents push to our bare repo. We sync with the customer's repo using credentials they control. The agent sandbox never sees their git credentials.

**The problem:** both sides can make changes. Customer merges a PR on ADO. Meanwhile, agents are pushing feature branches. Now there's a conflict in our middle repo.

And here's what nobody tells you about bare repositories: **you can't merge in them**. There's no working directory. No `git merge`. No `git checkout --theirs`. You'd need to build an entire conflict resolution UI, or clone to a temp directory, resolve, push back. For every conflict. At scale.

We haven't built that. So we needed a different approach.

## The Constraint That Fixed Everything

After weeks of debugging, we realized we were fighting git instead of working with it. The problem was bidirectional sync - trying to make branches flow both ways through the middle repo.

So we stopped. **Every branch type now has exactly one direction:**

| Branch Type | Direction | Why |
|-------------|-----------|-----|
| `main` | PULL-ONLY | Customer's ADO/GitHub is source of truth. We only read. |
| `helix-specs` | PUSH-ONLY | Our design docs branch. Customer never writes to it. |
| Feature branches | PUSH-ONLY | Agents create and push. They never pull updates. |

This is a type system for git branches. Once you declare a direction, the code enforces it. Conflicts in the middle repo become structurally impossible.

Conflicts still happen - but they happen in the PR on GitHub/ADO, where they belong, where the customer's existing tooling handles them.

## Main Branch: Pull-Only, Fast-Forward Only

Before any agent starts a new task, we sync main. But only via fast-forward:

```go
// If local has commits not in remote = something is deeply wrong
if ahead > 0 {
    return &BranchDivergenceError{BranchName: branchName}
}

// Fast-forward: just move the ref pointer. No merge commits.
newLocalRef := plumbing.NewHashReference(
    plumbing.NewBranchReferenceName(branchName),
    remoteRef.Hash(),
)
repo.Storer.SetReference(newLocalRef)
```

If main has diverged, we refuse to continue. This has caught bugs where agents accidentally committed to main instead of their feature branch.

The error handling was critical. Our first implementation just said "divergence detected." Useless. Now we report exactly how many commits are ahead/behind and on which branch. Debugging went from hours to minutes.

## The Lab Notebook Problem

We do spec-driven development - agents write requirements and design docs before coding (similar to AWS's [Kiro](https://kiro.dev/)). These specs need to live somewhere.

"Just put them in the feature branch."

No. Here's what actually happens during development:

```
Agent debugging auth:
  - Writes notes in feature/add-auth/design.md
  - Tries different approach, creates feature/add-auth-v2
  - Reverts some commits that didn't work
  - Checks out main to reproduce bug
  - Creates fix/auth-edge-case from different base
  - 💨 Where are the notes? Which branch has the current thinking?
```

Development is messy. You switch branches, revert commits, try different approaches. But your **thinking shouldn't switch with your code**. Your notes are a lab notebook - the paper you keep beside you while debugging across 5 branches in 3 repos. You don't rip out pages when you `git checkout`.

**The trick:** an orphan branch called `helix-specs` with no shared history with main.

```go
// Orphan commit = no parents = completely separate history
commit := &object.Commit{
    Message:  "Initialize helix-specs branch",
    TreeHash: emptyTreeHash,
    // No ParentHashes - this branch exists in a parallel universe
}
```

This is undersold in git tutorials. An orphan branch is essentially a separate repository living inside your repo. It never merges with main. It never conflicts with your code. It's a completely independent namespace.

The structure:

```
helix-specs/
├── .helix/startup.sh           # Agent environment setup
└── design/tasks/
    ├── 2025-12-18_add-auth_42/
    │   ├── requirements.md
    │   ├── design.md
    │   └── tasks.md
    └── ...
```

Each task gets a dated directory. The notebook grows monotonically. You never lose the record of what the agent was thinking.

**Side benefit:** No markdown pollution in code PRs. Some of our devs *hated* when agents dumped huge piles of design docs alongside code changes. Separate branch = clean code reviews.

## Feature Branches: Push-Only

Agents can start work in three ways:

1. **New branch from main.** Easy. Pull main, branch, work, push.
2. **Continue on existing branch.** Easy. Pull the branch, work, push.
3. **New branch from existing branch.** Hard. This is where directionality matters.

Case 3 is the problem:

```
Agent B: feature/add-auth (commits A, B, C)
Agent A: checks out add-auth, branches to feature/add-login
Agent A: makes commits D, E, F

# Meanwhile Agent B pushes commits G, H to add-auth
# Should Agent A pull G, H?

NO.
```

Agents CAN resolve conflicts - they're actually pretty good at it. But remember the architecture: there's a bare repo between agents and your external repo. If Agent A pulls while working, that pull has to flow through our middle repo, potentially conflicting with what Agent B is pushing.

The alternative we're considering: surface upstream's branch under a different name (like `upstream/feature/add-auth`) so the agent sees both versions and can merge locally. We haven't built that yet.

For now, once an agent branches off, they don't pull updates from the parent. The flow is simple:

1. Agent finishes work, pushes to Helix
2. Helix pushes to your repo
3. Conflicts? Resolve them in the PR UI like a normal human

```go
if branchToSync != repo.DefaultBranch {
    // Feature branches are PUSH-ONLY. No sync needed.
    continue
}
```

## Why Bare Repos?

This is the part most people don't think about. We use a bare git repo (no working directory) as the middle layer because:

- **Concurrent pushes work.** Multiple agents can push via HTTP simultaneously without checkout conflicts.
- **No disk space for working copies.** Repositories can be huge. We only store objects.
- **Security isolation.** The bare repo is just plumbing. No scripts execute, no hooks run (we control that).

The tradeoff is brutal: you lose `git merge`, `git checkout`, `git diff` with working tree. Everything is ref manipulation and object storage. The go-git library helps, but you're operating at the plumbing level.

This constraint - no merge in bare repos - is what drove the entire single-direction design. If we could merge easily, we might have built bidirectional sync. The limitation forced a better architecture.

## Why Humans Resolve Conflicts

We're not building autonomous agents that replace developers. We tried. The error rate was unacceptable for enterprise customers who actually care about their codebase.

The model is **pair programming with a fleet**. You're still the engineer. You define tasks, review designs, approve implementations. The agents do the typing.

This means humans are the natural place to resolve conflicts:

- Feature branches are PUSH-ONLY → conflicts surface in the PR, where humans review anyway
- helix-specs is PUSH-ONLY → humans approve designs before agents start coding
- Main is PULL-ONLY → agents always work from the latest blessed version

Every design decision assumes a human is in the loop - even if they're supervising a fleet of 10 agents in parallel. Our enterprise customers wanted it that way. Fully autonomous agents pushing to production without review is a horror story waiting to happen.

## Results

Before single-direction branches:
- ~5% of task starts hit divergence errors
- Confusing error messages that told us nothing
- Hours debugging race conditions in sync logic

After:
- Zero divergence errors in production
- 10+ agents working on same repo simultaneously
- 200ms average sync time for repos up to 1GB
- Sync code went from ~800 lines to ~200 lines

The constraint (single direction per branch) turned out to be a feature, not a limitation. The code got simpler. The bugs disappeared. The mental model became obvious.

---

## How Others Are Solving This

We're not the only ones thinking about multi-agent git coordination:

**Devin's MultiDevin** uses a manager/worker model - one "manager" Devin distributes tasks to up to 10 "worker" Devins, then merges all successful changes into one branch. This works well for repeated, isolated tasks (lint fixes, migrations). It sidesteps the coordination problem by having a single merge point. [Cognition's approach](https://docs.devin.ai/release-notes/overview) is agent-centric - the manager agent handles conflicts, not infrastructure.

**Git worktrees** are another approach. [Nick Mitchinson wrote](https://www.nrmitchi.com/2025/10/using-git-worktrees-for-multi-feature-development-with-ai-agents/) about using worktrees for AI agent isolation - each agent gets a persistent working directory for its branch. This eliminates context switching friction and gives agents bounded workspaces. We haven't tried this yet but it's compelling.

**GitHub Copilot's agent** takes a different approach: branch protections still apply, and the agent's PRs require human approval before any CI/CD runs. [The agent spins up secure dev environments](https://news.ycombinator.com/item?id=44031432) via GitHub Actions. Similar to our human-in-loop model, but tighter integration with GitHub's existing permissions.

We chose the middle bare repo approach because we need to work with any git host (ADO, GitHub, GitLab, Bitbucket) and can't assume specific platform features.

## Open Questions

We're not certain this is the right design. Some things we're still thinking about:

1. **Bidirectional sync.** Single-direction is restrictive. We could surface upstream changes as `upstream/<branch>` and let agents merge. That's significant infrastructure.

2. **Manager agent for merging.** Devin's approach - have a manager agent consolidate worker outputs - is interesting. Could we add a "coordinator" agent that handles merges in our architecture?

3. **Git worktrees.** Would worktrees solve some of our problems more elegantly than the bare repo approach? We haven't experimented with this yet.

4. **Orphan branches for design docs.** Useful pattern or weird hack? We're biased. But it's worked for 6 months now.

---

## The Code

Helix is source available: [github.com/helixml/helix](https://github.com/helixml/helix)

The git sync code is in `api/pkg/services/git_repository_service_pull.go`.

---

*We're a bootstrapped company selling AI agent infrastructure to enterprises. We need this to work correctly more than we need it to be elegant. If you have better ideas, we're listening. If you want to tell us we're doing it wrong, you're probably right.*

*— Luke*
