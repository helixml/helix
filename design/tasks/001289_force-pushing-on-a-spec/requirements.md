# Requirements: Force Push Support for Agent Feature Branches

## Problem Statement

When an agent force-pushes to their feature branch (e.g., after a rebase), the push is accepted by the local Helix git server but fails when Helix tries to push to the upstream repository (GitHub). This is because `PushBranchToRemote` is always called with `force: false`.

Error observed:
```
[rejected] feature/001036-we-badly-need-to -> feature/001036-we-badly-need-to (non-fast-forward)
```

## User Stories

### US-1: Agent Force Push
**As an** agent working on a feature branch  
**I want to** be able to force-push after rebasing or amending commits  
**So that** I can keep my branch up-to-date with the latest base branch changes

## Acceptance Criteria

### AC-1: Force Push Detection
- [ ] System detects when an incoming push is a force push (old commit is not ancestor of new commit)
- [ ] Force push detection works for both new branches and existing branches

### AC-2: Force Push Propagation
- [ ] When a force push is detected on a feature branch, the upstream push uses `--force`
- [ ] Force push is ONLY allowed on feature branches (not `helix-specs` or default branch)

### AC-3: Protected Branches Unchanged
- [ ] `helix-specs` branch remains protected from force push (existing behavior preserved)
- [ ] Default/main branch remains protected from force push

### AC-4: Logging
- [ ] Force push events are logged with appropriate context (branch, old commit, new commit)

## Out of Scope

- Force push on `helix-specs` branch (intentionally blocked)
- Force push by regular users (this is about agent workflows)
- UI changes