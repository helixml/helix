# Requirements: Consistent Conventional Commits in Helix

## Problem Statement

Claude Code agents working on helix tasks inconsistently use conventional commit format (e.g., `feat: add X`, `fix: resolve Y`). Sometimes commits are well-formed, sometimes they're informal or descriptive-only. This makes the git history harder to navigate and changelog generation harder to automate.

## Findings

### CLAUDE.md — Claude Code Reads This

`/home/retro/work/helix-4/CLAUDE.md` exists (symlinked as `AGENTS.md`) and **is read by Claude Code** automatically when it starts in that directory. It contains detailed rules around git, commits, filesystem, stack commands, etc.

**Current commit guidance in CLAUDE.md (lines 27–30):**
> - Commit and push frequently, keep commits atomic, update design docs
> - No unsubstantiated claims about code severity/importance without evidence
> - Ask user to verify UI changes; when stuck, use `git bisect`

**Missing:** No mention of conventional commit format (`feat:`, `fix:`, `chore:`, etc.).

### commit-msg Hook — Only Adds Spec-Ref Trailer

`/home/retro/work/helix-4/.git/hooks/commit-msg` is the only active git hook. It:
- Appends a `Spec-Ref: helix-specs@<hash>` trailer to every commit
- Does **not** validate or enforce commit message format
- Does not require conventional commit prefixes

### agent_instruction_service.go — Planning and Approval Prompts

The helix backend sends prompts to agents during planning and implementation phases (in `approvalPromptTemplate` etc.). These prompts:
- Instruct agents to commit frequently (`git add -A && git commit -m "Progress update"`)
- Use generic commit messages in examples (e.g., `"Progress update"`, `"Add PR description"`)
- Do **not** mention conventional commit format anywhere

### PR Intercept — Affects PR Titles, Not Commit Messages

The PR intercept system (task 001320) reads `pull_request.md` from the helix-specs branch to set PR title/description. This affects **PR names only** — it has no effect on commit message format.

## Root Cause

Claude Code uses conventional commits **when it decides to** based on general LLM training — not because helix enforces it. Since neither CLAUDE.md, the commit-msg hook, nor the agent prompts mention conventional commit format, Claude uses its own judgement inconsistently.

## User Stories

- **As a developer**, I want all helix task commits to use conventional commit format so I can scan git history quickly.
- **As a reviewer**, I want PR titles derived from `pull_request.md` to use a clear, professional title (not necessarily conventional commit format — PRs are summaries, not atomic changes).
- **As a maintainer**, I want commit message enforcement to be automated so agents can't forget it.

## Acceptance Criteria

1. CLAUDE.md explicitly requires conventional commit format with examples
2. The `commit-msg` hook validates conventional commit prefix and rejects non-conforming messages (or auto-prefixes them)
3. Agent prompts in `agent_instruction_service.go` use conventional commit examples
4. PR titles (via `pull_request.md`) continue to use descriptive prose titles (not conventional commit format)
