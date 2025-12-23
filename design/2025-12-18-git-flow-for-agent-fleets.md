# We Mass-Produce Merge Conflicts (And That's Fine)

*What happens when you can't just give AI agents your Azure DevOps credentials*

---

We're [Helix](https://github.com/helixml/helix), a bootstrapped team selling AI agent infrastructure to enterprises. The pitch: run 10 coding agents in parallel on your codebase, connected to Azure DevOps, behind your firewall.

The constraint: enterprise security won't let AI agents have direct git credentials. Fair enough. One hallucinating agent with push access to production is one too many.

So we built a middle layer:

```
Customer's Repo (ADO/GitHub)  ←→  Helix Bare Repo  ←→  Agent Sandboxes
            │                           │                     │
       source of truth           we control this         no git creds
```

Agents push to our bare repo. We sync with the customer's repo using credentials they control. The agent sandbox never sees their git credentials.

**The problem:** both sides can make changes. Customer merges a PR on ADO. Meanwhile, agents are pushing feature branches. Now there's a conflict in our middle repo.

Resolving conflicts in a bare repository is painful. There's no working directory. You'd need to build an entire conflict resolution UI so someone (the customer? the agent?) can reconcile changes. Or surface upstream's branch under a different name so the agent sees both versions and can merge.

We haven't built either of those. We probably should. But we're bootstrapped with a burn rate that keeps me up at night. We needed something that works now.

## The Constraint That Fixed Everything

After weeks of 2am debugging, we stopped trying to sync branches bidirectionally. Instead: **every branch type has exactly one direction**.

| Branch Type | Direction | Why |
|-------------|-----------|-----|
| `main` | PULL-ONLY | Customer's ADO/GitHub is source of truth. We only read from it. |
| `helix-specs` | PUSH-ONLY | Our design docs branch. Customer never writes to it. |
| Feature branches | PUSH-ONLY | Agents create and push. They never pull updates mid-work. |

If a branch only moves in one direction, conflicts in the middle repo become impossible. Customer changes flow inward. Agent changes flow outward. They never collide in our infrastructure.

Conflicts still happen - but they happen in the PR on GitHub/ADO, where they belong, where the customer's existing tooling handles them.

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

Helix is source available: [github.com/helixml/helix](https://github.com/helixml/helix)

The git sync code is in `api/pkg/services/git_repository_service_pull.go`.

---

*We're a bootstrapped company selling AI agent infrastructure to enterprises. We need this to work correctly more than we need it to be elegant. If you have better ideas, we're listening. If you want to tell us we're doing it wrong, you're probably right.*

*— Luke*
