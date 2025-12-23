# We Mass-Produce Merge Conflicts (And That's Fine)

*How we handle git when 10 AI agents are all pushing to your repo at once*

---

Everyone's building AI coding agents. Cursor, Windsurf, Devin, Claude Code, Codex. Cool. Ship it.

But nobody talks about what happens when you have *multiple* agents working on the same codebase. Simultaneously. Stepping on each other's toes. Creating the kind of git chaos that would make your senior dev cry.

We're [Helix](https://github.com/helixml/helix), a bootstrapped team building an open-source platform for running fleets of AI coding agents. We're not VC-funded, which means we can't afford to spend 6 months on elegant solutions. We need things that work yesterday.

Here's what happens when you naively parallelize agents:

```
Agent 1: git checkout main && git checkout -b feature/add-login
Agent 2: git checkout main && git checkout -b feature/add-signup
Agent 3: git checkout main && git checkout -b feature/fix-auth

# Meanwhile, a human merges a PR on GitHub...

Agent 4: git checkout main  # Wait, which main? Local or remote?
```

## The One Weird Trick: Single-Direction Branches

After weeks of staring at divergence errors at 2am, we discovered something embarrassingly simple: **every branch type has exactly ONE direction**.

| Branch Type | Direction | Why |
|-------------|-----------|-----|
| `main` | PULL-ONLY | Your ADO/GitHub repo is truth. We're just visiting. |
| `helix-specs` | PUSH-ONLY | We own this. Nobody else touches it. |
| Feature branches | PUSH-ONLY | Agents create, push, and forget. |

That's it. That's the insight that took us three weeks to figure out. You're welcome.

## Why This Matters: The Middle Repo Problem

"Just give agents git credentials and let them push to GitHub."

No. Absolutely not. Enterprise customers don't want AI agents with direct write access to their repos. Neither do we. One hallucinating agent with push access to production is one too many.

So we built a **security boundary**: a bare git repo hosted by Helix that sits between agents and your external repo.

```
Your Repo (ADO/GitHub)  ←→  Helix Bare Repo  ←→  Agent Sandbox
        │                         │                    │
   source of truth          security layer        no credentials
```

Agents push to Helix. Helix syncs with your repo using credentials we control. The agent sandbox never touches your git credentials.

The problem? If both you AND the agents are making changes, conflicts happen in **our** middle repo. And resolving conflicts in a bare repository is... have you ever tried that? You'd need to build an entire conflict resolution UI so users (or agents) could reconcile changes in the middle layer.

We haven't built that. Maybe we should. What do you think, HN?

For now, the simpler solution: enforce direction per branch so conflicts in the middle repo are literally impossible.

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
// Orphan commit = no parents = separate history
commit := &object.Commit{
    Message:  "Initialize helix-specs branch",
    TreeHash: emptyTreeHash,
    // No ParentHashes - this is the trick
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

## Humans In The Loop (Sorry, Not Sorry)

We're not trying to replace developers with agents. We tried. It doesn't work.

The mental model is **pair programming with a fleet**. You're still the engineer. You still make decisions. But now you're coordinating multiple AI pair programmers instead of typing every line yourself.

Some of our team joke "we're all managers now." It's... uncomfortably accurate:

- Define tasks clearly enough for agents to execute
- Review design docs before approving implementation
- Monitor progress on the Kanban board
- Step in when an agent goes off-track
- Merge PRs and resolve conflicts

The git flow assumes humans are in the loop. Feature branches are PUSH-ONLY because humans resolve conflicts in PR review. helix-specs keeps design visible because humans approve specs before coding starts.

Agents do grunt work. You do thinking. For now, anyway.

## Does It Work?

Before single-direction branches:
- ~5% of task starts hit divergence errors
- Debugging sessions that made me question my career choices

After:
- Zero divergence errors in production
- 10+ agents working on same repo simultaneously
- 200ms average sync time for repos up to 1GB

Not bad for three weeks of 2am debugging sessions.

---

## Questions We'd Actually Like Answered

1. **Has anyone built conflict resolution for bare repos?** We're pushing conflicts to PR review, but maybe there's a better way.

2. **Is single-direction-per-branch too restrictive?** We could build bidirectional sync with proper reconciliation UI. Should we?

3. **How do other agent platforms handle multi-agent git?** Genuinely curious. We couldn't find anyone talking about this publicly.

4. **Orphan branches for design docs - good idea or hack?** It works for us, but we're biased.

---

## The Code

Helix is open source: [github.com/helixml/helix](https://github.com/helixml/helix)

The git sync code lives in `api/pkg/services/git_repository_service_pull.go`. Steal it. We stole ideas from everyone else.

---

*We're a bootstrapped team trying to make AI coding agents work for enterprises. If this post helped you, star the repo. If you have answers to any of our questions, we're all ears. If you want to tell us we're doing it wrong, we're listening - we probably are.*

*— Luke, who has mass-produced more merge conflicts than any human should*
