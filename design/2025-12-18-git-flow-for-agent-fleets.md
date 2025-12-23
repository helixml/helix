# Single-Direction Git Branches: Running Agent Fleets on Enterprise Repos

*Or: what happens when you can't just give AI agents your Azure DevOps credentials*

---

Everyone's building AI coding agents. But they're all single-player. One agent, one human, one repo.

We sell to enterprises. They want 10 agents working in parallel on the same codebase, connected to Azure DevOps, behind their firewall, with their SSO, without giving AI direct access to their git credentials.

That last part is the problem.

We're [Helix](https://github.com/helixml/helix), a bootstrapped team building open-source infrastructure for AI agent fleets. We're not VC-funded. We don't have 6 months to build elegant solutions. We have enterprise customers who need this working yesterday, and a burn rate that keeps me up at night.

Here's what happens when you naively parallelize agents:

```
Agent 1: git checkout main && git checkout -b feature/add-login
Agent 2: git checkout main && git checkout -b feature/add-signup
Agent 3: git checkout main && git checkout -b feature/fix-auth

# Meanwhile, a human merges a PR on GitHub...

Agent 4: git checkout main  # Wait, which main? Local or remote?
```

## The Solution We Eventually Stumbled Into

After weeks of staring at divergence errors at 2am, we realized the problem was bidirectional sync. Every time we tried to sync branches in both directions, edge cases multiplied. So we stopped.

**Every branch type now has exactly one direction:**

| Branch Type | Direction | Why |
|-------------|-----------|-----|
| `main` | PULL-ONLY | Customer's ADO/GitHub is source of truth. We never write to it directly. |
| `helix-specs` | PUSH-ONLY | Our design docs branch. Customer never writes to it. |
| Feature branches | PUSH-ONLY | Agents create branches and push. They never pull updates. |

This constraint sounds limiting until you realize it eliminates an entire class of problems.

## The Security Constraint That Drove Everything

Enterprise security teams have opinions about AI agents with direct write access to their repos. Strong opinions. Opinions that sound like "absolutely not" and "over my dead body."

Fair enough. We wouldn't want that either. One hallucinating agent with push access to production is one too many.

So we built a security boundary: a bare git repo hosted by Helix that sits between agent sandboxes and the customer's external repo.

```
Customer's Repo (ADO/GitHub)  ←→  Helix Bare Repo  ←→  Agent Sandbox
            │                           │                    │
       source of truth           credential boundary     no git creds
```

Agents push to our repo. We sync with the customer's repo using a service connection they control. The agent sandbox never sees their git credentials.

The problem? If customers make changes upstream while agents are making changes, conflicts happen in **our** middle repo. And resolving conflicts in a bare repository means building an entire conflict resolution UI so someone (the customer? the agent?) can reconcile changes in the middle layer.

We haven't built that. We probably should. The alternative is surfacing upstream's branch under a different name so the agent can see both versions. We haven't figured out how to do that elegantly either.

For now: enforce direction per branch. Conflicts in the middle repo become impossible. Problem solved, kind of.

## Main Branch: Pull-Only, Fast-Forward Only

Before any agent starts a new task, we sync main. But only via fast-forward:

```go
// If local has commits not in remote = we screwed up somehow
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

If main has diverged, something is deeply wrong and we refuse to continue. This has caught bugs where agents accidentally committed to main instead of their feature branch. Ask me how I know.

## The Lab Notebook Problem

We do spec-driven development - agents write requirements and design docs before coding (similar to AWS's [Kiro](https://kiro.dev/)). These specs need to live somewhere.

"Just put them in the feature branch."

No. Here's why:

```
Agent debugging auth:
  - Writes notes in feature/add-auth/design.md
  - Tries different approach, creates feature/add-auth-v2
  - Reverts some commits that didn't work
  - Checks out main to reproduce bug
  - Creates fix/auth-edge-case from different base
  - 💨 Where are my notes? Which branch has my current thinking?
```

Development is messy. You switch branches, revert commits, try different approaches. But your **thinking shouldn't switch with your code**. Your notes are a lab notebook - the paper you keep beside you while debugging across 5 branches in 3 repos. You don't rip out pages when you `git checkout`.

Solution: an **orphan branch** called `helix-specs` with no shared history with main.

```go
// Orphan commit = no parents = completely separate history from main
commit := &object.Commit{
    Message:  "Initialize helix-specs branch",
    TreeHash: emptyTreeHash,
    // No ParentHashes - orphan branch
}
```

The branch structure:

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

**Bonus:** No markdown pollution in code PRs. Some of our devs *hated* when agents dumped huge piles of design docs alongside code changes. Separate branch = clean code reviews.

## Feature Branches: Push-Only

"But what if Agent A needs to pull Agent B's changes mid-work?"

They don't. Here's the scenario:

```
Agent B: feature/add-auth (commits A, B, C)
Agent A: checks out add-auth, branches to feature/add-login
Agent A: makes commits D, E, F

# Meanwhile Agent B pushes commits G, H to add-auth
# Should Agent A pull G, H?

NO.
```

Agents CAN resolve conflicts - they're actually pretty good at it. But remember: there's a middle bare repo between agents and your external repo. If we allow bidirectional sync, conflicts happen in that middle layer, and we'd need to surface upstream's version as a different branch, build a reconciliation UI, etc.

We haven't figured that out yet. Maybe we will. For now:

1. Agent finishes work, pushes to Helix
2. Helix pushes to your repo
3. Conflicts? Resolve them in the PR UI like a normal human

```go
if branchToSync != repo.DefaultBranch {
    // Feature branches are PUSH-ONLY. No sync needed.
    continue
}
```

Is this the right design? Honestly, we're not sure. The alternative is building proper bidirectional sync with conflict resolution surfaced to agents. That's a lot of work for a bootstrapped team. We'd love to hear if anyone's solved this elegantly.

## Why Bare Repos?

Quick aside on why the middle repo is bare (no working directory):

- No working directory = no checkout conflicts
- Multiple agents can push via HTTP simultaneously
- We control when to sync upstream
- Agents never touch your enterprise credentials

The single-direction rule exists specifically to make this architecture work without building conflict resolution infrastructure in the middle layer. Maybe someday. For now, it works.

## Why Humans Resolve Conflicts

We're not building autonomous agents that replace developers. We tried. The error rate was unacceptable for enterprise customers who actually care about their codebase.

The model is **pair programming with a fleet**. You're still the engineer. You define tasks, review designs, approve implementations. The agents do the typing.

This means humans are the natural place to resolve conflicts:

- Feature branches are PUSH-ONLY → conflicts surface in the PR, where humans review anyway
- helix-specs is PUSH-ONLY → humans approve designs before agents start coding
- Main is PULL-ONLY → agents work from the latest blessed version

Every design decision assumes a human is watching. Our enterprise customers wanted it that way. So did we, frankly. Fully autonomous agents pushing to production without review is a horror story waiting to happen.

## Results

Before single-direction branches:
- ~5% of task starts hit divergence errors
- Confusing error messages that told us nothing
- Hours of debugging sessions

After:
- Zero divergence errors in production
- 10+ agents working on same repo simultaneously
- 200ms average sync time for repos up to 1GB

The constraint (single direction per branch) turned out to be a feature, not a limitation.

---

## Open Questions

We're not sure this is the right design. Some things we're still thinking about:

1. **Conflict resolution in bare repos.** We push this to PR review. There might be a better way. We haven't found it.

2. **Bidirectional sync.** Single-direction is restrictive. We could build proper reconciliation - surface upstream changes as a separate branch, let agents or users merge. That's a lot of infrastructure for a small team.

3. **What are other agent platforms doing?** Multi-agent git coordination isn't discussed publicly anywhere we've found. If you're solving this differently, we'd genuinely like to know.

4. **Orphan branches for design docs.** Useful hack or proper pattern? We're biased. It works for us.

---

## The Code

Helix is open source: [github.com/helixml/helix](https://github.com/helixml/helix)

The git sync code is in `api/pkg/services/git_repository_service_pull.go`.

---

*We're a bootstrapped company selling AI agent infrastructure to enterprises. We need this to work correctly more than we need it to be elegant. If you have better ideas, we're listening. If you want to tell us we're doing it wrong, you're probably right.*

*— Luke*
